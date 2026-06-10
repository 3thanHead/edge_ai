// Package gateway implements the mini-PC gateway: it ingests sensor readings
// over HTTP (from the mesh network, or the simulator), republishes them to the
// MQTT broker for the AI service to consume, and subscribes to the inference
// results that come back so they can be surfaced or acted on.
package gateway

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/3thanHead/iot_ai/internal/telemetry"
)

// Broker is the subset of the MQTT client the gateway needs. Defining it here
// (rather than depending on *mqttx.Client directly) keeps the gateway testable
// with a fake.
type Broker interface {
	PublishJSON(topic string, v any) error
	Subscribe(topic string, handler func(topic string, payload []byte)) error
}

// Gateway bridges HTTP ingest to MQTT and tracks the latest inference per
// site/node/sensor.
type Gateway struct {
	site   string
	broker Broker
	log    *slog.Logger

	mu     sync.RWMutex
	latest map[string]telemetry.Inference // key: site/node/sensor
}

// New constructs a Gateway. Call Start to begin consuming inference results.
func New(site string, broker Broker, log *slog.Logger) *Gateway {
	return &Gateway{
		site:   site,
		broker: broker,
		log:    log,
		latest: make(map[string]telemetry.Inference),
	}
}

// Start subscribes to the inference results published by the AI service.
func (g *Gateway) Start() error {
	return g.broker.Subscribe(telemetry.InferenceWildcard, g.onInference)
}

func (g *Gateway) onInference(topic string, payload []byte) {
	var inf telemetry.Inference
	if err := json.Unmarshal(payload, &inf); err != nil {
		g.log.Warn("bad inference payload", "topic", topic, "error", err)
		return
	}
	key := inf.Site + "/" + inf.NodeID + "/" + inf.Sensor
	g.mu.Lock()
	g.latest[key] = inf
	g.mu.Unlock()

	if inf.Label != "normal" {
		g.log.Warn("anomaly detected",
			"site", inf.Site, "node", inf.NodeID, "sensor", inf.Sensor,
			"value", inf.Value, "score", inf.Score, "model", inf.Model)
	}
}

// Routes returns the gateway's HTTP handler.
func (g *Gateway) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ingest", g.handleIngest)
	mux.HandleFunc("GET /inferences", g.handleInferences)
	mux.HandleFunc("GET /healthz", g.handleHealth)
	return mux
}

func (g *Gateway) handleIngest(w http.ResponseWriter, r *http.Request) {
	var reading telemetry.SensorReading
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&reading); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if reading.Site == "" {
		reading.Site = g.site
	}
	if reading.Timestamp.IsZero() {
		reading.Timestamp = time.Now().UTC()
	}
	if err := validate(reading); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	topic := telemetry.TelemetryTopic(reading.Site, reading.NodeID)
	if err := g.broker.PublishJSON(topic, reading); err != nil {
		g.log.Error("publish failed", "topic", topic, "error", err)
		http.Error(w, "publish failed", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "topic": topic})
}

func (g *Gateway) handleInferences(w http.ResponseWriter, _ *http.Request) {
	g.mu.RLock()
	out := make([]telemetry.Inference, 0, len(g.latest))
	for _, v := range g.latest {
		out = append(out, v)
	}
	g.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (g *Gateway) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func validate(r telemetry.SensorReading) error {
	if r.NodeID == "" {
		return errors.New("node_id is required")
	}
	if r.Sensor == "" {
		return errors.New("sensor is required")
	}
	return nil
}
