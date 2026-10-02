# AI IoT Platform — build & run helpers
#
# `make build` builds the frontend into backend/web/dist and compiles all Go
# binaries into backend/bin. NATS (with JetStream) is a mandatory component:
# start it with `make run-nats`, then `make run` for the core and
# `make run-mqtt` / `make run-coap` / `make run-custom` in separate terminals.

BACKEND := backend
BIN     := $(BACKEND)/bin

.PHONY: all build frontend backend clean \
        run run-nats run-mqtt run-coap run-custom run-all \
        tidy test

all: build

## Build everything ---------------------------------------------------------

build: frontend backend

frontend:
	cd frontend && pnpm install && pnpm build

backend:
	cd $(BACKEND) && go build -o bin/core           ./cmd/core
	cd $(BACKEND) && go build -o bin/mqtt-gateway   ./cmd/mqtt-gateway
	cd $(BACKEND) && go build -o bin/coap-gateway   ./cmd/coap-gateway
	cd $(BACKEND) && go build -o bin/custom-gateway ./cmd/custom-gateway

tidy:
	cd $(BACKEND) && go mod tidy

## Run (from backend/ so .env and web/dist resolve) -------------------------

run-nats:
	nats-server -js

run:
	cd $(BACKEND) && ./bin/core

run-mqtt:
	cd $(BACKEND) && GATEWAY_CORE_URL=$${GATEWAY_CORE_URL:-http://127.0.0.1:8080} ./bin/mqtt-gateway

run-coap:
	cd $(BACKEND) && GATEWAY_CORE_URL=$${GATEWAY_CORE_URL:-http://127.0.0.1:8080} ./bin/coap-gateway

run-custom:
	cd $(BACKEND) && GATEWAY_CORE_URL=$${GATEWAY_CORE_URL:-http://127.0.0.1:8080} ./bin/custom-gateway

clean:
	rm -rf $(BIN) $(BACKEND)/web/dist frontend/dist
