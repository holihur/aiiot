package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
)

// SendOptions controls alert de-duplication and aggregation.
type SendOptions struct {
	// SilenceSeconds suppresses identical alerts (channel+rule+device) within
	// the window; suppressed alerts are recorded but not delivered.
	SilenceSeconds int
	// AggregateSeconds buffers alerts and delivers one summary per window.
	AggregateSeconds int
}

// Notifier delivers alert messages to configured channels (webhook, DingTalk,
// email) and records every attempt in notification_logs.
type Notifier struct {
	db     *gorm.DB
	client *http.Client
	smtp   smtpConfig
	log    *slog.Logger

	mu      sync.Mutex
	pending map[string]*aggEntry
}

type aggEntry struct {
	channelID uint
	ruleID    *uint
	deviceID  uint
	title     string
	count     int
	lastBody  string
	deadline  time.Time
}

type smtpConfig struct {
	Host    string
	Port    int
	User    string
	Pass    string
	From    string
	Enabled bool
}

func NewNotifier(db *gorm.DB, log *slog.Logger) *Notifier {
	port := 587
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	host := os.Getenv("SMTP_HOST")
	return &Notifier{
		db:      db,
		client:  &http.Client{Timeout: 10 * time.Second},
		pending: map[string]*aggEntry{},
		smtp: smtpConfig{
			Host:    host,
			Port:    port,
			User:    os.Getenv("SMTP_USER"),
			Pass:    os.Getenv("SMTP_PASS"),
			From:    os.Getenv("SMTP_FROM"),
			Enabled: host != "",
		},
		log: log,
	}
}

// Start launches the aggregation flusher.
func (n *Notifier) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				n.flush(time.Now().Add(time.Hour))
				return
			case now := <-ticker.C:
				n.flush(now)
			}
		}
	}()
}

func (n *Notifier) flush(now time.Time) {
	n.mu.Lock()
	var due []*aggEntry
	for k, e := range n.pending {
		if !e.deadline.After(now) {
			due = append(due, e)
			delete(n.pending, k)
		}
	}
	n.mu.Unlock()
	for _, e := range due {
		title := e.title
		body := e.lastBody
		if e.count > 1 {
			title = fmt.Sprintf("%s (%d events)", title, e.count)
			body = fmt.Sprintf("%d events in window; latest: %s", e.count, e.lastBody)
		}
		var ch models.NotifyChannel
		if err := n.db.First(&ch, e.channelID).Error; err != nil {
			continue
		}
		_ = n.deliverAndLog(context.Background(), &ch, title, body, e.ruleID, e.deviceID, e.count, false)
	}
}

// Send delivers an alert through a channel by id, applying silence/aggregation.
func (n *Notifier) Send(ctx context.Context, channelID uint, title, body string, ruleID *uint, deviceID uint, opts SendOptions) error {
	var ch models.NotifyChannel
	if err := n.db.WithContext(ctx).First(&ch, channelID).Error; err != nil {
		return fmt.Errorf("channel %d not found", channelID)
	}
	if !ch.Enabled {
		return n.deliverAndLog(ctx, &ch, title, body, ruleID, deviceID, 1, false)
	}

	// Channel-level default silence window, overridable per action.
	silence := opts.SilenceSeconds
	if silence == 0 {
		if v, ok := ch.Config["silenceSeconds"].(float64); ok {
			silence = int(v)
		}
	}

	if opts.AggregateSeconds > 0 {
		key := fmt.Sprintf("%d|%v|%d", channelID, ruleIDValue(ruleID), deviceID)
		n.mu.Lock()
		if e, ok := n.pending[key]; ok {
			e.count++
			e.lastBody = body
			e.deadline = time.Now().Add(time.Duration(opts.AggregateSeconds) * time.Second)
		} else {
			n.pending[key] = &aggEntry{
				channelID: channelID,
				ruleID:    ruleID,
				deviceID:  deviceID,
				title:     title,
				count:     1,
				lastBody:  body,
				deadline:  time.Now().Add(time.Duration(opts.AggregateSeconds) * time.Second),
			}
		}
		n.mu.Unlock()
		return nil
	}

	if silence > 0 {
		var count int64
		n.db.WithContext(ctx).Model(&models.NotificationLog{}).
			Where("channel_id = ? AND device_id = ? AND success = ? AND suppressed = ? AND created_at > ?",
				channelID, deviceID, true, false, time.Now().Add(-time.Duration(silence)*time.Second)).
			Count(&count)
		if count > 0 {
			row := &models.NotificationLog{
				ProjectID: ch.ProjectID, ChannelID: ch.ID, RuleID: ruleID, DeviceID: deviceID,
				Success: true, Suppressed: true, Count: 1, Title: title, Body: body,
			}
			n.db.WithContext(ctx).Create(row)
			return nil
		}
	}
	return n.deliverAndLog(ctx, &ch, title, body, ruleID, deviceID, 1, false)
}

func ruleIDValue(id *uint) uint {
	if id == nil {
		return 0
	}
	return *id
}

func (n *Notifier) deliverAndLog(ctx context.Context, ch *models.NotifyChannel, title, body string, ruleID *uint, deviceID uint, count int, suppressed bool) error {
	var err error
	switch {
	case !ch.Enabled:
		err = fmt.Errorf("channel %q is disabled", ch.Name)
	case ch.Type == models.ChannelWebhook:
		err = n.sendWebhook(ctx, ch, title, body)
	case ch.Type == models.ChannelDingTalk:
		err = n.sendDingTalk(ctx, ch, title, body)
	case ch.Type == models.ChannelEmail:
		err = n.sendEmail(ch, title, body)
	default:
		err = fmt.Errorf("unknown channel type %q", ch.Type)
	}

	row := &models.NotificationLog{
		ProjectID:  ch.ProjectID,
		ChannelID:  ch.ID,
		RuleID:     ruleID,
		DeviceID:   deviceID,
		Success:    err == nil && !suppressed,
		Suppressed: suppressed,
		Count:      count,
		Title:      title,
		Body:       body,
	}
	if err != nil {
		row.Error = err.Error()
	}
	if e := n.db.WithContext(ctx).Create(row).Error; e != nil {
		n.log.Warn("notification log failed", "error", e)
	}
	return err
}

func (n *Notifier) sendWebhook(ctx context.Context, ch *models.NotifyChannel, title, body string) error {
	url := strVal(ch.Config["url"])
	if url == "" {
		return fmt.Errorf("webhook channel missing url")
	}
	method := strVal(ch.Config["method"])
	if method == "" {
		method = http.MethodPost
	}
	payload, _ := json.Marshal(map[string]any{"title": title, "body": body, "channel": ch.Name})
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if headers, ok := ch.Config["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, strVal(v))
		}
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) sendDingTalk(ctx context.Context, ch *models.NotifyChannel, title, body string) error {
	url := strVal(ch.Config["webhook"])
	if url == "" {
		url = strVal(ch.Config["url"])
	}
	if url == "" {
		return fmt.Errorf("dingtalk channel missing webhook")
	}
	content := title
	if body != "" {
		content = title + "\n" + body
	}
	payload, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": content},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("dingtalk status %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) sendEmail(ch *models.NotifyChannel, title, body string) error {
	if !n.smtp.Enabled {
		return fmt.Errorf("SMTP is not configured (set SMTP_HOST/SMTP_USER/SMTP_PASS/SMTP_FROM)")
	}
	to := strList(ch.Config["to"])
	if len(to) == 0 {
		return fmt.Errorf("email channel missing recipients")
	}
	from := n.smtp.From
	if from == "" {
		from = n.smtp.User
	}
	msg := []byte(
		"From: " + from + "\r\n" +
			"To: " + strings.Join(to, ",") + "\r\n" +
			"Subject: " + title + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n" +
			body + "\r\n",
	)
	auth := smtp.PlainAuth("", n.smtp.User, n.smtp.Pass, n.smtp.Host)
	addr := fmt.Sprintf("%s:%d", n.smtp.Host, n.smtp.Port)
	return smtp.SendMail(addr, auth, from, to, msg)
}

func strVal(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprint(s)
	}
}

func strList(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s := strVal(e); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
