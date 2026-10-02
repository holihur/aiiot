package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aiiot/server/internal/access"
)

// ClientConfig configures the gateway -> core client.
type ClientConfig struct {
	CoreURL     string
	Token       string
	InstanceID  string
	Protocol    string
	Version     string
	DownlinkURL string
	Timeout     time.Duration
}

// Client is used by standalone gateways to talk to the core control plane.
type Client struct {
	cfg  ClientConfig
	http *http.Client
}

func NewClient(cfg ClientConfig) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) url(path string) string {
	return strings.TrimRight(c.cfg.CoreURL, "/") + path
}

func (c *Client) doJSON(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gateway-Token", c.cfg.Token)
	req.Header.Set("X-Gateway-Protocol", c.cfg.Protocol)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gateway: %s %s: status %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("gateway: decode %s: %w", path, err)
		}
	}
	return nil
}

func (c *Client) Register(ctx context.Context) (*RegisterResponse, error) {
	req := RegisterRequest{
		InstanceID:  c.cfg.InstanceID,
		Protocol:    c.cfg.Protocol,
		Version:     c.cfg.Version,
		DownlinkURL: c.cfg.DownlinkURL,
	}
	var resp RegisterResponse
	if err := c.doJSON(ctx, http.MethodPost, "/internal/gateway/register", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Heartbeat(ctx context.Context, activeDevices int, uptime time.Duration) error {
	return c.doJSON(ctx, http.MethodPost, "/internal/gateway/heartbeat", HeartbeatRequest{
		InstanceID:    c.cfg.InstanceID,
		ActiveDevices: activeDevices,
		UptimeSeconds: int64(uptime.Seconds()),
	}, nil)
}

func (c *Client) Authenticate(ctx context.Context, req *access.AuthRequest) (*access.AuthResponse, error) {
	if req.Protocol == "" {
		req.Protocol = c.cfg.Protocol
	}
	var resp access.AuthResponse
	if err := c.doJSON(ctx, http.MethodPost, "/internal/gateway/auth", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Uplink(ctx context.Context, msg *access.UplinkMessage) error {
	if msg.Protocol == "" {
		msg.Protocol = c.cfg.Protocol
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	return c.doJSON(ctx, http.MethodPost, "/internal/gateway/uplink", UplinkFromAccess(msg), nil)
}

func (c *Client) InstanceID() string { return c.cfg.InstanceID }
func (c *Client) Protocol() string   { return c.cfg.Protocol }
