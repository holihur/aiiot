package api

import (
	"net/http"

	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// ListAuditLogs returns mutating-request history. System admins see everything;
// other users see only their own entries.
func (h *Handlers) ListAuditLogs(c *gin.Context) {
	limit := int(queryUint(c, "limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := h.DB.WithContext(c).Order("id DESC").Limit(limit)
	if middleware.SystemRole(c) != models.SystemRoleAdmin {
		q = q.Where("user_id = ?", middleware.UserID(c))
	}
	if uid := queryUint(c, "userId"); uid != 0 {
		q = q.Where("user_id = ?", uid)
	}
	if pid := queryUint(c, "projectId"); pid != 0 {
		q = q.Where("project_id = ?", pid)
	}
	if path := c.Query("path"); path != "" {
		q = q.Where("path ILIKE ?", "%"+path+"%")
	}
	var logs []models.AuditLog
	if err := q.Find(&logs).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, logs)
}
