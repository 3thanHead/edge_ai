// Package ai contains the inference engine the AI service runs on each reading.
// The Engine interface is the seam where you plug in something real — a model
// running on the Jetson GPU, an ONNX runtime, a call to a local LLM, etc. The
// ThresholdEngine here is a deliberately trivial placeholder so the end-to-end
// pipeline works on day one.
package ai

import (
	"math"

	"github.com/3thanHead/iot_ai/internal/telemetry"
)

// Engine turns a sensor reading into an inference.
type Engine interface {
	// Name identifies the model/version; it is recorded on every Inference.
	Name() string
	// Infer is a pure function of the reading: same input, same output.
	Infer(r telemetry.SensorReading) telemetry.Inference
}

// Range is an inclusive expected range for a sensor's value.
type Range struct{ Min, Max float64 }

// ThresholdEngine flags readings that fall outside a per-sensor expected range
// as anomalies. Replace it with a real model by implementing Engine.
type ThresholdEngine struct {
	name   string
	ranges map[string]Range
}

// NewThresholdEngine returns a ThresholdEngine seeded with reasonable default
// ranges for the demo sensor types.
func NewThresholdEngine(name string) *ThresholdEngine {
	return &ThresholdEngine{
		name: name,
		ranges: map[string]Range{
			"temperature": {Min: -10, Max: 45},
			"humidity":    {Min: 0, Max: 100},
			"vibration":   {Min: 0, Max: 5},
		},
	}
}

// Name implements Engine.
func (e *ThresholdEngine) Name() string { return e.name }

// Infer implements Engine. Readings for unknown sensors are always "normal".
func (e *ThresholdEngine) Infer(r telemetry.SensorReading) telemetry.Inference {
	label, score := "normal", 0.0
	if rng, ok := e.ranges[r.Sensor]; ok {
		switch {
		case r.Value < rng.Min:
			label, score = "anomaly", normalizedDistance(rng.Min-r.Value, rng)
		case r.Value > rng.Max:
			label, score = "anomaly", normalizedDistance(r.Value-rng.Max, rng)
		}
	}
	return telemetry.Inference{
		Site:      r.Site,
		NodeID:    r.NodeID,
		Sensor:    r.Sensor,
		Value:     r.Value,
		Label:     label,
		Score:     score,
		Model:     e.name,
		Timestamp: r.Timestamp,
	}
}

// normalizedDistance maps how far a value is outside its range onto 0..1,
// relative to the width of the range.
func normalizedDistance(d float64, rng Range) float64 {
	span := rng.Max - rng.Min
	if span <= 0 {
		return 1
	}
	return math.Min(1, d/span)
}
