# AI IoT Platform

A multi-user, multi-project IoT platform with a pluggable protocol **access
layer**, a visual thing-model editor, time-series storage in PostgreSQL, and a
CEL-based rule engine.

```
                               ┌───────────────────────┐
                               │        Devices        │
                               │ MQTT · CoAP · TCP · … │
                               └───┬───────┬───────┬────┘
                                   │       │       │  device protocols
                                   ▼       ▼       ▼
                      ┌─────────────────────────────┐
                      │  mqtt-gw  coap-gw  custom-gw│
                      │  (each: downlink HTTP +     │
                      │   /metrics on 9101..9103)   │
                      └───────────┬─────────────────┘
                                  │ uplink (UplinkEnvelope JSON)
                      ┌───────────▼─────────────────┐
                      │  HTTP direct ingest         │
                      │  /api/v1/ingest/…           │
                      └───────────┬─────────────────┘
                                  ▼
                      ┌─────────────────────────────┐
                      │        NATS JetStream       │  mandatory uplink bus
                      │  stream AIOT_UPLINK         │
                      │  subject aiiot.uplink       │
                      └───────────┬─────────────────┘
                                  │ durable consumer `core-uplink`
                                  │ (queue group → horizontal scale)
                                  ▼
      ┌────────────────────────────────────────────────────────────┐
      │                     C O R E  (control plane)               │
      │  REST API (JWT) · ingest + resolver · shadow · rules (CEL) │
      │  OTA · gateway registry · SSE events · notify · admin API  │
      │  (Prometheus /metrics on every HTTP server)                │
      └──────┬──────────────┬─────────────────┬────────────────────┘
             │              │                 │
   HTTP      │  (auth/      │  telemetry      │  webhook notify /
   control   │   register/  ▼                 │  set_desired / device
   plane     │   downlink)  ┌───────────────┐ │  command
             ▼              │  PostgreSQL   │ ▼
       ┌────────────┐       │  telemetry    │ ┌────────────────────┐
       │ gateways   │       │  (monthly     │ │ webhook · dingtalk │
       │ downlink   │       │  partitions,  │ │ · email (SMTP)     │
       │ HTTP       │       │  rollup)      │ └────────────────────┘
       │ 9101..9103 │       └───────────────┘
       └────────────┘

  Frontend: SPA (business) ──HTTP──▶ core :8080
            Admin console (isolated) ──▶ /admin/* (NATS ops page ⬅ bus stats)
  Observability: Prometheus ──/metrics──▶ core + each gateway
```

## Highlights

- **Multi-user & multi-project** — JWT auth, project members/roles, per-project
  workspaces, devices and rules.
- **Products are global** — a product (device model + thing model + protocol)
  is a shared library, not tied to a project.
- **Workspaces** — the isolation boundary: devices can only exchange messages
  with peers in the *same workspace* (enforced by the gateway ACL and the core
  peer router).
- **Thing model (物模型)** — properties / services / events with data types,
  access modes, units and ranges, edited in the UI.
- **Device shadow** — per-device `desired` / `reported` state with a computed
  `delta` that is pushed to the device as a property-set so it converges.
- **Access abstraction layer** — every protocol implements one Go interface
  (`access.Adapter`) and runs under a common harness. MQTT, CoAP and a
  reference custom TCP protocol ship as **separate gateway programs**.
- **NATS uplink bus** — device uplinks (the data plane) flow through a NATS
  JetStream stream with a durable, queue-grouped consumer, so the core can
  buffer and scale horizontally. Register/heartbeat/auth and downlinks keep
  using HTTP (request/response).
- **CEL rule engine** — conditions are Google CEL expressions; actions include
  webhook, downlink/publish, device command and log.
- **Time-series in PostgreSQL** — a declaratively partitioned table
  (`telemetry_data`, monthly partitions) plus a latest-value cache and
  `date_bin` downsampling.
- **Responsive UI** — shadcn/Radix + Tailwind, adaptive from mobile to desktop.

## Repository layout

```
backend/
  cmd/core/             control plane (API, ingest, rules, static frontend)
  cmd/mqtt-gateway/     standalone MQTT gateway (embedded broker)
  cmd/coap-gateway/     standalone CoAP gateway
  cmd/custom-gateway/   reference custom TCP gateway
  internal/access/      access layer interface + wire types
  internal/bus/         NATS JetStream uplink bus (publisher + subscriber)
  internal/metrics/     Prometheus metrics + dashboard snapshot
  internal/gateway/     gateway protocol, client, harness, registry
  internal/adapters/    mqtt | coap | customtcp implementations
  internal/service/     resolver, ingest, telemetry, CEL rule engine, downlink
  internal/api/         REST handlers + static SPA hosting
  examples/coapclient/  tiny CoAP example client
  web/dist/             built frontend (served by the core)
frontend/               React + Vite + Tailwind + shadcn source
docs/
  custom-protocol.md    how to add a protocol / the gateway protocol
```

## Quick start

Prerequisites: Go 1.25+, PostgreSQL 14+, NATS 2.10+ (with JetStream), Node 18+,
pnpm.

```bash
# 1. Database
createdb aiiot

# 1b. NATS (mandatory uplink bus; JetStream enabled)
nats-server -js          # or point NATS_URL at an existing cluster

# 2. Configure
cp backend/.env.example backend/.env      # edit DB + JWT_SECRET + GATEWAY_TOKEN + NATS_URL

# 3. Build the frontend into backend/web/dist
cd frontend && pnpm install && pnpm build && cd ..

# 4. Build the backend binaries
cd backend
go build -o bin/core          ./cmd/core
go build -o bin/mqtt-gateway  ./cmd/mqtt-gateway
go build -o bin/coap-gateway  ./cmd/coap-gateway
go build -o bin/custom-gateway ./cmd/custom-gateway

# 5. Run the core (from backend/ so .env and web/dist resolve)
./bin/core

# 6. Run one or more gateways (separate terminals)
GATEWAY_TOKEN=$(grep GATEWAY_TOKEN .env | cut -d= -f2) ./bin/mqtt-gateway
GATEWAY_TOKEN=... ./bin/coap-gateway
GATEWAY_TOKEN=... ./bin/custom-gateway
```

Prefer shortcuts? `make build` builds the frontend + all Go binaries, and
`make run` / `make run-mqtt` / `make run-coap` / `make run-custom` start the
services. `docker compose up --build` runs the whole stack (PostgreSQL +
NATS + core + the three gateways).

Open <http://localhost:8080>. The **first registered account becomes a system
admin**.

## Default ports

| Service        | Protocol | Address            |
|----------------|----------|--------------------|
| core HTTP/SPA  | HTTP     | `0.0.0.0:8080`     |
| NATS (JetStream) | NATS   | `0.0.0.0:4222`     |
| NATS monitoring | HTTP    | `0.0.0.0:8222`     |
| mqtt-gateway   | MQTT     | `0.0.0.0:1883`     |
| mqtt-gateway   | MQTT/WS  | `0.0.0.0:8083`     |
| coap-gateway   | CoAP UDP | `0.0.0.0:5683`     |
| custom-gateway | TCP      | `0.0.0.0:9000`     |
| gateway downlinks | HTTP  | `0.0.0.0:9101..9103` |

All services bind `0.0.0.0` by default and can be overridden with env vars
(`HTTP_ADDR`, `MQTT_TCP_ADDR`, `COAP_UDP_ADDR`, `CUSTOM_TCP_ADDR`, ...).

## Device addressing & credentials

Topics/resources are **device-scoped** (no workspace/project segment):

```
{productKey}/{deviceKey}/{suffix}
```

MQTT/CoAP credentials:

- username: `{productKey}/{deviceKey}`
- password: device secret

The core resolves the device and returns its `workspaceKey`. The gateway
restricts a device to its **own topic subtree**; cross-device (peer) messages
are routed by the core, which enforces same-workspace delivery. Workspace keys
are therefore unique per project, not globally.

## Configuration

See `backend/.env.example`. Important variables:

| Variable | Purpose |
|---|---|
| `JWT_SECRET` | token signing key |
| `GATEWAY_TOKEN` | shared secret between core and all gateways |
| `HTTP_ADDR` | core listen address |
| `WEB_DIR` | directory of the built SPA (default `./web/dist`) |
| `GATEWAY_CORE_URL` | core URL used by a gateway |
| `GATEWAY_ADVERTISE_HOST` | host the core uses to reach a gateway's downlink |
| `NATS_URL` | NATS server URL (required) |
| `NATS_USER` / `NATS_PASSWORD` | optional NATS credentials |
| `NATS_CREDS` | optional path to a `.creds` file |
| `NATS_STREAM` / `NATS_UPLINK_SUBJECT` | JetStream stream name / subject |
| `NATS_DURABLE` / `NATS_QUEUE` | durable consumer + queue group (core scaling) |
| `TELEMETRY_RETENTION_DAYS` | drop partitions older than this |

## Device shadow

Every device has a shadow with `desired`, `reported` and a computed `delta`
(desired keys that differ from reported):

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/devices/:id/shadow` | read desired/reported/delta |
| `PATCH` | `/api/v1/devices/:id/shadow/desired` | merge-patch desired (null deletes a key) |
| `DELETE` | `/api/v1/devices/:id/shadow/desired` | clear desired |
| `GET` | `/api/v1/devices/:id/shadow/history` | audit trail of changes |
| `GET` | `/api/v1/devices/:id/connection` | ready-to-use connection instructions |

The **Connect** panel on the device page turns the connection endpoint into
copyable credentials and protocol-specific topics/resources (MQTT, CoAP, custom
TCP) plus a downloadable credentials JSON — the fastest path to bringing a
device online. Set `PUBLIC_HOST` when the platform is reached via a NAT/ingress
so the advertised host is correct.

Reported state is updated automatically from telemetry. Whenever the delta is
non-empty (on a desired update or a new report) it is pushed to the device as a
`property` downlink; devices converge by reporting the applied value. Offline
devices receive the retained delta on their next report.

Every desired/reported change is written to `device_shadow_logs` with its
source (`api`, `rule`, `telemetry`), so the DeviceDetail → Shadow tab shows a
complete audit trail.

## Device timeline & downlink journal

Every device detail page has a **Timeline** view merging device creation,
online/offline transitions, thing-model events, shadow changes, downlinks and
OTA progress into one reverse-chronological feed with per-type filters:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/devices/:id/timeline` | unified timeline (device / event / shadow / downlink / ota) |
| `GET` | `/api/v1/devices/:id/downlinks` | downlink journal (API, batch, rule, shadow delta, OTA) |
| `GET` | `/api/v1/devices/:id/events?q=&limit=` | events with keyword search (identifier / type / payload text) |

Downlinks are journaled centrally in `device_downlink_logs` by the
`DownlinkService`, so every platform-originated message — API command, batch
command, rule action, shadow delta push and OTA dispatch — is traceable with
its result. Lifecycle transitions are journaled to `device_events`, so the
timeline shows exactly when a device came online / went offline.

## OTA firmware upgrade

Firmware is uploaded per product and rolled out to devices:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/products/:id/firmwares` | upload (multipart: `file`, `version`, `name`) |
| `GET` | `/api/v1/products/:id/firmwares` | list |
| `DELETE` | `/api/v1/firmwares/:id` | delete |
| `GET` | `/ota/firmware/:id` | device download (checksum in `X-Checksum-Sha256`) |
| `POST` | `/api/v1/projects/:id/ota-tasks` | create + dispatch rollout |
| `GET` | `/api/v1/ota-tasks/:id` | task with per-device progress |
| `POST` | `/api/v1/ota-tasks/:id/cancel` | cancel |
| `POST` | `/api/v1/ota-tasks/:id/rollback` | re-install the previous firmware |

Devices receive an `ota` downlink (`{taskId,version,url,checksum,size}`), download
the binary and report progress by publishing to `.../ota/status` (any protocol):

```json
{ "taskId": 1, "status": "downloading|upgrading|succeeded|failed", "progress": 42, "message": "..." }
```

On success the device's `firmwareVersion` is updated and the task is marked
`succeeded`. Firmware lives on disk (`OTA_DIR`); download URLs use
`OTA_PUBLIC_BASE_URL`.

### Canary / batched rollouts

Set `batchSize` when creating a task to split the device list into waves. Only
the first wave is dispatched; when every device in a wave reaches a terminal
state the next wave is dispatched automatically. With `haltOnFailure` (default
true) the rollout stops as soon as a wave contains a failure, leaving later
waves `pending`.

```bash
curl -X POST .../projects/1/ota-tasks -d '{
  "firmwareId": 2, "name": "Canary 1.3.0",
  "productId": 1, "workspaceId": 1,
  "batchSize": 5, "haltOnFailure": true
}'
```

`POST /api/v1/ota-tasks/:id/rollback` creates a new rollout that reinstalls the
previous firmware version on the same devices.

## Notification channels

Per-project alert destinations used by the `notify` rule action:

| Method | Path | Purpose |
|---|---|---|
| `GET`/`POST` | `/api/v1/projects/:id/channels` | list / create |
| `PUT`/`DELETE` | `/api/v1/channels/:id` | update / delete |
| `POST` | `/api/v1/channels/:id/test` | send a test alert |
| `GET` | `/api/v1/channels/:id/logs` | delivery attempts |

Types: `webhook`, `dingtalk`, `email` (email needs `SMTP_HOST`, `SMTP_USER`,
`SMTP_PASS`, `SMTP_FROM`).

The `notify` rule action supports templates in addition to a plain message:

```json
{
  "type": "notify", "channelId": 1,
  "titleTemplate": "Overheat on {{.deviceKey}}",
  "bodyTemplate": "{{.identifier}} = {{.value}} (kind={{.kind}}) at {{.now}}"
}
```

Template variables: `ruleName`, `deviceKey`, `deviceId`, `identifier`,
`value`, `params`, `kind`, `state`, `now`. (`messageExpr`/`message` remain
available for non-templated payloads.)

### Silencing & aggregation

The `notify` action supports de-duplication and batching:

```json
{ "type": "notify", "channelId": 1,
  "silenceSeconds": 300, "aggregateSeconds": 60, "title": "Overheat" }
```

- `silenceSeconds` suppresses identical alerts (channel+rule+device) that follow
a successful delivery within the window; suppressed alerts are still logged
(`suppressed=true`) for auditing.
- `aggregateSeconds` buffers alerts and delivers a single summary (with event
  count) per window.
- A channel can set a default window via `config.silenceSeconds`.

## Storage & retention

`GET /api/v1/storage` returns the retention policy plus the telemetry partition
inventory (ranges, estimated rows, on-disk size) and the state of the hourly
rollup. `PUT /api/v1/storage/retention` (`{days}`) sets a runtime override
stored in `system_settings`; the retention job drops partitions older than it.
`DELETE /api/v1/storage/partitions/:name` (admin) drops a single partition.
`POST /api/v1/storage/rollup/refresh` rebuilds the rollup view on demand. The
**Storage** page visualises all of this.

### Hourly rollup (continuous aggregate)

The materialized view `telemetry_hourly` pre-aggregates
`avg/min/max/count` per device/identifier/hour. The core refreshes it every 5
minutes (and on demand). Device telemetry queries automatically read from the
rollup when the requested `interval >= 1h`, which keeps long-range charts fast;
the response reports `source: "rollup" | "raw"`.

## Batch device operations

`POST /api/v1/projects/:id/devices/batch` applies one operation to many devices
and returns a per-device result:

```json
{ "deviceIds": [1, 2], "action": "command",
  "kind": "property", "payload": { "reportInterval": 30 } }
```

`action` is `command` (downlink property/service/peer) or `set_desired` (shadow
desired state, which pushes the delta). Devices not belonging to the project are
reported as failures. Either list `deviceIds` or pass a `groupId`. The
**Devices** page exposes this via row checkboxes and a **Batch** dialog.

### Device groups

Named device sets per project (`/api/v1/projects/:id/device-groups`), with
members managed via `/api/v1/device-groups/:id/devices`. Groups can be used as a
batch target (`{"groupId": 1, ...}`) and are managed on the **Groups** page.

## Audit log

Every mutating API request is recorded in `audit_logs` (user, method, path,
status, IP, project) by the audit middleware; device ingest and auth endpoints
are excluded. `GET /api/v1/audit-logs` returns the history — system admins see
everything, other users only their own entries. Visualised on the **Audit** page.

## Administration console (isolated)

The administration console is a **separate identity system** from business users:

- Administrators live in their own `admin_users` table and log in at
  `/admin/login` (API: `POST /api/v1/admin/auth/login`).
- Admin tokens carry `kind=admin` and can only call `/api/v1/admin/*`; business
  tokens (kind=user) are rejected there, and admin tokens are rejected on
  business APIs.
- Admin endpoints: gateways, storage/retention/rollup, audit logs.
- The initial administrator is seeded on first start from `ADMIN_USERNAME` /
  `ADMIN_PASSWORD` (a random password is generated and logged if unset).

Business users can never gain admin access, and administrators are not project
members.

## Tenant (workspace) via access subdomain

The workspace is identified by the **access subdomain** (`{workspaceKey}.example.com`),
not by the topic. A device connects to `factory1.example.com` (MQTT over TLS) and
the gateway derives `workspace=factory1` from the TLS SNI, passing it to the core
as an auth hint; the core rejects the device unless it belongs to that workspace.

- MQTT: enable `MQTT_TLS_ADDR` / `MQTT_TLS_CERT` / `MQTT_TLS_KEY`; SNI → tenant.
- Plain TCP (no hostname) falls back to the device-derived workspace.
- Requires wildcard DNS `*.example.com` + a wildcard certificate, routed to the
gateway Service.

## Real-time events (SSE)

`GET /api/v1/events/stream?token=<jwt>&projectId=<id>` streams device telemetry
and online/offline transitions as Server-Sent Events. The **Dashboard** shows a
live activity feed and connection indicator; the device page refreshes latest
values in real time. Keep-alive comments are sent every 25s.

## Observability (Prometheus metrics)

Both the core and every gateway expose Prometheus metrics:

- core: `GET /metrics` on the API server,
- gateways: `GET /metrics` on each gateway's downlink HTTP server
  (`GATEWAY_DOWNLINK_ADDR`).

```bash
curl -s localhost:8080/metrics | head
```

Key metrics:

| Metric | Meaning |
|---|---|
| `aiiot_http_requests_total` / `aiiot_http_request_duration_seconds` | HTTP API load, per route (low-cardinality route templates) |
| `aiiot_ingest_messages_total` / `aiiot_ingest_duration_seconds` | uplinks processed by the core, by protocol/kind/outcome |
| `aiiot_nats_published_total` / `aiiot_nats_publish_duration_seconds` | NATS publish (gateway side) |
| `aiiot_nats_consumed_total` / `aiiot_nats_consume_duration_seconds` / `aiiot_nats_redelivered_total` | NATS consume + ingest and redeliveries (core side) |
| `aiiot_rule_evaluations_total` / `aiiot_rule_triggers_total` | rule engine activity |
| `aiiot_devices_online` / `aiiot_gateways_healthy` | gauges refreshed every 30s |

Point a Prometheus/Grafana (or any `/metrics` scraper) at the core and gateways.

A built-in **NATS ops page** (`/admin/nats`, in the isolated administration
console, via `GET /api/v1/admin/nats-stats`) shows the uplink stream and
consumer state (messages, delivered/ack floor, pending, redeliveries, push
binding) plus the dashboard counters — a lightweight `nats-top` without extra
tooling.

## Rule engine (CEL)

Rules match a trigger (telemetry / event / device_online / device_offline /
peer_message) and evaluate a CEL condition against the event context
(`identifier`, `value`, `params`, `payload`, `kind`, `state`, `device_key`,
`now`, ...). Example:

```cel
identifier == "temperature" && double(value) > 40.0
```

Triggers include `telemetry`, `event`, `device_online`, `device_offline`,
`peer_message` and `shadow_delta`.

Actions are JSON objects: `webhook`, `mqtt_publish`/`downlink`,
`device_command`, `set_desired`, `notify`, `log`. The **Rules** page provides a
visual action editor for all of these (with channel pickers, shadow desired and
downlink payload fields, and notification templates), plus a JSON advanced mode
for arbitrary action definitions.
For example, an overheating rule can drive the device back to a safe setpoint
via the shadow and alert an operator:

```json
{
  "name": "Cool down on overheat",
  "triggerType": "telemetry",
  "triggerSource": "temperature",
  "condition": "identifier == \"temperature\" && double(value) > 90.0",
  "actions": [
    { "type": "set_desired", "desired": { "temperature": 80 } },
    { "type": "notify", "channelId": 1, "title": "Overheat on " + device_key }
  ]
}
```

## Documentation

- **[docs/architecture.md](docs/architecture.md)** — Mermaid architecture
  diagram (renders in GitLab / 语雀 / GitHub).
- **[docs/custom-protocol.md](docs/custom-protocol.md)** — access-layer
  abstraction, the gateway protocol, and how to add a new protocol.
- **deploy/k8s/aiiot.yaml** — Kubernetes manifests (PostgreSQL, core, the three
gateways, Service/Ingress/HPA). Build and push the image, set `image:`, then
`kubectl apply -f deploy/k8s/aiiot.yaml`.
