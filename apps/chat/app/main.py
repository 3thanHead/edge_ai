#!/usr/bin/env python3
"""chat — a minimal streaming chatbot UI on top of the home LLM cluster.

Serves a single-page chat frontend and streams tokens from Ollama back to the
browser over a WebSocket as they are generated (like Claude Code's live output).
Models are listed straight from the cluster, so the dropdown always reflects
whatever is actually loaded across the Jetson / MacBook / Windows nodes.

    uvicorn app.main:app --host 0.0.0.0 --port 8800
"""
import asyncio
import json
import os
from pathlib import Path

import httpx
from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles

# Cluster endpoint (HAProxy on the Mini PC -> whichever Ollama node is up).
# Native Ollama API: /api/chat to stream a chat — this is the single, load-balanced,
# fault-tolerant, power-routed endpoint that ALL generation goes through.
OLLAMA_URL = os.environ.get("LLM_BASE_URL", "http://192.168.1.111:11434").rstrip("/")

# Model DISCOVERY is different from generation: /api/tags through the LB only reflects the
# one node it routes to, so the dropdown would miss models that live on other nodes. When
# the deploy injects CLUSTER_NODES (comma-separated node URLs, from fleet.json), list the
# UNION of models across nodes. Falls back to the LB endpoint alone for local/dev runs.
CLUSTER_NODES = [u.strip().rstrip("/") for u in os.environ.get("CLUSTER_NODES", "").split(",") if u.strip()]

STATIC_DIR = Path(__file__).parent / "static"

app = FastAPI(title="cluster-chat")


async def _node_models(client, url):
    try:
        resp = await client.get(f"{url}/api/tags")
        resp.raise_for_status()
        return [m["name"] for m in resp.json().get("models", [])]
    except httpx.HTTPError:
        return []  # a down node just contributes nothing to the union


@app.get("/api/models")
async def list_models():
    """Return the union of model names available across the cluster's nodes."""
    sources = CLUSTER_NODES or [OLLAMA_URL]
    async with httpx.AsyncClient(timeout=5) as client:
        per_node = await asyncio.gather(*(_node_models(client, u) for u in sources))
    return {"models": sorted({name for names in per_node for name in names})}


@app.websocket("/ws/chat")
async def chat(ws: WebSocket):
    """Stream a chat completion token-by-token to the browser.

    Client sends: {"model": "...", "messages": [{"role", "content"}, ...]}
    Server sends: {"type": "token", "content": "..."} repeatedly,
                  then {"type": "done"} (or {"type": "error", "message": "..."}).
    """
    await ws.accept()
    try:
        while True:
            req = await ws.receive_json()
            model = req.get("model")
            messages = req.get("messages", [])
            if not model or not messages:
                await ws.send_json({"type": "error", "message": "model and messages are required"})
                continue

            payload = {"model": model, "messages": messages, "stream": True}
            try:
                async with httpx.AsyncClient(timeout=None) as client:
                    async with client.stream("POST", f"{OLLAMA_URL}/api/chat", json=payload) as resp:
                        resp.raise_for_status()
                        async for line in resp.aiter_lines():
                            if not line.strip():
                                continue
                            chunk = json.loads(line)
                            token = chunk.get("message", {}).get("content", "")
                            if token:
                                await ws.send_json({"type": "token", "content": token})
                            if chunk.get("done"):
                                break
                await ws.send_json({"type": "done"})
            except httpx.HTTPError as exc:
                await ws.send_json({"type": "error", "message": f"cluster error: {exc}"})
    except WebSocketDisconnect:
        pass


# Serve the frontend. Mounted last so the API routes above take precedence.
app.mount("/static", StaticFiles(directory=STATIC_DIR), name="static")


@app.get("/")
async def index():
    return FileResponse(STATIC_DIR / "index.html")
