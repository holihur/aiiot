# API & SDKs

## OpenAPI 3.0

The full machine-readable contract is served by the core:

```
GET /api/v1/openapi.yaml
```

It also lives in source form at [`backend/internal/api/openapi.yaml`](../backend/internal/api/openapi.yaml)
and covers authentication, projects/workspaces, products and thing models,
devices (CRUD, bulk import/export, certificates), telemetry (latest, range,
downsampled, multi-device compare, CSV export), rules, alerts and notification
channels. Every path uses `Authorization: Bearer <token>` from
`POST /api/v1/auth/login` (returns `{token, expiresAt}`).

### Common conventions

- Path ids: `/projects/{id}`, `/products/{id}`, `/devices/{id}` — use the
  numeric id returned by list/create responses.
- Time ranges: `from` / `to` accept RFC3339 (`2026-10-01T00:00:00Z`).
- Interval syntax is Go: `1m`, `5m`, `1h`, `24h`. Intervals `>= 1h` are served
  from the hourly rollup (`source: "rollup"`), smaller ones from the raw
  partitioned table (`source: "raw"`).
- Identifiers: `GET /devices/{id}/telemetry` accepts one or comma-separated
  identifiers (`identifier=temperature,humidity`); the compare endpoint takes
  arrays.
- Errors: non-2xx responses carry `{"error": "..."}`; rate limiting returns
  429; validation failures return 400 with the field error text.
- Device secrets are shown **once** at creation; certificates return the
  private key **once** at issuance.

## SDKs

Generated from the OpenAPI contract (maintained in this repo):

### Python (`sdk/python/aiiot.py`)

Zero dependencies (standard library only).

```python
from aiiot import Client

c = Client("http://localhost:8080")   # or your deployment URL
c.login("admin", "your-password")

projects = c.projects()
devices  = c.devices(projects[0]["id"])

# live values + downsampled series
latest = c.latest(devices[0]["id"])
points = c.telemetry(devices[0]["id"], latest[0]["identifier"], interval="5m")

# multi-device comparison
series = c.compare([devices[0]["id"]], ["temperature", "humidity"], interval="1h")

# CSV export
c.export_telemetry_csv(devices[0]["id"], ["temperature"], "out.csv", interval="1h")

# device certificates (X.509, mTLS over MQTT)
mat = c.issue_certificate(devices[0]["id"])   # {certPem, keyPem, caPem, serial}
# ... hand mat["keyPem"] to the device (shown once) ...
c.revoke_certificate(devices[0]["id"])
```

### TypeScript (`sdk/ts/aiiot.ts`)

Single-file, dependency-free (browser `fetch` or Node 18+).

```ts
import { AiiotClient } from "./aiiot";

const aiiot = new AiiotClient("http://localhost:8080");
await aiiot.login("admin", "your-password");

const devices = await aiiot.devices(1);
const rows = await aiiot.telemetry(devices[0].id, "temperature", "5m");
const series = await aiiot.compare([devices[0].id], ["temperature"], "1h");
```

## Device protocols

Devices do not talk REST: they connect through the protocol gateways
(MQTT/CoAP/Modbus/custom TCP) using their `key`+`secret` (or an issued
X.509 client certificate over TLS). The generated connection snippets in the
UI's *Connect* dialog are the fastest way to onboard a device; the SDKs above
are for the *platform* (management) API.

## Postman / clients

Import `GET /api/v1/openapi.yaml` into Postman/Insomnia/CodeGen for
exploration or to generate clients in other languages. The base URL is
`/api/v1` and auth is a bearer token from `/auth/login`.