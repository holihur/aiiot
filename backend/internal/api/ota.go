package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// UploadFirmware stores a firmware binary for a product (multipart form:
// file, version, name, description).
func (h *Handlers) UploadFirmware(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, productID)
	if !allowed {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "file is required")
		return
	}
	if h.OTA.Cfg().MaxUploadBytes > 0 && fileHeader.Size > h.OTA.Cfg().MaxUploadBytes {
		fail(c, http.StatusRequestEntityTooLarge, "firmware exceeds the upload limit")
		return
	}
	version := strings.TrimSpace(c.PostForm("version"))
	if version == "" {
		fail(c, http.StatusBadRequest, "version is required")
		return
	}

	fw := models.Firmware{
		ProductID:   product.ID,
		Version:     version,
		Name:        c.PostForm("name"),
		Description: c.PostForm("description"),
		FileName:    filepath.Base(fileHeader.Filename),
		Status:      models.FirmwareStatusPublished,
	}
	if err := h.DB.Create(&fw).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := os.MkdirAll(h.OTA.Cfg().Dir, 0o755); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	src, err := fileHeader.Open()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer src.Close()

	path := h.OTA.StoredPath(fw.ID)
	dst, err := os.Create(path)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(dst, hasher), src)
	closeErr := dst.Close()
	if err != nil || closeErr != nil {
		fail(c, http.StatusInternalServerError, "failed to store firmware")
		return
	}
	fw.FileSize = written
	fw.Checksum = hex.EncodeToString(hasher.Sum(nil))
	fw.StoredPath = path
	h.DB.Model(&fw).Updates(map[string]any{
		"file_size":   fw.FileSize,
		"checksum":    fw.Checksum,
		"stored_path": path,
	})
	created(c, fw)
}

func (h *Handlers) ListFirmwares(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if _, allowed := h.loadProduct(c, productID); !allowed {
		return
	}
	limit := parseLimit(c.Query("limit"), 50, 500)
	var rows []models.Firmware
	if err := h.DB.WithContext(c).Where("product_id = ?", productID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, rows)
}

func (h *Handlers) DeleteFirmware(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var fw models.Firmware
	if err := h.DB.WithContext(c).First(&fw, id).Error; err != nil {
		fail(c, http.StatusNotFound, "firmware not found")
		return
	}
	if _, allowed := h.loadProductManage(c, fw.ProductID); !allowed {
		return
	}
	_ = os.Remove(h.OTA.StoredPath(fw.ID))
	if err := h.DB.Delete(&models.Firmware{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

// DownloadFirmware serves a firmware binary to devices. It is intentionally
// unauthenticated (checksum-protected) so constrained devices can fetch it;
// deploy behind a network boundary or add a signed URL for production.
func (h *Handlers) DownloadFirmware(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var fw models.Firmware
	if err := h.DB.WithContext(c).First(&fw, id).Error; err != nil {
		fail(c, http.StatusNotFound, "firmware not found")
		return
	}
	path := h.OTA.StoredPath(fw.ID)
	if _, err := os.Stat(path); err != nil {
		fail(c, http.StatusNotFound, "firmware file missing")
		return
	}
	c.Header("X-Checksum-Sha256", fw.Checksum)
	c.Header("Content-Type", "application/octet-stream")
	c.FileAttachment(path, fw.FileName)
}

type otaTaskRequest struct {
	FirmwareID uint   `json:"firmwareId" binding:"required"`
	Name       string `json:"name"`
	DeviceIDs  []uint `json:"deviceIds"`
	// WorkspaceID targets every enabled device in a workspace.
	WorkspaceID uint `json:"workspaceId"`
	// ProductID targets every enabled device of a product within the project.
	ProductID uint `json:"productId"`
	// BatchSize splits the rollout into canary waves (0 = all at once).
	BatchSize int `json:"batchSize"`
	// HaltOnFailure stops the rollout if any device in a wave fails.
	HaltOnFailure *bool `json:"haltOnFailure"`
}

func (h *Handlers) CreateOTATask(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req otaTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	deviceIDs := req.DeviceIDs
	if len(deviceIDs) == 0 {
		q := h.DB.WithContext(c).Model(&models.Device{}).
			Where("project_id = ? AND status = ?", projectID, models.DeviceStatusEnabled)
		if req.WorkspaceID != 0 {
			q = q.Where("workspace_id = ?", req.WorkspaceID)
		}
		if req.ProductID != 0 {
			q = q.Where("product_id = ?", req.ProductID)
		}
		var devices []models.Device
		if err := q.Find(&devices).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		for _, d := range devices {
			deviceIDs = append(deviceIDs, d.ID)
		}
	}
	if len(deviceIDs) == 0 {
		fail(c, http.StatusBadRequest, "no target devices selected")
		return
	}
	halt := true
	if req.HaltOnFailure != nil {
		halt = *req.HaltOnFailure
	}
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("Rollout %d", len(deviceIDs))
	}
	task, err := h.OTA.CreateTask(c, projectID, req.FirmwareID, name, deviceIDs, middlewareUserID(c), req.BatchSize, halt)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, task)
}

// RollbackOTATask creates a new rollout that reinstalls the previous firmware.
func (h *Handlers) RollbackOTATask(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	task, err := h.OTA.GetTask(c, id)
	if err != nil {
		fail(c, http.StatusNotFound, "task not found")
		return
	}
	if !h.requireProject(c, task.ProjectID, models.ProjectRoleMember) {
		return
	}
	rollback, err := h.OTA.Rollback(c, id, middlewareUserID(c))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	created(c, rollback)
}

func (h *Handlers) ListOTATasks(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	tasks, err := h.OTA.ListTasks(c, projectID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, tasks)
}

func (h *Handlers) GetOTATask(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	task, err := h.OTA.GetTask(c, id)
	if err != nil {
		fail(c, http.StatusNotFound, "task not found")
		return
	}
	if !h.requireProject(c, task.ProjectID, models.ProjectRoleViewer) {
		return
	}
	ok(c, task)
}

func (h *Handlers) CancelOTATask(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	task, err := h.OTA.GetTask(c, id)
	if err != nil {
		fail(c, http.StatusNotFound, "task not found")
		return
	}
	if !h.requireProject(c, task.ProjectID, models.ProjectRoleMember) {
		return
	}
	if err := h.OTA.CancelTask(c, id); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

func middlewareUserID(c *gin.Context) uint {
	if v, ok := c.Get("ctx_user_id"); ok {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}
