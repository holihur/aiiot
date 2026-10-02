package models

import "time"

const (
	GatewayStatusActive  = "active"
	GatewayStatusOffline = "offline"
)

// GatewayInstance is the operations-side record of a protocol gateway. The
// core (control plane) owns this state: gateways register and heartbeat, and
// operators can see and prune instances even when a gateway is offline.
type GatewayInstance struct {
	Base
	InstanceID    string     `gorm:"uniqueIndex;size:128;not null" json:"instanceId"`
	Protocol      string     `gorm:"size:32;index;not null" json:"protocol"`
	Version       string     `gorm:"size:32" json:"version"`
	DownlinkURL   string     `gorm:"size:255" json:"downlinkUrl"`
	Metadata      JSONMap    `gorm:"type:jsonb" json:"metadata,omitempty"`
	Status        string     `gorm:"size:32;not null;default:active" json:"status"`
	ActiveDevices int        `json:"activeDevices"`
	UptimeSeconds int64      `json:"uptimeSeconds"`
	FirstSeenAt   time.Time  `json:"firstSeenAt"`
	LastHeartbeat *time.Time `json:"lastHeartbeat"`
}

func (GatewayInstance) TableName() string { return "gateway_instances" }
