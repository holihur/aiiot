package api

import (
	"net/http"
	"time"

	"github.com/aiiot/server/internal/certs"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// DeviceCertificate issues a fresh client certificate for a device. The
// private key is returned exactly once (the platform does not retain it).
func (h *Handlers) DeviceCertificate(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleAdmin)
	if !allowed {
		return
	}
	if h.Certs == nil {
		fail(c, http.StatusInternalServerError, "certificate manager not configured")
		return
	}
	// one live certificate at a time: revoke any previous issuance
	now := time.Now().UTC()
	if err := h.DB.Model(&models.DeviceCert{}).
		Where("device_id = ? AND revoked = ?", device.ID, false).
		Updates(map[string]any{"revoked": true, "revoked_at": now}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	certPEM, keyPEM, serial, err := h.Certs.SignDevice(device.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	rec := &models.DeviceCert{
		DeviceID: device.ID,
		Serial:   serial,
		CN:       certs.DeviceCN(device.ID),
		NotAfter: now.Add(certs.DefaultTTL),
	}
	if err := h.DB.Create(rec).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{
		"certPem":   string(certPEM),
		"keyPem":    string(keyPEM),
		"caPem":     string(h.Certs.CAPEM()),
		"serial":    serial,
		"expiresAt": rec.NotAfter,
		"conn":      "mqtts",
	})
}

// DeviceCertificateStatus reports whether the device has an active certificate.
func (h *Handlers) DeviceCertificateStatus(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var rec models.DeviceCert
	err := h.DB.Where("device_id = ? AND revoked = ?", device.ID, false).
		Order("id desc").First(&rec).Error
	if err != nil {
		ok(c, gin.H{"active": false})
		return
	}
	ok(c, gin.H{"active": true, "serial": rec.Serial, "expiresAt": rec.NotAfter, "cn": rec.CN})
}

// RevokeDeviceCertificate revokes the device's live certificate.
func (h *Handlers) RevokeDeviceCertificate(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleAdmin)
	if !allowed {
		return
	}
	now := time.Now().UTC()
	affected := h.DB.Model(&models.DeviceCert{}).
		Where("device_id = ? AND revoked = ?", device.ID, false).
		Updates(map[string]any{"revoked": true, "revoked_at": now}).RowsAffected
	ok(c, gin.H{"revoked": affected > 0})
}
