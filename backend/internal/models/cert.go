package models

import "time"

// DeviceCert holds the platform's knowledge about an issued device client
// certificate (serial + validity + revocation). The private key never leaves
// the issuing moment.
type DeviceCert struct {
	Base
	DeviceID uint       `gorm:"index;not null" json:"deviceId"`
	Serial   string     `gorm:"size:64;uniqueIndex;not null" json:"serial"`
	CN       string     `gorm:"size:64;not null" json:"cn"`
	NotAfter time.Time  `json:"notAfter"`
	Revoked  bool       `gorm:"default:false" json:"revoked"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

func (DeviceCert) TableName() string { return "device_certs" }