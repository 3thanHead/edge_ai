# iot_ai

A small Go starting point for an edge IoT + AI pipeline. A mesh network reports
to an **IoT gateway** (mini PC), which forwards readings over an MQTT broker to
an **AI service** running on a Jetson Nano or a MacBook Air (M1). The AI service
runs inference and publishes results back to the gateway.

```
  mesh nodes                mini PC                         Jetson Nano / MacBook Air
 ┌───────────┐   HTTP    ┌───────────────┐   MQTT pub    ┌────────────────────────┐
 │ sensors   │ ───POST──▶│   gateway     │ ──telemetry──▶│      ai-service        │
 │ (simulator│  /ingest  │ (this repo)   │               │  (inference engine)    │
 │  in dev)  │           │               │◀──inference───│                        │
 └───────────┘           └───────┬───────┘   MQTT sub    └────────────┬───────────┘
                                 │                                     │
                                 └──────────► MQTT broker ◀────────────┘
                                              (Mosquitto)
```

Everything talks through the broker, so each service can live on its own
machine on the WiFi network — just point them at the same `MQTT_BROKER_URL`.

## Services

| Service       | Runs on            | What it does                                                        |
| ------------- | ------------------ | ------------------------------------------------------------------ |
| `gateway`     | mini PC            | HTTP `/ingest` → publishes telemetry to MQTT; tracks inferences    |
| `ai-service`  | Jetson / MacBook   | Subscribes to telemetry, runs inference, publishes results         |
| `simulator`   | dev box            | Stands in for the mesh: POSTs randomized readings to the gateway   |

## Layout

```
cmd/                 service entrypoints (one binary each)
  gateway/           the mini-PC gateway
  ai-service/        the inference service
  simulator/         the mesh stand-in
internal/
  config/            env-based configuration
  telemetry/         shared message types + MQTT topic scheme
  mqttx/             thin MQTT client wrapper
  gateway/           gateway HTTP + MQTT logic (testable)
  ai/                inference Engine interface + threshold placeholder
deploy/
  Dockerfile         one parameterized multi-stage build for all services
  docker-compose.yml local broker + services + optional simulator
  mosquitto/         broker config
.github/workflows/   CI: fmt, vet, test, cross-build, docker build
```

## Quickstart

### Option A — Docker (simulate the whole system on one machine)

> Requires Docker with Compose v2.

```bash
make docker-sim          # broker + gateway + ai-service + simulator
# or, without the simulator:
make docker-up
```

Watch it work:

```bash
curl localhost:8080/inferences        # latest inference per node/sensor
curl localhost:8080/healthz           # gateway health
```

Tear down:

```bash
make docker-down
```

### Option B — run locally with Go

Go is at `/usr/local/go` on this machine but may not be on your `PATH`:

```bash
export PATH="$PATH:/usr/local/go/bin"
```

Start a broker (via Docker), then run each service in its own terminal:

```bash
docker run --rm -p 1883:1883 eclipse-mosquitto:2   # terminal 1
make run-ai-service                                # terminal 2
make run-gateway                                   # terminal 3
make run-simulator                                 # terminal 4 (or send your own readings)
```

Send a reading by hand:

```bash
curl -X POST localhost:8080/ingest -H 'content-type: application/json' -d '{
  "node_id": "node-01",
  "sensor": "temperature",
  "value": 82.0
}'
```

## Configuration

All config is environment variables with defaults — see [`.env.example`](.env.example).
Key ones:

| Variable            | Default                        | Used by      |
| ------------------- | ------------------------------ | ------------ |
| `MQTT_BROKER_URL`   | `tcp://localhost:1883`         | all          |
| `GATEWAY_HTTP_ADDR` | `:8080`                        | gateway      |
| `SITE`              | `default`                      | gateway      |
| `AI_HTTP_ADDR`      | `:8081`                        | ai-service   |
| `AI_MODEL`          | `threshold-v0`                 | ai-service   |
| `SIM_TARGET_URL`    | `http://localhost:8080/ingest` | simulator    |
| `SIM_INTERVAL`      | `2s`                           | simulator    |

## MQTT topics

```
iot/{site}/{node}/telemetry    sensor readings (gateway → ai-service)
iot/{site}/{node}/inference    inference results (ai-service → gateway)
```

Subscribe to everything while debugging:

```bash
docker exec -it iot-broker mosquitto_sub -t 'iot/#' -v
```

## Deploying across machines

Build once in CI (the `cross-build` job produces `linux/arm64`, `darwin/arm64`,
and `linux/amd64` binaries), or cross-compile locally:

```bash
# AI service for the Jetson Nano (arm64 Linux)
GOOS=linux  GOARCH=arm64 CGO_ENABLED=0 go build -o ai-service ./cmd/ai-service

# AI service for the MacBook Air M1
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o ai-service ./cmd/ai-service
```

Run the broker + gateway on the mini PC, then start the AI service on the
Jetson/MacBook pointing at the mini PC:

```bash
MQTT_BROKER_URL=tcp://<mini-pc-ip>:1883 ./ai-service
```

## Where to take it next

- **Real inference**: implement `ai.Engine` (e.g. ONNX Runtime / a model on the
  Jetson GPU, or a call to a local model) and swap it in `cmd/ai-service`.
- **Mesh ingestion**: replace the simulator/HTTP `/ingest` with your real mesh
  protocol (e.g. a serial bridge or LoRa/Zigbee gateway) on the mini PC.
- **Security**: add a Mosquitto password file + TLS before exposing the broker
  beyond localhost; the current config allows anonymous access for dev only.
- **Persistence**: have the gateway or AI service write telemetry/inferences to
  a time-series DB.

## License

MIT — see [LICENSE](LICENSE).
