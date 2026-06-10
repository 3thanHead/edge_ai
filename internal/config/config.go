// Package config loads service configuration from environment variables so the
// same binary can run unchanged on a laptop, in Docker, or on a Jetson. Every
// value has a sensible default; nothing is required to boot a local stack.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// MQTT holds the broker connection settings shared by all services.
type MQTT struct {
	BrokerURL    string        // e.g. tcp://localhost:1883
	ClientID     string        // must be unique per connection
	Username     string        // optional
	Password     string        // optional
	QoS          byte          // 0, 1, or 2
	KeepAlive    time.Duration // ping interval
	ConnectRetry time.Duration // delay between connection attempts
}

// Gateway is the config for the mini-PC gateway service.
type Gateway struct {
	HTTPAddr string // address the ingest/API server listens on
	Site     string // default site stamped onto readings that omit one
	MQTT     MQTT
}

// AIService is the config for the Jetson/MacBook AI service.
type AIService struct {
	HTTPAddr  string // address the health server listens on
	ModelName string // identifier reported on every inference
	MQTT      MQTT
}

// Simulator is the config for the sensor simulator that stands in for the mesh.
type Simulator struct {
	TargetURL string        // gateway ingest endpoint to POST readings to
	Site      string        // site stamped onto generated readings
	Nodes     int           // number of distinct mesh nodes to simulate
	Sensors   []string      // sensor types to cycle through
	Interval  time.Duration // delay between readings
}

// LoadGateway builds the gateway config from the environment.
func LoadGateway() Gateway {
	return Gateway{
		HTTPAddr: getenv("GATEWAY_HTTP_ADDR", ":8080"),
		Site:     getenv("SITE", "default"),
		MQTT:     loadMQTT("iot-gateway"),
	}
}

// LoadAIService builds the AI service config from the environment.
func LoadAIService() AIService {
	return AIService{
		HTTPAddr:  getenv("AI_HTTP_ADDR", ":8081"),
		ModelName: getenv("AI_MODEL", "threshold-v0"),
		MQTT:      loadMQTT("iot-ai-service"),
	}
}

// LoadSimulator builds the simulator config from the environment.
func LoadSimulator() Simulator {
	return Simulator{
		TargetURL: getenv("SIM_TARGET_URL", "http://localhost:8080/ingest"),
		Site:      getenv("SIM_SITE", "default"),
		Nodes:     getenvInt("SIM_NODES", 3),
		Sensors:   splitCSV(getenv("SIM_SENSORS", "temperature,humidity,vibration")),
		Interval:  getenvDuration("SIM_INTERVAL", 2*time.Second),
	}
}

func loadMQTT(defaultClientID string) MQTT {
	// Append the PID so two instances of the same service don't fight over a
	// client ID (the broker kicks the older connection when IDs collide).
	clientID := getenv("MQTT_CLIENT_ID", fmt.Sprintf("%s-%d", defaultClientID, os.Getpid()))
	return MQTT{
		BrokerURL:    getenv("MQTT_BROKER_URL", "tcp://localhost:1883"),
		ClientID:     clientID,
		Username:     getenv("MQTT_USERNAME", ""),
		Password:     getenv("MQTT_PASSWORD", ""),
		QoS:          byte(getenvInt("MQTT_QOS", 1)),
		KeepAlive:    getenvDuration("MQTT_KEEPALIVE", 30*time.Second),
		ConnectRetry: getenvDuration("MQTT_CONNECT_RETRY", 5*time.Second),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
