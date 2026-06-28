#!/usr/bin/env bash
# Set up the MacBook Air M1 (192.168.1.12) as an Ollama LLM node.
# Ollama on macOS uses the Apple Silicon GPU (Metal) automatically.  Usage: bash setup.sh
set -euo pipefail

MODEL="${LLM_MODEL:-llama3.2:3b}"

if ! command -v ollama >/dev/null 2>&1; then
  echo "==> Installing Ollama…"
  if command -v brew >/dev/null 2>&1; then
    brew install --cask ollama
  else
    echo "Homebrew not found. Install the app from https://ollama.com/download/mac, then re-run."
    exit 1
  fi
fi

echo "==> Exposing Ollama on the LAN (bind 0.0.0.0)…"
# Make it stick across logins, then restart any running server so it takes effect.
launchctl setenv OLLAMA_HOST "0.0.0.0:11434"
osascript -e 'quit app "Ollama"' >/dev/null 2>&1 || true
sleep 1
OLLAMA_HOST="0.0.0.0:11434" nohup ollama serve >/tmp/ollama.log 2>&1 &
sleep 2

echo "==> Pulling the shared model: ${MODEL}…"
ollama pull "${MODEL}"

echo "==> Self-check:"
curl -s http://localhost:11434/api/tags | head -c 300; echo
cat <<'NOTE'
Done.
Note: if you normally launch Ollama from the menubar app, set OLLAMA_HOST=0.0.0.0:11434
in its environment (or rely on the launchctl setenv above) so it stays LAN-reachable
after a reboot. Verify from another machine:  curl http://192.168.1.12:11434/api/tags
NOTE
