package models

import "time"

// Telemetry is a single time-series sample. It is stored in a PostgreSQL
// declaratively-partitioned table (one partition per month) created by
// raw DDL in the database package -- GORM AutoMigrate is intentionally not
// used for this table.
type Telemetry struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	Time        time.Time `gorm:"primaryKey;type:timestamptz;not null;index:idx_telemetry_series,priority:3" json:"time"`
	ProjectID   uint      `gorm:"not null;index:idx_telemetry_project,priority:1" json:"projectId"`
	WorkspaceID uint      `gorm:"not null" json:"workspaceId"`
	ProductID   uint      `gorm:"not null" json:"productId"`
	DeviceID    uint      `gorm:"not null;index:idx_telemetry_series,priority:1" json:"deviceId"`
	Identifier  string    `gorm:"size:128;not null;index:idx_telemetry_series,priority:2" json:"identifier"`
	DataType    string    `gorm:"size:32" json:"dataType"`
	NumValue    *float64  `json:"numValue,omitempty"`
	BoolValue   *bool     `json:"boolValue,omitempty"`
	StrValue    *string   `json:"strValue,omitempty"`
	JSONValue   JSONMap   `gorm:"type:jsonb" json:"jsonValue,omitempty"`
}

func (Telemetry) TableName() string { return "telemetry_data" }

// DeviceLatestValue caches the most recent value per (device, identifier) so
// that dashboards do not need to scan the partitioned time-series table.
type DeviceLatestValue struct {
	DeviceID    uint      `gorm:"primaryKey" json:"deviceId"`
	Identifier  string    `gorm:"primaryKey;size:128" json:"identifier"`
	ProjectID   uint      `gorm:"index" json:"projectId"`
	WorkspaceID uint      `json:"workspaceId"`
	ProductID   uint      `json:"productId"`
	DataType    string    `gorm:"size:32" json:"dataType"`
	NumValue    *float64  `json:"numValue,omitempty"`
	BoolValue   *bool     `json:"boolValue,omitempty"`
	StrValue    *string   `json:"strValue,omitempty"`
	JSONValue   JSONMap   `gorm:"type:jsonb" json:"jsonValue,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (DeviceLatestValue) TableName() string { return "device_latest_values" }

// DeviceEvent records non-numeric/event uplinks (thing-model events, alerts).
type DeviceEvent struct {
	Base
	ProjectID   uint      `gorm:"index" json:"projectId"`
	WorkspaceID uint      `json:"workspaceId"`
	ProductID   uint      `json:"productId"`
	DeviceID    uint      `gorm:"index" json:"deviceId"`
	Identifier  string    `gorm:"size:128" json:"identifier"`
	Type        string    `gorm:"size:64" json:"type"`
	Payload     JSONMap   `gorm:"type:jsonb" json:"payload"`
	OccurredAt  time.Time `gorm:"index" json:"occurredAt"`
}

func (DeviceEvent) TableName() string { return "device_events" }
