// Package gateway implements the wire protocol between standalone protocol
// gateways and the core control plane, plus the harness that runs an
// access.Adapter under that protocol.
//
// Direction summary:
//
//	Gateway -> Core   POST /internal/gateway/register
//	                  POST /internal/gateway/heartbeat
//	                  POST /internal/gateway/auth
//	                  POST /internal/gateway/uplink
//	Core    -> Gateway POST {downlinkUrl}/downlink
//
// All gateway->core requests carry the shared secret in X-Gateway-Token.
package gateway

import (
	"encoding/json"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/models"
)

// ProtocolVersion is bumped when the wire protocol changes incompatibly.
const ProtocolVersion = "v1"

// RegisterRequest is sent by a gateway on startup and whenever it restarts.
type RegisterRequest struct {
	InstanceID  string         `json:"instanceId"`
	Protocol    string         `json:"protocol"`
	Version     string         `json:"version"`
	DownlinkURL string         `json:"downlinkUrl"`
	Metadata    models.JSONMap `json:"metadata,omitempty"`
}

// RegisterResponse tells the gateway how to behave.
type RegisterResponse struct {
	OK                       bool `json:"ok"`
	HeartbeatIntervalSeconds int  `json:"heartbeatIntervalSeconds"`
	DownlinkEnabled          bool `json:"downlinkEnabled"`
}

// HeartbeatRequest keeps the gateway marked healthy in the core registry.
type HeartbeatRequest struct {
	InstanceID    string `json:"instanceId"`
	ActiveDevices int    `json:"activeDevices"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

// UplinkEnvelope is the JSON form of an access.UplinkMessage. Payload is kept
// as raw JSON so the core protocol is self-describing.
type UplinkEnvelope struct {
	Protocol   string            `json:"protocol"`
	Device     access.DeviceRef  `json:"device"`
	Kind       string            `json:"kind"`
	Identifier string            `json:"identifier,omitempty"`
	Payload    json.RawMessage   `json:"payload,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// DownlinkEnvelope is the JSON form of an access.DownlinkMessage.
type DownlinkEnvelope struct {
	Device     access.DeviceRef  `json:"device"`
	Kind       string            `json:"kind"`
	Identifier string            `json:"identifier,omitempty"`
	Payload    json.RawMessage   `json:"payload,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// UplinkFromAccess converts a normalized uplink into its wire form.
func UplinkFromAccess(m *access.UplinkMessage) UplinkEnvelope {
	return UplinkEnvelope{
		Protocol:   m.Protocol,
		Device:     m.Device,
		Kind:       string(m.Kind),
		Identifier: m.Identifier,
		Payload:    m.Payload,
		Timestamp:  m.Timestamp,
		Metadata:   m.Metadata,
	}
}

// ToAccess converts a wire uplink into the normalized form.
func (e UplinkEnvelope) ToAccess() *access.UplinkMessage {
	return &access.UplinkMessage{
		Protocol:   e.Protocol,
		Device:     e.Device,
		Kind:       access.UplinkKind(e.Kind),
		Identifier: e.Identifier,
		Payload:    []byte(e.Payload),
		Timestamp:  e.Timestamp,
		Metadata:   e.Metadata,
	}
}

// ToAccess converts a wire downlink into the normalized form.
func (e DownlinkEnvelope) ToAccess() *access.DownlinkMessage {
	return &access.DownlinkMessage{
		Device:     e.Device,
		Kind:       access.UplinkKind(e.Kind),
		Identifier: e.Identifier,
		Payload:    []byte(e.Payload),
		Metadata:   e.Metadata,
	}
}
