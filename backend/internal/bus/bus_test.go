package bus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
)

// startTestServer runs an embedded NATS server with JetStream on a random port.
func startTestServer(t *testing.T) *natsserver.Server {
	t.Helper()
	opts := test.DefaultTestOptions
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	opts.Port = -1
	srv := test.RunServer(&opts)
	t.Cleanup(func() { srv.Shutdown() })
	return srv
}

func testConfig(srv *natsserver.Server) Config {
	return Config{
		URL: srv.ClientURL(),
	}
}

func TestConfigWithDefaults(t *testing.T) {
	c := Config{URL: "nats://127.0.0.1:4222"}.WithDefaults()
	if c.Stream != DefaultStream || c.Subject != DefaultSubject ||
		c.Durable != DefaultDurable || c.Queue != DefaultQueue {
		t.Fatalf("defaults not applied: %+v", c)
	}
	if c.AckWait != 60*time.Second {
		t.Fatalf("default ack wait = %v, want 60s", c.AckWait)
	}
}

func TestPublisherSubscriberRoundTrip(t *testing.T) {
	srv := startTestServer(t)
	cfg := testConfig(srv)

	var mu sync.Mutex
	received := map[string][]byte{}
	var recv atomic.Int32
	ready := make(chan struct{}, 1)

	sub, err := NewSubscriber(cfg, func(data []byte, delivered uint64) error {
		mu.Lock()
		received[string(data)] = data
		mu.Unlock()
		recv.Add(1)
		if recv.Load() == 1 {
			ready <- struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	payload := []byte(`{"kind":"property","protocol":"mqtt"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pub.Publish(ctx, payload); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for delivery")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("received %d messages, want 1", len(received))
	}
	got := string(received[string(payload)])
	if got != string(payload) {
		t.Fatalf("payload mismatch: got %q", got)
	}
}

func TestSubscriberRedeliversOnError(t *testing.T) {
	srv := startTestServer(t)
	cfg := testConfig(srv)
	cfg.AckWait = 300 * time.Millisecond // redeliver quickly

	calls := make([]uint64, 0, 2)
	var mu sync.Mutex
	done := make(chan struct{})

	sub, err := NewSubscriber(cfg, func(data []byte, delivered uint64) error {
		mu.Lock()
		calls = append(calls, delivered)
		n := len(calls)
		mu.Unlock()
		if n >= 2 {
			close(done)
			return nil // succeed on the 2nd delivery
		}
		return fmt.Errorf("transient failure") // 1st delivery -> Nak
	})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pub.Publish(ctx, []byte(`{}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for redelivery (calls=%v)", calls)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 2 {
		t.Fatalf("expected >=2 deliveries, got %d", len(calls))
	}
	if calls[0] != 1 {
		t.Fatalf("first delivery count = %d, want 1", calls[0])
	}
	if calls[len(calls)-1] < 2 {
		t.Fatalf("redelivery count = %d, want >= 2", calls[len(calls)-1])
	}
}

func TestTwoSubscribersShareQueueGroup(t *testing.T) {
	srv := startTestServer(t)
	cfg := testConfig(srv)

	var count atomic.Int32
	mk := func() *Subscriber {
		s, err := NewSubscriber(cfg, func(data []byte, delivered uint64) error {
			count.Add(1)
			return nil
		})
		if err != nil {
			t.Fatalf("NewSubscriber: %v", err)
		}
		return s
	}
	s1 := mk()
	defer s1.Close()
	s2 := mk()
	defer s2.Close()

	pub, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const total = 20
	for i := 0; i < total; i++ {
		if err := pub.Publish(ctx, []byte(fmt.Sprintf("msg-%d", i))); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	// Each message should be delivered to exactly one of the two subscribers
	// (durable queue group semantics): no message is processed twice.
	deadline := time.Now().Add(5 * time.Second)
	for count.Load() < total {
		if time.Now().After(deadline) {
			t.Fatalf("received %d of %d", count.Load(), total)
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond) // allow any (invalid) duplicate to surface
	if got := count.Load(); got != total {
		t.Fatalf("processed %d times, want exactly %d (no duplicates)", got, total)
	}
}

func TestEnsureStreamIdempotent(t *testing.T) {
	srv := startTestServer(t)
	cfg := testConfig(srv)

	for i := 0; i < 2; i++ {
		s, err := NewSubscriber(cfg, func(data []byte, delivered uint64) error { return nil })
		if err != nil {
			t.Fatalf("NewSubscriber attempt %d: %v", i+1, err)
		}
		s.Close()
		p, err := NewPublisher(cfg)
		if err != nil {
			t.Fatalf("NewPublisher attempt %d: %v", i+1, err)
		}
		p.Close()
	}
}
