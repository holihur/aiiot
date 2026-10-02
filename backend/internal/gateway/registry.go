package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Instance is a registered protocol gateway as seen by the core.
type Instance struct {
	InstanceID    string            `json:"instanceId"`
	Protocol      string            `json:"protocol"`
	Version       string            `json:"version"`
	DownlinkURL   string            `json:"downlinkUrl"`
	Metadata      models.JSONMap    `json:"metadata,omitempty"`
	RegisteredAt  time.Time         `json:"registeredAt"`
	LastHeartbeat time.Time         `json:"lastHeartbeat"`
	ActiveDevices int               `json:"activeDevices"`
	UptimeSeconds int64             `json:"uptimeSeconds"`
	Healthy       bool              `json:"healthy"`
}

// Registry tracks live gateways and routes downlinks to them. Gateway state is
// owned by the core (operations side): it is persisted in PostgreSQL so that
// instances remain visible after restarts and while offline.
type Registry struct {
	mu      sync.RWMutex
	timeout time.Duration
	byID    map[string]*Instance
	db      *gorm.DB
	http    *http.Client
}

func NewRegistry(db *gorm.DB, timeout time.Duration) *Registry {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Registry{
		timeout: timeout,
		byID:    make(map[string]*Instance),
		db:      db,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Load restores persisted gateway instances into the in-memory routing table.
func (r *Registry) Load(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	var rows []models.GatewayInstance
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range rows {
		inst := &Instance{
			InstanceID:    row.InstanceID,
			Protocol:      row.Protocol,
			Version:       row.Version,
			DownlinkURL:   row.DownlinkURL,
			Metadata:      row.Metadata,
			RegisteredAt:  row.FirstSeenAt,
			ActiveDevices: row.ActiveDevices,
			UptimeSeconds: row.UptimeSeconds,
		}
		if row.LastHeartbeat != nil {
			inst.LastHeartbeat = *row.LastHeartbeat
		}
		inst.Healthy = now.Sub(inst.LastHeartbeat) <= r.timeout
		r.byID[row.InstanceID] = inst
	}
	return nil
}

func (r *Registry) Register(req *RegisterRequest) *Instance {
	now := time.Now().UTC()
	inst := &Instance{
		InstanceID:    req.InstanceID,
		Protocol:      req.Protocol,
		Version:       req.Version,
		DownlinkURL:   req.DownlinkURL,
		Metadata:      req.Metadata,
		RegisteredAt:  now,
		LastHeartbeat: now,
		Healthy:       true,
	}
	r.mu.Lock()
	if existing, ok := r.byID[req.InstanceID]; ok {
		inst.RegisteredAt = existing.RegisteredAt
	}
	r.byID[req.InstanceID] = inst
	r.mu.Unlock()

	if r.db != nil {
		row := &models.GatewayInstance{
			InstanceID:    req.InstanceID,
			Protocol:      req.Protocol,
			Version:       req.Version,
			DownlinkURL:   req.DownlinkURL,
			Metadata:      req.Metadata,
			Status:        models.GatewayStatusActive,
			FirstSeenAt:   now,
			LastHeartbeat: &now,
		}
		_ = r.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "instance_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"protocol", "version", "downlink_url", "metadata", "status", "last_heartbeat",
			}),
		}).Create(row).Error
	}

	clone := *inst
	return &clone
}

func (r *Registry) Heartbeat(req *HeartbeatRequest) bool {
	now := time.Now().UTC()
	r.mu.Lock()
	inst, ok := r.byID[req.InstanceID]
	if ok {
		inst.LastHeartbeat = now
		inst.ActiveDevices = req.ActiveDevices
		inst.UptimeSeconds = req.UptimeSeconds
		inst.Healthy = true
	}
	r.mu.Unlock()

	if r.db != nil {
		res := r.db.Model(&models.GatewayInstance{}).Where("instance_id = ?", req.InstanceID).Updates(map[string]any{
			"last_heartbeat": now,
			"active_devices": req.ActiveDevices,
			"uptime_seconds": req.UptimeSeconds,
			"status":         models.GatewayStatusActive,
		})
		if res.Error == nil && res.RowsAffected > 0 {
			return true
		}
	}
	return ok
}

func (r *Registry) healthyLocked(i *Instance) bool {
	return time.Since(i.LastHeartbeat) <= r.timeout
}

// List returns all gateway instances (including offline ones) with a computed
// health flag, so the operations console can display the full fleet.
func (r *Registry) List() []Instance {
	if r.db != nil {
		var rows []models.GatewayInstance
		if err := r.db.Order("protocol, instance_id").Find(&rows).Error; err == nil {
			out := make([]Instance, 0, len(rows))
			r.mu.RLock()
			defer r.mu.RUnlock()
			for _, row := range rows {
				inst := Instance{
					InstanceID:    row.InstanceID,
					Protocol:      row.Protocol,
					Version:       row.Version,
					DownlinkURL:   row.DownlinkURL,
					Metadata:      row.Metadata,
					RegisteredAt:  row.FirstSeenAt,
					ActiveDevices: row.ActiveDevices,
					UptimeSeconds: row.UptimeSeconds,
				}
				if row.LastHeartbeat != nil {
					inst.LastHeartbeat = *row.LastHeartbeat
				}
				// Prefer live in-memory values when present.
				if live, ok := r.byID[row.InstanceID]; ok {
					inst.LastHeartbeat = live.LastHeartbeat
					inst.ActiveDevices = live.ActiveDevices
					inst.UptimeSeconds = live.UptimeSeconds
				}
				inst.Healthy = r.healthyLocked(&inst)
				out = append(out, inst)
			}
			return out
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Instance, 0, len(r.byID))
	for _, i := range r.byID {
		clone := *i
		clone.Healthy = r.healthyLocked(i)
		out = append(out, clone)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Protocol != out[b].Protocol {
			return out[a].Protocol < out[b].Protocol
		}
		return out[a].InstanceID < out[b].InstanceID
	})
	return out
}

// Deregister removes a gateway from the operations registry (memory + DB).
func (r *Registry) Deregister(ctx context.Context, instanceID string) error {
	r.mu.Lock()
	delete(r.byID, instanceID)
	r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Where("instance_id = ?", instanceID).Delete(&models.GatewayInstance{}).Error
}

// Downlink delivers a message by POSTing it to a healthy gateway that speaks
// the device's protocol. It tries every candidate until one succeeds.
func (r *Registry) Downlink(ctx context.Context, protocol string, msg *access.DownlinkMessage) error {
	r.mu.RLock()
	var candidates []*Instance
	for _, i := range r.byID {
		if (protocol == "" || i.Protocol == protocol) && r.healthyLocked(i) {
			candidates = append(candidates, i)
		}
	}
	r.mu.RUnlock()

	if len(candidates) == 0 {
		return fmt.Errorf("no healthy gateway for protocol %q", protocol)
	}

	env := DownlinkEnvelope{
		Device:     msg.Device,
		Kind:       string(msg.Kind),
		Identifier: msg.Identifier,
		Payload:    msg.Payload,
		Metadata:   msg.Metadata,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}

	var lastErr error
	for _, inst := range candidates {
		if inst.DownlinkURL == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.DownlinkURL, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := r.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("gateway %s downlink status %d: %s", inst.InstanceID, resp.StatusCode, string(raw))
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no gateway accepted downlink for protocol %q", protocol)
	}
	return lastErr
}
