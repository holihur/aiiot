// Command mqtt-gateway runs the standalone MQTT access gateway: an embedded
// MQTT broker that authenticates devices against the core, enforces
// workspace-scoped topic ACLs and forwards uplinks to the core.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/adapters/mqtt"
	"github.com/aiiot/server/internal/gateway"
)

const version = "1.0.0"

func main() {
	log := slog.New(slog.NewJSONHandler(gateway.LogWriter(), &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	hc := gateway.LoadHarnessConfig(access.ProtocolMQTTName, "0.0.0.0:9101", version)
	hc.Logger = log

	adapter := mqtt.New(mqtt.Options{
		InstanceID: hc.InstanceID,
		TCPAddr:    gateway.GetStr("MQTT_TCP_ADDR", "0.0.0.0:1883"),
		WSAddr:     gateway.GetStr("MQTT_WS_ADDR", "0.0.0.0:8083"),
		EnableWS:   gateway.GetBool("MQTT_ENABLE_WS", false),
		TLSAddr:    gateway.GetStr("MQTT_TLS_ADDR", ""),
		TLSCert:    gateway.GetStr("MQTT_TLS_CERT", ""),
		TLSKey:     gateway.GetStr("MQTT_TLS_KEY", ""),
		TLSCAFile:  gateway.GetStr("MQTT_TLS_CA_FILE", ""),
		Logger:     log,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := gateway.Run(ctx, adapter, hc); err != nil {
		log.Error("mqtt gateway exited", "error", err)
		os.Exit(1)
	}
}
