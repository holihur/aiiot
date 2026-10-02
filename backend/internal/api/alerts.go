package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// alertSummary joins an alert with its rule name.
type alertSummary struct {
	models.Alert
	RuleName string `json:"ruleName,omitempty"`
}

// ListAlerts returns project alerts with optional status/device filters.
func (h *Handlers) ListAlerts(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	limit := parseLimit(c.Query("limit"), 100, 500)
	q := h.DB.WithContext(c).
		Table("alerts a").
		Select("a.*, r.name AS rule_name").
		Joins("LEFT JOIN rules r ON r.id = a.rule_id").
		Where("a.project_id = ?", projectID)
	if st := c.Query("status"); st != "" {
		q = q.Where("a.status = ?", st)
	}
	if did := c.Query("deviceId"); did != "" {
		if id, err := strconv.ParseUint(did, 10, 32); err == nil {
			q = q.Where("a.device_id = ?", id)
		}
	}
	var rows []alertSummary
	if err := q.Order("CASE WHEN a.status = 'firing' THEN 0 WHEN a.status = 'acknowledged' THEN 1 ELSE 2 END, a.updated_at DESC").
		Limit(limit).Scan(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Status counts for the summary strip.
	var counts []struct {
		Status string `gorm:"column:status"`
		N      int64  `gorm:"column:n"`
	}
	h.DB.WithContext(c).Model(&models.Alert{}).
		Select("status, count(*) AS n").
		Where("project_id = ?", projectID).
		Group("status").Scan(&counts)
	countMap := map[string]int64{}
	for _, cc := range counts {
		countMap[cc.Status] = cc.N
	}
	ok(c, gin.H{"items": rows, "counts": countMap})
}

// loadAlert loads an alert the current user may access (project member of the
// project the alert belongs to).
func (h *Handlers) loadAlert(c *gin.Context, id uint) (*models.Alert, bool) {
	var al models.Alert
	if err := h.DB.WithContext(c).First(&al, id).Error; err != nil {
		fail(c, http.StatusNotFound, "alert not found")
		return nil, false
	}
	if !h.requireProject(c, al.ProjectID, models.ProjectRoleViewer) {
		return nil, false
	}
	return &al, true
}

// AckAlert marks a firing alert as acknowledged.
func (h *Handlers) AckAlert(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	al, allowed := h.loadAlert(c, id)
	if !allowed {
		return
	}
	if !h.requireProject(c, al.ProjectID, models.ProjectRoleMember) {
		return
	}
	now := time.Now().UTC()
	if err := h.DB.Model(al).Updates(map[string]any{
		"status":   models.AlertStatusAcknowledged,
		"acked_at": now,
		"acked_by": middleware.UserID(c),
	}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

// ResolveAlert manually resolves an alert.
func (h *Handlers) ResolveAlert(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	al, allowed := h.loadAlert(c, id)
	if !allowed {
		return
	}
	if !h.requireProject(c, al.ProjectID, models.ProjectRoleMember) {
		return
	}
	reason := c.Query("reason")
	if reason == "" {
		reason = "manual"
	}
	now := time.Now().UTC()
	if err := h.DB.Model(al).Updates(map[string]any{
		"status":         models.AlertStatusResolved,
		"resolved_at":    now,
		"resolve_reason": reason,
	}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}
