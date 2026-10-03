package api

import (
	"net/http"

	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/aiiot/server/internal/totp"
	"github.com/gin-gonic/gin"
)

// AdminTOTPStatus reports 2FA state for the current admin.
func (h *Handlers) AdminTOTPStatus(c *gin.Context) {
	var admin models.AdminUser
	if err := h.DB.First(&admin, middleware.UserID(c)).Error; err != nil {
		fail(c, http.StatusNotFound, "admin not found")
		return
	}
	ok(c, gin.H{"enabled": admin.TotpEnabled})
}

// AdminTOTPSetup generates a fresh secret and returns the provisioning
// details (QR URL). It does not activate 2FA yet.
func (h *Handlers) AdminTOTPSetup(c *gin.Context) {
	secret := totp.GenerateSecret()
	sealed, err := h.TOTP.Seal(secret)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	var admin models.AdminUser
	if err := h.DB.First(&admin, middleware.UserID(c)).Error; err != nil {
		fail(c, http.StatusNotFound, "admin not found")
		return
	}
	// keep the pending secret until activation (write-through)
	admin.TotpSecretEnc = sealed
	if err := h.DB.Model(&admin).Update("totp_secret_enc", sealed).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	url := totp.ProvisioningURL("aiiot", admin.Username, secret)
	ok(c, gin.H{"secret": secret, "otpauthUrl": url, "period": totp.Period, "enabled": false})
}

// AdminTOTPEnable validates a code against the pending secret and enables 2FA.
func (h *Handlers) AdminTOTPEnable(c *gin.Context) {
	var body struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var admin models.AdminUser
	if err := h.DB.First(&admin, middleware.UserID(c)).Error; err != nil {
		fail(c, http.StatusNotFound, "admin not found")
		return
	}
	secret, err := h.TOTP.Open(admin.TotpSecretEnc)
	if err != nil || secret == "" {
		fail(c, http.StatusBadRequest, "run setup first (no pending secret)")
		return
	}
	if !totp.Verify(secret, body.Code, 1) {
		fail(c, http.StatusBadRequest, "invalid code")
		return
	}
	if err := h.DB.Model(&admin).Updates(map[string]any{"totp_enabled": true}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"enabled": true})
}

// AdminTOTPDisable validates the current code and turns 2FA off.
func (h *Handlers) AdminTOTPDisable(c *gin.Context) {
	var body struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var admin models.AdminUser
	if err := h.DB.First(&admin, middleware.UserID(c)).Error; err != nil {
		fail(c, http.StatusNotFound, "admin not found")
		return
	}
	secret, err := h.TOTP.Open(admin.TotpSecretEnc)
	if err != nil || secret == "" {
		ok(c, gin.H{"enabled": false})
		return
	}
	if !totp.Verify(secret, body.Code, 1) {
		fail(c, http.StatusBadRequest, "invalid code")
		return
	}
	if err := h.DB.Model(&admin).Updates(map[string]any{
		"totp_enabled": false, "totp_secret_enc": "",
	}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"enabled": false})
}

// verifyAdminTOTP checks a login code when the admin has 2FA enabled.
// Returns an error explaining what is missing/wrong.
func (h *Handlers) verifyAdminTOTP(admin *models.AdminUser, code string) error {
	if !admin.TotpEnabled {
		return nil
	}
	if code == "" {
		return errTOTPRequired
	}
	secret, err := h.TOTP.Open(admin.TotpSecretEnc)
	if err != nil || secret == "" {
		return errTOTPBroken
	}
	if !totp.Verify(secret, code, 1) {
		return errTOTPInvalid
	}
	return nil
}

var (
	errTOTPRequired = &totpChallengeError{kind: "required"}
	errTOTPInvalid  = &totpChallengeError{kind: "invalid"}
	errTOTPBroken   = &totpChallengeError{kind: "unavailable"}
)

type totpChallengeError struct{ kind string }

func (e *totpChallengeError) Error() string {
	switch e.kind {
	case "required":
		return "two-factor code required"
	case "invalid":
		return "invalid two-factor code"
	default:
		return "two-factor setup unavailable"
	}
}

// totpRequired marks the response contract for clients: 401 with challenge.
func totpRequiredResponse(kind string) gin.H {
	return gin.H{"error": "two-factor", "totp": kind}
}
