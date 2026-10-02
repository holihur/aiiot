package models

import "time"

// DeviceShadow holds the desired and reported state of a device.
//
//   - Reported is the last state the device reported (merged from telemetry).
//   - Desired is the state the platform wants the device to reach.
//   - Delta (computed, not stored) is the subset of Desired that differs from
//     Reported; it is pushed to the device as a downlink property-set.
type DeviceShadow struct {
	Base
	DeviceID uint    `gorm:"uniqueIndex;not null" json:"deviceId"`
	Desired  JSONMap `gorm:"type:jsonb" json:"desired"`
	Reported JSONMap `gorm:"type:jsonb" json:"reported"`
	Version  int64   `gorm:"not null;default:0" json:"version"`
}

func (DeviceShadow) TableName() string { return "device_shadows" }

// Shadow change sources.
const (
	ShadowSourceAPI       = "api"
	ShadowSourceRule      = "rule"
	ShadowSourceTelemetry = "telemetry"
)

// DeviceShadowLog is an audit record of a shadow change. It captures the
// full state after the change plus where the change came from, so operators
// can trace how a device reached its current desired/reported values.
type DeviceShadowLog struct {
	Base
	DeviceID   uint      `gorm:"index;not null" json:"deviceId"`
	ProjectID  uint      `gorm:"index" json:"projectId"`
	Source     string    `gorm:"size:32;not null" json:"source"`
	RuleID     *uint     `gorm:"index" json:"ruleId,omitempty"`
	Desired    JSONMap   `gorm:"type:jsonb" json:"desired"`
	Reported   JSONMap   `gorm:"type:jsonb" json:"reported"`
	Delta      JSONMap   `gorm:"type:jsonb" json:"delta"`
	OccurredAt time.Time `gorm:"index" json:"occurredAt"`
}

func (DeviceShadowLog) TableName() string { return "device_shadow_logs" }
