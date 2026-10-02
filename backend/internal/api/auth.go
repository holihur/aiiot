package api

import (
	"net/http"
	"strings"

	"github.com/aiiot/server/internal/auth"
	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

type registerRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	DisplayName string `json:"displayName"`
}

func (h *Handlers) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	var existing int64
	h.DB.Model(&models.User{}).Where("username = ? OR email = ?", req.Username, req.Email).Count(&existing)
	if existing > 0 {
		fail(c, http.StatusConflict, "username or email already exists")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		fail(c, http.StatusInternalServerError, "hash password failed")
		return
	}
	// Business users are never administrators; administration is a separate
	// identity store (admin_users).
	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		SystemRole:   models.SystemRoleUser,
		Status:       "active",
	}
	if err := h.DB.Create(&user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "create user failed")
		return
	}
	token, exp, err := h.Tokens.Issue(user.ID, user.Username, user.SystemRole)
	if err != nil {
		fail(c, http.StatusInternalServerError, "issue token failed")
		return
	}
	created(c, gin.H{"token": token, "expiresAt": exp, "user": user})
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handlers) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var user models.User
	if err := h.DB.Where("username = ? OR email = ?", req.Username, req.Username).First(&user).Error; err != nil {
		fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if user.Status != "active" {
		fail(c, http.StatusForbidden, "account disabled")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, exp, err := h.Tokens.Issue(user.ID, user.Username, user.SystemRole)
	if err != nil {
		fail(c, http.StatusInternalServerError, "issue token failed")
		return
	}
	ok(c, gin.H{"token": token, "expiresAt": exp, "user": user})
}

func (h *Handlers) Me(c *gin.Context) {
	var user models.User
	if err := h.DB.First(&user, middleware.UserID(c)).Error; err != nil {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	ok(c, user)
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8"`
}

func (h *Handlers) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var user models.User
	if err := h.DB.First(&user, middleware.UserID(c)).Error; err != nil {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.OldPassword) {
		fail(c, http.StatusUnauthorized, "old password incorrect")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		fail(c, http.StatusInternalServerError, "hash failed")
		return
	}
	h.DB.Model(&user).Update("password_hash", hash)
	ok(c, gin.H{"ok": true})
}
