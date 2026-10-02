package service

import (
	"context"
	"strconv"

	"gorm.io/gorm"
)

// Settings provides typed access to the system_settings key/value table so
// operators can change behaviour (e.g. telemetry retention) at runtime.
type Settings struct {
	db *gorm.DB
}

func NewSettings(db *gorm.DB) *Settings { return &Settings{db: db} }

func (s *Settings) Get(ctx context.Context, key, def string) string {
	var row struct{ Value string }
	if err := s.db.WithContext(ctx).Table("system_settings").Select("value").Where("key = ?", key).Scan(&row).Error; err != nil || row.Value == "" {
		return def
	}
	return row.Value
}

func (s *Settings) GetInt(ctx context.Context, key string, def int) int {
	if v, err := strconv.Atoi(s.Get(ctx, key, "")); err == nil {
		return v
	}
	return def
}

func (s *Settings) Set(ctx context.Context, key, value string) error {
	return s.db.WithContext(ctx).Exec(
		`INSERT INTO system_settings (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		key, value,
	).Error
}

// GetSetting is a package helper for one-off reads.
func GetSetting(ctx context.Context, db *gorm.DB, key, def string) string {
	return NewSettings(db).Get(ctx, key, def)
}
