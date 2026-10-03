// Command custom-gateway runs the reference custom-protocol gateway. It speaks
// newline-delimited JSON over TCP and demonstrates how to add a new protocol
// by implementing access.Adapter and running it under gateway.Run.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/adapters/customtcp"
	"github.com/aiiot/server/internal/gateway"
)

const version = "1.0.0"

func main() {
	log := slog.New(slog.NewJSONHandler(gateway.LogWriter(), &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	hc := gateway.LoadHarnessConfig(access.ProtocolCustomName, "0.0.0.0:9103", version)
	hc.Logger = log

	adapter := customtcp.New(customtcp.Options{
		Addr:   gateway.GetStr("CUSTOM_TCP_ADDR", "0.0.0.0:9000"),
		Logger: log,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := gateway.Run(ctx, adapter, hc); err != nil {
		log.Error("custom gateway exited", "error", err)
		os.Exit(1)
	}
}
