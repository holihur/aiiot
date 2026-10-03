package api

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/aiiot/server/internal/auth"
	"github.com/aiiot/server/internal/metrics"
	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SeedAdmin creates the initial administrator account when none exists. The
// password comes from ADMIN_PASSWORD; if unset a random one is generated and
// logged once so operators can retrieve it.
func SeedAdmin(db *gorm.DB, username, password string, log *slog.Logger) {
	var count int64
	db.Model(&models.AdminUser{}).Count(&count)
	if count > 0 {
		return
	}
	if username == "" {
		username = "admin"
	}
	generated := false
	if password == "" {
		b := make([]byte, 9)
		_, _ = rand.Read(b)
		password = hex.EncodeToString(b)
		generated = true
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Error("seed admin: hash failed", "error", err)
		return
	}
	admin := models.AdminUser{Username: username, PasswordHash: hash, DisplayName: "Administrator", Status: "active"}
	if err := db.Create(&admin).Error; err != nil {
		log.Error("seed admin: create failed", "error", err)
		return
	}
	if generated {
		log.Warn("seeded administrator account with a generated password — change it after first login",
			"username", username, "password", password)
	} else {
		log.Info("seeded administrator account", "username", username)
	}
}

type adminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	// TOTPCode is required once 2FA is enabled for the account.
	TOTPCode string `json:"totpCode"`
}

// AdminLogin authenticates an administrator and returns an admin-audience token.
func (h *Handlers) AdminLogin(c *gin.Context) {
	var req adminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var admin models.AdminUser
	if err := h.DB.WithContext(c).Where("username = ?", req.Username).First(&admin).Error; err != nil {
		fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if admin.Status != "active" {
		fail(c, http.StatusForbidden, "account disabled")
		return
	}
	if !auth.CheckPassword(admin.PasswordHash, req.Password) {
		fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := h.verifyAdminTOTP(&admin, req.TOTPCode); err != nil {
		ce, isChallenge := err.(*totpChallengeError)
		if !isChallenge {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.JSON(http.StatusUnauthorized, totpRequiredResponse(ce.kind))
		return
	}
	token, exp, err := h.Tokens.IssueAdmin(admin.ID, admin.Username, admin.TokenVersion)
	if err != nil {
		fail(c, http.StatusInternalServerError, "issue token failed")
		return
	}
	ok(c, gin.H{"token": token, "expiresAt": exp, "admin": admin})
}

// AdminMe returns the authenticated administrator.
func (h *Handlers) AdminMe(c *gin.Context) {
	var admin models.AdminUser
	if err := h.DB.WithContext(c).First(&admin, middleware.UserID(c)).Error; err != nil {
		fail(c, http.StatusNotFound, "admin not found")
		return
	}
	ok(c, admin)
}

type adminPasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8"`
}

// AdminChangePassword lets an administrator rotate their own password.
func (h *Handlers) AdminChangePassword(c *gin.Context) {
	var req adminPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var admin models.AdminUser
	if err := h.DB.WithContext(c).First(&admin, middleware.UserID(c)).Error; err != nil {
		fail(c, http.StatusNotFound, "admin not found")
		return
	}
	if !auth.CheckPassword(admin.PasswordHash, req.OldPassword) {
		fail(c, http.StatusUnauthorized, "old password incorrect")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		fail(c, http.StatusInternalServerError, "hash failed")
		return
	}
	next := admin.TokenVersion + 1
	if err := h.DB.Model(&admin).Updates(map[string]any{"password_hash": hash, "token_version": next}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "update failed")
		return
	}
	token, exp, err := h.Tokens.IssueAdmin(admin.ID, admin.Username, next)
	if err != nil {
		fail(c, http.StatusInternalServerError, "issue token failed")
		return
	}
	ok(c, gin.H{"ok": true, "token": token, "expiresAt": exp})
}

// AdminNATSStats returns the JetStream uplink stream/consumer state plus the
// dashboard counters, powering the NATS ops page.
func (h *Handlers) AdminNATSStats(c *gin.Context) {
	resp := gin.H{"counters": metrics.Collect(h.NATSSubject)}
	if h.Bus == nil {
		resp["jetstream"] = nil
		ok(c, resp)
		return
	}
	st, err := h.Bus.Stats(c)
	if err != nil {
		h.Log.Warn("admin nats stats failed", "error", err)
		fail(c, http.StatusBadGateway, "nats stats: "+err.Error())
		return
	}
	resp["jetstream"] = st
	ok(c, resp)
}
