// Package metrics exposes Prometheus metrics for the core and gateways.
//
// The core scrapes /metrics from its HTTP server; each gateway also exposes
// /metrics on its downlink HTTP server so NATS publish metrics can be scraped
// per gateway.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// registry is isolated so the package never pollutes the global default.
var registry = prometheus.NewRegistry()

var (
	httpRequestsTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "aiiot_http_requests_total",
		Help: "Total HTTP requests handled.",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.With(registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aiiot_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	ingestMessagesTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "aiiot_ingest_messages_total",
		Help: "Uplink messages processed by the core, by protocol/kind/outcome.",
	}, []string{"protocol", "kind", "outcome"})

	ingestDedupDropped = promauto.With(registry).NewCounter(prometheus.CounterOpts{
		Name: "aiiot_ingest_dedup_dropped_total",
		Help: "Redelivered uplinks dropped by the ingest idempotency claim.",
	})

	ingestDuration = promauto.With(registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aiiot_ingest_duration_seconds",
		Help:    "Time to process one uplink message.",
		Buckets: prometheus.DefBuckets,
	}, []string{"protocol", "kind"})

	natsPublishedTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "aiiot_nats_published_total",
		Help: "Uplink messages published to NATS by gateways, by subject/outcome.",
	}, []string{"subject", "outcome"})

	natsPublishDuration = promauto.With(registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aiiot_nats_publish_duration_seconds",
		Help:    "NATS publish latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"subject"})

	natsConsumedTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "aiiot_nats_consumed_total",
		Help: "Uplink messages consumed from NATS by the core, by subject/outcome.",
	}, []string{"subject", "outcome"})

	natsConsumeDuration = promauto.With(registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aiiot_nats_consume_duration_seconds",
		Help:    "NATS consume + ingest latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"subject"})

	natsRedeliveredTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "aiiot_nats_redelivered_total",
		Help: "NATS uplink messages redelivered (delivery count > 1).",
	}, []string{"subject"})

	ruleEvaluationsTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "aiiot_rule_evaluations_total",
		Help: "CEL rule evaluations by trigger type.",
	}, []string{"trigger"})

	ruleTriggersTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "aiiot_rule_triggers_total",
		Help: "Rule actions fired, by rule name.",
	}, []string{"rule"})

	devicesOnline = promauto.With(registry).NewGauge(prometheus.GaugeOpts{
		Name: "aiiot_devices_online",
		Help: "Number of devices currently online.",
	})

	gatewaysHealthy = promauto.With(registry).NewGauge(prometheus.GaugeOpts{
		Name: "aiiot_gateways_healthy",
		Help: "Number of healthy gateway instances.",
	})
)

func init() {
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// Handler returns the /metrics exposition handler.
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// HTTP returns a Gin middleware that records request count and latency using
// the route template (c.FullPath) to keep cardinality bounded.
func HTTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}

// ObserveIngestDedupDrop records a duplicate uplink dropped by idempotency.
func ObserveIngestDedupDrop() { ingestDedupDropped.Inc() }

// ObserveIngest records one processed uplink message.
func ObserveIngest(protocol, kind string, err error, start time.Time) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	if protocol == "" {
		protocol = "unknown"
	}
	ingestMessagesTotal.WithLabelValues(protocol, kind, outcome).Inc()
	ingestDuration.WithLabelValues(protocol, kind).Observe(time.Since(start).Seconds())
}

// ObserveNATSPublish records one NATS publish attempt (gateway side).
func ObserveNATSPublish(subject string, err error, start time.Time) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	natsPublishedTotal.WithLabelValues(subject, outcome).Inc()
	natsPublishDuration.WithLabelValues(subject).Observe(time.Since(start).Seconds())
}

// ObserveNATSConsume records one NATS consume + ingest (core side).
func ObserveNATSConsume(subject string, err error, delivered uint64, start time.Time) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	natsConsumedTotal.WithLabelValues(subject, outcome).Inc()
	natsConsumeDuration.WithLabelValues(subject).Observe(time.Since(start).Seconds())
	if delivered > 1 {
		natsRedeliveredTotal.WithLabelValues(subject).Inc()
	}
}

// ObserveRuleEvaluation increments the rule-evaluation counter for a trigger.
func ObserveRuleEvaluation(trigger string) {
	ruleEvaluationsTotal.WithLabelValues(trigger).Inc()
}

// ObserveRuleTrigger increments the rule-trigger counter for a rule name.
func ObserveRuleTrigger(rule string) {
	ruleTriggersTotal.WithLabelValues(rule).Inc()
}

// SetDevicesOnline sets the online-device gauge.
func SetDevicesOnline(n float64) { devicesOnline.Set(n) }

// SetGatewaysHealthy sets the healthy-gateway gauge.
func SetGatewaysHealthy(n float64) { gatewaysHealthy.Set(n) }

// Snapshot is the subset of counter/gauge values shown on the ops dashboard.
type Snapshot struct {
	NATSConsumedTotal uint64  `json:"natsConsumedTotal"`
	NATSConsumeErrors uint64  `json:"natsConsumeErrors"`
	NATSRedelivered   uint64  `json:"natsRedelivered"`
	IngestTotal       uint64  `json:"ingestTotal"`
	IngestErrors      uint64  `json:"ingestErrors"`
	RuleEvaluations   uint64  `json:"ruleEvaluations"`
	RuleTriggers      uint64  `json:"ruleTriggers"`
	DevicesOnline     float64 `json:"devicesOnline"`
	GatewaysHealthy   float64 `json:"gatewaysHealthy"`
}

// Collect returns the current dashboard counters. subject is the NATS uplink
// subject used to break out consume errors.
func Collect(subject string) Snapshot {
	snap := Snapshot{
		NATSConsumedTotal: counterSum(natsConsumedTotal, nil),
		NATSRedelivered:   counterSum(natsRedeliveredTotal, nil),
		IngestTotal:       counterSum(ingestMessagesTotal, nil),
		IngestErrors:      counterSum(ingestMessagesTotal, func(l prometheus.Labels) bool { return l["outcome"] == "error" }),
		RuleEvaluations:   counterSum(ruleEvaluationsTotal, nil),
		RuleTriggers:      counterSum(ruleTriggersTotal, nil),
		DevicesOnline:     gaugeValue(devicesOnline),
		GatewaysHealthy:   gaugeValue(gatewaysHealthy),
	}
	if subject != "" {
		snap.NATSConsumeErrors = counterSum(natsConsumedTotal, func(l prometheus.Labels) bool {
			return l["subject"] == subject && l["outcome"] == "error"
		})
	}
	return snap
}

// counterSum sums all children of vec, optionally filtered by match.
// prometheus.Collect does not close the channel (the registry does), so the
// collection runs on a goroutine that closes it after Collect returns.
func counterSum(vec *prometheus.CounterVec, match func(prometheus.Labels) bool) uint64 {
	ch := make(chan prometheus.Metric, 32)
	go func() {
		defer close(ch)
		vec.Collect(ch)
	}()
	var sum float64
	for m := range ch {
		var meta dto.Metric
		if err := m.Write(&meta); err != nil || meta.Counter == nil {
			continue
		}
		if match != nil {
			labels := prometheus.Labels{}
			for _, lp := range meta.Label {
				labels[lp.GetName()] = lp.GetValue()
			}
			if !match(labels) {
				continue
			}
		}
		sum += meta.Counter.GetValue()
	}
	return uint64(sum)
}

func gaugeValue(g prometheus.Gauge) float64 {
	var dto dto.Metric
	_ = g.Write(&dto)
	if dto.Gauge == nil {
		return 0
	}
	return dto.Gauge.GetValue()
}
