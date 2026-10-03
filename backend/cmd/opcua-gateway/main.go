// Command opcua-gateway runs the OPC-UA gateway. It is master-driven: it
// dials configured OPC-UA servers and polls mapped node values as
// thing-model property uplinks.
//
// Device table: OPCUA_CONFIG_FILE (JSON, see internal/adapters/opcua and
// deploy/opcua.example.json).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aiiot/server/internal/adapters/opcua"
	"github.com/aiiot/server/internal/gateway"
)

const version = "1.0.0"

func main() {
	log := slog.New(slog.NewJSONHandler(gateway.LogWriter(), &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	hc := gateway.LoadHarnessConfig("opcua", "0.0.0.0:9105", version)
	hc.Logger = log

	adapter, err := opcua.New(opcua.Options{
		ConfigFile: gateway.GetStr("OPCUA_CONFIG_FILE", ""),
		Logger:     log,
	})
	if err != nil {
		log.Error("opcua gateway init failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := gateway.Run(ctx, adapter, hc); err != nil {
		log.Error("opcua gateway exited", "error", err)
		os.Exit(1)
	}
}
