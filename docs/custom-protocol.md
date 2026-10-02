# Custom Protocol Access Guide

This document explains the **access abstraction layer**, the **gateway
protocol** used between the core control plane and standalone protocol
gateways, and how to add a brand-new protocol.

---

## 1. Why gateways are separate programs

The core (control plane) never speaks a device protocol directly. Each protocol
runs as its own process — an **access gateway** — which:

1. terminates the device protocol (MQTT broker, CoAP server, TCP listener, …),
2. authenticates devices against the core,
3. normalizes traffic into `UplinkMessage`s and forwards them to the core,
4. receives `DownlinkMessage`s from the core and delivers them to devices.

```
device ──protocol──▶ gateway ──uplink (NATS JetStream)──▶ core
                       ▲                                    │
                       └── register/auth (HTTP), downlink ──┘
```

Uplinks (the data plane) flow through a **NATS JetStream** stream so the core
can buffer and scale horizontally; register / heartbeat / auth and downlinks
keep using HTTP because they are request/response. This keeps
protocol-specific dependencies and blast radius out of the control plane and
lets you scale or restart each protocol independently.

---

## 2. The `access.Adapter` interface

Every protocol gateway implements this single Go interface
(`backend/internal/access/adapter.go`):

```go
type Adapter interface {
    Name() string
    Start(ctx context.Context) error       // bind listener(s); block until ctx done
    Stop(ctx context.Context) error
    OnUplink(handler UplinkHandler)        // core ← device messages
    OnAuth(handler AuthHandler)            // authenticate a device
    Downlink(ctx context.Context, msg *DownlinkMessage) error // core → device
}
```

Your gateway calls the registered `UplinkHandler` for every inbound message and
the `AuthHandler` before accepting a device. It must implement `Downlink` to
deliver commands pushed by the core (rules, API).

### Normalized message model

```go
type DeviceRef struct {
    WorkspaceKey string `json:"workspaceKey,omitempty"`
    ProductKey   string `json:"productKey"`
    DeviceKey    string `json:"deviceKey"`
}

type UplinkMessage struct {
    Protocol   string            `json:"protocol"`
    Device     DeviceRef         `json:"device"`
    Kind       UplinkKind        `json:"kind"`
    Identifier string            `json:"identifier,omitempty"`
    Payload    []byte            `json:"payload"`
    Timestamp  time.Time         `json:"timestamp"`
    Metadata   map[string]string `json:"metadata,omitempty"`
}

type DownlinkMessage struct {
    Protocol   string            `json:"protocol"`
    Device     DeviceRef         `json:"device"`
    Kind       UplinkKind        `json:"kind"`
    Identifier string            `json:"identifier,omitempty"`
    Payload    []byte            `json:"payload"`
    Metadata   map[string]string `json:"metadata,omitempty"`
}
```

`UplinkKind` is one of:

| Kind | Meaning |
|---|---|
| `property` | thing-model property report (batch if `identifier` empty) |
| `event` | thing-model event report |
| `service_reply` | reply to a service invocation |
| `service_call` | (downlink) invoke a service on the device |
| `peer` | device-to-device message, workspace-scoped |
| `lifecycle` | online/offline transition (`metadata["state"]`) |

### Device authentication

```go
type AuthRequest struct {
    ProductKey string
    DeviceKey  string
    Username   string
    Password   string
    RemoteAddr string
    Protocol   string
}

type AuthResponse struct {
    Authorized   bool
    Reason       string
    DeviceID     uint
    ProjectID    uint
    WorkspaceID  uint
    ProductID    uint
    WorkspaceKey string   // used for workspace isolation
    ProductKey   string
    DeviceKey    string
    Protocol     string
}
```

The handler delegates to the core. **You must reject the device when
`Authorized == false`.** Keep the returned `WorkspaceKey`/`DeviceKey` to enforce
that a device may only access its own workspace.

---

## 3. The gateway harness (recommended)

You almost never call the client directly. Run your adapter under the shared
harness (`backend/internal/gateway`):

```go
hc := gateway.LoadHarnessConfig(access.ProtocolCustomName, "0.0.0.0:9103", "1.0.0")
adapter := myproto.New(myproto.Options{Addr: ":7000"})
log.Fatal(gateway.Run(ctx, adapter, hc))
```

`gateway.Run` wires everything:

- `Adapter.OnAuth` → core `/internal/gateway/auth` (with a TTL cache),
- `Adapter.OnUplink` → NATS JetStream subject `aiiot.uplink` (publish ack;
  falls back to HTTP `/internal/gateway/uplink` on transient NATS failures),
- exposes `POST /downlink` on `GATEWAY_DOWNLINK_ADDR` and forwards to
  `Adapter.Downlink`,
- registers the gateway and sends heartbeats (re-registering if the core
  restarts).

NATS is a **mandatory** component: `gateway.Run` refuses to start if it cannot
reach the NATS server (`NATS_URL`, default `nats://127.0.0.1:4222`).

---

## 4. Gateway wire protocol (core ⇄ gateway)

All gateway→core requests send the shared secret header:

```
X-Gateway-Token: <GATEWAY_TOKEN>
Content-Type: application/json
```

### 4.1 Register

`POST /internal/gateway/register`

```json
{
  "instanceId": "custom-1",
  "protocol": "custom",
  "version": "1.0.0",
  "downlinkUrl": "http://10.0.0.5:9103/downlink",
  "metadata": {}
}
```

Response:

```json
{ "ok": true, "heartbeatIntervalSeconds": 30, "downlinkEnabled": true }
```

### 4.2 Heartbeat

`POST /internal/gateway/heartbeat`

```json
{ "instanceId": "custom-1", "activeDevices": 12, "uptimeSeconds": 3841 }
```

`200 {"ok":true}` on success, `410` if the instance is unknown (the gateway
should re-register).

### 4.3 Authenticate a device

`POST /internal/gateway/auth`

```json
{ "productKey": "sensor", "deviceKey": "sensor-a", "password": "<secret>",
  "remoteAddr": "10.0.0.9:51000", "protocol": "custom" }
```

Response (see `AuthResponse` above). Reject on `authorized: false`.

### 4.4 Uplink

The primary uplink path is **NATS JetStream**: the harness serializes the
`UplinkEnvelope` JSON (same shape below) and publishes it to the subject
`NATS_UPLINK_SUBJECT` (default `aiiot.uplink`) of the stream `NATS_STREAM`
(default `AIOT_UPLINK`). The core consumes it through a durable,
queue-grouped consumer, so multiple core replicas share the ingest load.

The HTTP endpoint below remains as a fallback (and for direct use by tests):

`POST /internal/gateway/uplink`

```json
{
  "protocol": "custom",
  "device": { "workspaceKey": "factory1", "productKey": "sensor", "deviceKey": "sensor-a" },
  "kind": "property",
  "identifier": "temperature",
  "payload": { "value": 25.4 },     // arbitrary JSON
  "timestamp": "2026-01-01T12:00:00Z",
  "metadata": { "topic": "..." }
}
```

`202 {"accepted":true}` when ingested, `422` with an error otherwise.

### 4.5 Downlink (core → gateway)

The core POSTs to the gateway's registered `downlinkUrl`:

```json
{
  "device": { "workspaceKey": "factory1", "productKey": "sensor", "deviceKey": "sensor-a" },
  "kind": "property",
  "identifier": "",
  "payload": { "temperature": 30 },
  "metadata": { "source": "rule_engine" }
}
```

The gateway responds `202` once it has queued/delivered the message.

---

## 5. Enforcing workspace isolation

The platform guarantee is: **a device can only exchange messages with devices
in the same workspace.** Your adapter must enforce this on the data plane; the
core re-checks it for `peer` messages.

- Reject any subscribe/publish/read outside the authenticated `WorkspaceKey`.
- Allow a device to publish only under its own `DeviceKey`; allow subscribing to
  any device in the same workspace (so forwarded peer messages arrive).
- The core's peer router (`IngestService.handlePeer`) resolves the target
  strictly within the source device's workspace.

---

## 6. Reference protocol mappings

### 6.1 MQTT (`adapters/mqtt`)

```
{productKey}/{deviceKey}/properties/post
{productKey}/{deviceKey}/properties/{identifier}
{productKey}/{deviceKey}/events/{identifier}/post
{productKey}/{deviceKey}/services/{identifier}/reply
{productKey}/{deviceKey}/peer/{targetDeviceKey}
{productKey}/{deviceKey}/peer/broadcast
```

- username `{productKey}/{deviceKey}`, password = secret.
- The ACL hook rejects topics outside the device's own subtree; cross-device
  traffic is mediated by the core.
- Downlink: `properties/set`, `services/{identifier}/call`, `peer/{sourceDeviceKey}`.

### 6.2 CoAP (`adapters/coap`)

```
POST /{productKey}/{deviceKey}/properties?secret=<secret>
POST /{productKey}/{deviceKey}/events/{identifier}?secret=<secret>
POST /{productKey}/{deviceKey}/peer?secret=<secret>
```

Downlink uses the device's last-known address and POSTs to
`/{productKey}/{dev}/downlink` (or `/services/{id}/call`).

### 6.3 Reference custom TCP protocol (`adapters/customtcp`)

Newline-delimited JSON (NDJSON) over TCP. First frame must authenticate:

```json
{"type":"auth","productKey":"sensor","deviceKey":"sensor-a","secret":"<secret>"}
```

Server replies `{"type":"auth_ok"}`. Then uplink frames:

```json
{"type":"property","identifier":"temperature","value":25.4,"id":"m1"}
{"type":"properties","params":{"temperature":25.4,"humidity":60},"id":"m2"}
{"type":"event","identifier":"alarm","params":{"level":3}}
{"type":"service_reply","identifier":"reboot","params":{"status":"ok"}}
{"type":"peer","to":"sensor-b","payload":{"msg":"hi"}}
{"type":"peer","broadcast":true,"payload":{"msg":"all"}}
{"type":"lifecycle","state":"online"}
```

Each uplink may carry an `id`; the gateway answers `{"type":"ack","id":"m1"}`.

Downlink frames from the gateway:

```json
{"type":"property_set","params":{"temperature":30}}
{"type":"service_call","identifier":"reboot","params":{}}
{"type":"peer","payload":{"msg":"hi"},"deviceKey":"sensor-a"}
```

---

## 7. Protocol-agnostic HTTP ingest (no gateway needed)

For simple bridges you can skip the gateway protocol entirely and POST directly
to the core:

```
POST /api/v1/ingest/{productKey}/{deviceKey}
X-Device-Secret: <secret>
Content-Type: application/json

{ "kind": "property", "params": { "temperature": 25.4 } }
```

```bash
curl -X POST http://localhost:8080/api/v1/ingest/sensor/sensor-a \
  -H "X-Device-Secret: $SECRET" -H 'Content-Type: application/json' \
  -d '{"kind":"property","params":{"temperature":25.4}}'
```

Supported `kind`s: `property`/`properties`, `event`, `service_reply`, `peer`
(with `to` or `broadcast`), `lifecycle` (with `state`).

---

## 8. Step by step: adding a new protocol

Example: a Modbus/TCP gateway.

1. Create `backend/internal/adapters/modbus/adapter.go`.
2. Define `Options` (listen address, poll interval, …) and a `type Adapter`.
3. Implement the six `access.Adapter` methods:
   - `Start`: dial/accept devices; for each, call the `AuthHandler`.
   - On inbound data, build an `UplinkMessage` and call the `UplinkHandler`.
   - `Downlink`: write a Modbus register/coil for the target device.
   - Enforce workspace isolation using the `AuthResponse` you cached.
4. Create `backend/cmd/modbus-gateway/main.go`:

   ```go
   hc := gateway.LoadHarnessConfig("modbus", "0.0.0.0:9104", "1.0.0")
   adapter := modbus.New(modbus.Options{Addr: gateway.GetStr("MODBUS_ADDR", "0.0.0.0:502")})
   if err := gateway.Run(ctx, adapter, hc); err != nil { ... }
   ```

5. Build and run it with the same `GATEWAY_TOKEN`/`GATEWAY_CORE_URL` env vars.
6. Register products with `protocol: "custom"` (or add a new protocol value)
   and create devices. The core needs **no changes**.

### Sanity checks

```bash
# core is reachable
curl -s localhost:8080/healthz

# gateway registered and healthy
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/gateways | jq

# push a sample through the generic ingest path
curl -s -X POST localhost:8080/api/v1/ingest/sensor/sensor-a \
  -H "X-Device-Secret: $SECRET" -d '{"kind":"property","params":{"temperature":25.4}}'
```
