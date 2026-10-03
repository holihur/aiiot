// Command coap-gateway runs the standalone CoAP access gateway. Devices POST
// to workspace-scoped CoAP resources; the gateway authenticates them against
// the core and forwards normalized uplinks.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/adapters/coap"
	"github.com/aiiot/server/internal/gateway"
)

const version = "1.0.0"

func main() {
	log := slog.New(slog.NewJSONHandler(gateway.LogWriter(), &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	hc := gateway.LoadHarnessConfig(access.ProtocolCoAPName, "0.0.0.0:9102", version)
	hc.Logger = log

	adapter := coap.New(coap.Options{
		UDPAddr:      gateway.GetStr("COAP_UDP_ADDR", "0.0.0.0:5683"),
		TCPAddr:      gateway.GetStr("COAP_TCP_ADDR", ""),
		DTLSPSKAddr:  gateway.GetStr("COAP_DTLS_PSK_ADDR", ""),
		CoreURL:      gateway.GetStr("GATEWAY_CORE_URL", "http://127.0.0.1:8080"),
		GatewayToken: gateway.GetStr("GATEWAY_TOKEN", ""),
		Logger:       log,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := gateway.Run(ctx, adapter, hc); err != nil {
		log.Error("coap gateway exited", "error", err)
		os.Exit(1)
	}
}
