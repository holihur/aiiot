package service

import (
	"testing"
	"time"

	"github.com/aiiot/server/internal/access"
)

func TestIngestDedupKeyStableForSameMessage(t *testing.T) {
	msg := &access.UplinkMessage{
		Protocol:   "mqtt",
		Device:     access.DeviceRef{ProductKey: "sensor", DeviceKey: "d1"},
		Kind:       access.KindProperty,
		Identifier: "temperature",
		Payload:    []byte(`{"value":30.5}`),
		Timestamp:  time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Metadata:   map[string]string{},
	}
	k1 := ingestDedupKey(msg)
	k2 := ingestDedupKey(msg)
	if k1 == "" || k1 != k2 {
		t.Fatalf("dedup key must be stable: %q vs %q", k1, k2)
	}
}

func TestIngestDedupKeyDiffersForDifferentPayload(t *testing.T) {
	base := func(payload string) *access.UplinkMessage {
		return &access.UplinkMessage{
			Protocol: "custom", Device: access.DeviceRef{ProductKey: "p", DeviceKey: "d"},
			Kind: access.KindProperty, Identifier: "x", Payload: []byte(payload),
			Timestamp: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), Metadata: map[string]string{},
		}
	}
	if ingestDedupKey(base(`{"a":1}`)) == ingestDedupKey(base(`{"a":2}`)) {
		t.Fatal("different payloads must produce different keys")
	}
	if ingestDedupKey(base(`{"a":1}`)) == ingestDedupKey(base(`{"a":1}`)) == false {
		t.Fatal("same payload must produce same key")
	}
}

func TestIngestDedupKeyPrefersGatewayMsgID(t *testing.T) {
	m := &access.UplinkMessage{
		Protocol: "custom", Device: access.DeviceRef{ProductKey: "p", DeviceKey: "d"},
		Kind: access.KindProperty, Identifier: "x", Payload: []byte(`{"a":1}`),
		Timestamp: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Metadata:  map[string]string{"id": "m42"},
	}
	k := ingestDedupKey(m)
	if !containsStr(k, "m:m42") {
		t.Fatalf("expected gateway msg id in key, got %q", k)
	}
}

func TestIngestDedupKeySkipsEmptyDevice(t *testing.T) {
	m := &access.UplinkMessage{Protocol: "mqtt", Device: access.DeviceRef{}, Kind: access.KindProperty}
	if ingestDedupKey(m) != "" {
		t.Fatal("empty device should yield empty key")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
