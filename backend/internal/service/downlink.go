package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/gateway"
	"github.com/aiiot/server/internal/models"
)

// DownlinkService routes platform-originated messages to devices through the
// gateway registry. It is the single place that knows how to address a device.
type DownlinkService struct {
	resolver *Resolver
	registry *gateway.Registry
	log      *slog.Logger
}

func NewDownlinkService(resolver *Resolver, registry *gateway.Registry, log *slog.Logger) *DownlinkService {
	return &DownlinkService{resolver: resolver, registry: registry, log: log}
}

// Send delivers a downlink to a device resolved by ID.
func (s *DownlinkService) Send(ctx context.Context, deviceID uint, kind access.UplinkKind, identifier string, payload []byte, meta map[string]string) error {
	dc, err := s.resolver.ResolveByDeviceID(ctx, deviceID)
	if err != nil {
		return err
	}
	return s.SendTo(ctx, dc, kind, identifier, payload, meta)
}

// SendTo delivers a downlink using an already-resolved device context.
func (s *DownlinkService) SendTo(ctx context.Context, dc *DeviceContext, kind access.UplinkKind, identifier string, payload []byte, meta map[string]string) error {
	msg := &access.DownlinkMessage{
		Protocol:   dc.Product.Protocol,
		Device:     dc.Ref(),
		Kind:       kind,
		Identifier: identifier,
		Payload:    payload,
		Metadata:   meta,
	}
	if err := s.registry.Downlink(ctx, dc.Product.Protocol, msg); err != nil {
		return fmt.Errorf("downlink to device %s: %w", dc.Device.Key, err)
	}
	return nil
}

// BroadcastWorkspace delivers a message to every enabled device in a
// workspace except the excluded device (usually the origin).
func (s *DownlinkService) BroadcastWorkspace(ctx context.Context, workspaceID, excludeDeviceID uint, kind access.UplinkKind, identifier string, payload []byte, meta map[string]string) (int, error) {
	dcs, err := s.WorkspaceDevices(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	sent := 0
	var lastErr error
	for _, dc := range dcs {
		if dc.Device.ID == excludeDeviceID {
			continue
		}
		if err := s.SendTo(ctx, dc, kind, identifier, payload, meta); err != nil {
			lastErr = err
			continue
		}
		sent++
	}
	return sent, lastErr
}

// WorkspaceDevices returns resolved contexts for all enabled devices in a
// workspace. It is also used to enforce workspace membership for peer routing.
func (s *DownlinkService) WorkspaceDevices(ctx context.Context, workspaceID uint) ([]*DeviceContext, error) {
	var devices []models.Device
	if err := s.resolver.db.WithContext(ctx).
		Where("workspace_id = ? AND status = ?", workspaceID, models.DeviceStatusEnabled).
		Find(&devices).Error; err != nil {
		return nil, err
	}
	out := make([]*DeviceContext, 0, len(devices))
	for i := range devices {
		dc, err := s.resolver.ResolveByDeviceID(ctx, devices[i].ID)
		if err != nil {
			continue
		}
		out = append(out, dc)
	}
	return out, nil
}

// ResolveDeviceKey finds a device inside a workspace by key. Used to validate
// directed peer messages (both devices must share a workspace).
func (s *DownlinkService) ResolveDeviceKey(ctx context.Context, workspaceID uint, deviceKey string) (*DeviceContext, error) {
	var device models.Device
	if err := s.resolver.db.WithContext(ctx).
		Where("workspace_id = ? AND key = ?", workspaceID, deviceKey).
		First(&device).Error; err != nil {
		return nil, fmt.Errorf("device %q not in workspace %d", deviceKey, workspaceID)
	}
	return s.resolver.ResolveByDeviceID(ctx, device.ID)
}
