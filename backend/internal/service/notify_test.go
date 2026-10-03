package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aiiot/server/internal/models"
)

func notifyForTest(t *testing.T) (*Notifier, *models.NotifyChannel) {
	t.Helper()
	return NewNotifier(nil, nil), &models.NotifyChannel{}
}

func TestSendWeComPayloadAndURL(t *testing.T) {
	var gotPath, gotBody string
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		gotPath = r.URL.String()
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n, ch := notifyForTest(t)
	ch.Config = models.JSONMap{"webhook": srv.URL + "/webhook/send?key=abc"}
	if err := n.sendWeCom(context.Background(), ch, "高温告警", "温度 42°C"); err != nil {
		t.Fatalf("sendWeCom: %v", err)
	}
	if gotPath != "/webhook/send?key=abc" {
		t.Fatalf("path=%q", gotPath)
	}
	var body struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Content string `json:"content"`
		} `json:"markdown"`
	}
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("decode: %v (%q)", err, gotBody)
	}
	if body.MsgType != "markdown" || body.Markdown.Content == "" {
		t.Fatalf("wecom payload wrong: %+v", body)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestSendWeComBareKey(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.WriteHeader(200)
	}))
	defer srv.Close()
	n, ch := notifyForTest(t)
	ch.Config = models.JSONMap{"webhook": srv.URL + "/send?key=w1b2"}
	_ = n.sendWeCom(context.Background(), ch, "t", "")
	// bare key path: weComURL used with key only (not exercised through HTTP to
	// a real qyapi host; verify the builder directly instead)
	if weComURL("", "", "w1b2") != "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=w1b2" {
		t.Fatal("bare key URL builder wrong")
	}
	_ = gotURL
}

func TestSendLarkPayload(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()
	n, ch := notifyForTest(t)
	ch.Config = models.JSONMap{"webhook": srv.URL + "/bot/v2/hook/x"}
	if err := n.sendLark(context.Background(), ch, "告警", "内容"); err != nil {
		t.Fatalf("sendLark: %v", err)
	}
	var body struct {
		MsgType string `json:"msg_type"`
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.MsgType != "text" || body.Content.Text == "" {
		t.Fatalf("lark payload wrong: %+v", body)
	}
}

func TestPostJSONRetriesOn5xxAndSucceeds(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	n, _ := notifyForTest(t)
	if err := n.postJSON(context.Background(), srv.URL, http.MethodPost, map[string]string{"a": "1"}, "webhook", nil); err != nil {
		t.Fatalf("postJSON: %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts=%d, want 3 (retry twice)", attempts.Load())
	}
}

func TestPostJSONFailsFastOn4xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(400)
	}))
	defer srv.Close()
	n, _ := notifyForTest(t)
	start := time.Now()
	err := n.postJSON(context.Background(), srv.URL, http.MethodPost, map[string]string{}, "lark", nil)
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if attempts.Load() != 1 {
		t.Fatalf("4xx must not retry, attempts=%d", attempts.Load())
	}
	if time.Since(start) > time.Second {
		t.Fatalf("4xx path should fail fast")
	}
}

func TestPostJSONTransportErrorRetries(t *testing.T) {
	// a closed port produces a transport error on every attempt; this only
	// exercises the retry path without asserting timing
	n, _ := notifyForTest(t)
	_ = n.postJSON(context.Background(), "http://127.0.0.1:1/none", http.MethodPost, map[string]string{}, "webhook", nil)
}
