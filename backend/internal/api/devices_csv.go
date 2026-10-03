package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// ExportDevices streams the product's devices as CSV:
// key,name,workspaceKey,status,secret (secret included for migration/backup).
func (h *Handlers) ExportDevices(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, productID)
	if !allowed {
		return
	}
	var devices []models.Device
	if err := h.DB.WithContext(c).
		Where("product_id = ?", product.ID).
		Preload("Workspace").Order("key").Find(&devices).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=devices_%s.csv", product.Key))
	c.Writer.Write([]byte("\uFEFF")) // BOM for Excel
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"key", "name", "workspaceKey", "status", "secret"})
	for _, d := range devices {
		ws := ""
		if d.Workspace != nil {
			ws = d.Workspace.Key
		}
		_ = w.Write([]string{d.Key, d.Name, ws, d.Status, d.Secret})
	}
	w.Flush()
}

// ImportDevices bulk-creates devices from an uploaded CSV:
// key,name,workspaceKey[,secret]. Existing keys are reported as skipped and
// never overwritten. Returns per-line results.
func (h *Handlers) ImportDevices(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, productID)
	if !allowed {
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "missing file (field \"file\")")
		return
	}
	fh, err := file.Open()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer fh.Close()

	rows, err := csv.NewReader(fh).ReadAll()
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid csv: "+err.Error())
		return
	}
	if len(rows) < 2 {
		fail(c, http.StatusBadRequest, "csv needs a header line and at least one row")
	}

	// workspace lookup by key within the product's project
	wsByKey := map[string]models.Workspace{}
	var workspaces []models.Workspace
	if err := h.DB.WithContext(c).Find(&workspaces).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	for _, ws := range workspaces {
		wsByKey[ws.Key] = ws
	}

	type result struct {
		Line   int    `json:"line"`
		Key    string `json:"key"`
		Status string `json:"status"` // created | skipped | failed
		Name   string `json:"name"`
		Error  string `json:"error,omitempty"`
	}
	var out []result
	created, skipped := 0, 0
	for i, row := range rows[1:] {
		line := i + 2
		key := strings.TrimSpace(cell(row, 0))
		name := strings.TrimSpace(cell(row, 1))
		wsKey := strings.TrimSpace(cell(row, 2))
		secret := strings.TrimSpace(cell(row, 3))
		if key == "" || name == "" {
			out = append(out, result{Line: line, Key: key, Status: "failed", Error: "key and name required"})
			continue
		}
		ws, ok := wsByKey[wsKey]
		if !ok {
			out = append(out, result{Line: line, Key: key, Status: "failed", Error: "workspace not found: " + wsKey})
			continue
		}
		var existing int64
		h.DB.Model(&models.Device{}).Where("key = ?", key).Count(&existing)
		if existing > 0 {
			skipped++
			out = append(out, result{Line: line, Key: key, Status: "skipped"})
			continue
		}
		if secret == "" {
			secret = newSecret()
		}
		dev := models.Device{
			ProjectID:   ws.ProjectID,
			WorkspaceID: ws.ID,
			ProductID:   product.ID,
			Key:         key,
			Secret:      secret,
			Name:        name,
			Status:      models.DeviceStatusEnabled,
		}
		if err := h.DB.WithContext(c).Create(&dev).Error; err != nil {
			out = append(out, result{Line: line, Key: key, Status: "failed", Error: err.Error()})
			continue
		}
		created++
		out = append(out, result{Line: line, Key: key, Status: "created", Name: name})
	}
	ok(c, gin.H{"created": created, "skipped": skipped, "failed": len(out) - created - skipped, "results": out})
}

func cell(row []string, i int) string {
	if i >= len(row) {
		return ""
	}
	return row[i]
}
