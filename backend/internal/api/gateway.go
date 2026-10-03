package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/gateway"
	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// gatewayAuth protects the internal gateway protocol endpoints.
func (h *Handlers) gatewayAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader("X-Gateway-Token")
		if len(got) != len(h.GatewayToken) ||
			subtle.ConstantTimeCompare([]byte(got), []byte(h.GatewayToken)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid gateway token"})
			return
		}
		c.Next()
	}
}

func (h *Handlers) GatewayRegister(c *gin.Context) {
	var req gateway.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.InstanceID == "" || req.Protocol == "" {
		fail(c, http.StatusBadRequest, "instanceId and protocol are required")
		return
	}
	h.Registry.Register(&req)
	h.Log.Info("gateway registered", "instance", req.InstanceID, "protocol", req.Protocol, "downlink", req.DownlinkURL)
	ok(c, gateway.RegisterResponse{OK: true, HeartbeatIntervalSeconds: 30, DownlinkEnabled: true})
}

func (h *Handlers) GatewayHeartbeat(c *gin.Context) {
	var req gateway.HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if !h.Registry.Heartbeat(&req) {
		// Unknown instance: tell the gateway to re-register.
		c.JSON(http.StatusGone, gin.H{"error": "unknown instance, re-register required"})
		return
	}
	ok(c, gin.H{"ok": true})
}

func (h *Handlers) GatewayAuthenticate(c *gin.Context) {
	var req access.AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.Resolver.Authenticate(c, &req)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, resp)
}

func (h *Handlers) GatewayUplink(c *gin.Context) {
	var env gateway.UplinkEnvelope
	if err := c.ShouldBindJSON(&env); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	msg := env.ToAccess()
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	if err := h.Ingest.Handle(c, msg); err != nil {
		h.Log.Warn("ingest failed", "error", err, "device", msg.Device.DeviceKey, "kind", msg.Kind)
		fail(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"accepted": true})
}

// GatewayPSK returns the DTLS-PSK secret for a device so a gateway can derive
// the pre-shared key during the handshake. Internal endpoint (gateway token).
func (h *Handlers) GatewayPSK(c *gin.Context) {
	key := c.Query("deviceKey")
	if key == "" {
		fail(c, http.StatusBadRequest, "deviceKey required")
		return
	}
	var device models.Device
	if err := h.DB.Where("key = ?", key).First(&device).Error; err != nil {
		fail(c, http.StatusNotFound, "unknown device")
		return
	}
	if device.Secret == "" {
		fail(c, http.StatusNotFound, "device has no secret")
		return
	}
	ok(c, gin.H{"psk": device.Secret})
}

// ListGateways returns the live gateway instances registered with the core.
func (h *Handlers) ListGateways(c *gin.Context) {
	ok(c, h.Registry.List())
}

// DeregisterGateway removes a (typically offline) gateway from the operations
// registry.
func (h *Handlers) DeregisterGateway(c *gin.Context) {
	instanceID := c.Param("instanceId")
	if instanceID == "" {
		fail(c, http.StatusBadRequest, "instanceId is required")
		return
	}
	if err := h.Registry.Deregister(c, instanceID); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.Log.Info("gateway deregistered", "instance", instanceID, "by", middleware.UserID(c))
	ok(c, gin.H{"ok": true})
}

// httpIngestRequest is the body for the generic HTTP ingest endpoint, used by
// custom protocol gateways.
type httpIngestRequest struct {
	Kind       string          `json:"kind"`
	Identifier string          `json:"identifier"`
	Value      json.RawMessage `json:"value"`
	Params     json.RawMessage `json:"params"`
	Payload    json.RawMessage `json:"payload"`
	To         string          `json:"to"`
	Broadcast  bool            `json:"broadcast"`
	State      string          `json:"state"`
	Ts         *time.Time      `json:"ts"`
}

// HTTPIngest is a protocol-agnostic HTTP uplink endpoint. Devices (or protocol
// bridges) authenticate with X-Device-Secret.
func (h *Handlers) HTTPIngest(c *gin.Context) {
	productKey := c.Param("productKey")
	deviceKey := c.Param("deviceKey")
	secret := c.GetHeader("X-Device-Secret")
	if secret == "" {
		secret = c.Query("secret")
	}
	resp, err := h.Resolver.Authenticate(c, &access.AuthRequest{
		ProductKey: productKey,
		DeviceKey:  deviceKey,
		Password:   secret,
		RemoteAddr: c.ClientIP(),
		Protocol:   access.ProtocolCustomName,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if !resp.Authorized {
		fail(c, http.StatusUnauthorized, resp.Reason)
		return
	}

	var req httpIngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	msg := &access.UplinkMessage{
		Protocol:  access.ProtocolCustomName,
		Device:    access.DeviceRef{WorkspaceKey: resp.WorkspaceKey, ProductKey: resp.ProductKey, DeviceKey: resp.DeviceKey},
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]string{},
	}
	if req.Ts != nil {
		msg.Timestamp = *req.Ts
	}
	switch req.Kind {
	case "property", "properties":
		msg.Kind = access.KindProperty
		msg.Identifier = req.Identifier
		msg.Payload = firstNonEmpty(req.Value, req.Params)
	case "event":
		msg.Kind = access.KindEvent
		msg.Identifier = req.Identifier
		msg.Payload = req.Params
	case "service_reply":
		msg.Kind = access.KindServiceReply
		msg.Identifier = req.Identifier
		msg.Payload = req.Params
	case "peer":
		msg.Kind = access.KindPeer
		msg.Payload = firstNonEmpty(req.Payload, req.Params)
		if req.Broadcast {
			msg.Metadata["broadcast"] = "true"
		} else {
			msg.Metadata["to"] = req.To
		}
	case "lifecycle":
		msg.Kind = access.KindLifecycle
		msg.Metadata["state"] = req.State
	default:
		fail(c, http.StatusBadRequest, "unsupported kind "+req.Kind)
		return
	}
	if len(msg.Payload) == 0 {
		msg.Payload = []byte("{}")
	}
	if err := h.Ingest.Handle(c, msg); err != nil {
		fail(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"accepted": true})
}

func firstNonEmpty(values ...json.RawMessage) json.RawMessage {
	for _, v := range values {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}
