#!/usr/bin/env python3
"""chat — a minimal streaming chatbot UI on top of the home LLM cluster.

Serves a single-page chat frontend and streams tokens from Ollama back to the
browser over a WebSocket as they are generated (like Claude Code's live output).
Models are listed straight from the cluster, so the dropdown always reflects
whatever is actually loaded across the Jetson / MacBook / Windows nodes.

    uvicorn app.main:app --host 0.0.0.0 --port 8800
"""
import json
import os
from pathlib import Path

import httpx
from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles

# Cluster endpoint (HAProxy on the Mini PC -> whichever Ollama node is up).
# Native Ollama API: /api/tags to list models, /api/chat to stream a chat.
OLLAMA_URL = os.environ.get("LLM_BASE_URL", "http://192.168.1.111:11434").rstrip("/")

STATIC_DIR = Path(__file__).parent / "static"

app = FastAPI(title="cluster-chat")


@app.get("/api/models")
async def list_models():
    """Return the model names available on the cluster."""
    async with httpx.AsyncClient(timeout=10) as client:
        resp = await client.get(f"{OLLAMA_URL}/api/tags")
        resp.raise_for_status()
        data = resp.json()
    models = sorted(m["name"] for m in data.get("models", []))
    return {"models": models}


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
