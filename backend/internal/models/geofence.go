package models

// Geofence is a named, project/workspace-scoped polygon. Devices reporting a
// "gps" json attribute are tagged with the fences they fall inside, so rules
// can trigger on e.g. `"zone-a" in geofences`.
type Geofence struct {
	Base
	ProjectID   uint   `gorm:"index;not null" json:"projectId"`
	WorkspaceID uint   `gorm:"index;not null;default:0" json:"workspaceId"`
	Name        string `gorm:"size:120;not null" json:"name"`
	Enabled     bool   `gorm:"default:true" json:"enabled"`
	// Polygon is a GeoJSON polygon: {"type":"Polygon","coordinates":[[[lng,lat],...]]}
	Polygon JSONMap `gorm:"type:jsonb" json:"polygon"`
}

func (Geofence) TableName() string { return "geofences" }
