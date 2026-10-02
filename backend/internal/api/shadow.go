package api

import (
	"net/http"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// GetDeviceShadow returns the desired/reported/delta state of a device.
func (h *Handlers) GetDeviceShadow(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	shadow, err := h.Shadow.Get(c, device.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, shadow)
}

// PatchDeviceShadowDesired merges a JSON merge-patch into the desired state and
// pushes the resulting delta to the device.
func (h *Handlers) PatchDeviceShadowDesired(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	dc, err := h.Resolver.ResolveByDeviceID(c, device.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	shadow, err := h.Shadow.ApplyDesired(c, dc, patch, models.ShadowSourceAPI, nil)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, shadow)
}

// GetDeviceShadowHistory returns the audit trail of shadow changes.
func (h *Handlers) GetDeviceShadowHistory(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	logs, err := h.Shadow.History(c, device.ID, int(queryUint(c, "limit")))
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, logs)
}
// ClearDeviceShadowDesired removes all desired keys.
func (h *Handlers) ClearDeviceShadowDesired(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	dc, err := h.Resolver.ResolveByDeviceID(c, device.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	shadow, err := h.Shadow.ClearDesired(c, dc, models.ShadowSourceAPI)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, shadow)
}
