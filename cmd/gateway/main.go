// Command gateway runs the IoT gateway service, intended to run on the mini PC.
// It ingests sensor readings over HTTP, republishes them to MQTT for the AI
// service, and subscribes to inference results coming back.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/3thanHead/iot_ai/internal/config"
	"github.com/3thanHead/iot_ai/internal/gateway"
	"github.com/3thanHead/iot_ai/internal/mqttx"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.LoadGateway()

	mq, err := mqttx.Connect(cfg.MQTT, log.With("component", "mqtt"))
	if err != nil {
		log.Error("mqtt connect", "error", err)
		os.Exit(1)
	}
	defer mq.Disconnect()

	gw := gateway.New(cfg.Site, mq, log.With("component", "gateway"))
	if err := gw.Start(); err != nil {
		log.Error("subscribe to inferences", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           gw.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("gateway listening", "addr", cfg.HTTPAddr, "site", cfg.Site)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "error", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
