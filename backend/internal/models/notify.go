package models

// Notification channel types.
const (
	ChannelWebhook  = "webhook"
	ChannelDingTalk = "dingtalk"
	ChannelEmail    = "email"
)

// NotifyChannel is an alert destination a rule can target.
type NotifyChannel struct {
	Base
	ProjectID   uint    `gorm:"index;not null" json:"projectId"`
	Name        string  `gorm:"size:160;not null" json:"name"`
	Type        string  `gorm:"size:32;not null" json:"type"`
	Enabled     bool    `gorm:"default:true" json:"enabled"`
	Config      JSONMap `gorm:"type:jsonb" json:"config"`
	Description string  `gorm:"type:text" json:"description"`
}

func (NotifyChannel) TableName() string { return "notify_channels" }

// NotificationLog records a delivery attempt.
type NotificationLog struct {
	Base
	ProjectID  uint    `gorm:"index" json:"projectId"`
	ChannelID  uint    `gorm:"index" json:"channelId"`
	RuleID     *uint   `gorm:"index" json:"ruleId,omitempty"`
	DeviceID   uint    `json:"deviceId"`
	Success    bool    `json:"success"`
	Suppressed bool    `json:"suppressed"`
	Count      int     `gorm:"default:1" json:"count"`
	Error      string  `gorm:"type:text" json:"error"`
	Title      string  `gorm:"size:255" json:"title"`
	Body       string  `gorm:"type:text" json:"body"`
	Config     JSONMap `gorm:"type:jsonb" json:"-"`
}

func (NotificationLog) TableName() string { return "notification_logs" }

// Setting is a runtime-mutable key/value system setting.
type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

func (Setting) TableName() string { return "system_settings" }

const SettingTelemetryRetentionDays = "telemetry.retention_days"
