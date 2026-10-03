package api

import (
	"net/http"

	"github.com/aiiot/server/internal/models"
	"github.com/aiiot/server/internal/service"
	"github.com/gin-gonic/gin"
)

type channelRequest struct {
	Name        string         `json:"name" binding:"required,max=160"`
	Type        string         `json:"type" binding:"required,oneof=webhook dingtalk wecom lark email"`
	Enabled     *bool          `json:"enabled"`
	Config      models.JSONMap `json:"config"`
	Description string         `json:"description"`
}

func (h *Handlers) ListChannels(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	var rows []models.NotifyChannel
	if err := h.DB.WithContext(c).Where("project_id = ?", projectID).Order("id").Find(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, rows)
}

func (h *Handlers) CreateChannel(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req channelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ch := models.NotifyChannel{
		ProjectID:   projectID,
		Name:        req.Name,
		Type:        req.Type,
		Enabled:     true,
		Config:      req.Config,
		Description: req.Description,
	}
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}
	if err := h.DB.Create(&ch).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created(c, ch)
}

func (h *Handlers) UpdateChannel(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var ch models.NotifyChannel
	if err := h.DB.WithContext(c).First(&ch, id).Error; err != nil {
		fail(c, http.StatusNotFound, "channel not found")
		return
	}
	if !h.requireProject(c, ch.ProjectID, models.ProjectRoleMember) {
		return
	}
	var req channelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ch.Name = req.Name
	ch.Type = req.Type
	ch.Config = req.Config
	ch.Description = req.Description
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}
	if err := h.DB.Save(&ch).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, ch)
}

func (h *Handlers) DeleteChannel(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var ch models.NotifyChannel
	if err := h.DB.WithContext(c).First(&ch, id).Error; err != nil {
		fail(c, http.StatusNotFound, "channel not found")
		return
	}
	if !h.requireProject(c, ch.ProjectID, models.ProjectRoleAdmin) {
		return
	}
	if err := h.DB.Delete(&models.NotifyChannel{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

func (h *Handlers) TestChannel(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var ch models.NotifyChannel
	if err := h.DB.WithContext(c).First(&ch, id).Error; err != nil {
		fail(c, http.StatusNotFound, "channel not found")
		return
	}
	if !h.requireProject(c, ch.ProjectID, models.ProjectRoleMember) {
		return
	}
	err := h.Notifier.Send(c, ch.ID, "AI IoT test notification", "This is a test alert from the AI IoT platform.", nil, 0, service.SendOptions{})
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

func (h *Handlers) ChannelLogs(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var ch models.NotifyChannel
	if err := h.DB.WithContext(c).First(&ch, id).Error; err != nil {
		fail(c, http.StatusNotFound, "channel not found")
		return
	}
	if !h.requireProject(c, ch.ProjectID, models.ProjectRoleViewer) {
		return
	}
	var rows []models.NotificationLog
	if err := h.DB.WithContext(c).Where("channel_id = ?", id).
		Order("id DESC").Limit(100).Find(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, rows)
}
