// Package bus provides the NATS JetStream message bus that carries device
// uplinks from protocol gateways to the core control plane.
//
// NATS is a mandatory platform component (like PostgreSQL). Gateways publish
// uplinks to a JetStream stream; the core consumes them through a durable,
// queue-grouped consumer so that multiple core replicas can share the ingest
// load. Control-plane traffic (register / heartbeat / auth) and downlinks keep
// using HTTP, which already has request/response semantics.
package bus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// Defaults shared by every process so stream/consumer creation is idempotent.
const (
	DefaultStream  = "AIOT_UPLINK"
	DefaultSubject = "aiiot.uplink"
	DefaultDurable = "core-uplink"
	DefaultQueue   = "core-uplink"
)

// Config describes how to reach NATS and which stream/consumer to use for
// device uplinks.
type Config struct {
	URL      string        // nats://host:4222 (required; the bus is disabled when empty)
	User     string        // optional username
	Password string        // optional password
	Creds    string        // optional path to a .creds file (takes precedence)
	Stream   string        // JetStream stream name (default AIOT_UPLINK)
	Subject  string        // uplink subject (default aiiot.uplink)
	Durable  string        // durable consumer name (default core-uplink)
	Queue    string        // queue group for shared consumption (default core-uplink)
	AckWait  time.Duration // how long the core may take before the server redelivers (default 60s)
}

// WithDefaults returns a copy with empty fields filled in.
func (c Config) WithDefaults() Config {
	if c.Stream == "" {
		c.Stream = DefaultStream
	}
	if c.Subject == "" {
		c.Subject = DefaultSubject
	}
	if c.Durable == "" {
		c.Durable = DefaultDurable
	}
	if c.Queue == "" {
		c.Queue = DefaultQueue
	}
	if c.AckWait <= 0 {
		c.AckWait = 60 * time.Second
	}
	return c
}

// Enabled reports whether NATS is configured.
func (c Config) Enabled() bool { return c.URL != "" }

// connect opens a NATS connection with auto-reconnect and retry-on-failed
// connect so a brief NATS restart does not take the process down.
func connect(cfg Config) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("aiiot"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.RetryOnFailedConnect(true),
		nats.Timeout(10 * time.Second),
	}
	switch {
	case cfg.Creds != "":
		opts = append(opts, nats.UserCredentials(cfg.Creds))
	case cfg.User != "":
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}
	return nats.Connect(cfg.URL, opts...)
}

// ensureStream creates the uplink stream when it does not exist yet. It is
// safe to call from every process; identical configuration is idempotent.
func ensureStream(js nats.JetStreamContext, stream, subject string) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name:      stream,
		Subjects:  []string{subject},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		Discard:   nats.DiscardOld,
		MaxAge:    24 * time.Hour,
		MaxMsgs:   10_000_000,
	})
	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("ensure stream %q: %w", stream, err)
	}
	return nil
}

// Publisher publishes uplinks into the JetStream stream. It is used by
// protocol gateways.
type Publisher struct {
	nc      *nats.Conn
	js      nats.JetStreamContext
	subject string
}

// NewPublisher connects to NATS, ensures the stream exists and returns a
// ready-to-use publisher.
func NewPublisher(cfg Config) (*Publisher, error) {
	cfg = cfg.WithDefaults()
	nc, err := connect(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}
	if err := ensureStream(js, cfg.Stream, cfg.Subject); err != nil {
		nc.Close()
		return nil, err
	}
	return &Publisher{nc: nc, js: js, subject: cfg.Subject}, nil
}

// Publish delivers data to the uplink stream. It blocks until the server has
// persisted the message (JetStream publish ack), so callers can rely on
// at-least-once delivery.
func (p *Publisher) Publish(ctx context.Context, data []byte) error {
	_, err := p.js.PublishMsg(&nats.Msg{Subject: p.subject, Data: data}, nats.Context(ctx))
	return err
}

// Subject returns the uplink subject this publisher writes to.
func (p *Publisher) Subject() string { return p.subject }

// Close releases the NATS connection.
func (p *Publisher) Close() { p.nc.Close() }

// NewEventConn opens a plain (non-JetStream) NATS connection used to fan out
// real-time SSE events across every core replica. Every subscriber receives
// every event (no queue group): each replica forwards them to its local hub.
func NewEventConn(cfg Config) (*nats.Conn, error) {
	cfg = cfg.WithDefaults()
	return connect(cfg)
}

// Handler consumes one uplink payload. delivered is the JetStream delivery
// count (1 on first delivery, higher after redeliveries). Returning a non-nil
// error causes the message to be redelivered (Nak); returning nil acknowledges
// it.
type Handler func(data []byte, delivered uint64) error

// Subscriber consumes uplinks from the JetStream stream. It is used by the
// core. Multiple subscribers sharing the same durable + queue group form a
// consumer group, so ingest scales horizontally.
type Subscriber struct {
	nc  *nats.Conn
	js  nats.JetStreamContext
	sub *nats.Subscription
	cfg Config
}

// NewSubscriber connects to NATS, ensures the stream and durable consumer
// exist and registers handler.
func NewSubscriber(cfg Config, handler Handler) (*Subscriber, error) {
	cfg = cfg.WithDefaults()
	nc, err := connect(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}
	if err := ensureStream(js, cfg.Stream, cfg.Subject); err != nil {
		nc.Close()
		return nil, err
	}
	sub, err := js.QueueSubscribe(cfg.Subject, cfg.Queue, func(m *nats.Msg) {
		var delivered uint64 = 1
		if md, err := m.Metadata(); err == nil {
			delivered = md.NumDelivered
		}
		if err := handler(m.Data, delivered); err != nil {
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	},
		nats.Durable(cfg.Durable),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.AckWait(cfg.AckWait),
		nats.MaxAckPending(2048),
		nats.DeliverAll(),
	)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("subscribe %q: %w", cfg.Subject, err)
	}
	return &Subscriber{nc: nc, js: js, sub: sub, cfg: cfg}, nil
}

// Stats describes the current JetStream stream + consumer state, used by the
// ops dashboard (a lightweight nats-top).
type Stats struct {
	Stream   string `json:"stream"`
	Messages uint64 `json:"messages"`
	Bytes    uint64 `json:"bytes"`
	FirstSeq uint64 `json:"firstSeq"`
	LastSeq  uint64 `json:"lastSeq"`
	Consumer struct {
		Name               string    `json:"name"`
		Created            time.Time `json:"created"`
		DeliveredStreamSeq uint64    `json:"deliveredStreamSeq"`
		AckFloorStreamSeq  uint64    `json:"ackFloorStreamSeq"`
		NumPending         int       `json:"numPending"`
		NumAckPending      int       `json:"numAckPending"`
		NumRedelivered     int       `json:"numRedelivered"`
		NumWaiting         int       `json:"numWaiting"`
		PushBound          bool      `json:"pushBound"`
	} `json:"consumer"`
}

// Stats returns the stream and consumer state observed through this
// subscriber's own NATS connection.
func (s *Subscriber) Stats(ctx context.Context) (*Stats, error) {
	si, err := s.js.StreamInfo(s.cfg.Stream, nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("stream info: %w", err)
	}
	ci, err := s.js.ConsumerInfo(s.cfg.Stream, s.cfg.Durable, nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("consumer info: %w", err)
	}
	st := &Stats{
		Stream:   si.Config.Name,
		Messages: si.State.Msgs,
		Bytes:    si.State.Bytes,
		FirstSeq: si.State.FirstSeq,
		LastSeq:  si.State.LastSeq,
	}
	st.Consumer.Name = ci.Name
	st.Consumer.Created = ci.Created
	st.Consumer.DeliveredStreamSeq = ci.Delivered.Stream
	st.Consumer.AckFloorStreamSeq = ci.AckFloor.Stream
	st.Consumer.NumPending = int(ci.NumPending)
	st.Consumer.NumAckPending = int(ci.NumAckPending)
	st.Consumer.NumRedelivered = int(ci.NumRedelivered)
	st.Consumer.NumWaiting = int(ci.NumWaiting)
	st.Consumer.PushBound = ci.PushBound
	return st, nil
}

// Close stops the subscription and releases the NATS connection.
func (s *Subscriber) Close() {
	if s.sub != nil {
		_ = s.sub.Unsubscribe()
	}
	s.nc.Close()
}
