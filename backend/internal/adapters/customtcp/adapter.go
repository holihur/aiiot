// Package customtcp implements a reference "custom protocol" access adapter.
//
// Wire format: newline-delimited JSON (NDJSON) over TCP. Every frame is one
// JSON object per line. A session must authenticate with an {"type":"auth"}
// frame before any telemetry is accepted.
//
// See docs/custom-protocol.md for the full specification.
package customtcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
)

// Options configures the custom TCP adapter.
type Options struct {
	Addr   string
	Logger *slog.Logger
}

// Adapter implements the custom TCP protocol.
type Adapter struct {
	opts Options
	log  *slog.Logger

	uplink access.UplinkHandler
	auth   access.AuthHandler

	mu    sync.RWMutex
	conns map[string]*session // deviceKey -> session
}

type session struct {
	key    access.DeviceRef
	conn   net.Conn
	writer *bufio.Writer
	wmu    sync.Mutex
}

func New(opts Options) *Adapter {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{opts: opts, log: log, conns: map[string]*session{}}
}

var _ access.Adapter = (*Adapter)(nil)

func (a *Adapter) Name() string                    { return access.ProtocolCustomName }
func (a *Adapter) OnUplink(h access.UplinkHandler) { a.uplink = h }
func (a *Adapter) OnAuth(h access.AuthHandler)      { a.auth = h }

func (a *Adapter) ActiveDevices() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.conns)
}

func (a *Adapter) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.opts.Addr)
	if err != nil {
		return fmt.Errorf("custom tcp listen: %w", err)
	}
	a.log.Info("custom tcp gateway listening", "addr", a.opts.Addr)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}
		go a.serve(ctx, conn)
	}
}

func (a *Adapter) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.conns {
		_ = s.conn.Close()
	}
	a.conns = map[string]*session{}
	return nil
}

type frame struct {
	Type       string          `json:"type"`
	ProductKey string          `json:"productKey,omitempty"`
	DeviceKey  string          `json:"deviceKey,omitempty"`
	Secret     string          `json:"secret,omitempty"`
	Identifier string          `json:"identifier,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	To         string          `json:"to,omitempty"`
	Broadcast  bool            `json:"broadcast,omitempty"`
	State      string          `json:"state,omitempty"`
	ID         string          `json:"id,omitempty"`
	Ts         *time.Time      `json:"ts,omitempty"`
}

func (a *Adapter) serve(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReaderSize(conn, 64<<10)
	s := &session{conn: conn, writer: bufio.NewWriter(conn)}

	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	line, err := readLine(reader)
	if err != nil {
		return
	}
	var authFrame frame
	if err := json.Unmarshal(line, &authFrame); err != nil || authFrame.Type != "auth" {
		a.write(s, frame{Type: "error", ID: authFrame.ID, Payload: raw(`"first frame must be auth"`)})
		return
	}
	ref, ok := a.authenticate(ctx, conn, &authFrame, s)
	if !ok {
		return
	}
	a.register(ref, s)
	defer a.unregister(ref)

	a.write(s, frame{Type: "auth_ok", ID: authFrame.ID})
	a.forward(&access.UplinkMessage{
		Protocol:  access.ProtocolCustomName,
		Device:    ref,
		Kind:      access.KindLifecycle,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]string{"state": access.StateOnline},
	})

	_ = conn.SetReadDeadline(time.Time{})
	for {
		line, err := readLine(reader)
		if err != nil {
			break
		}
		var f frame
		if err := json.Unmarshal(line, &f); err != nil {
			a.write(s, frame{Type: "error", Payload: raw(`"invalid json frame"`)})
			continue
		}
		if msg := a.toUplink(ref, &f); msg != nil {
			a.forward(msg)
			if f.ID != "" {
				a.write(s, frame{Type: "ack", ID: f.ID})
			}
		} else {
			a.write(s, frame{Type: "error", ID: f.ID, Payload: raw(fmt.Sprintf("%q", "unsupported frame type "+f.Type))})
		}
	}
	a.forward(&access.UplinkMessage{
		Protocol:  access.ProtocolCustomName,
		Device:    ref,
		Kind:      access.KindLifecycle,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]string{"state": access.StateOffline},
	})
}

func (a *Adapter) authenticate(ctx context.Context, conn net.Conn, f *frame, _ *session) (access.DeviceRef, bool) {
	if a.auth == nil {
		return access.DeviceRef{}, false
	}
	remote := ""
	if addr := conn.RemoteAddr(); addr != nil {
		remote = addr.String()
	}
	resp, err := a.auth(ctx, &access.AuthRequest{
		ProductKey: f.ProductKey,
		DeviceKey:  f.DeviceKey,
		Password:   f.Secret,
		RemoteAddr: remote,
		Protocol:   access.ProtocolCustomName,
	})
	if err != nil || resp == nil || !resp.Authorized {
		reason := "unauthorized"
		if err != nil {
			reason = err.Error()
		}
		a.write(&session{writer: bufio.NewWriter(conn), conn: conn}, frame{Type: "auth_error", Payload: raw(fmt.Sprintf("%q", reason))})
		return access.DeviceRef{}, false
	}
	return access.DeviceRef{WorkspaceKey: resp.WorkspaceKey, ProductKey: resp.ProductKey, DeviceKey: resp.DeviceKey}, true
}

func (a *Adapter) toUplink(ref access.DeviceRef, f *frame) *access.UplinkMessage {
	msg := &access.UplinkMessage{
		Protocol:  access.ProtocolCustomName,
		Device:    ref,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]string{},
	}
	if f.Ts != nil {
		msg.Timestamp = *f.Ts
	}
	switch f.Type {
	case "property":
		msg.Kind = access.KindProperty
		msg.Identifier = f.Identifier
		if len(f.Value) > 0 {
			msg.Payload = f.Value
		} else if len(f.Params) > 0 {
			msg.Payload = f.Params
		} else {
			return nil
		}
	case "properties":
		msg.Kind = access.KindProperty
		msg.Payload = f.Params
	case "event":
		msg.Kind = access.KindEvent
		msg.Identifier = f.Identifier
		msg.Payload = f.Params
	case "service_reply":
		msg.Kind = access.KindServiceReply
		msg.Identifier = f.Identifier
		msg.Payload = f.Params
	case "peer":
		msg.Kind = access.KindPeer
		if f.Broadcast {
			msg.Metadata["broadcast"] = "true"
		} else {
			msg.Metadata["to"] = f.To
		}
		msg.Payload = f.Payload
		if len(msg.Payload) == 0 {
			msg.Payload = f.Params
		}
	case "lifecycle":
		msg.Kind = access.KindLifecycle
		msg.Metadata["state"] = f.State
	case "ota":
		msg.Kind = access.KindOTA
		msg.Identifier = f.Identifier
		msg.Payload = f.Params
		if len(msg.Payload) == 0 {
			msg.Payload = f.Payload
		}
	default:
		return nil
	}
	if msg.Payload == nil {
		msg.Payload = []byte("{}")
	}
	return msg
}

func (a *Adapter) forward(msg *access.UplinkMessage) {
	if a.uplink == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.uplink(ctx, msg); err != nil {
		a.log.Error("custom uplink handling failed", "error", err, "device", msg.Device.DeviceKey)
	}
}

// Downlink writes a NDJSON frame to the device's TCP session.
func (a *Adapter) Downlink(ctx context.Context, msg *access.DownlinkMessage) error {
	a.mu.RLock()
	s, ok := a.conns[msg.Device.DeviceKey]
	a.mu.RUnlock()
	if !ok {
		return fmt.Errorf("device %s not connected", msg.Device.DeviceKey)
	}

	var out frame
	switch msg.Kind {
	case access.KindProperty:
		out = frame{Type: "property_set", Params: msg.Payload}
	case access.KindServiceCall:
		out = frame{Type: "service_call", Identifier: msg.Identifier, Params: msg.Payload}
	case access.KindPeer:
		out = frame{Type: "peer", Payload: msg.Payload}
		out.DeviceKey = msg.Metadata["from"]
	case access.KindOTA:
		out = frame{Type: "ota", Params: msg.Payload}
	default:
		out = frame{Type: "command", Payload: msg.Payload}
	}
	return a.write(s, out)
}

func (a *Adapter) write(s *session, f frame) error {
	line, err := json.Marshal(f)
	if err != nil {
		return err
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if _, err := s.writer.Write(append(line, '\n')); err != nil {
		return err
	}
	return s.writer.Flush()
}

func (a *Adapter) register(ref access.DeviceRef, s *session) {
	a.mu.Lock()
	a.conns[ref.DeviceKey] = s
	a.mu.Unlock()
}

func (a *Adapter) unregister(ref access.DeviceRef) {
	a.mu.Lock()
	delete(a.conns, ref.DeviceKey)
	a.mu.Unlock()
}

func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(line), nil
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }
