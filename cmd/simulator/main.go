// Command simulator stands in for the mesh network during development: it POSTs
// randomized sensor readings to the gateway's /ingest endpoint on an interval,
// occasionally emitting out-of-range values so the AI service has anomalies to
// flag.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/3thanHead/iot_ai/internal/config"
	"github.com/3thanHead/iot_ai/internal/telemetry"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.LoadSimulator()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	log.Info("simulator starting",
		"target", cfg.TargetURL, "interval", cfg.Interval,
		"nodes", cfg.Nodes, "sensors", cfg.Sensors)

	for {
		select {
		case <-ctx.Done():
			log.Info("simulator stopping")
			return
		case <-ticker.C:
			reading := randomReading(cfg)
			if err := post(ctx, client, cfg.TargetURL, reading); err != nil {
				log.Warn("post failed", "error", err)
				continue
			}
			log.Info("sent", "node", reading.NodeID, "sensor", reading.Sensor, "value", reading.Value)
		}
	}
}

func randomReading(cfg config.Simulator) telemetry.SensorReading {
	node := fmt.Sprintf("node-%02d", rand.IntN(cfg.Nodes)+1)
	sensor := cfg.Sensors[rand.IntN(len(cfg.Sensors))]
	value, unit := sampleValue(sensor)
	return telemetry.SensorReading{
		Site:      cfg.Site,
		NodeID:    node,
		Sensor:    sensor,
		Value:     value,
		Unit:      unit,
		Timestamp: time.Now().UTC(),
	}
}

// sampleValue returns a plausible value for a sensor, with a 10% chance of an
// out-of-range "anomaly" so the downstream pipeline has something to catch.
func sampleValue(sensor string) (float64, string) {
	anomaly := rand.Float64() < 0.1
	switch sensor {
	case "temperature":
		if anomaly {
			return 60 + rand.Float64()*20, "C"
		}
		return 18 + rand.Float64()*10, "C"
	case "humidity":
		if anomaly {
			return 100 + rand.Float64()*20, "%"
		}
		return 30 + rand.Float64()*40, "%"
	case "vibration":
		if anomaly {
			return 6 + rand.Float64()*4, "g"
		}
		return rand.Float64() * 3, "g"
	default:
		return rand.Float64() * 100, ""
	}
}

func post(ctx context.Context, c *http.Client, url string, r telemetry.SensorReading) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
