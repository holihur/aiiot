package models

// AuditLog records a mutating API request for the audit centre.
type AuditLog struct {
	Base
	UserID    uint   `gorm:"index" json:"userId"`
	Username  string `gorm:"size:64" json:"username"`
	ProjectID uint   `gorm:"index" json:"projectId"`
	Method    string `gorm:"size:12" json:"method"`
	Path      string `gorm:"size:255" json:"path"`
	Status    int    `json:"status"`
	IP        string `gorm:"size:64" json:"ip"`
}

func (AuditLog) TableName() string { return "audit_logs" }
