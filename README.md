# iot_ai

A general-purpose **AI + IoT platform** for the home network — a monorepo of
independent apps that share a common, fault-tolerant LLM backend. Each app can be
enabled or disabled on its own; the camera vision pipeline is just the first one.

## Layout

```
apps/
  camera-vision/     ESP32 camera -> YOLO detection + VLM narration -> live browser view
  lead-gen/          (placeholder) LangChain lead generation, talks to the LLM cluster
infra/
  llm-cluster/       distributed Ollama (Jetson / MacBook / Windows) behind a HAProxy
                     load balancer on the Mini PC -> one always-up, load-balanced endpoint
tools/
  fleetctl/          deploy CLI: build + flash ESP32 firmware over USB, track versions
.env / .env.example  root: ESP32 Wi-Fi creds only (firmware build). Each app has its own .env.
```

## The apps

| App | What it is | Status |
|-----|-----------|--------|
| [**camera-vision**](apps/camera-vision/) | Wi-Fi camera → real-time object detection + on-device vision-language narration, served as an annotated live view. Runs on a laptop or a Jetson edge box (Docker). | working |
| [**lead-gen**](apps/lead-gen/) | LangChain pipelines for lead generation, using the shared LLM cluster. | placeholder |

## The shared infrastructure

[**infra/llm-cluster**](infra/llm-cluster/) — native **Ollama** on each AI machine
(each uses its own GPU/Metal), fronted by a **HAProxy** load balancer on the non-AI
**Mini PC**. The whole house gets **one** endpoint that is load-balanced and
auto-fails-over across nodes:

```
http://192.168.1.111:11434      # Ollama native API + OpenAI /v1 — used by any app
http://192.168.1.111:8404       # live health dashboard
```

| Node | IP | Accelerator |
|------|----|-------------|
| Jetson Orin Nano Super | 192.168.1.188 | CUDA |
| MacBook Air M1 | 192.168.1.99 | Metal |
| Windows PC | 192.168.1.222 | NVIDIA |
| Mini PC (master, load balancer) | 192.168.1.111 | — |

All nodes serve the same model (`llama3.2:3b`) so any node can answer any request.
See [infra/llm-cluster/README.md](infra/llm-cluster/) for setup and the failover test.

## CLI

One control tool, run from the repo root, that detects the OS (Linux / macOS /
Windows) and drives every app + the infrastructure. It's a stdlib-only Python
brain ([`tools/iotctl/`](tools/iotctl/)) behind native launchers — **`./iot`** on
Linux/macOS, **`.\iot.ps1`** on Windows. (Needs Python 3; on Windows:
`winget install Python.Python.3`.)

```bash
./iot doctor                 # check this machine: OS, docker, ollama, role
./iot list                   # discoverable apps/infra + the LLM nodes
./iot install-node           # set THIS machine up as an Ollama LLM node (OS-sensed)
./iot up camera-vision       # start an app   (docker compose up -d; add --build)
./iot down camera-vision     # stop it
./iot up all                 # start everything with a compose file
./iot status                 # running containers + machine role
./iot cluster                # live health of every LLM node + the load balancer
./iot model pull             # pull the cluster model on this node
./iot model set llama3.1:8b  # switch the WHOLE cluster: pull on every node + save it
./iot flash --board sunfounder --version 1.0.0   # passthrough to fleetctl (firmware)
```

`install-node` is the "install on anything" path: run it on each AI machine and it
runs the right native Ollama setup (systemd / launchd / Windows service), binds the
LAN, and pulls the model — no per-OS steps to remember.

### Enable / disable an app
Each app is self-contained, so just start/stop it by name:
```bash
./iot up camera-vision       # enable the camera
./iot down camera-vision     # disable it
./iot up llm-cluster         # bring up the HAProxy load balancer (on the Mini PC)
```

## Conventions

- **Per-app config:** each app/infra dir has its own `.env` (gitignored) + `.env.example`.
  The **root** `.env` holds only the ESP32 Wi-Fi creds that `fleetctl` injects into the
  firmware build.
- **One LLM endpoint:** apps point at the cluster (`http://192.168.1.111:11434`) rather
  than at any single machine, so inference is load-balanced and survives a node going down.
