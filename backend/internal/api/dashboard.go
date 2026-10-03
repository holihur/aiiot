package api

import (
	"net/http"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// ListDashboards returns the project's boards (optionally filtered by
// workspace). Creates the default board lazily on first list.
func (h *Handlers) ListDashboards(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	var count int64
	h.DB.WithContext(c).Model(&models.DashboardBoard{}).Where("project_id = ?", projectID).Count(&count)
	if count == 0 {
		_ = h.DB.Create(&models.DashboardBoard{ProjectID: projectID, Name: "默认看板", Panels: models.JSONList{}})
	}
	query := h.DB.WithContext(c).Where("project_id = ?", projectID)
	if ws := queryUint(c, "workspaceId"); ws > 0 {
		query = query.Where("workspace_id = ?", ws)
	}
	var boards []models.DashboardBoard
	if err := query.Order("id").Find(&boards).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, boards)
}

type dashboardCreateRequest struct {
	Name        string `json:"name" binding:"required,max=120"`
	WorkspaceID uint   `json:"workspaceId"`
}

// CreateDashboard adds a new board to a project (project members may).
func (h *Handlers) CreateDashboard(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req dashboardCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		req.Name = "看板"
	}
	board := models.DashboardBoard{
		ProjectID: projectID, WorkspaceID: req.WorkspaceID,
		Name: req.Name, Panels: models.JSONList{},
	}
	if err := h.DB.Create(&board).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created(c, board)
}

// loadManagedBoard loads a board and enforces project member (edit) access.
func (h *Handlers) loadManagedBoard(c *gin.Context, id uint) (*models.DashboardBoard, bool) {
	var board models.DashboardBoard
	if err := h.DB.WithContext(c).First(&board, id).Error; err != nil {
		fail(c, http.StatusNotFound, "dashboard not found")
		return nil, false
	}
	if !h.requireProject(c, board.ProjectID, models.ProjectRoleMember) {
		return nil, false
	}
	return &board, true
}

// GetDashboardByID returns one board with its panels.
func (h *Handlers) GetDashboardByID(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var board models.DashboardBoard
	if err := h.DB.WithContext(c).First(&board, id).Error; err != nil {
		fail(c, http.StatusNotFound, "dashboard not found")
		return
	}
	if !h.requireProject(c, board.ProjectID, models.ProjectRoleViewer) {
		return
	}
	ok(c, board)
}

type dashboardPutRequest struct {
	Panels models.JSONList `json:"panels"`
	Name   string          `json:"name"`
}

// PutDashboardByID saves panels (and optionally renames) an existing board.
func (h *Handlers) PutDashboardByID(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	board, allowed := h.loadManagedBoard(c, id)
	if !allowed {
		return
	}
	var req dashboardPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Panels != nil {
		board.Panels = req.Panels
	}
	if req.Name != "" {
		board.Name = req.Name
	}
	if err := h.DB.WithContext(c).Save(board).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true, "panels": len(board.Panels), "name": board.Name})
}

// DeleteDashboardByID removes a board (the default board may be deleted; the
// next list recreates it).
func (h *Handlers) DeleteDashboardByID(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	if _, allowed := h.loadManagedBoard(c, id); !allowed {
		return
	}
	if err := h.DB.Delete(&models.DashboardBoard{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

// --- legacy compatibility: single per-project board ------------------------

// boardOrDefault returns the board with the lowest id for a project, creating
// the default board when none exists.
func (h *Handlers) boardOrDefault(c *gin.Context, projectID uint) (models.DashboardBoard, bool) {
	var board models.DashboardBoard
	if err := h.DB.WithContext(c).Where("project_id = ?", projectID).Order("id").First(&board).Error; err == nil {
		return board, true
	}
	board = models.DashboardBoard{ProjectID: projectID, Name: "默认看板", Panels: models.JSONList{}}
	if err := h.DB.Create(&board).Error; err != nil {
		return board, false
	}
	return board, true
}

// GetDashboard returns the project's default/legacy board (panels JSON).
func (h *Handlers) GetDashboard(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	board, found := h.boardOrDefault(c, projectID)
	if !found {
		fail(c, http.StatusInternalServerError, "create default dashboard failed")
		return
	}
	ok(c, gin.H{"panels": board.Panels})
}

// PutDashboard replaces the project's default/legacy board. Project members may edit.
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
	board, found := h.boardOrDefault(c, projectID)
	if !found {
		fail(c, http.StatusInternalServerError, "create default dashboard failed")
		return
	}
	board.Panels = req.Panels
	if err := h.DB.WithContext(c).Save(&board).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true, "panels": len(board.Panels)})
}
