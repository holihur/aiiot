package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/metrics"
	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
)

// IngestService is the protocol-agnostic message pipeline. Every gateway
// forwards uplinks here; it resolves the device, persists telemetry, updates
// device state, routes peer messages and drives the rule engine.
type IngestService struct {
	db        *gorm.DB
	resolver  *Resolver
	telemetry *TelemetryService
	rules     *RuleEngine
	downlink  *DownlinkService
	shadow    *ShadowService
	ota       *OTAService
	hub       *Hub
	log       *slog.Logger

	offlineAfter time.Duration

	typesMu sync.RWMutex
	types   map[uint]thingTypes // productID -> identifier -> dataType
}

type thingTypes struct {
	byIdentifier map[string]string
	expires      time.Time
}

func NewIngestService(db *gorm.DB, resolver *Resolver, telemetry *TelemetryService, rules *RuleEngine, downlink *DownlinkService, shadow *ShadowService, ota *OTAService, hub *Hub, offlineAfter time.Duration, log *slog.Logger) *IngestService {
	if offlineAfter <= 0 {
		offlineAfter = 5 * time.Minute
	}
	return &IngestService{
		db:           db,
		resolver:     resolver,
		telemetry:    telemetry,
		rules:        rules,
		downlink:     downlink,
		shadow:       shadow,
		ota:          ota,
		hub:          hub,
		log:          log,
		offlineAfter: offlineAfter,
		types:        map[uint]thingTypes{},
	}
}

// Handle processes one normalized uplink message and records ingest metrics.
func (s *IngestService) Handle(ctx context.Context, msg *access.UplinkMessage) error {
	start := time.Now()
	err := s.handle(ctx, msg)
	metrics.ObserveIngest(msg.Protocol, string(msg.Kind), err, start)
	return err
}

func (s *IngestService) handle(ctx context.Context, msg *access.UplinkMessage) error {
	dc, err := s.resolver.Resolve(ctx, msg.Device.ProductKey, msg.Device.DeviceKey)
	if err != nil {
		return fmt.Errorf("resolve device: %w", err)
	}
	// Defense in depth: the gateway-provided workspace must match the device.
	if msg.Device.WorkspaceKey != "" && msg.Device.WorkspaceKey != dc.WorkspaceKey {
		return fmt.Errorf("workspace mismatch for device %s", dc.Device.Key)
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}

	switch msg.Kind {
	case access.KindProperty:
		return s.handleProperty(ctx, dc, msg)
	case access.KindEvent, access.KindServiceReply:
		return s.handleEvent(ctx, dc, msg)
	case access.KindLifecycle:
		return s.handleLifecycle(ctx, dc, msg)
	case access.KindPeer:
		return s.handlePeer(ctx, dc, msg)
	case access.KindOTA:
		return s.handleOTA(ctx, dc, msg)
	default:
		return fmt.Errorf("unsupported uplink kind %q", msg.Kind)
	}
}

func (s *IngestService) handleProperty(ctx context.Context, dc *DeviceContext, msg *access.UplinkMessage) error {
	params, value, err := decodePropertyPayload(msg)
	if err != nil {
		return fmt.Errorf("decode property payload: %w", err)
	}
	s.resolver.TouchSeen(ctx, dc.Device.ID)

	if msg.Identifier != "" {
		value = params[msg.Identifier]
	}
	if len(params) == 0 {
		return fmt.Errorf("empty property payload")
	}

	types := s.dataTypes(ctx, dc.Product.ID)
	samples := make([]*Sample, 0, len(params))
	for identifier, v := range params {
		dataType := types[identifier]
		if dataType == "" {
			dataType = inferType(v)
		}
		smp := Coerce(dataType, v)
		smp.DeviceID = dc.Device.ID
		smp.ProjectID = dc.Project.ID
		smp.WorkspaceID = dc.Workspace.ID
		smp.ProductID = dc.Product.ID
		smp.Identifier = identifier
		smp.Time = msg.Timestamp
		samples = append(samples, smp)

		s.rules.Evaluate(ctx, &RuleEvent{
			Kind:        access.KindProperty,
			ProjectID:   dc.Project.ID,
			WorkspaceID: dc.Workspace.ID,
			ProductID:   dc.Product.ID,
			DeviceID:    dc.Device.ID,
			DeviceKey:   dc.Device.Key,
			Identifier:  identifier,
			Value:       v,
			Params:      params,
			Payload:     value,
			Now:         msg.Timestamp,
			Device:      dc.Device,
		})
	}
	if err := s.telemetry.Write(ctx, samples); err != nil {
		return err
	}
	if s.hub != nil {
		for identifier, v := range params {
			s.hub.Publish(Event{
				Type: "telemetry", ProjectID: dc.Project.ID, DeviceID: dc.Device.ID,
				DeviceKey: dc.Device.Key, Identifier: identifier, Value: v, Time: msg.Timestamp,
			})
		}
	}
	// Keep the device shadow's reported state in sync with telemetry.
	if s.shadow != nil {
		if _, err := s.shadow.Report(ctx, dc, params); err != nil {
			s.log.Warn("shadow report failed", "error", err, "device", dc.Device.Key)
		}
	}
	return nil
}

func (s *IngestService) handleEvent(ctx context.Context, dc *DeviceContext, msg *access.UplinkMessage) error {
	params, value, err := decodePropertyPayload(msg)
	if err != nil {
		return fmt.Errorf("decode event payload: %w", err)
	}
	s.resolver.TouchSeen(ctx, dc.Device.ID)

	event := &models.DeviceEvent{
		ProjectID:   dc.Project.ID,
		WorkspaceID: dc.Workspace.ID,
		ProductID:   dc.Product.ID,
		DeviceID:    dc.Device.ID,
		Identifier:  msg.Identifier,
		Type:        string(msg.Kind),
		Payload:     models.JSONMap(params),
		OccurredAt:  msg.Timestamp,
	}
	if err := s.db.WithContext(ctx).Create(event).Error; err != nil {
		s.log.Warn("persist device event failed", "error", err)
	}

	s.rules.Evaluate(ctx, &RuleEvent{
		Kind:        msg.Kind,
		ProjectID:   dc.Project.ID,
		WorkspaceID: dc.Workspace.ID,
		ProductID:   dc.Product.ID,
		DeviceID:    dc.Device.ID,
		DeviceKey:   dc.Device.Key,
		Identifier:  msg.Identifier,
		Params:      params,
		Payload:     value,
		Now:         msg.Timestamp,
		Device:      dc.Device,
	})
	return nil
}

func (s *IngestService) handleLifecycle(ctx context.Context, dc *DeviceContext, msg *access.UplinkMessage) error {
	state := msg.Metadata["state"]
	now := msg.Timestamp
	updates := map[string]any{"last_seen_at": now}
	switch state {
	case access.StateOnline:
		updates["online"] = true
		updates["last_online_at"] = now
	case access.StateOffline:
		updates["online"] = false
	default:
		return fmt.Errorf("unknown lifecycle state %q", state)
	}
	if err := s.db.WithContext(ctx).Model(&models.Device{}).Where("id = ?", dc.Device.ID).Updates(updates).Error; err != nil {
		return err
	}
	if s.hub != nil {
		online := state == access.StateOnline
		s.hub.Publish(Event{
			Type: "lifecycle", ProjectID: dc.Project.ID, DeviceID: dc.Device.ID,
			DeviceKey: dc.Device.Key, Online: &online, Time: now,
		})
	}

	// Journal the transition as an event so the device timeline can show
	// when the device came online / went offline.
	event := &models.DeviceEvent{
		ProjectID:   dc.Project.ID,
		WorkspaceID: dc.Workspace.ID,
		ProductID:   dc.Product.ID,
		DeviceID:    dc.Device.ID,
		Type:        "lifecycle",
		Payload:     models.JSONMap{"state": state},
		OccurredAt:  now,
	}
	if err := s.db.WithContext(ctx).Create(event).Error; err != nil {
		s.log.Warn("persist lifecycle event failed", "error", err)
	}

	s.rules.Evaluate(ctx, &RuleEvent{
		Kind:        access.KindLifecycle,
		State:       state,
		ProjectID:   dc.Project.ID,
		WorkspaceID: dc.Workspace.ID,
		ProductID:   dc.Product.ID,
		DeviceID:    dc.Device.ID,
		DeviceKey:   dc.Device.Key,
		Payload:     string(msg.Payload),
		Now:         now,
		Device:      dc.Device,
	})
	return nil
}

func (s *IngestService) handlePeer(ctx context.Context, dc *DeviceContext, msg *access.UplinkMessage) error {
	s.resolver.TouchSeen(ctx, dc.Device.ID)

	var payload any
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}
	meta := map[string]string{"from": dc.Device.Key, "fromDeviceId": fmt.Sprint(dc.Device.ID)}

	s.rules.Evaluate(ctx, &RuleEvent{
		Kind:        access.KindPeer,
		ProjectID:   dc.Project.ID,
		WorkspaceID: dc.Workspace.ID,
		ProductID:   dc.Product.ID,
		DeviceID:    dc.Device.ID,
		DeviceKey:   dc.Device.Key,
		Payload:     payload,
		Now:         msg.Timestamp,
		Device:      dc.Device,
	})

	if msg.Metadata["broadcast"] == "true" {
		sent, err := s.downlink.BroadcastWorkspace(ctx, dc.Workspace.ID, dc.Device.ID, access.KindPeer, "", msg.Payload, meta)
		s.log.Debug("peer broadcast", "workspace", dc.Workspace.Key, "from", dc.Device.Key, "sent", sent)
		return err
	}

	target := msg.Metadata["to"]
	if target == "" {
		return fmt.Errorf("peer message missing target")
	}
	// Workspace isolation: the target must be in the same workspace.
	targetDC, err := s.downlink.ResolveDeviceKey(ctx, dc.Workspace.ID, target)
	if err != nil {
		return fmt.Errorf("peer target: %w", err)
	}
	return s.downlink.SendTo(ctx, targetDC, access.KindPeer, "", msg.Payload, meta)
}

func (s *IngestService) handleOTA(ctx context.Context, dc *DeviceContext, msg *access.UplinkMessage) error {
	s.resolver.TouchSeen(ctx, dc.Device.ID)
	if s.ota == nil {
		return fmt.Errorf("ota service unavailable")
	}
	return s.ota.HandleReport(ctx, dc, msg)
}

// SweepOffline marks devices offline whose last-seen timestamp is too old.
// It also broadcasts device_offline rule events.
func (s *IngestService) SweepOffline(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-s.offlineAfter)
	var devices []models.Device
	if err := s.db.WithContext(ctx).
		Where("online = ? AND (last_seen_at IS NULL OR last_seen_at < ?)", true, cutoff).
		Find(&devices).Error; err != nil {
		s.log.Warn("offline sweep query failed", "error", err)
		return
	}
	for i := range devices {
		d := devices[i]
		if err := s.db.WithContext(ctx).Model(&models.Device{}).Where("id = ?", d.ID).
			Update("online", false).Error; err != nil {
			continue
		}
		dc, err := s.resolver.ResolveByDeviceID(ctx, d.ID)
		if err != nil {
			continue
		}
		s.rules.Evaluate(ctx, &RuleEvent{
			Kind:        access.KindLifecycle,
			State:       access.StateOffline,
			ProjectID:   dc.Project.ID,
			WorkspaceID: dc.Workspace.ID,
			ProductID:   dc.Product.ID,
			DeviceID:    dc.Device.ID,
			DeviceKey:   dc.Device.Key,
			Now:         time.Now().UTC(),
			Device:      dc.Device,
		})
	}
}

// dataTypes returns (and caches) the thing-model data type per identifier.
func (s *IngestService) dataTypes(ctx context.Context, productID uint) map[string]string {
	s.typesMu.RLock()
	entry, ok := s.types[productID]
	s.typesMu.RUnlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.byIdentifier
	}

	var model models.ThingModel
	result := map[string]string{}
	if err := s.db.WithContext(ctx).Where("product_id = ?", productID).First(&model).Error; err == nil {
		var elements []models.ThingModelElement
		if err := s.db.WithContext(ctx).Where("thing_model_id = ?", model.ID).Find(&elements).Error; err == nil {
			for _, el := range elements {
				result[el.Identifier] = el.DataType
			}
		}
	}
	s.typesMu.Lock()
	s.types[productID] = thingTypes{byIdentifier: result, expires: time.Now().Add(time.Minute)}
	s.typesMu.Unlock()
	return result
}

// decodePropertyPayload accepts either:
//
//	{"temperature":25.4,"humidity":60}          (bare params)
//	{"id":"1","ts":1699999999,"params":{...}}   (envelope)
//	25.4                                        (single value + identifier)
func decodePropertyPayload(msg *access.UplinkMessage) (map[string]any, any, error) {
	if len(msg.Payload) == 0 {
		return nil, nil, fmt.Errorf("empty payload")
	}
	var raw any
	if err := json.Unmarshal(msg.Payload, &raw); err != nil {
		return nil, nil, err
	}
	if msg.Identifier != "" {
		if m, ok := raw.(map[string]any); ok {
			return m, m, nil
		}
		return map[string]any{msg.Identifier: raw}, raw, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, raw, fmt.Errorf("expected JSON object")
	}
	if params, ok := obj["params"].(map[string]any); ok {
		return params, params, nil
	}
	return obj, obj, nil
}

func inferType(v any) string {
	switch v.(type) {
	case float64, float32, int, int64:
		return "double"
	case bool:
		return "bool"
	case string:
		return "text"
	default:
		return "json"
	}
}
