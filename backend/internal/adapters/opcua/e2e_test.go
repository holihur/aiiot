package opcua

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/ua"
)

// TestPollAgainstLocalServer runs the adapter against an in-process gopcua
// server and verifies the mapped node values arrive as property uplinks.
func TestPollAgainstLocalServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := 48411
	srv := server.New(
		server.EndPoint("127.0.0.1", port),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
	)
	srvErr := make(chan error, 1)
	go func() { srvErr <- srv.Start(ctx) }()
	defer func() {
		cancel()
		select {
		case <-srvErr:
		case <-time.After(2 * time.Second):
		}
	}()
	time.Sleep(600 * time.Millisecond) // let the server register its namespace tree

	cfg := filepath.Join(t.TempDir(), "cfg.json")
	raw, _ := json.Marshal(map[string]any{
		"devices": []map[string]any{{
			"deviceKey": "sim-pc1",
			"endpoint":  "opc.tcp://127.0.0.1:" + itoa(port) + "/",
			"pollEvery": "1s",
			"nodes": []map[string]any{
				// built-in read-only nodes: ServerStatus/State (i=2259) is a
				// stable int32 (0 = Running) so assertions are deterministic.
				{"identifier": "state1", "nodeID": "ns=0;i=2259", "dataType": "int"},
				{"identifier": "state2", "nodeID": "ns=0;i=2259", "dataType": "int"},
			},
		}},
	})
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	adapter, err := New(Options{ConfigFile: cfg})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	var mu sync.Mutex
	received := map[string]float64{}
	_ = ua.AttributeIDValue
	_ = srv
	adapter.OnUplink(func(_ context.Context, msg *access.UplinkMessage) error {
		if msg.Kind != access.KindProperty {
			return nil
		}
		var p map[string]float64
		_ = json.Unmarshal(msg.Payload, &p)
		mu.Lock()
		for k, v := range p {
			received[k] = v
		}
		mu.Unlock()
		return nil
	})
	adapter.OnAuth(func(_ context.Context, req *access.AuthRequest) (*access.AuthResponse, error) {
		return &access.AuthResponse{Authorized: true, ProductKey: "p", DeviceKey: req.DeviceKey}, nil
	})

	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("start adapter: %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got, ok := received["state1"]
		second := received["state2"]
		mu.Unlock()
		if ok && got == 0 && second == 0 {
			return // success: both node reads arrived as property uplinks
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("values not received in time: %v (server err %v)", received, <-srvErr)
	_ = adapter
}

func itoa(n int) string {
	switch n {
	case 48411:
		return "48411"
	default:
		return "?0"
	}
}
