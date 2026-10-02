package models

import "time"

const (
	TriggerTelemetry     = "telemetry"
	TriggerEvent         = "event"
	TriggerDeviceOnline  = "device_online"
	TriggerDeviceOffline = "device_offline"
	TriggerDeviceMessage = "device_message"
	TriggerPeerMessage   = "peer_message"
	// TriggerShadowDelta fires when a device shadow delta is produced.
	TriggerShadowDelta = "shadow_delta"

	ActionWebhook       = "webhook"
	ActionMQTTPublish   = "mqtt_publish"
	ActionDeviceCommand = "device_command"
	ActionLog           = "log"
	// ActionSetDesired updates a device's shadow desired state; the resulting
	// delta is pushed to the device as a downlink property-set.
	ActionSetDesired = "set_desired"
	// ActionNotify sends an alert through a notification channel.
	ActionNotify = "notify"
)

// Rule is a CEL-based automation. When an event matches TriggerType and
// TriggerSource, the CEL Condition is evaluated with the event context; if it
// returns true the Actions are executed.
type Rule struct {
	Base
	ProjectID     uint     `gorm:"index;not null" json:"projectId"`
	ProductID     *uint    `gorm:"index" json:"productId"`
	WorkspaceID   *uint    `gorm:"index" json:"workspaceId"`
	Name          string   `gorm:"size:160;not null" json:"name"`
	Description   string   `gorm:"type:text" json:"description"`
	Enabled       bool     `gorm:"default:true;index" json:"enabled"`
	TriggerType   string   `gorm:"size:32;not null;default:telemetry" json:"triggerType"`
	TriggerSource string   `gorm:"size:128;not null;default:*" json:"triggerSource"` // identifier or *
	Condition     string   `gorm:"type:text;not null" json:"condition"`              // CEL expression
	Actions       JSONList `gorm:"type:jsonb" json:"actions"`
	Priority      int      `gorm:"default:0" json:"priority"`

	LastTriggeredAt *time.Time `json:"lastTriggeredAt"`
	TriggerCount    int64      `gorm:"default:0" json:"triggerCount"`
}

func (Rule) TableName() string { return "rules" }

// RuleExecutionLog is an audit record for a single rule evaluation.
type RuleExecutionLog struct {
	Base
	RuleID        uint      `gorm:"index;not null" json:"ruleId"`
	ProjectID     uint      `gorm:"index" json:"projectId"`
	DeviceID      uint      `gorm:"index" json:"deviceId"`
	Identifier    string    `gorm:"size:128" json:"identifier"`
	Matched       bool      `json:"matched"`
	Success       bool      `json:"success"`
	Error         string    `gorm:"type:text" json:"error"`
	Context       JSONMap   `gorm:"type:jsonb" json:"context"`
	ActionsResult JSONList  `gorm:"type:jsonb" json:"actionsResult"`
	OccurredAt    time.Time `gorm:"index" json:"occurredAt"`
}

func (RuleExecutionLog) TableName() string { return "rule_execution_logs" }

// AccessConfig persists the runtime configuration of an access adapter.
// Changed records are picked up by the access manager on reload.
type AccessConfig struct {
	Base
	ProjectID *uint   `gorm:"index" json:"projectId"`
	Adapter   string  `gorm:"size:32;not null" json:"adapter"` // mqtt|coap|custom
	Name      string  `gorm:"size:120;not null" json:"name"`
	Enabled   bool    `gorm:"default:true" json:"enabled"`
	Config    JSONMap `gorm:"type:jsonb" json:"config"`
}

func (AccessConfig) TableName() string { return "access_configs" }
