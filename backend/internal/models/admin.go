package models

// AdminUser is a platform administrator account. It is intentionally a
// separate identity store from business users (models.User): administrators do
// not belong to projects and business users can never gain admin access.
type AdminUser struct {
	Base
	Username     string `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	DisplayName  string `gorm:"size:120" json:"displayName"`
	Status       string `gorm:"size:32;not null;default:active" json:"status"`
	// TotpSecretEnc holds the AES-GCM sealed TOTP secret (platform secret
	// derived key); TotpEnabled switches login 2FA on.
	TotpSecretEnc string `gorm:"type:text" json:"-"`
	TotpEnabled   bool   `gorm:"default:false" json:"totpEnabled"`
}

func (AdminUser) TableName() string { return "admin_users" }
