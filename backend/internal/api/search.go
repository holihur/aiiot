package api

import (
	"net/http"
	"strings"

	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// Search is the cross-project quick-search backing the command palette. It
// returns the devices, rules and device groups the caller can access, matched
// by name/key. Products and projects are global/cheap and stay client-side.
func (h *Handlers) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		ok(c, gin.H{"devices": []any{}, "rules": []any{}, "groups": []any{}})
		return
	}
	like := "%" + q + "%"
	uid := middleware.UserID(c)
	sysAdmin := middleware.SystemRole(c) == models.SystemRoleAdmin
	ids := h.accessibleProjectIDs(c, uid, sysAdmin)
	limit := parseLimit(c.Query("limit"), 8, 20)

	// An empty id set yields "IN (NULL)" → no rows, which is the desired
	// result for a user with no accessible projects.
	var devices []models.Device
	if err := h.DB.WithContext(c).Where("project_id IN (?)", ids).
		Where("name ILIKE ? OR key ILIKE ?", like, like).
		Order("id DESC").Limit(limit).Find(&devices).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	var rules []models.Rule
	if err := h.DB.WithContext(c).Where("project_id IN (?)", ids).
		Where("name ILIKE ?", like).
		Order("id DESC").Limit(limit).Find(&rules).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	var groups []models.DeviceGroup
	if err := h.DB.WithContext(c).Where("project_id IN (?)", ids).
		Where("name ILIKE ?", like).
		Order("id DESC").Limit(limit).Find(&groups).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	ok(c, gin.H{"devices": devices, "rules": rules, "groups": groups})
}

// accessibleProjectIDs returns the ids of projects the caller owns or is a
// member of (all projects for a system admin).
func (h *Handlers) accessibleProjectIDs(c *gin.Context, uid uint, sysAdmin bool) []uint {
	ids := []uint{}
	q := h.DB.WithContext(c).Model(&models.Project{})
	if !sysAdmin {
		q = q.Where("owner_id = ? OR id IN (?)", uid,
			h.DB.Model(&models.ProjectMember{}).Select("project_id").Where("user_id = ?", uid))
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return []uint{}
	}
	return ids
}
