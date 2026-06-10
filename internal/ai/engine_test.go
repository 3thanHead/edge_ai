package ai

import (
	"testing"

	"github.com/3thanHead/iot_ai/internal/telemetry"
)

func TestThresholdEngineInfer(t *testing.T) {
	e := NewThresholdEngine("test")

	cases := []struct {
		name      string
		sensor    string
		value     float64
		wantLabel string
	}{
		{"normal temperature", "temperature", 22, "normal"},
		{"hot temperature", "temperature", 80, "anomaly"},
		{"cold temperature", "temperature", -40, "anomaly"},
		{"normal vibration", "vibration", 2, "normal"},
		{"high vibration", "vibration", 9, "anomaly"},
		{"unknown sensor", "pressure", 9999, "normal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := e.Infer(telemetry.SensorReading{Sensor: tc.sensor, Value: tc.value})
			if got.Label != tc.wantLabel {
				t.Errorf("label = %q, want %q", got.Label, tc.wantLabel)
			}
			if got.Model != "test" {
				t.Errorf("model = %q, want %q", got.Model, "test")
			}
			if tc.wantLabel == "anomaly" && got.Score <= 0 {
				t.Errorf("anomaly score = %v, want > 0", got.Score)
			}
			if got.Score < 0 || got.Score > 1 {
				t.Errorf("score = %v, want within [0,1]", got.Score)
			}
		})
	}
}
