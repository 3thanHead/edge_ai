// Package mqttx is a thin wrapper around the Paho MQTT client that exposes just
// the operations our services need (publish JSON, subscribe with a simple
// callback) and handles connect/reconnect concerns in one place.
package mqttx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/3thanHead/iot_ai/internal/config"
)

// initialConnectTimeout bounds how long Connect waits for the first successful
// connection before giving up. With ConnectRetry enabled the client keeps trying
// in the background, so we fail fast here and let the orchestrator restart us.
const initialConnectTimeout = 30 * time.Second

// Client is a connected MQTT client. It is safe for concurrent use.
type Client struct {
	c   mqtt.Client
	qos byte
	log *slog.Logger
}

// Connect dials the broker and returns once connected (or errors out after
// initialConnectTimeout). Auto-reconnect is enabled, so transient drops after
// the initial connection are handled transparently.
func Connect(cfg config.MQTT, log *slog.Logger) (*Client, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.BrokerURL).
		SetClientID(cfg.ClientID).
		SetKeepAlive(cfg.KeepAlive).
		SetConnectTimeout(10 * time.Second).
		SetConnectRetry(true).
		SetConnectRetryInterval(cfg.ConnectRetry).
		SetAutoReconnect(true).
		SetCleanSession(true)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	opts.OnConnect = func(_ mqtt.Client) {
		log.Info("connected", "broker", cfg.BrokerURL, "client_id", cfg.ClientID)
	}
	opts.OnConnectionLost = func(_ mqtt.Client, err error) {
		log.Warn("connection lost", "error", err)
	}

	c := mqtt.NewClient(opts)
	tok := c.Connect()
	if !tok.WaitTimeout(initialConnectTimeout) {
		c.Disconnect(0)
		return nil, fmt.Errorf("mqtt: could not connect to %s within %s", cfg.BrokerURL, initialConnectTimeout)
	}
	if err := tok.Error(); err != nil {
		return nil, fmt.Errorf("mqtt connect: %w", err)
	}

	return &Client{c: c, qos: cfg.QoS, log: log}, nil
}

// Publish sends a raw payload to a topic and waits for the broker handoff.
func (c *Client) Publish(topic string, payload []byte) error {
	tok := c.c.Publish(topic, c.qos, false, payload)
	tok.Wait()
	return tok.Error()
}

// PublishJSON marshals v and publishes it to a topic.
func (c *Client) PublishJSON(topic string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return c.Publish(topic, b)
}

// Subscribe registers handler for messages on topic (which may contain
// wildcards). Subscriptions are restored automatically on reconnect.
func (c *Client) Subscribe(topic string, handler func(topic string, payload []byte)) error {
	tok := c.c.Subscribe(topic, c.qos, func(_ mqtt.Client, m mqtt.Message) {
		handler(m.Topic(), m.Payload())
	})
	tok.Wait()
	if err := tok.Error(); err != nil {
		return fmt.Errorf("subscribe %q: %w", topic, err)
	}
	c.log.Info("subscribed", "topic", topic)
	return nil
}

// Disconnect cleanly closes the connection, waiting up to 250ms for in-flight
// work to drain.
func (c *Client) Disconnect() {
	c.c.Disconnect(250)
}
