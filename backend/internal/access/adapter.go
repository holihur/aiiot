// Package access defines the protocol-agnostic access abstraction layer.
//
// Every protocol gateway (MQTT, CoAP, custom TCP, ...) implements the Adapter
// interface. The gateway harness (package gateway) wires an Adapter to the
// core control plane: uplinks flow Adapter -> core, downlinks flow core ->
// Adapter, and device authentication is delegated to the core.
//
// Adding a new protocol therefore means implementing Adapter and running it
// under gateway.Run; the core does not need to change.
package access

import (
	"context"
	"time"
)

// UplinkKind enumerates the logical message categories carried by any protocol.
type UplinkKind string

const (
	// KindProperty is a thing-model property report.
	KindProperty UplinkKind = "property"
	// KindEvent is a thing-model event report.
	KindEvent UplinkKind = "event"
	// KindServiceReply is a reply to a previously invoked service.
	KindServiceReply UplinkKind = "service_reply"
	// KindServiceCall is an inbound service invocation sent to a device.
	KindServiceCall UplinkKind = "service_call"
	// KindPeer is a device-to-device message (only valid inside one workspace).
	KindPeer UplinkKind = "peer"
	// KindOTA carries OTA firmware progress/result reports from a device.
	KindOTA UplinkKind = "ota"
	// KindShadowDelta is an internal (non-wire) event raised when a device
	// shadow delta is produced; rules can trigger on it.
	KindShadowDelta UplinkKind = "shadow_delta"
	// KindLifecycle reports online/offline/register transitions.
	KindLifecycle UplinkKind = "lifecycle"
)

// Lifecycle states carried in UplinkMessage.Metadata["state"].
const (
	StateOnline  = "online"
	StateOffline = "offline"
)

// Protocol names used across the access layer.
const (
	ProtocolMQTTName   = "mqtt"
	ProtocolCoAPName   = "coap"
	ProtocolCustomName = "custom"
)

// DeviceRef identifies a device on the wire (before the core resolves it to an
// internal ID).
type DeviceRef struct {
	WorkspaceKey string `json:"workspaceKey,omitempty"`
	ProductKey   string `json:"productKey"`
	DeviceKey    string `json:"deviceKey"`
}

// UplinkMessage is a normalized message travelling from a device to the core.
type UplinkMessage struct {
	Protocol string     `json:"protocol"`
	Device   DeviceRef  `json:"device"`
	Kind     UplinkKind `json:"kind"`
	// Identifier is the thing-model property/event/service identifier. It may
	// be empty for KindPeer when the payload carries routing information.
	Identifier string            `json:"identifier,omitempty"`
	Payload    []byte            `json:"payload"`
	Timestamp  time.Time         `json:"timestamp"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// DownlinkMessage is a normalized message travelling from the core to a device.
type DownlinkMessage struct {
	Protocol   string            `json:"protocol"`
	Device     DeviceRef         `json:"device"`
	Kind       UplinkKind        `json:"kind"`
	Identifier string            `json:"identifier,omitempty"`
	Payload    []byte            `json:"payload"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// AuthRequest is presented by a device when it connects/authenticates.
type AuthRequest struct {
	ProductKey string `json:"productKey"`
	DeviceKey  string `json:"deviceKey"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	RemoteAddr string `json:"remoteAddr,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	// Workspace is an optional tenant hint derived from the access subdomain
	// (TLS SNI / Host). When set, the core requires the device to belong to
	// that workspace.
	Workspace string `json:"workspace,omitempty"`
}

// AuthResponse is returned by the core. Workspace fields are used by the
// gateway to enforce workspace isolation (e.g. MQTT topic ACL).
type AuthResponse struct {
	Authorized   bool   `json:"authorized"`
	Reason       string `json:"reason,omitempty"`
	DeviceID     uint   `json:"deviceId"`
	ProjectID    uint   `json:"projectId"`
	WorkspaceID  uint   `json:"workspaceId"`
	ProductID    uint   `json:"productId"`
	WorkspaceKey string `json:"workspaceKey"`
	ProductKey   string `json:"productKey"`
	DeviceKey    string `json:"deviceKey"`
	Protocol     string `json:"protocol"`
}

// UplinkHandler consumes a normalized uplink message.
type UplinkHandler func(ctx context.Context, msg *UplinkMessage) error

// AuthHandler authenticates a device, delegating to the core.
type AuthHandler func(ctx context.Context, req *AuthRequest) (*AuthResponse, error)

// Adapter is implemented by every standalone protocol gateway.
type Adapter interface {
	// Name returns the protocol name, e.g. "mqtt".
	Name() string
	// Start begins serving; it must return once the listener is ready to
	// accept traffic, and block until ctx is cancelled or a fatal error occurs.
	Start(ctx context.Context) error
	// Stop gracefully shuts the adapter down.
	Stop(ctx context.Context) error
	// OnUplink registers the handler invoked for every inbound message.
	OnUplink(handler UplinkHandler)
	// OnAuth registers the handler used to authenticate devices.
	OnAuth(handler AuthHandler)
	// Downlink delivers a message to a device. It is invoked by the gateway
	// harness when the core pushes a command.
	Downlink(ctx context.Context, msg *DownlinkMessage) error
}
