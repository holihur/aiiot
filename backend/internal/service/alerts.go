package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
)

// Alerter maintains the alert lifecycle driven by the rule engine:
//
//	rule matches  -> Raise   (create or refresh the open alert for rule+device)
//	telemetry rule no longer matches for that device -> ResolveIf (auto-close)
//	operator ack / resolve                        -> Ack / Resolve
type Alerter struct {
	db  *gorm.DB
	log *slog.Logger
}

// NewAlerter creates the alert service.
func NewAlerter(db *gorm.DB, log *slog.Logger) *Alerter {
	if log == nil {
		log = slog.Default()
	}
	return &Alerter{db: db, log: log}
}

// severity maps a rule priority to an alert level: 0=info, 1=warning, >=2=critical.
func severity(priority int) string {
	switch {
	case priority >= 2:
		return models.AlertLevelCritical
	case priority == 1:
		return models.AlertLevelWarning
	default:
		return models.AlertLevelInfo
	}
}

// Raise records (or refreshes) the open alert for a rule+device.
func (a *Alerter) Raise(ctx context.Context, rule *models.Rule, ev *RuleEvent) error {
	if a == nil || a.db == nil {
		return nil
	}
	now := ev.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	summary := summarize(rule, ev)

	var al models.Alert
	err := a.db.WithContext(ctx).
		Where("rule_id = ? AND device_id = ? AND status IN ?",
			rule.ID, ev.DeviceID, []string{models.AlertStatusFiring, models.AlertStatusAcknowledged}).
		Order("id DESC").First(&al).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		al = models.Alert{
			ProjectID:   ev.ProjectID,
			RuleID:      rule.ID,
			DeviceID:    ev.DeviceID,
			DeviceKey:   ev.DeviceKey,
			Identifier:  ev.Identifier,
			Title:       rule.Name,
			Level:       severity(rule.Priority),
			Status:      models.AlertStatusFiring,
			Message:     summary,
			StartsAt:    now,
			LastFiredAt: now,
			FireCount:   1,
		}
		return a.db.WithContext(ctx).Create(&al).Error
	}
	if err != nil {
		a.log.Warn("alert lookup failed", "error", err)
		return err
	}
	updates := map[string]any{
		"last_fired_at": now,
		"message":       summary,
		"fire_count":    al.FireCount + 1,
		"identifier":    ev.Identifier,
	}
	if al.Status == models.AlertStatusFiring {
		if lvl := severity(rule.Priority); lvl != al.Level {
			updates["level"] = lvl
		}
	}
	return a.db.WithContext(ctx).Model(&al).Updates(updates).Error
}

// ResolveIf auto-resolves a firing telemetry alert when the rule's condition
// no longer holds for that device.
func (a *Alerter) ResolveIf(ctx context.Context, rule *models.Rule, ev *RuleEvent) error {
	if a == nil || a.db == nil || ev.DeviceID == 0 {
		return nil
	}
	return a.db.WithContext(ctx).
		Model(&models.Alert{}).
		Where("rule_id = ? AND device_id = ? AND status = ?",
			rule.ID, ev.DeviceID, models.AlertStatusFiring).
		Updates(map[string]any{
			"status":         models.AlertStatusResolved,
			"resolved_at":    ev.Now,
			"resolve_reason": "condition cleared",
		}).Error
}

// EscalateDue promotes open alerts whose age has exceeded escalateAfter
// (info -> warning -> critical). Critical alerts no longer escalate; the
// counter still records how many times it was promoted.
func (a *Alerter) EscalateDue(ctx context.Context, escalateAfter time.Duration) (int, error) {
	if a == nil || a.db == nil || escalateAfter <= 0 {
		return 0, nil
	}
	var open []models.Alert
	if err := a.db.WithContext(ctx).
		Where("status IN ?", []string{models.AlertStatusFiring, models.AlertStatusAcknowledged}).
		Find(&open).Error; err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	escalated := 0
	for _, al := range open {
		if now.Sub(al.StartsAt) <= escalateAfter*time.Duration(al.Escalations+1) {
			continue
		}
		lvl := nextLevel(al.Level)
		if lvl == al.Level {
			al.Escalations++ // critical reached; record the tick
		} else {
			al.Level = lvl
			al.Escalations++
		}
		escalated++
		a.log.Info("alert escalated", "alert", al.ID, "rule", al.RuleID, "level", al.Level, "times", al.Escalations)
		if err := a.db.WithContext(ctx).Model(&al).Updates(map[string]any{
			"level":        al.Level,
			"escalations":  al.Escalations,
			"escalated_at": now,
		}).Error; err != nil {
			a.log.Warn("escalate alert failed", "error", err, "alert", al.ID)
		}
	}
	return escalated, nil
}

// nextLevel promotes a severity one step up.
func nextLevel(l string) string {
	switch l {
	case models.AlertLevelInfo:
		return models.AlertLevelWarning
	case models.AlertLevelWarning:
		return models.AlertLevelCritical
	default:
		return l
	}
}

// summarize builds the alert message from the triggering event.
func summarize(rule *models.Rule, ev *RuleEvent) string {
	if ev.Value == nil {
		return rule.Name
	}
	return rule.Name + "  (" + ev.Identifier + " = " + fmt.Sprint(ev.Value) + ")"
}
