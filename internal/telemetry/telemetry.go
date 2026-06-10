// Package telemetry defines the message types and MQTT topic scheme shared by
// every service in the system. Keeping these in one place means the gateway and
// the AI service can never disagree about wire format or topic names.
package telemetry

import (
	"fmt"
	"time"
)

// SensorReading is a single measurement produced by a mesh node and ingested by
// the gateway. The gateway publishes one of these per reading to the broker.
type SensorReading struct {
	Site      string    `json:"site"`           // logical grouping, e.g. "home", "lab-a"
	NodeID    string    `json:"node_id"`        // mesh node identifier
	Sensor    string    `json:"sensor"`         // e.g. "temperature", "humidity", "vibration"
	Value     float64   `json:"value"`          // measured value
	Unit      string    `json:"unit,omitempty"` // optional unit, e.g. "C", "%"
	Timestamp time.Time `json:"timestamp"`      // when the reading was taken (UTC)
}

// Inference is the AI service's verdict for a single reading. The AI service
// publishes one of these per reading it processes; the gateway subscribes to
// them so it can act on (or just surface) the results.
type Inference struct {
	Site      string    `json:"site"`
	NodeID    string    `json:"node_id"`
	Sensor    string    `json:"sensor"`
	Value     float64   `json:"value"`
	Label     string    `json:"label"`     // e.g. "normal", "anomaly"
	Score     float64   `json:"score"`     // 0..1, higher = more anomalous / more confident
	Model     string    `json:"model"`     // which model/version produced this
	Timestamp time.Time `json:"timestamp"` // event time of the source reading
}

// Topic scheme. Everything lives under a single prefix so a broker can be shared
// with other systems. The {site}/{node} segments let you subscribe narrowly
// (one node) or broadly (all nodes) with wildcards.
const (
	// TopicPrefix is the root of every topic this system uses.
	TopicPrefix = "iot"

	// TelemetryWildcard matches telemetry from every node at every site.
	TelemetryWildcard = TopicPrefix + "/+/+/telemetry"
	// InferenceWildcard matches inference results from every node at every site.
	InferenceWildcard = TopicPrefix + "/+/+/inference"
)

// TelemetryTopic returns the topic a reading from the given site/node is
// published to, e.g. "iot/home/node-01/telemetry".
func TelemetryTopic(site, nodeID string) string {
	return fmt.Sprintf("%s/%s/%s/telemetry", TopicPrefix, site, nodeID)
}

// InferenceTopic returns the topic an inference result for the given site/node
// is published to, e.g. "iot/home/node-01/inference".
func InferenceTopic(site, nodeID string) string {
	return fmt.Sprintf("%s/%s/%s/inference", TopicPrefix, site, nodeID)
}
