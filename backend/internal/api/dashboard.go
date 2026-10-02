package api

import (
	"net/http"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// GetDashboard returns the project's dashboard board (panels JSON).
func (h *Handlers) GetDashboard(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	var board models.DashboardBoard
	if err := h.DB.WithContext(c).Where("project_id = ?", projectID).First(&board).Error; err != nil {
		ok(c, gin.H{"panels": []any{}})
		return
	}
	ok(c, gin.H{"panels": board.Panels})
}

type dashboardPutRequest struct {
	Panels models.JSONList `json:"panels"`
}

// PutDashboard replaces the project dashboard board. Project members may edit.
func (h *Handlers) PutDashboard(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req dashboardPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Panels == nil {
		req.Panels = models.JSONList{}
	}
	var board models.DashboardBoard
	err := h.DB.WithContext(c).Where("project_id = ?", projectID).First(&board).Error
	if err != nil {
		board = models.DashboardBoard{ProjectID: projectID}
	}
	board.Panels = req.Panels
	if err := h.DB.WithContext(c).Save(&board).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true, "panels": len(board.Panels)})
}
