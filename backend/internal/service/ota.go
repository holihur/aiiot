package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/config"
	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
)

// OTAService manages firmware rollouts. Firmware files live on disk; devices
// download them from the core and report progress back through the access
// layer (KindOTA uplinks). Rollouts may be split into waves (canary/batched).
type OTAService struct {
	db       *gorm.DB
	downlink *DownlinkService
	resolver *Resolver
	cfg      config.OTAConfig
	log      *slog.Logger
}

func NewOTAService(db *gorm.DB, downlink *DownlinkService, resolver *Resolver, cfg config.OTAConfig, log *slog.Logger) *OTAService {
	return &OTAService{db: db, downlink: downlink, resolver: resolver, cfg: cfg, log: log}
}

// Cfg exposes the OTA configuration to the API layer.
func (s *OTAService) Cfg() config.OTAConfig { return s.cfg }

// StoredPath returns the on-disk path for a firmware binary.
func (s *OTAService) StoredPath(firmwareID uint) string {
	return filepath.Join(s.cfg.Dir, fmt.Sprintf("%d.bin", firmwareID))
}

// DownloadURL is the URL handed to devices.
func (s *OTAService) DownloadURL(firmwareID uint) string {
	base := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	return fmt.Sprintf("%s/ota/firmware/%d", base, firmwareID)
}

// CreateTask creates a rollout, assigns waves and dispatches the first wave.
func (s *OTAService) CreateTask(ctx context.Context, projectID, firmwareID uint, name string, deviceIDs []uint, createdBy uint, batchSize int, haltOnFailure bool) (*models.OTATask, error) {
	var fw models.Firmware
	if err := s.db.WithContext(ctx).First(&fw, firmwareID).Error; err != nil {
		return nil, fmt.Errorf("firmware not found")
	}
	if len(deviceIDs) == 0 {
		return nil, fmt.Errorf("no target devices")
	}
	if batchSize < 0 {
		batchSize = 0
	}
	now := time.Now().UTC()
	task := &models.OTATask{
		ProjectID:     projectID,
		ProductID:     fw.ProductID,
		FirmwareID:    fw.ID,
		Name:          name,
		Status:        models.OTATaskRunning,
		Total:         len(deviceIDs),
		CreatedBy:     createdBy,
		StartedAt:     &now,
		BatchSize:     batchSize,
		CurrentWave:   0,
		HaltOnFailure: haltOnFailure,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		for i, id := range deviceIDs {
			wave := 0
			if batchSize > 0 {
				wave = i / batchSize
			}
			td := models.OTATaskDevice{TaskID: task.ID, DeviceID: id, Status: models.OTADevicePending, Wave: wave}
			if err := tx.Create(&td).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.dispatchWave(ctx, task, &fw, 0)
	s.advance(ctx, task.ID)
	return s.GetTask(ctx, task.ID)
}

// Rollback creates a new rollout that reinstalls the previous firmware version
// on the same set of devices.
func (s *OTAService) Rollback(ctx context.Context, taskID, createdBy uint) (*models.OTATask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	var prev models.Firmware
	err = s.db.WithContext(ctx).Where("product_id = ? AND id < ?", task.ProductID, task.FirmwareID).
		Order("id DESC").First(&prev).Error
	if err != nil {
		return nil, fmt.Errorf("no previous firmware to roll back to")
	}
	deviceIDs := make([]uint, 0, len(task.Devices))
	for _, d := range task.Devices {
		deviceIDs = append(deviceIDs, d.DeviceID)
	}
	name := fmt.Sprintf("Rollback to %s", prev.Version)
	return s.CreateTask(ctx, task.ProjectID, prev.ID, name, deviceIDs, createdBy, task.BatchSize, task.HaltOnFailure)
}

// dispatchWave sends the upgrade command to the pending devices of one wave.
func (s *OTAService) dispatchWave(ctx context.Context, task *models.OTATask, fw *models.Firmware, wave int) {
	var tds []models.OTATaskDevice
	s.db.WithContext(ctx).Where("task_id = ? AND wave = ?", task.ID, wave).Find(&tds)

	payload, _ := json.Marshal(map[string]any{
		"taskId":   task.ID,
		"version":  fw.Version,
		"url":      s.DownloadURL(fw.ID),
		"checksum": fw.Checksum,
		"size":     fw.FileSize,
		"wave":     wave,
	})
	for i := range tds {
		td := &tds[i]
		if td.Status != models.OTADevicePending {
			continue
		}
		dc, err := s.resolver.ResolveByDeviceID(ctx, td.DeviceID)
		if err != nil {
			s.markDevice(ctx, td, models.OTADeviceFailed, 0, "device not found")
			continue
		}
		if err := s.downlink.SendTo(ctx, dc, access.KindOTA, "", payload, map[string]string{"source": "ota"}); err != nil {
			s.log.Warn("ota dispatch failed", "device", dc.Device.Key, "error", err)
			s.markDevice(ctx, td, models.OTADeviceFailed, 0, "dispatch failed: "+err.Error())
			continue
		}
		s.markDevice(ctx, td, models.OTADeviceDispatched, 0, "")
	}
}

func (s *OTAService) markDevice(ctx context.Context, td *models.OTATaskDevice, status string, progress int, message string) {
	s.db.WithContext(ctx).Model(td).Updates(map[string]any{
		"status":   status,
		"progress": progress,
		"message":  message,
	})
}

// HandleReport processes an OTA progress/result uplink from a device.
func (s *OTAService) HandleReport(ctx context.Context, dc *DeviceContext, msg *access.UplinkMessage) error {
	var body struct {
		TaskID   uint   `json:"taskId"`
		Status   string `json:"status"`
		Progress int    `json:"progress"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(msg.Payload, &body); err != nil {
		return fmt.Errorf("invalid ota report: %w", err)
	}
	if body.TaskID == 0 {
		return fmt.Errorf("ota report missing taskId")
	}
	var td models.OTATaskDevice
	if err := s.db.WithContext(ctx).Where("task_id = ? AND device_id = ?", body.TaskID, dc.Device.ID).First(&td).Error; err != nil {
		return fmt.Errorf("ota task device not found")
	}
	status := normalizeOTAStatus(body.Status)
	if status == "" {
		status = models.OTADeviceDownloading
	}
	s.markDevice(ctx, &td, status, body.Progress, body.Message)

	if status == models.OTADeviceSucceeded {
		var task models.OTATask
		if err := s.db.WithContext(ctx).First(&task, body.TaskID).Error; err == nil {
			var fw models.Firmware
			if err := s.db.WithContext(ctx).First(&fw, task.FirmwareID).Error; err == nil {
				s.db.WithContext(ctx).Model(&models.Device{}).Where("id = ?", dc.Device.ID).
					Update("firmware_version", fw.Version)
			}
		}
	}
	s.advance(ctx, body.TaskID)
	return nil
}

func normalizeOTAStatus(s string) string {
	switch strings.ToLower(s) {
	case "downloading", "download":
		return models.OTADeviceDownloading
	case "upgrading", "installing", "upgrade":
		return models.OTADeviceUpgrading
	case "succeeded", "success", "done":
		return models.OTADeviceSucceeded
	case "failed", "error":
		return models.OTADeviceFailed
	}
	return ""
}

func terminalStatus(status string) bool {
	return status == models.OTADeviceSucceeded || status == models.OTADeviceFailed
}

// advance recomputes counters and, when the current wave is fully terminal,
// either finishes the task or dispatches the next wave.
func (s *OTAService) advance(ctx context.Context, taskID uint) {
	var task models.OTATask
	if err := s.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return
	}
	if task.Status == models.OTATaskCanceled {
		return
	}
	var tds []models.OTATaskDevice
	s.db.WithContext(ctx).Where("task_id = ?", taskID).Find(&tds)

	succeeded, failed, inProgress, pending := 0, 0, 0, 0
	waveTotal, waveDone, waveFailed := map[int]int{}, map[int]int{}, map[int]int{}
	maxWave := 0
	for _, td := range tds {
		waveTotal[td.Wave]++
		if td.Wave > maxWave {
			maxWave = td.Wave
		}
		switch td.Status {
		case models.OTADeviceSucceeded:
			succeeded++
			waveDone[td.Wave]++
		case models.OTADeviceFailed:
			failed++
			waveDone[td.Wave]++
			waveFailed[td.Wave]++
		case models.OTADevicePending:
			pending++
		default:
			inProgress++
		}
	}

	updates := map[string]any{
		"succeeded":    succeeded,
		"failed":       failed,
		"in_progress":  inProgress,
		"total":        len(tds),
		"current_wave": task.CurrentWave,
	}

	waveComplete := waveDone[task.CurrentWave] == waveTotal[task.CurrentWave] && waveTotal[task.CurrentWave] > 0
	if !waveComplete {
		s.db.WithContext(ctx).Model(&models.OTATask{}).Where("id = ?", taskID).Updates(updates)
		return
	}

	finished := time.Now().UTC()
	if waveFailed[task.CurrentWave] > 0 && task.HaltOnFailure {
		updates["status"] = models.OTATaskFailed
		updates["finished_at"] = finished
	} else if task.CurrentWave >= maxWave {
		if failed > 0 {
			updates["status"] = models.OTATaskFailed
		} else {
			updates["status"] = models.OTATaskSucceeded
		}
		updates["finished_at"] = finished
	} else {
		// Advance to the next wave.
		task.CurrentWave++
		updates["current_wave"] = task.CurrentWave
		updates["status"] = models.OTATaskRunning
		var fw models.Firmware
		if err := s.db.WithContext(ctx).First(&fw, task.FirmwareID).Error; err == nil {
			s.dispatchWave(ctx, &task, &fw, task.CurrentWave)
		}
	}
	s.db.WithContext(ctx).Model(&models.OTATask{}).Where("id = ?", taskID).Updates(updates)
}

// GetTask returns a task with its firmware and per-device rows.
func (s *OTAService) GetTask(ctx context.Context, id uint) (*models.OTATask, error) {
	var task models.OTATask
	if err := s.db.WithContext(ctx).Preload("Firmware").Preload("Devices").Preload("Devices.Device").First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// ListTasks returns tasks for a project.
func (s *OTAService) ListTasks(ctx context.Context, projectID uint) ([]models.OTATask, error) {
	var tasks []models.OTATask
	err := s.db.WithContext(ctx).Preload("Firmware").Where("project_id = ?", projectID).
		Order("id DESC").Limit(200).Find(&tasks).Error
	return tasks, err
}

// CancelTask marks a running task as canceled.
func (s *OTAService) CancelTask(ctx context.Context, id uint) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.OTATask{}).Where("id = ?", id).Updates(map[string]any{
		"status":      models.OTATaskCanceled,
		"finished_at": now,
	}).Error
}
