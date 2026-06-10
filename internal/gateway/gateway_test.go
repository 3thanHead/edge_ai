package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/3thanHead/iot_ai/internal/telemetry"
)

// fakeBroker records publishes and lets tests drive subscribe handlers directly.
type fakeBroker struct {
	mu        sync.Mutex
	published []published
	handlers  map[string]func(string, []byte)
}

type published struct {
	topic string
	value any
}

func newFakeBroker() *fakeBroker {
	return &fakeBroker{handlers: map[string]func(string, []byte){}}
}

func (f *fakeBroker) PublishJSON(topic string, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, published{topic, v})
	return nil
}

func (f *fakeBroker) Subscribe(topic string, handler func(string, []byte)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[topic] = handler
	return nil
}

func newTestGateway() (*Gateway, *fakeBroker) {
	fb := newFakeBroker()
	g := New("test-site", fb, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return g, fb
}

func TestIngestPublishesTelemetry(t *testing.T) {
	g, fb := newTestGateway()

	body, _ := json.Marshal(telemetry.SensorReading{
		NodeID: "node-01", Sensor: "temperature", Value: 21.5,
	})
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	g.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body)
	}
	if len(fb.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(fb.published))
	}
	if want := "iot/test-site/node-01/telemetry"; fb.published[0].topic != want {
		t.Errorf("topic = %q, want %q", fb.published[0].topic, want)
	}
}

func TestIngestRejectsMissingNodeID(t *testing.T) {
	g, fb := newTestGateway()

	body, _ := json.Marshal(telemetry.SensorReading{Sensor: "temperature", Value: 1})
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	g.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if len(fb.published) != 0 {
		t.Errorf("published %d messages, want 0", len(fb.published))
	}
}

func TestInferenceResultsSurfaced(t *testing.T) {
	g, fb := newTestGateway()
	if err := g.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	handler := fb.handlers[telemetry.InferenceWildcard]
	if handler == nil {
		t.Fatal("gateway did not subscribe to inference topic")
	}

	inf := telemetry.Inference{
		Site: "test-site", NodeID: "node-01", Sensor: "temperature",
		Value: 80, Label: "anomaly", Score: 0.5, Model: "test",
	}
	payload, _ := json.Marshal(inf)
	handler(telemetry.InferenceTopic(inf.Site, inf.NodeID), payload)

	req := httptest.NewRequest(http.MethodGet, "/inferences", nil)
	rec := httptest.NewRecorder()
	g.Routes().ServeHTTP(rec, req)

	var got []telemetry.Inference
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Label != "anomaly" {
		t.Fatalf("inferences = %+v, want one anomaly", got)
	}
}
