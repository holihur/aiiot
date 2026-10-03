// Command modbus-gateway runs the Modbus TCP gateway. Unlike the passive
// protocol gateways it is master-driven: it polls registers of configured
// PLCs on an interval and reports them as thing-model property uplinks.
//
// The device table comes from MODBUS_CONFIG_FILE (see internal/adapters/modbus
// for the JSON schema and an example in deploy/modbus.example.json).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aiiot/server/internal/adapters/modbus"
	"github.com/aiiot/server/internal/gateway"
)

const version = "1.0.0"

func main() {
	log := slog.New(slog.NewJSONHandler(gateway.LogWriter(), &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	hc := gateway.LoadHarnessConfig("modbus", "0.0.0.0:9104", version)
	hc.Logger = log

	adapter, err := modbus.New(modbus.Options{
		ConfigFile: gateway.GetStr("MODBUS_CONFIG_FILE", os.Getenv("MODBUS_CONFIG_FILE")),
		Logger:     log,
	})
	if err != nil {
		log.Error("modbus gateway init failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := gateway.Run(ctx, adapter, hc); err != nil {
		log.Error("modbus gateway exited", "error", err)
		os.Exit(1)
	}
}
