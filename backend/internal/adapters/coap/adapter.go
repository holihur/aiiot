// Package coap implements the CoAP access adapter. Devices POST to
// workspace-scoped resources; the adapter authenticates them against the core
// and forwards normalized uplinks. Downlinks are delivered as CoAP POSTs to
// the device's last known address.
package coap

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
	coap "github.com/plgd-dev/go-coap/v3"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/mux"
	"github.com/plgd-dev/go-coap/v3/udp"
)

// Options configures the CoAP gateway adapter.
type Options struct {
	UDPAddr string
	TCPAddr string
	Logger  *slog.Logger
}

// Adapter is the CoAP access adapter.
type Adapter struct {
	opts Options
	log  *slog.Logger

	uplink access.UplinkHandler
	auth   access.AuthHandler

	addrs sync.Map // deviceKey -> string remote address
}

func New(opts Options) *Adapter {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{opts: opts, log: log}
}

var _ access.Adapter = (*Adapter)(nil)

func (a *Adapter) Name() string                    { return access.ProtocolCoAPName }
func (a *Adapter) OnUplink(h access.UplinkHandler) { a.uplink = h }
func (a *Adapter) OnAuth(h access.AuthHandler)      { a.auth = h }

// ActiveDevices reports how many distinct device addresses are known.
func (a *Adapter) ActiveDevices() int {
	n := 0
	a.addrs.Range(func(_, _ any) bool { n++; return true })
	return n
}

func (a *Adapter) Start(ctx context.Context) error {
	router := a.router()

	errCh := make(chan error, 2)
	if a.opts.UDPAddr != "" {
		go func() {
			a.log.Info("coap udp listening", "addr", a.opts.UDPAddr)
			errCh <- coap.ListenAndServe("udp", a.opts.UDPAddr, router)
		}()
	}
	if a.opts.TCPAddr != "" {
		go func() {
			a.log.Info("coap tcp listening", "addr", a.opts.TCPAddr)
			errCh <- coap.ListenAndServe("tcp", a.opts.TCPAddr, router)
		}()
	}
	if a.opts.UDPAddr == "" && a.opts.TCPAddr == "" {
		return fmt.Errorf("coap adapter requires at least one listen address")
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func (a *Adapter) Stop(ctx context.Context) error { return nil }

func (a *Adapter) router() *mux.Router {
	r := mux.NewRouter()
	base := "/{productKey}/{deviceKey}"

	r.HandleFunc(base+"/properties", a.handle("property", ""))
	r.HandleFunc(base+"/properties/{identifier}", a.handle("property", ""))
	r.HandleFunc(base+"/events/{identifier}", a.handle("event", ""))
	r.HandleFunc(base+"/services/{identifier}/reply", a.handle("service_reply", ""))
	r.HandleFunc(base+"/peer", a.handle("peer", ""))
	r.HandleFunc(base+"/peer/{target}", a.handle("peer", ""))
	r.HandleFunc(base+"/lifecycle", a.handle("lifecycle", ""))
	r.HandleFunc(base+"/ota", a.handle("ota", ""))
	r.HandleFunc(base+"/ota/{identifier}", a.handle("ota", ""))
	return r
}

func (a *Adapter) handle(kind, _ string) mux.HandlerFunc {
	return func(w mux.ResponseWriter, r *mux.Message) {
		vars := map[string]string{}
		if r.RouteParams != nil {
			vars = r.RouteParams.Vars
		}
		productKey := vars["productKey"]
		deviceKey := vars["deviceKey"]

		resp, err := a.authorize(r, productKey, deviceKey, w)
		if err != nil {
			a.log.Warn("coap device authentication denied", "device", deviceKey, "error", err)
			return
		}

		body, _ := r.ReadBody()
		msg := &access.UplinkMessage{
			Protocol:  access.ProtocolCoAPName,
			Device:    access.DeviceRef{WorkspaceKey: resp.WorkspaceKey, ProductKey: resp.ProductKey, DeviceKey: deviceKey},
			Kind:      access.UplinkKind(kind),
			Payload:   body,
			Timestamp: time.Now().UTC(),
			Metadata:  map[string]string{"path": rPath(r)},
		}
		switch kind {
		case "property":
			msg.Identifier = vars["identifier"]
		case "event", "service_reply":
			msg.Identifier = vars["identifier"]
		case "ota":
			msg.Identifier = vars["identifier"]
		case "peer":
			if t := vars["target"]; t != "" {
				msg.Metadata["to"] = t
			} else {
				msg.Metadata["broadcast"] = "true"
			}
		}
		a.remember(deviceKey, w)
		a.forward(msg)
		_ = w.SetResponse(codes.Changed, message.TextPlain, bytes.NewReader([]byte("ok")))
	}
}

func (a *Adapter) authorize(r *mux.Message, productKey, deviceKey string, w mux.ResponseWriter) (*access.AuthResponse, error) {
	if a.auth == nil {
		return nil, fmt.Errorf("auth handler not configured")
	}
	secret := queryParam(r, "secret")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := a.auth(ctx, &access.AuthRequest{
		ProductKey: productKey,
		DeviceKey:  deviceKey,
		Password:   secret,
		RemoteAddr: remoteAddr(w),
		Protocol:   access.ProtocolCoAPName,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.Authorized {
		return nil, fmt.Errorf("unauthorized")
	}
	return resp, nil
}

func (a *Adapter) remember(deviceKey string, w mux.ResponseWriter) {
	if addr := remoteAddr(w); addr != "" {
		a.addrs.Store(deviceKey, addr)
	}
}

func (a *Adapter) forward(msg *access.UplinkMessage) {
	if a.uplink == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.uplink(ctx, msg); err != nil {
		a.log.Error("coap uplink handling failed", "error", err, "device", msg.Device.DeviceKey)
	}
}

// Downlink sends a CoAP POST to the device's last known address.
func (a *Adapter) Downlink(ctx context.Context, msg *access.DownlinkMessage) error {
	raw, ok := a.addrs.Load(msg.Device.DeviceKey)
	if !ok {
		return fmt.Errorf("unknown address for device %s", msg.Device.DeviceKey)
	}
	addr := raw.(string)

	conn, err := udp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dial device %s: %w", addr, err)
	}
	defer conn.Close()

	path := fmt.Sprintf("/%s/%s/downlink", msg.Device.ProductKey, msg.Device.DeviceKey)
	if msg.Kind == access.KindServiceCall {
		path = fmt.Sprintf("/%s/%s/services/%s/call", msg.Device.ProductKey, msg.Device.DeviceKey, msg.Identifier)
	}
	if msg.Kind == access.KindOTA {
		path = fmt.Sprintf("/%s/%s/ota/upgrade", msg.Device.ProductKey, msg.Device.DeviceKey)
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err = conn.Post(reqCtx, path, message.AppJSON, bytes.NewReader(msg.Payload))
	if err != nil {
		return fmt.Errorf("coap post to device %s: %w", msg.Device.DeviceKey, err)
	}
	return nil
}

func queryParam(r *mux.Message, key string) string {
	qs, err := r.Queries()
	if err != nil {
		return ""
	}
	for _, q := range qs {
		k, v, _ := strings.Cut(q, "=")
		if k == key {
			return v
		}
	}
	return ""
}

func remoteAddr(w mux.ResponseWriter) string {
	if w.Conn() == nil {
		return ""
	}
	if a := w.Conn().RemoteAddr(); a != nil {
		return a.String()
	}
	return ""
}

func rPath(r *mux.Message) string {
	if r.RouteParams != nil {
		return r.RouteParams.Path
	}
	if p, err := r.Path(); err == nil {
		return p
	}
	return ""
}
