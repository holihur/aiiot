package models

import "time"

// IngestDedup is an idempotency record for uplink ingestion. NATS delivers
// at-least-once: a failed ingest is NAK'd and redelivered, so the same message
// would otherwise be ingested (and trigger rules) twice. The ingest path
// claims a row keyed by a stable digest of the message; redeliveries fail the
// claim and are dropped. Rows are cleaned up after DedupWindow.
type IngestDedup struct {
	Key        string    `gorm:"primaryKey;size:96" json:"key"`
	DeviceKey  string    `gorm:"size:128;index" json:"deviceKey"`
	IngestedAt time.Time `gorm:"index" json:"ingestedAt"`
}

// TableName returns the dedup table name.
func (IngestDedup) TableName() string { return "ingest_dedup" }
