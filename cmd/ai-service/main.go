// Command ai-service runs the inference service, intended to run on the Jetson
// Nano or a MacBook. It subscribes to telemetry from the broker, runs each
// reading through the inference engine, and publishes the result back.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/3thanHead/iot_ai/internal/ai"
	"github.com/3thanHead/iot_ai/internal/config"
	"github.com/3thanHead/iot_ai/internal/mqttx"
	"github.com/3thanHead/iot_ai/internal/telemetry"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.LoadAIService()

	engine := ai.NewThresholdEngine(cfg.ModelName)

	mq, err := mqttx.Connect(cfg.MQTT, log.With("component", "mqtt"))
	if err != nil {
		log.Error("mqtt connect", "error", err)
		os.Exit(1)
	}
	defer mq.Disconnect()

	err = mq.Subscribe(telemetry.TelemetryWildcard, func(topic string, payload []byte) {
		var reading telemetry.SensorReading
		if err := json.Unmarshal(payload, &reading); err != nil {
			log.Warn("bad telemetry payload", "topic", topic, "error", err)
			return
		}
		inf := engine.Infer(reading)
		outTopic := telemetry.InferenceTopic(reading.Site, reading.NodeID)
		if err := mq.PublishJSON(outTopic, inf); err != nil {
			log.Error("publish inference", "topic", outTopic, "error", err)
			return
		}
		log.Info("inferred",
			"node", reading.NodeID, "sensor", reading.Sensor,
			"value", reading.Value, "label", inf.Label, "score", inf.Score)
	})
	if err != nil {
		log.Error("subscribe to telemetry", "error", err)
		os.Exit(1)
	}

	// Minimal health endpoint so orchestrators can probe liveness.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		log.Info("ai-service listening", "addr", cfg.HTTPAddr, "model", cfg.ModelName)
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
