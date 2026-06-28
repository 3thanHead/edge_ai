#!/usr/bin/env bash
# Set up a Jetson (Linux/systemd) as an Ollama LLM node for the cluster.
# Run on the Jetson at 192.168.1.11.  Usage:  bash setup.sh
set -euo pipefail

MODEL="${LLM_MODEL:-llama3.2:3b}"

echo "==> Installing Ollama (native arm64 + Jetson GPU)…"
curl -fsSL https://ollama.com/install.sh | sh

echo "==> Exposing Ollama on the LAN (bind 0.0.0.0) via a systemd override…"
sudo mkdir -p /etc/systemd/system/ollama.service.d
sudo tee /etc/systemd/system/ollama.service.d/override.conf >/dev/null <<'EOF'
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
EOF
sudo systemctl daemon-reload
sudo systemctl restart ollama

echo "==> Pulling the shared model: ${MODEL}…"
ollama pull "${MODEL}"

echo "==> Self-check:"
curl -s http://localhost:11434/api/tags | head -c 300; echo
echo "Done. This node should now appear UP at http://192.168.1.10:8404"
