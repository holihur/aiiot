package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
)

// Shadow is the computed view of a device shadow returned to callers.
type Shadow struct {
	DeviceID  uint           `json:"deviceId"`
	Desired   models.JSONMap `json:"desired"`
	Reported  models.JSONMap `json:"reported"`
	Delta     models.JSONMap `json:"delta"`
	Version   int64          `json:"version"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// ShadowService maintains per-device desired/reported state and pushes the
// delta to connected devices so they converge on the desired state. Every
// change is recorded in device_shadow_logs for auditing.
type ShadowService struct {
	db       *gorm.DB
	downlink *DownlinkService
	log      *slog.Logger

	// onDelta is invoked whenever a non-empty delta is produced, so rules can
	// trigger on shadow-state changes without creating an import cycle.
	onDelta func(ctx context.Context, dc *DeviceContext, delta map[string]any)
}

// SetDeltaHook registers the callback used for shadow-delta rule triggers.
func (s *ShadowService) SetDeltaHook(fn func(ctx context.Context, dc *DeviceContext, delta map[string]any)) {
	s.onDelta = fn
}

func NewShadowService(db *gorm.DB, downlink *DownlinkService, log *slog.Logger) *ShadowService {
	return &ShadowService{db: db, downlink: downlink, log: log}
}

func (s *ShadowService) loadOrCreate(ctx context.Context, deviceID uint) (*models.DeviceShadow, error) {
	var sh models.DeviceShadow
	err := s.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&sh).Error
	if err == nil {
		return &sh, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	sh = models.DeviceShadow{
		DeviceID: deviceID,
		Desired:  models.JSONMap{},
		Reported: models.JSONMap{},
	}
	if err := s.db.WithContext(ctx).Create(&sh).Error; err != nil {
		if e := s.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&sh).Error; e == nil {
			return &sh, nil
		}
		return nil, err
	}
	return &sh, nil
}

func toShadow(deviceID uint, sh *models.DeviceShadow) *Shadow {
	desired := sh.Desired
	if desired == nil {
		desired = models.JSONMap{}
	}
	reported := sh.Reported
	if reported == nil {
		reported = models.JSONMap{}
	}
	return &Shadow{
		DeviceID:  deviceID,
		Desired:   desired,
		Reported:  reported,
		Delta:     computeDelta(desired, reported),
		Version:   sh.Version,
		UpdatedAt: sh.UpdatedAt,
	}
}

func computeDelta(desired, reported models.JSONMap) models.JSONMap {
	delta := models.JSONMap{}
	for k, dv := range desired {
		rv, ok := reported[k]
		if !ok || !reflect.DeepEqual(rv, dv) {
			delta[k] = dv
		}
	}
	return delta
}

// Get returns the current shadow (creating an empty one on first access).
func (s *ShadowService) Get(ctx context.Context, deviceID uint) (*Shadow, error) {
	sh, err := s.loadOrCreate(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	return toShadow(deviceID, sh), nil
}

// History returns recent shadow changes for a device, newest first.
func (s *ShadowService) History(ctx context.Context, deviceID uint, limit int) ([]models.DeviceShadowLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var logs []models.DeviceShadowLog
	err := s.db.WithContext(ctx).Where("device_id = ?", deviceID).
		Order("occurred_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// ApplyDesired merges a JSON merge-patch into the desired state (null removes a
// key), records the change and pushes the resulting delta to the device.
func (s *ShadowService) ApplyDesired(ctx context.Context, dc *DeviceContext, patch map[string]any, source string, ruleID *uint) (*Shadow, error) {
	sh, err := s.loadOrCreate(ctx, dc.Device.ID)
	if err != nil {
		return nil, err
	}
	desired := mergePatch(sh.Desired, patch)
	sh.Desired = desired
	sh.Version++
	if err := s.db.WithContext(ctx).Model(sh).Updates(map[string]any{
		"desired": desired,
		"version": sh.Version,
	}).Error; err != nil {
		return nil, err
	}
	shadow := toShadow(dc.Device.ID, sh)
	s.record(ctx, dc, source, ruleID, shadow)
	s.pushDelta(ctx, dc, shadow.Delta)
	return shadow, nil
}

// ClearDesired removes all desired keys.
func (s *ShadowService) ClearDesired(ctx context.Context, dc *DeviceContext, source string) (*Shadow, error) {
	sh, err := s.loadOrCreate(ctx, dc.Device.ID)
	if err != nil {
		return nil, err
	}
	sh.Desired = models.JSONMap{}
	sh.Version++
	if err := s.db.WithContext(ctx).Model(sh).Updates(map[string]any{
		"desired": models.JSONMap{},
		"version": sh.Version,
	}).Error; err != nil {
		return nil, err
	}
	shadow := toShadow(dc.Device.ID, sh)
	s.record(ctx, dc, source, nil, shadow)
	return shadow, nil
}

// Report merges reported state (from telemetry) and pushes any remaining delta.
// A log entry is written only when the reported state actually changes, to
// keep audit volume proportional to meaningful changes.
func (s *ShadowService) Report(ctx context.Context, dc *DeviceContext, reported map[string]any) (*Shadow, error) {
	if len(reported) == 0 {
		return s.Get(ctx, dc.Device.ID)
	}
	sh, err := s.loadOrCreate(ctx, dc.Device.ID)
	if err != nil {
		return nil, err
	}
	before := models.JSONMap{}
	for k, v := range sh.Reported {
		before[k] = v
	}
	sh.Reported = mergePatch(sh.Reported, reported)
	sh.Version++
	if err := s.db.WithContext(ctx).Model(sh).Updates(map[string]any{
		"reported": sh.Reported,
		"version":  sh.Version,
	}).Error; err != nil {
		return nil, err
	}
	shadow := toShadow(dc.Device.ID, sh)
	if !reflect.DeepEqual(before, shadow.Reported) {
		s.record(ctx, dc, models.ShadowSourceTelemetry, nil, shadow)
	}
	s.pushDelta(ctx, dc, shadow.Delta)
	return shadow, nil
}

func (s *ShadowService) record(ctx context.Context, dc *DeviceContext, source string, ruleID *uint, shadow *Shadow) {
	if source == "" {
		source = models.ShadowSourceAPI
	}
	row := &models.DeviceShadowLog{
		DeviceID:   dc.Device.ID,
		ProjectID:  dc.Project.ID,
		Source:     source,
		RuleID:     ruleID,
		Desired:    shadow.Desired,
		Reported:   shadow.Reported,
		Delta:      shadow.Delta,
		OccurredAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		s.log.Warn("shadow audit log failed", "error", err, "device", dc.Device.Key)
	}
}

func (s *ShadowService) pushDelta(ctx context.Context, dc *DeviceContext, delta models.JSONMap) {
	if len(delta) == 0 {
		return
	}
	if s.onDelta != nil {
		s.onDelta(ctx, dc, delta)
	}
	if s.downlink == nil {
		return
	}
	payload, err := json.Marshal(delta)
	if err != nil {
		return
	}
	if err := s.downlink.SendTo(ctx, dc, access.KindProperty, "", payload, map[string]string{
		"source": "device_shadow",
	}); err != nil {
		// Device may be offline; the delta is retained and re-pushed later.
		s.log.Debug("shadow delta push failed", "device", dc.Device.Key, "error", err)
		return
	}
	s.log.Info("shadow delta pushed", "device", dc.Device.Key, "keys", len(delta))
}

// mergePatch applies an RFC 7386 style JSON merge patch to a shallow map.
func mergePatch(dst models.JSONMap, patch map[string]any) models.JSONMap {
	out := models.JSONMap{}
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return out
}
