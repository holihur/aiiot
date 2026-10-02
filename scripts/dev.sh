# Local development helpers
# Builds the frontend into backend/web/dist and starts the core + all gateways.

set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND="$ROOT/backend"
RUN_DIR="$BACKEND/.run"
mkdir -p "$RUN_DIR"

# Export shared settings (GATEWAY_TOKEN, etc.) so gateways match the core.
if [ -f "$BACKEND/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$BACKEND/.env"
  set +a
fi

start() { # name command...
  local name="$1"; shift
  if [ -f "$RUN_DIR/$name.pid" ] && kill -0 "$(cat "$RUN_DIR/$name.pid")" 2>/dev/null; then
    echo "$name already running (pid $(cat "$RUN_DIR/$name.pid"))"
    return
  fi
  "$@" > "$RUN_DIR/$name.log" 2>&1 &
  echo $! > "$RUN_DIR/$name.pid"
  echo "started $name (pid $!) -> $RUN_DIR/$name.log"
}

stop() { # name
  local name="$1"
  if [ -f "$RUN_DIR/$name.pid" ]; then
    kill "$(cat "$RUN_DIR/$name.pid")" 2>/dev/null || true
    rm -f "$RUN_DIR/$name.pid"
    echo "stopped $name"
  fi
}

case "${1:-start}" in
  start)
    cd "$BACKEND"
    # NATS is mandatory; reuse an already-running instance, otherwise start one.
    if systemctl is-active --quiet nats-server 2>/dev/null; then
      echo "nats already running (systemd)"
    elif ss -tln 2>/dev/null | grep -q ':4222 '; then
      echo "nats already running (port 4222)"
    elif command -v nats-server >/dev/null 2>&1; then
      start nats nats-server -js
    else
      echo "WARNING: nats-server not found — start NATS with JetStream (-js) on ${NATS_URL:-nats://127.0.0.1:4222} before the core/gateways." >&2
    fi
    start core          ./bin/core
    sleep 1
    start mqtt-gateway   env GATEWAY_CORE_URL=http://127.0.0.1:8080 ./bin/mqtt-gateway
    start coap-gateway   env GATEWAY_CORE_URL=http://127.0.0.1:8080 ./bin/coap-gateway
    start custom-gateway env GATEWAY_CORE_URL=http://127.0.0.1:8080 ./bin/custom-gateway
    ;;
  stop)
    stop mqtt-gateway; stop coap-gateway; stop custom-gateway; stop core; stop nats
    ;;
  restart)
    "$0" stop; sleep 1; "$0" start
    ;;
  *)
    echo "usage: $0 {start|stop|restart}" >&2
    exit 1
    ;;
esac
