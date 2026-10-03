# Scaling notes

This document describes the operational scaling envelope of the platform, the
knobs that exist today, and the migration path to larger deployments
(TimescaleDB, dedicated MQTT broker, more NATS replicas).

## Current envelope

| Component | Scale today | Ceiling driver |
|---|---|---|
| Device count (registry) | thousands | PostgreSQL (fine) |
| Protocol gateways | one per protocol, horizontally replicable via `GATEWAY_INSTANCE_ID` | broker state (MQTT) |
| Uplink ingestion | NATS JetStream durable/queue-group consumer; core replicas scale horizontally | consumer throughput |
| Time-series | partitioned PostgreSQL (`telemetry_data` monthly partitions) + hourly rollup + retention drop | single-node write throughput |
| Deployments | `docker compose` (PostgreSQL + NATS + core + gateways) | single VM |
| Logging | lumberjack size/age rotation per process | — |
| Time-series abstraction | `tsdb.Store` interface; PostgreSQL backend today | plug in Timescale/other |

## MQTT broker: durable sessions

The bundled MQTT gateway (`mochi-mqtt` embedded) is an in-memory broker:
subscriptions and QoS-1 in-flight messages survive per-connection, but **not a
process restart**. For fleets that must survive broker restarts, two options:

1. **Use clean sessions + reconnect semantics (default today).** Devices
   reconnect and re-report their state; the platform is shadow-driven, so the
   device shadow (`desired/reported/delta`) is the source of truth and lost
   broker memory is harmless for property reporting.
2. **Move MQTT to a durable broker** (EMQX, VerneMQ, or a NATS-based bridge)
   when you need QoS-1 offline delivery at scale. The gateway protocol is
   standard MQTT 3.1.1 — point the same consumer devices at the new broker and
   keep the platform behind it. The core/device contract (topics,
   `key`+`secret` auth, TLS/X.509) does not change.

TLS/mTLS is already supported (`MQTT_TLS_ADDR`, `MQTT_TLS_CA_FILE`); a durable
broker in front can terminate the same client certificates using the platform
CA (`CERTS_DIR`).

## Time-series: TimescaleDB migration path

The platform only talks to the `tsdb.Store` interface, so the storage backend
is swappable. To move `telemetry_data` to TimescaleDB:

1. Run the TimescaleDB extension on PostgreSQL, create the hypertable with the
   same columns:

   ```sql
   CREATE EXTENSION IF NOT EXISTS timescaledb;
   CREATE TABLE telemetry_data (
     time TIMESTAMPTZ NOT NULL,
     device_id BIGINT NOT NULL,
     identifier TEXT NOT NULL,
     data_type TEXT,
     num_value DOUBLE PRECISION, bool_value BOOLEAN,
     str_value TEXT, json_value JSONB
   );
   SELECT create_hypertable('telemetry_data', by_range('time'));
   ```

2. Implement `tsdb.Store` against it in `internal/tsdb/timescale`
   (the aggregate query becomes `time_bucket(interval, time)` instead of
   `date_bin`; the rollup and latest-value cache stay identical). The
   `pg.Store` in `internal/tsdb/pg` is the reference implementation.
3. Wire it in `cmd/core/main.go`:

   ```go
   telemetry := service.NewTelemetryService(timescale.New(db), cfg.Telemetry, log)
   ```

4. Backfill: insert from the old partitions (`INSERT INTO telemetry_data
   SELECT ... FROM telemetry_data_<month>`), or migrate via the hourly rollup
   for the retention window. The ingest dedup table and `device_latest_values`
   are plain rows and need no change.

## Core replicas & NATS

- Gateways publish uplinks to JetStream; every core replica joins the same
  durable queue group, so ingestion and rules partition across replicas.
  Background jobs (partition maintenance, retention, rollup, offline sweep,
  dedup cleanup) run on every replica but are fenced with PostgreSQL advisory
  locks — exactly one replica executes each job, regardless of restarts.
- Replica-safe real-time events fan out over a plain NATS subject; every
  replica forwards them to its local SSE hub.
- For site reliability: run NATS clustered (`nats://n1:4222,n2:4222,n3:4222`),
  set `NATS_DURABLE`/`NATS_QUEUE` so the consumer survives broker failover.

## Deployment

- `docker compose up --build` runs the whole stack on one host (see
  `deploy/`). For production: separate the three services, use managed
  PostgreSQL/NATS, keep `CERTS_DIR` + gateway secrets in a secrets store.
- Logging: processes write rotating logs via lumberjack (`LOG_FILE` +
  `LOG_MAX_*`), or emit JSON to stdout when the platform/container stack
  collects logs (leave `LOG_FILE` empty).