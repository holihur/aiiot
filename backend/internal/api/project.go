package api

import (
	"net/http"
	"strings"

	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type projectRequest struct {
	Key         string `json:"key" binding:"required,min=2,max=64"`
	Name        string `json:"name" binding:"required,max=160"`
	Description string `json:"description"`
}

func (h *Handlers) ListProjects(c *gin.Context) {
	uid := middleware.UserID(c)
	var projects []models.Project
	q := h.DB.WithContext(c).Order("id")
	if middleware.SystemRole(c) != models.SystemRoleAdmin {
		q = q.Where("owner_id = ? OR id IN (?)",
			uid,
			h.DB.Model(&models.ProjectMember{}).Select("project_id").Where("user_id = ?", uid),
		)
	}
	if err := q.Find(&projects).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Attach the caller's effective role so the UI can adapt.
	roleByProject := map[uint]string{}
	var members []models.ProjectMember
	h.DB.WithContext(c).Where("user_id = ?", uid).Find(&members)
	for _, m := range members {
		roleByProject[m.ProjectID] = m.Role
	}
	sysAdmin := middleware.SystemRole(c) == models.SystemRoleAdmin
	out := make([]gin.H, 0, len(projects))
	for _, p := range projects {
		role := roleByProject[p.ID]
		if p.OwnerID == uid || sysAdmin {
			role = models.ProjectRoleOwner
		}
		out = append(out, gin.H{
			"id": p.ID, "key": p.Key, "name": p.Name, "description": p.Description,
			"ownerId": p.OwnerID, "myRole": role,
		})
	}
	ok(c, out)
}

func (h *Handlers) CreateProject(c *gin.Context) {
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	req.Key = strings.ToLower(strings.TrimSpace(req.Key))

	var count int64
	h.DB.Model(&models.Project{}).Where("key = ?", req.Key).Count(&count)
	if count > 0 {
		fail(c, http.StatusConflict, "project key already exists")
		return
	}

	uid := middleware.UserID(c)
	project := models.Project{
		Key:         req.Key,
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     uid,
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		return tx.Create(&models.ProjectMember{
			ProjectID: project.ID,
			UserID:    uid,
			Role:      models.ProjectRoleOwner,
		}).Error
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created(c, project)
}

func (h *Handlers) GetProject(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, id, models.ProjectRoleViewer) {
		return
	}
	var project models.Project
	if err := h.DB.WithContext(c).Preload("Owner").First(&project, id).Error; err != nil {
		fail(c, http.StatusNotFound, "project not found")
		return
	}
	var members []models.ProjectMember
	h.DB.WithContext(c).Preload("User").Where("project_id = ?", id).Find(&members)
	project.Members = members
	role, _ := h.projectRole(c, id)
	ok(c, gin.H{
		"id": project.ID, "key": project.Key, "name": project.Name, "description": project.Description,
		"ownerId": project.OwnerID, "owner": project.Owner, "members": members, "myRole": role,
	})
}

func (h *Handlers) UpdateProject(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, id, models.ProjectRoleAdmin) {
		return
	}
	// The project key is immutable; only name and description are editable.
	var req struct {
		Name        string `json:"name" binding:"required,max=160"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var project models.Project
	if err := h.DB.First(&project, id).Error; err != nil {
		fail(c, http.StatusNotFound, "project not found")
		return
	}
	project.Name = req.Name
	project.Description = req.Description
	if err := h.DB.Save(&project).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, project)
}

func (h *Handlers) DeleteProject(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, id, models.ProjectRoleOwner) {
		return
	}
	// Cascade-delete the project's resources in dependency order to satisfy
	// foreign keys (members, devices and their children, workspaces, ...).
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var deviceIDs []uint
		tx.Model(&models.Device{}).Where("project_id = ?", id).Pluck("id", &deviceIDs)
		var groupIDs []uint
		tx.Model(&models.DeviceGroup{}).Where("project_id = ?", id).Pluck("id", &groupIDs)

		if len(groupIDs) > 0 {
			tx.Where("group_id IN ?", groupIDs).Delete(&models.DeviceGroupMember{})
		}
		if len(deviceIDs) > 0 {
			tx.Where("device_id IN ?", deviceIDs).Delete(&models.OTATaskDevice{})
			tx.Where("device_id IN ?", deviceIDs).Delete(&models.DeviceShadow{})
			tx.Where("device_id IN ?", deviceIDs).Delete(&models.DeviceShadowLog{})
			tx.Where("device_id IN ?", deviceIDs).Delete(&models.DeviceLatestValue{})
			tx.Where("device_id IN ?", deviceIDs).Delete(&models.DeviceEvent{})
			tx.Where("device_id IN ?", deviceIDs).Delete(&models.Telemetry{})
		}
		tx.Where("project_id = ?", id).Delete(&models.OTATask{})
		tx.Where("project_id = ?", id).Delete(&models.DeviceGroup{})
		tx.Where("project_id = ?", id).Delete(&models.RuleExecutionLog{})
		tx.Where("project_id = ?", id).Delete(&models.Rule{})
		tx.Where("project_id = ?", id).Delete(&models.NotificationLog{})
		tx.Where("project_id = ?", id).Delete(&models.NotifyChannel{})
		tx.Where("project_id = ?", id).Delete(&models.Device{})
		tx.Where("project_id = ?", id).Delete(&models.Workspace{})
		tx.Where("project_id = ?", id).Delete(&models.ProjectMember{})
		return tx.Delete(&models.Project{}, id).Error
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

type memberRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	UserID   uint   `json:"userId"`
	Role     string `json:"role" binding:"required"`
}

func (h *Handlers) ListMembers(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, id, models.ProjectRoleViewer) {
		return
	}
	var members []models.ProjectMember
	if err := h.DB.WithContext(c).Preload("User").Where("project_id = ?", id).Find(&members).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, members)
}

func (h *Handlers) AddMember(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, id, models.ProjectRoleAdmin) {
		return
	}
	var req memberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Role != models.ProjectRoleAdmin && req.Role != models.ProjectRoleMember && req.Role != models.ProjectRoleViewer {
		fail(c, http.StatusBadRequest, "invalid role")
		return
	}
	var user models.User
	q := h.DB.WithContext(c)
	switch {
	case req.UserID != 0:
		q = q.Where("id = ?", req.UserID)
	case req.Email != "":
		q = q.Where("email = ?", req.Email)
	default:
		q = q.Where("username = ?", req.Username)
	}
	if err := q.First(&user).Error; err != nil {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	var existing int64
	h.DB.Model(&models.ProjectMember{}).Where("project_id = ? AND user_id = ?", id, user.ID).Count(&existing)
	if existing > 0 {
		fail(c, http.StatusConflict, "user is already a member")
		return
	}
	member := models.ProjectMember{ProjectID: id, UserID: user.ID, Role: req.Role}
	if err := h.DB.Create(&member).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	member.User = &user
	created(c, member)
}

func (h *Handlers) UpdateMember(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	memberID, valid := parseID(c, "memberId")
	if !valid {
		return
	}
	if !h.requireProject(c, id, models.ProjectRoleAdmin) {
		return
	}
	var req memberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Role != models.ProjectRoleAdmin && req.Role != models.ProjectRoleMember && req.Role != models.ProjectRoleViewer {
		fail(c, http.StatusBadRequest, "invalid role")
		return
	}
	var target models.ProjectMember
	if err := h.DB.First(&target, "id = ? AND project_id = ?", memberID, id).Error; err != nil {
		fail(c, http.StatusNotFound, "member not found")
		return
	}
	// demoting the (only) owner would leave the project without an owner
	if target.Role == models.ProjectRoleOwner && req.Role != models.ProjectRoleOwner && h.ownerCount(id) <= 1 {
		fail(c, http.StatusBadRequest, "the project must keep at least one owner")
		return
	}
	if err := h.DB.Model(&models.ProjectMember{}).Where("id = ? AND project_id = ?", memberID, id).
		Update("role", req.Role).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

func (h *Handlers) RemoveMember(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	memberID, valid := parseID(c, "memberId")
	if !valid {
		return
	}
	if !h.requireProject(c, id, models.ProjectRoleAdmin) {
		return
	}
	var target models.ProjectMember
	if err := h.DB.First(&target, "id = ? AND project_id = ?", memberID, id).Error; err != nil {
		fail(c, http.StatusNotFound, "member not found")
		return
	}
	if target.Role == models.ProjectRoleOwner && h.ownerCount(id) <= 1 {
		fail(c, http.StatusBadRequest, "the project must keep at least one owner")
		return
	}
	if err := h.DB.Delete(&models.ProjectMember{}, memberID).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

// ownerCount returns how many owner members a project currently has.
func (h *Handlers) ownerCount(projectID uint) int64 {
	var n int64
	h.DB.Model(&models.ProjectMember{}).
		Where("project_id = ? AND role = ?", projectID, models.ProjectRoleOwner).Count(&n)
	return n
}

// --- workspaces -----------------------------------------------------------

type workspaceRequest struct {
	Key         string `json:"key" binding:"required,min=2,max=64"`
	Name        string `json:"name" binding:"required,max=160"`
	Description string `json:"description"`
}

func (h *Handlers) ListWorkspaces(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	var workspaces []models.Workspace
	if err := h.DB.WithContext(c).Where("project_id = ?", projectID).Order("id").Find(&workspaces).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, workspaces)
}

func (h *Handlers) CreateWorkspace(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req workspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	req.Key = strings.ToLower(strings.TrimSpace(req.Key))
	var existing int64
	h.DB.Model(&models.Workspace{}).Where("project_id = ? AND key = ?", projectID, req.Key).Count(&existing)
	if existing > 0 {
		fail(c, http.StatusConflict, "workspace key already exists in this project")
		return
	}
	ws := models.Workspace{ProjectID: projectID, Key: req.Key, Name: req.Name, Description: req.Description}
	if err := h.DB.Create(&ws).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created(c, ws)
}

func (h *Handlers) UpdateWorkspace(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	ws, allowed := h.loadWorkspace(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	// The key is immutable: it identifies the workspace. Only name and
	// description are editable.
	var req struct {
		Name        string `json:"name" binding:"required,max=160"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ws.Name = req.Name
	ws.Description = req.Description
	if err := h.DB.Save(ws).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, ws)
}

func (h *Handlers) DeleteWorkspace(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	if _, allowed := h.loadWorkspace(c, id, models.ProjectRoleAdmin); !allowed {
		return
	}
	var deviceCount int64
	h.DB.Model(&models.Device{}).Where("workspace_id = ?", id).Count(&deviceCount)
	if deviceCount > 0 {
		fail(c, http.StatusConflict, "workspace still has devices")
		return
	}
	if err := h.DB.Delete(&models.Workspace{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}
