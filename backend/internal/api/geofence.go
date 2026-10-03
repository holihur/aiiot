package api

import (
	"context"
	"net/http"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// geofenceRequest mirrors the public shape of a fence.
type geofenceRequest struct {
	Name        string         `json:"name" binding:"required,max=120"`
	WorkspaceID uint           `json:"workspaceId"`
	Enabled     *bool          `json:"enabled"`
	Polygon     models.JSONMap `json:"polygon" binding:"required"`
}

func (h *Handlers) reloadGeofences(c *gin.Context) {
	if h.Geofence != nil {
		if err := h.Geofence.Reload(c); err != nil {
			h.Log.Warn("geofence reload failed", "error", err)
		}
	}
}

// ListGeofences returns the project's fences (optional workspace filter).
func (h *Handlers) ListGeofences(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	query := h.DB.WithContext(c).Where("project_id = ?", projectID)
	if ws := queryUint(c, "workspaceId"); ws > 0 {
		query = query.Where("workspace_id = ?", ws)
	}
	var rows []models.Geofence
	if err := query.Order("id").Find(&rows).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, rows)
}

// CreateGeofence adds a named polygon fence to a project.
func (h *Handlers) CreateGeofence(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req geofenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	f := models.Geofence{
		ProjectID: projectID, WorkspaceID: req.WorkspaceID,
		Name: req.Name, Enabled: enabled, Polygon: req.Polygon,
	}
	if err := h.DB.Create(&f).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.reloadGeofences(c)
	created(c, f)
}

// UpdateGeofence edits name/enabled/polygon/workspace.
func (h *Handlers) UpdateGeofence(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var f models.Geofence
	if err := h.DB.WithContext(c).First(&f, id).Error; err != nil {
		fail(c, http.StatusNotFound, "geofence not found")
		return
	}
	if !h.requireProject(c, f.ProjectID, models.ProjectRoleMember) {
		return
	}
	var req geofenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	f.Name = req.Name
	f.WorkspaceID = req.WorkspaceID
	f.Polygon = req.Polygon
	if req.Enabled != nil {
		f.Enabled = *req.Enabled
	}
	if err := h.DB.Save(&f).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.reloadGeofences(c)
	ok(c, f)
}

// DeleteGeofence removes a fence.
func (h *Handlers) DeleteGeofence(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	var f models.Geofence
	if err := h.DB.WithContext(c).First(&f, id).Error; err != nil {
		fail(c, http.StatusNotFound, "geofence not found")
		return
	}
	if !h.requireProject(c, f.ProjectID, models.ProjectRoleMember) {
		return
	}
	if err := h.DB.Delete(&f).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.reloadGeofences(c)
	ok(c, gin.H{"ok": true})
}

// DevicesGeoJSON exports located devices as a GeoJSON FeatureCollection for
// import into GIS tools (lat/lng from device tags).
func (h *Handlers) DevicesGeoJSON(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	var devices []models.Device
	if err := h.DB.WithContext(c).Where("project_id = ?", projectID).
		Preload("Product").Preload("Workspace").Order("id").Find(&devices).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	features := []map[string]any{}
	for _, d := range devices {
		lat, ok1 := d.Tags["lat"].(float64)
		lng, ok2 := d.Tags["lng"].(float64)
		if !ok1 || !ok2 {
			continue
		}
		features = append(features, map[string]any{
			"type":     "Feature",
			"geometry": map[string]any{"type": "Point", "coordinates": []any{lng, lat}},
			"properties": map[string]any{
				"key": d.Key, "name": d.Name, "online": d.Online, "status": d.Status,
				"product": func() string {
					if d.Product != nil {
						return d.Product.Key
					}
					return ""
				}(),
				"workspace": func() string {
					if d.Workspace != nil {
						return d.Workspace.Key
					}
					return ""
				}(),
			},
		})
	}
	ok(c, gin.H{
		"type":     "FeatureCollection",
		"features": features,
	})
}

var _ context.Context
