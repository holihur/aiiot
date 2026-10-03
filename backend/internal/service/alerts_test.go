package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aiiot/server/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// pure helpers ---------------------------------------------------------------

func TestSeverityMapping(t *testing.T) {
	cases := []struct {
		prio int
		want string
	}{
		{0, models.AlertLevelInfo},
		{1, models.AlertLevelWarning},
		{2, models.AlertLevelCritical},
		{9, models.AlertLevelCritical},
	}
	for _, c := range cases {
		if got := severity(c.prio); got != c.want {
			t.Errorf("severity(%d)=%s, want %s", c.prio, got, c.want)
		}
	}
}

func TestNextLevel(t *testing.T) {
	if nextLevel(models.AlertLevelInfo) != models.AlertLevelWarning {
		t.Fatal("info should escalate to warning")
	}
	if nextLevel(models.AlertLevelWarning) != models.AlertLevelCritical {
		t.Fatal("warning should escalate to critical")
	}
	if nextLevel(models.AlertLevelCritical) != models.AlertLevelCritical {
		t.Fatal("critical stays critical")
	}
}

func TestSummarize(t *testing.T) {
	r := &models.Rule{Name: "高温"}
	ev := &RuleEvent{Identifier: "temperature", Value: 42.5}
	if got := summarize(r, ev); got != "高温  (temperature = 42.5)" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

// postgres-backed integration tests (skipped when no DB is reachable) --------

// pgAlerterDB connects to a scratch database (created on the local Postgres)
// with just the alerts schema, returning a cleanup that drops the database.
func pgAlerterDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	host := envOr("TEST_PG_HOST", "127.0.0.1")
	port := envOr("TEST_PG_PORT", "5432")
	user := envOr("TEST_PG_USER", "postgres")
	pass := envOr("TEST_PG_PASSWORD", os.Getenv("PGPASSWORD"))
	if pass == "" {
		pass = "postgres"
	}
	dbname := fmt.Sprintf("aiiot_test_%d", time.Now().UnixNano()%1000000)

	adminDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable connect_timeout=5",
		host, port, user, pass)
	admin, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("no local postgres for integration tests: %v", err)
	}
	sqlDB, err := admin.DB()
	if err != nil {
		t.Skipf("no db handle: %v", err)
	}
	if _, err := sqlDB.Exec("CREATE DATABASE " + dbname); err != nil {
		t.Skipf("cannot create scratch db (%v); skipping", err)
	}
	cleanup := func() {
		_, _ = sqlDB.Exec("DROP DATABASE " + dbname)
		_ = sqlDB.Close()
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=5",
		host, port, user, pass, dbname)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		cleanup()
		t.Fatalf("open scratch db: %v", err)
	}
	if err := db.AutoMigrate(&models.Alert{}); err != nil {
		cleanup()
		t.Fatalf("migrate alerts: %v", err)
	}
	return db, cleanup
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func TestAlerterRaiseAndRefresh(t *testing.T) {
	db, cleanup := pgAlerterDB(t)
	defer cleanup()
	a := NewAlerter(db, nil)
	ctx := context.Background()
	rule := &models.Rule{Base: models.Base{ID: 1}, Name: "Overheat", Priority: 1}
	ev := &RuleEvent{ProjectID: 1, DeviceID: 10, DeviceKey: "d10", Identifier: "temperature", Value: 42.0, Now: time.Now().UTC()}

	if err := a.Raise(ctx, rule, ev); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := a.Raise(ctx, rule, ev); err != nil {
		t.Fatalf("raise again: %v", err)
	}
	var al models.Alert
	if err := db.Where("rule_id = ? AND device_id = ?", uint(1), uint(10)).First(&al).Error; err != nil {
		t.Fatalf("load alert: %v", err)
	}
	if al.FireCount != 2 {
		t.Fatalf("fireCount=%d, want 2", al.FireCount)
	}
	if al.Status != models.AlertStatusFiring {
		t.Fatalf("status=%s, want firing", al.Status)
	}
	if al.Level != models.AlertLevelWarning {
		t.Fatalf("level=%s, want warning", al.Level)
	}
}

func TestAlerterResolveIf(t *testing.T) {
	db, cleanup := pgAlerterDB(t)
	defer cleanup()
	a := NewAlerter(db, nil)
	ctx := context.Background()
	rule := &models.Rule{Base: models.Base{ID: 2}, Name: "Heat", Priority: 0}
	ev := &RuleEvent{ProjectID: 1, DeviceID: 11, DeviceKey: "d11", Now: time.Now().UTC()}

	if err := a.Raise(ctx, rule, ev); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if err := a.ResolveIf(ctx, rule, ev); err != nil {
		t.Fatalf("resolve if: %v", err)
	}
	var al models.Alert
	if err := db.Where("rule_id = ? AND device_id = ?", uint(2), uint(11)).First(&al).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if al.Status != models.AlertStatusResolved {
		t.Fatalf("status=%s, want resolved (reason %s)", al.Status, al.ResolveReason)
	}
}

func TestAlerterEscalateDue(t *testing.T) {
	db, cleanup := pgAlerterDB(t)
	defer cleanup()
	a := NewAlerter(db, nil)
	ctx := context.Background()
	rule := &models.Rule{Base: models.Base{ID: 3}, Name: "Slow", Priority: 0} // info

	ev := &RuleEvent{ProjectID: 1, DeviceID: 12, DeviceKey: "d12", Now: time.Now().UTC().Add(-time.Hour)}
	if err := a.Raise(ctx, rule, ev); err != nil {
		t.Fatalf("raise: %v", err)
	}
	n, err := a.EscalateDue(ctx, 15*time.Minute)
	if err != nil {
		t.Fatalf("escalate: %v", err)
	}
	if n != 1 {
		t.Fatalf("escalated=%d, want 1", n)
	}
	var al models.Alert
	if err := db.First(&al).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if al.Level != models.AlertLevelWarning || al.Escalations != 1 {
		t.Fatalf("got level=%s escalations=%d, want warning/1", al.Level, al.Escalations)
	}
	// backdate again -> should reach critical
	_ = db.Model(&al).Update("starts_at", time.Now().UTC().Add(-time.Hour)).Error
	if _, err := a.EscalateDue(ctx, 15*time.Minute); err != nil {
		t.Fatalf("escalate 2: %v", err)
	}
	_ = db.First(&al).Error
	if al.Level != models.AlertLevelCritical || al.Escalations < 2 {
		t.Fatalf("got level=%s escalations=%d, want critical/>=2", al.Level, al.Escalations)
	}
}
