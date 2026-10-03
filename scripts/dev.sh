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

# start launches a process with LOG_FILE pointed at the .run directory so the
# application writes through lumberjack: size/age rotation, pruning, gzip.
start() { # name command...
  local name="$1"; shift
  if [ -f "$RUN_DIR/$name.pid" ] && kill -0 "$(cat "$RUN_DIR/$name.pid")" 2>/dev/null; then
    echo "$name already running (pid $(cat "$RUN_DIR/$name.pid"))"
    return
  fi
  env LOG_FILE="$RUN_DIR/$name.log" \
    "$@" > "$RUN_DIR/$name.console.log" 2>&1 &
  echo $! > "$RUN_DIR/$name.pid"
  echo "started $name (pid $!) -> $RUN_DIR/$name.log (rotating)"
}

stop() { # name
  local name="$1"
  local pid=""
  if [ -f "$RUN_DIR/$name.pid" ]; then
    pid="$(cat "$RUN_DIR/$name.pid")"
    rm -f "$RUN_DIR/$name.pid"
  fi
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
  else
    # fallback: kill leftover processes by their executable name only (a
    # pkill -f pattern would match this script's own command line too)
    safe_pids=$(ps -eo pid=,comm= | awk -v n="$name" '$2 == n {print $1}')
    [ -n "$safe_pids" ] && kill $safe_pids 2>/dev/null || true
  fi
  echo "stopped $name"
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
    MQTT_PORT=1883
    if ss -tln 2>/dev/null | grep -q ':1883 '; then
      echo "note: :1883 taken (system mosquitto?) -> mqtt-gateway on :11883"
      MQTT_PORT=11883
    fi
    start mqtt-gateway   env GATEWAY_CORE_URL=http://127.0.0.1:8080 MQTT_TCP_ADDR="0.0.0.0:$MQTT_PORT" ./bin/mqtt-gateway
    start coap-gateway   env GATEWAY_CORE_URL=http://127.0.0.1:8080 ./bin/coap-gateway
    start custom-gateway env GATEWAY_CORE_URL=http://127.0.0.1:8080 ./bin/custom-gateway
    if [ -n "${MODBUS_CONFIG_FILE:-}" ]; then
      start modbus-gateway env GATEWAY_CORE_URL=http://127.0.0.1:8080 MODBUS_CONFIG_FILE="$MODBUS_CONFIG_FILE" ./bin/modbus-gateway
    else
      echo "modbus-gateway skipped (set MODBUS_CONFIG_FILE to enable)"
    fi
    ;;
  stop)
    stop modbus-gateway; stop mqtt-gateway; stop coap-gateway; stop custom-gateway; stop core; stop nats
    ;;
  restart)
    "$0" stop; sleep 1; "$0" start
    ;;
  *)
    echo "usage: $0 {start|stop|restart}" >&2
    exit 1
    ;;
esac
