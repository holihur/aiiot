package mqtt

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aiiot/server/internal/access"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

// brokerHook authenticates devices against the core and enforces workspace
// isolation through topic ACLs.
type brokerHook struct {
	mqtt.HookBase
	a *Adapter
}

func (h *brokerHook) ID() string { return "aiiot-auth-acl" }

func (h *brokerHook) Provides(b byte) bool {
	switch b {
	case mqtt.OnConnectAuthenticate, mqtt.OnACLCheck, mqtt.OnSessionEstablished,
		mqtt.OnDisconnect, mqtt.OnPublished, mqtt.OnStarted, mqtt.OnStopped:
		return true
	}
	return false
}

func (h *brokerHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	username := string(pk.Connect.Username)
	password := string(pk.Connect.Password)
	productKey, deviceKey := parseCredentials(username, pk.Connect.ClientIdentifier)
	if productKey == "" || deviceKey == "" || h.a.auth == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := h.a.auth(ctx, &access.AuthRequest{
		ProductKey: productKey,
		DeviceKey:  deviceKey,
		Username:   username,
		Password:   password,
		RemoteAddr: cl.Net.Remote,
		Protocol:   access.ProtocolMQTTName,
		Workspace:  tenantFromConn(cl.Net.Conn),
	})
	if err != nil || resp == nil || !resp.Authorized {
		h.a.log.Warn("mqtt device authentication denied", "username", username, "remote", cl.Net.Remote, "error", err)
		return false
	}
	h.a.sessions.Store(cl.ID, resp)
	atomic.AddInt64(&h.a.active, 1)
	return true
}

// OnACLCheck enforces that a device may only access topics inside its own
// project + workspace. Publishes must additionally be under the device's own
// subtree; subscriptions may cover any device in the same workspace so peer
// messages can be received.
// OnACLCheck restricts a device to its own topic subtree. Cross-device
// traffic is mediated by the core, which delivers peer messages into the
// target device's own subtree, so workspace isolation is preserved without a
// workspace segment in the topic.
func (h *brokerHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	v, ok := h.a.sessions.Load(cl.ID)
	if !ok {
		return false
	}
	auth := v.(*access.AuthResponse)

	if strings.HasPrefix(topic, "$SYS/") {
		return !write
	}
	info, err := parseTopic(topic)
	if err != nil {
		return false
	}
	return info.ProductKey == auth.ProductKey && info.DeviceKey == auth.DeviceKey
}

func (h *brokerHook) OnSessionEstablished(cl *mqtt.Client, _ packets.Packet) {
	if resp, ok := h.a.lookup(cl.ID); ok {
		h.a.forwardUplink(&access.UplinkMessage{
			Protocol:  access.ProtocolMQTTName,
			Device:    deviceRef(resp),
			Kind:      access.KindLifecycle,
			Timestamp: time.Now().UTC(),
			Metadata:  map[string]string{"state": access.StateOnline, "gateway": h.a.opts.InstanceID},
		})
	}
}

func (h *brokerHook) OnDisconnect(cl *mqtt.Client, _ error, _ bool) {
	if resp, ok := h.a.lookupAndDelete(cl.ID); ok {
		atomic.AddInt64(&h.a.active, -1)
		h.a.forwardUplink(&access.UplinkMessage{
			Protocol:  access.ProtocolMQTTName,
			Device:    deviceRef(resp),
			Kind:      access.KindLifecycle,
			Timestamp: time.Now().UTC(),
			Metadata:  map[string]string{"state": access.StateOffline, "gateway": h.a.opts.InstanceID},
		})
	}
}

func (h *brokerHook) OnPublished(cl *mqtt.Client, pk packets.Packet) {
	if cl == nil {
		return
	}
	resp, ok := h.a.lookup(cl.ID)
	if !ok {
		return // platform/inline/system publish, not a device
	}
	info, err := parseTopic(pk.TopicName)
	if err != nil {
		return
	}
	msg := h.a.detectUplink(resp, info, pk.Payload)
	if msg == nil {
		return
	}
	h.a.forwardUplink(msg)
}

func (a *Adapter) lookup(clientID string) (*access.AuthResponse, bool) {
	if v, ok := a.sessions.Load(clientID); ok {
		return v.(*access.AuthResponse), true
	}
	return nil, false
}

func (a *Adapter) lookupAndDelete(clientID string) (*access.AuthResponse, bool) {
	if v, ok := a.sessions.LoadAndDelete(clientID); ok {
		return v.(*access.AuthResponse), true
	}
	return nil, false
}

// tenantFromConn extracts the workspace/tenant from the TLS SNI
// (e.g. "factory1.example.com" -> "factory1"). Non-TLS connections return "".
func tenantFromConn(c net.Conn) string {
	tc, ok := c.(*tls.Conn)
	if !ok {
		return ""
	}
	name := tc.ConnectionState().ServerName
	if name == "" {
		return ""
	}
	if i := strings.IndexByte(name, '.'); i > 0 {
		return name[:i]
	}
	return name
}

func deviceRef(resp *access.AuthResponse) access.DeviceRef {
	return access.DeviceRef{
		WorkspaceKey: resp.WorkspaceKey,
		ProductKey:   resp.ProductKey,
		DeviceKey:    resp.DeviceKey,
	}
}
