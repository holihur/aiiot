package api

import (
	"net/http"
	"strconv"

	"github.com/aiiot/server/internal/database"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// GetStorage returns the telemetry retention policy and partition inventory.
func (h *Handlers) GetStorage(c *gin.Context) {
	retention := h.Settings.GetInt(c, models.SettingTelemetryRetentionDays, h.RetentionDefault)
	partitions, err := database.ListPartitions(c, h.DB)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	var totalRows, totalBytes int64
	for _, p := range partitions {
		totalRows += p.Rows
		totalBytes += p.SizeBytes
	}
	rollup, _ := database.GetRollupInfo(c, h.DB)
	ok(c, gin.H{
		"retentionDays": retention,
		"defaultDays":   h.RetentionDefault,
		"partitions":    partitions,
		"totalRows":     totalRows,
		"totalBytes":    totalBytes,
		"rollup":        rollup,
	})
}

// RefreshRollup rebuilds the hourly telemetry materialized view.
func (h *Handlers) RefreshRollup(c *gin.Context) {
	if err := database.RefreshRollup(c, h.DB); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	info, _ := database.GetRollupInfo(c, h.DB)
	ok(c, gin.H{"ok": true, "rollup": info})
}

type retentionRequest struct {
	Days int `json:"days" binding:"required"`
}

func (h *Handlers) UpdateRetention(c *gin.Context) {
	var req retentionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Days < 1 || req.Days > 3650 {
		fail(c, http.StatusBadRequest, "days must be between 1 and 3650")
		return
	}
	if err := h.Settings.Set(c, models.SettingTelemetryRetentionDays, strconv.Itoa(req.Days)); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"retentionDays": req.Days})
}

// DropPartition manually removes a telemetry partition.
func (h *Handlers) DropPartition(c *gin.Context) {
	name := c.Param("name")
	if !h.requireSystemAdmin(c) {
		return
	}
	if err := database.DropPartition(c, h.DB, name); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}
