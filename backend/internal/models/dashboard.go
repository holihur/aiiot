package models

import "time"

// Dashboard boards render on the project Dashboard page. Panels are stored as
// a JSON list; each panel carries its type and configuration.
//
//	{"type":"stat","title":"车间温度","config":{"deviceId":10,"identifier":"temperature","unit":"°C"}}
//	{"type":"trend","title":"温度趋势","config":{"deviceId":10,"identifier":"temperature","range":"24h"}}
//	{"type":"alerts","title":"告警概览"}
//	{"type":"devices","title":"设备状态"}
//	{"type":"text","title":"说明","config":{"text":"..."}}
type DashboardBoard struct {
	Base
	ProjectID uint      `gorm:"uniqueIndex;not null" json:"projectId"`
	Panels    JSONList  `gorm:"type:jsonb" json:"panels"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName returns the dashboard table name.
func (DashboardBoard) TableName() string { return "dashboard_boards" }

// Dashboard panel types.
const (
	PanelStat    = "stat"
	PanelTrend   = "trend"
	PanelAlerts  = "alerts"
	PanelDevices = "devices"
	PanelText    = "text"
)
