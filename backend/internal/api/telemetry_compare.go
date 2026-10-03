package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aiiot/server/internal/models"
	"github.com/aiiot/server/internal/service"
	"github.com/gin-gonic/gin"
)

// compareRequest compares numeric series across devices and identifiers.
type compareRequest struct {
	DeviceIDs   []uint   `json:"deviceIds" binding:"required,min=1,max=50"`
	Identifiers []string `json:"identifiers" binding:"required,min=1,max=20"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Interval    string   `json:"interval"`
}

// CompareTelemetry returns per-(device, identifier) time series, downsampled
// with the same date_bin machinery as the single-device query. Callers must be
// members of every device's project (checked server-side).
func (h *Handlers) CompareTelemetry(c *gin.Context) {
	var req compareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	from := parseTime(req.From, time.Now().Add(-24*time.Hour))
	to := parseTime(req.To, time.Now().Add(time.Minute))
	interval, err := time.ParseDuration(req.Interval)
	if err != nil || interval <= 0 {
		interval = 5 * time.Minute
	}

	// authorization: viewer+ in every device's project
	var devices []models.Device
	if err := h.DB.WithContext(c).Where("id IN ?", req.DeviceIDs).Find(&devices).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if len(devices) != len(req.DeviceIDs) {
		fail(c, http.StatusBadRequest, "one or more devices not found")
		return
	}
	seen := map[uint]bool{}
	for _, d := range devices {
		wid := d.WorkspaceID
		var ws models.Workspace
		if err := h.DB.First(&ws, wid).Error; err != nil {
			fail(c, http.StatusNotFound, "workspace missing")
			return
		}
		if !h.requireProject(c, ws.ProjectID, models.ProjectRoleViewer) {
			return
		}
		if seen[d.ID] {
			fail(c, http.StatusBadRequest, "duplicate device id")
			return
		}
		seen[d.ID] = true
	}

	type series struct {
		DeviceID   uint             `json:"deviceId"`
		DeviceKey  string           `json:"deviceKey"`
		Identifier string           `json:"identifier"`
		Points     []map[string]any `json:"points"`
	}
	var out []series
	for _, d := range devices {
		for _, id := range req.Identifiers {
			var pts []service.AggregatePoint
			q := service.RangeQuery{DeviceID: d.ID, Identifiers: []string{id}, From: from, To: to}
			if interval >= time.Hour {
				pts, err = h.Telemetry.QueryAggregateRollup(c, q, interval)
			} else {
				pts, err = h.Telemetry.QueryAggregate(c, q, interval)
			}
			if err != nil {
				fail(c, http.StatusInternalServerError, err.Error())
				return
			}
			points := make([]map[string]any, 0, len(pts))
			for _, p := range pts {
				points = append(points, map[string]any{
					"t": p.Bucket, "v": p.Avg,
				})
			}
			out = append(out, series{DeviceID: d.ID, DeviceKey: d.Key, Identifier: id, Points: points})
		}
	}
	ok(c, gin.H{"interval": interval.String(), "series": out})
}

// ExportTelemetry streams a device's numeric series as CSV:
// time,<identifier>,... (one column per identifier, one row per bucket).
func (h *Handlers) ExportTelemetry(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	from := parseTime(c.Query("from"), time.Now().Add(-24*time.Hour))
	to := parseTime(c.Query("to"), time.Now().Add(time.Minute))
	identifiers := strings.Split(c.Query("identifiers"), ",")
	var clean []string
	for _, s := range identifiers {
		if s = strings.TrimSpace(s); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		fail(c, http.StatusBadRequest, "identifiers required (comma separated)")
		return
	}
	interval, err := time.ParseDuration(c.Query("interval"))
	if err != nil || interval <= 0 {
		interval = 5 * time.Minute
	}

	// bucket -> values
	perID := map[string]map[string]any{}
	buckets := map[string]bool{}
	for _, idn := range clean {
		q := service.RangeQuery{DeviceID: device.ID, Identifiers: []string{idn}, From: from, To: to}
		var pts []service.AggregatePoint
		if interval >= time.Hour {
			pts, err = h.Telemetry.QueryAggregateRollup(c, q, interval)
		} else {
			pts, err = h.Telemetry.QueryAggregate(c, q, interval)
		}
		if err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		m := map[string]any{}
		for _, p := range pts {
			bk := p.Bucket.UTC().Format(time.RFC3339)
			buckets[bk] = true
			if p.Avg != nil {
				m[bk] = p.Avg
			}
		}
		perID[idn] = m
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=telemetry_%s.csv", device.Key))
	c.Writer.Write([]byte("\uFEFF"))
	w := csv.NewWriter(c.Writer)
	header := append([]string{"time"}, clean...)
	_ = w.Write(header)
	ordered := sortedKeys(buckets)
	for _, bk := range ordered {
		row := []string{bk}
		for _, idn := range clean {
			if v, ok := perID[idn][bk]; ok {
				s, _ := json.Marshal(v)
				row = append(row, string(s))
			} else {
				row = append(row, "")
			}
		}
		_ = w.Write(row)
	}
	w.Flush()
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	// RFC3339 strings sort chronologically
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
