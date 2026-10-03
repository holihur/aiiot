# Security review (2026-10)

Scope: backend (Go/gorm/gin), gateways (mqtt/coap/custom/modbus/opcua), admin
console, SDK surface. Findings verified against the codebase at commit
`30fcb2c` + the fixes in this review.

## 1. Findings fixed in this review

| # | Severity | Issue | Fix |
|---|---|---|---|
| 1 | Medium | Device CSV import had no upload size cap (unbounded disk fill via multipart) | 1 MiB cap → HTTP 413 (`devices_csv.go`) |
| 2 | Low | Gateway internal token compared with `!=` (non constant-time) | `crypto/subtle.ConstantTimeCompare` (`gateway.go`) |
| 3 | Medium | Device direct-HTTP ingest (`/api/v1/ingest/...`) unlimited after auth (abuse/flood surface) | per-device rate limit 600/min on the route |
| 4 | Medium (fixed earlier) | `totp/status` was unauthenticated and TOTP write ops missed `Audit` | middleware order fixed (AdminAuth + Audit before TOTP routes) |

## 2. Verified-good areas

- **Credentials**: bcrypt (`DefaultCost`) for passwords; device secrets compared
  constant-time; JWT is HS256 with an explicit `SigningMethodHMAC` guard and TTL;
  `config.Validate()` refuses to start in `production` with default/short
  `JWT_SECRET` / `GATEWAY_TOKEN`.
- **Device auth**: X.509 client certificates (issued per device, CN-bound, one
  live cert, revocation list `device_certs`); CoAP DTLS-PSK (secret-proven at
  handshake, channel encryption; app-layer auth still enforced).
- **Injection**: all SQL is parameterized (`Raw(..., args)`, GORM conditions); no
  `os/exec`, no string-built queries from user input; CEL conditions are
  server-side expressions compiled + bounded (webhook `bodyExpr` same).
- **Uploads**: firmware upload capped (`OTA.MaxUploadBytes` → 413).
- **Rate limiting**: login 30/min, register 10/min, direct ingest 600/min per
  device.
- **Auditing**: non-GET admin/business writes recorded in `audit_logs`
  (middleware); admin console routes auth + audit enforced.
- **Secrets at rest**: TOTP secrets AES-GCM sealed (platform-derived key); device
  private keys returned once and never persisted; gateway PSK fetched via
  token-protected internal endpoint.
- **Transport defaults**: no `InsecureSkipVerify` anywhere; mTLS/DTLS usable;
  CORS is an explicit allow-list (production: no credentialed cross-origin by
  default).

## 3. Recommendations (not blocking; team decision)

1. **Dependency scanning is not wired up**: the repo has no
   `package-lock.json` (npm audit fails) and no `govulncheck` in CI.
   → Commit a lockfile, add `npm audit --omit=dev` and
   `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` to CI.
2. **SSRF surface**: rule/notification webhooks and Modbus/OPC-UA endpoints are
   administrator-configured arbitrary URLs. Fine while admins are trusted; if
   untrusted operators exist, add egress allow-listing / proxy.
3. **Scale**: rate limiters are process-local maps; multi-replica needs a shared
   store (Redis) when scaling cores.
4. **Token lifecycle**: JWTs are now revocable — every user/admin row carries a
   `token_version` embedded in the token, and the auth middleware rejects
   tokens whose version is stale (checked through a short-TTL, bounded
   in-process cache). Password change and "sign out everywhere" bump the
   version. There is still no refresh token; the mitigation is the short TTL
   plus re-login. Add a refresh flow if long-lived sessions are required.
5. **TLS termination** is out of process (reverse proxy / gateway TLS). Ensure
   the public endpoint is HTTPS (Caddy present on this host).
6. Audit `client_ip` uses `X-Forwarded-For`; configure the proxy to set/trust it
   explicitly.

## 4. Runtime posture on this host

- Public entry: `caddy` reverse proxy (Caddyfile backed up before edits); core on
  `:8080`; caddy `:8080` block disabled to free the port — confirm intended
  public mapping before exposing.
- NATS/PostgreSQL are local; production should use managed/credentialed
  instances with TLS.