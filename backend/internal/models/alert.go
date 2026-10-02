package models

import "time"

// Alert lifecycle statuses.
const (
	AlertStatusFiring       = "firing"
	AlertStatusAcknowledged = "acknowledged"
	AlertStatusResolved     = "resolved"
)

// Alert severities.
const (
	AlertLevelInfo     = "info"
	AlertLevelWarning  = "warning"
	AlertLevelCritical = "critical"
)

// Alert is one active (or historical) alarm instance. Rules that match create
// and keep an alert firing per (rule, device); telemetry rules auto-resolve
// once the condition clears; operators can acknowledge and resolve manually.
type Alert struct {
	Base
	ProjectID     uint       `gorm:"index;not null" json:"projectId"`
	RuleID        uint       `gorm:"index;not null" json:"ruleId"`
	DeviceID      uint       `gorm:"index;not null" json:"deviceId"`
	DeviceKey     string     `gorm:"size:128" json:"deviceKey"`
	Identifier    string     `gorm:"size:128" json:"identifier"`
	Title         string     `gorm:"size:200" json:"title"` // rule name
	Level         string     `gorm:"size:16;not null;default:warning" json:"level"`
	Status        string     `gorm:"size:24;not null;default:firing;index" json:"status"`
	Message       string     `gorm:"type:text" json:"message"` // latest trigger summary
	StartsAt      time.Time  `json:"startsAt"`
	LastFiredAt   time.Time  `json:"lastFiredAt"`
	FireCount     int64      `gorm:"default:0" json:"fireCount"`
	ResolvedAt    *time.Time `json:"resolvedAt"`
	ResolveReason string     `gorm:"size:64" json:"resolveReason"`
	AckedAt       *time.Time `json:"ackedAt"`
	AckedBy       uint       `json:"ackedBy"`
	Escalations   int        `gorm:"default:0" json:"escalations"`
	EscalatedAt   *time.Time `json:"escalatedAt"`
}

// TableName returns the alerts table name.
func (Alert) TableName() string { return "alerts" }

// Open reports whether the alert is still active (firing or acknowledged).
func (a Alert) Open() bool {
	return a.Status == AlertStatusFiring || a.Status == AlertStatusAcknowledged
}
