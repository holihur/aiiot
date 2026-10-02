package models

import "time"

// ThingModelVersion is an immutable snapshot of a published thing model.
// Rollback restores a snapshot into the current (draft) thing model.
type ThingModelVersion struct {
	Base
	ThingModelID uint      `gorm:"index;not null" json:"thingModelId"`
	Version      string    `gorm:"size:32;not null" json:"version"`
	Payload      JSONList  `gorm:"type:jsonb;not null" json:"payload"` // []ThingModelElement
	CreatedBy    uint      `json:"createdBy"`
	PublishedAt  time.Time `json:"publishedAt"`
}

// TableName returns the snapshot table name.
func (ThingModelVersion) TableName() string { return "thing_model_versions" }
