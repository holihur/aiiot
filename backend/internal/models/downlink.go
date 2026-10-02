package models

import "time"

// Downlink log sources.
const (
	DownlinkSourceAPI      = "api"
	DownlinkSourceBatch    = "batch"
	DownlinkSourceRule     = "rule"
	DownlinkSourceShadow   = "shadow"
	DownlinkSourceOTA      = "ota"
	DownlinkSourcePlatform = "platform"
)

// Downlink log statuses.
const (
	DownlinkStatusSent   = "sent"
	DownlinkStatusFailed = "failed"
)

// DeviceDownlinkLog records every platform-originated message pushed to a
// device: API commands, batch commands, rule-engine actions, shadow delta
// pushes and OTA dispatches. It powers the device detail "commands / timeline"
// views so operators can trace what was sent, when and with what result.
type DeviceDownlinkLog struct {
	Base
	ProjectID  uint      `gorm:"index" json:"projectId"`
	DeviceID   uint      `gorm:"index;not null" json:"deviceId"`
	Kind       string    `gorm:"size:32" json:"kind"` // property / service_call / peer / ota / desired
	Identifier string    `gorm:"size:128" json:"identifier"`
	Payload    JSONMap   `gorm:"type:jsonb" json:"payload"`
	Source     string    `gorm:"size:32;not null" json:"source"`
	Status     string    `gorm:"size:16;not null;default:sent" json:"status"`
	Error      string    `gorm:"size:512" json:"error,omitempty"`
	Target     string    `gorm:"size:128" json:"target,omitempty"` // peer target deviceKey or "broadcast"
	OccurredAt time.Time `gorm:"index" json:"occurredAt"`
}

func (DeviceDownlinkLog) TableName() string { return "device_downlink_logs" }
