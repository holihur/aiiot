package api

import (
	"net/http"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) loadGroup(c *gin.Context, id uint, min string) (*models.DeviceGroup, bool) {
	var g models.DeviceGroup
	if err := h.DB.WithContext(c).First(&g, id).Error; err != nil {
		fail(c, http.StatusNotFound, "device group not found")
		return nil, false
	}
	if !h.requireProject(c, g.ProjectID, min) {
		return nil, false
	}
	return &g, true
}

type groupRequest struct {
	Name        string `json:"name" binding:"required,max=160"`
	Description string `json:"description"`
}

type groupView struct {
	models.DeviceGroup
	DeviceCount int64 `json:"deviceCount"`
}

func (h *Handlers) ListDeviceGroups(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	var groups []models.DeviceGroup
	if err := h.DB.WithContext(c).Where("project_id = ?", projectID).Order("id").Find(&groups).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]groupView, 0, len(groups))
	for _, g := range groups {
		var n int64
		h.DB.Model(&models.DeviceGroupMember{}).Where("group_id = ?", g.ID).Count(&n)
		views = append(views, groupView{DeviceGroup: g, DeviceCount: n})
	}
	ok(c, views)
}

func (h *Handlers) CreateDeviceGroup(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req groupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	g := models.DeviceGroup{ProjectID: projectID, Name: req.Name, Description: req.Description}
	if err := h.DB.Create(&g).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created(c, g)
}

func (h *Handlers) UpdateDeviceGroup(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	g, allowed := h.loadGroup(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var req groupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	g.Name = req.Name
	g.Description = req.Description
	if err := h.DB.Save(g).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, g)
}

func (h *Handlers) DeleteDeviceGroup(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	if _, allowed := h.loadGroup(c, id, models.ProjectRoleAdmin); !allowed {
		return
	}
	err := h.DB.Transaction(func(tx dbTx) error {
		if err := tx.Where("group_id = ?", id).Delete(&models.DeviceGroupMember{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.DeviceGroup{}, id).Error
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

func (h *Handlers) ListGroupDevices(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	g, allowed := h.loadGroup(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	var members []models.DeviceGroupMember
	h.DB.WithContext(c).Where("group_id = ?", g.ID).Find(&members)
	ids := make([]uint, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.DeviceID)
	}
	limit := parseLimit(c.Query("limit"), 100, 1000)
	var devices []models.Device
	if len(ids) > 0 {
		h.DB.WithContext(c).Where("id IN ?", ids).Preload("Product").Preload("Workspace").
			Order("id DESC").Limit(limit).Find(&devices)
	}
	ok(c, devices)
}

func (h *Handlers) AddGroupDevices(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	g, allowed := h.loadGroup(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var body struct {
		DeviceIDs []uint `json:"deviceIds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	added := 0
	for _, devID := range body.DeviceIDs {
		// only devices in the same project
		var device models.Device
		if err := h.DB.WithContext(c).Where("id = ? AND project_id = ?", devID, g.ProjectID).First(&device).Error; err != nil {
			continue
		}
		m := models.DeviceGroupMember{GroupID: g.ID, DeviceID: devID}
		res := h.DB.WithContext(c).Where("group_id = ? AND device_id = ?", g.ID, devID).FirstOrCreate(&m)
		if res.Error == nil && res.RowsAffected > 0 {
			added++
		}
	}
	ok(c, gin.H{"added": added})
}

func (h *Handlers) RemoveGroupDevice(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	deviceID, valid := parseID(c, "deviceId")
	if !valid {
		return
	}
	if _, allowed := h.loadGroup(c, id, models.ProjectRoleMember); !allowed {
		return
	}
	if err := h.DB.Where("group_id = ? AND device_id = ?", id, deviceID).Delete(&models.DeviceGroupMember{}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}
