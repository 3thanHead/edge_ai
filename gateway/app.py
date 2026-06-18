#!/usr/bin/env python3
"""iot_ai gateway web service.

Serves the camera viewer and relays the ESP32's MJPEG stream, so the browser
only ever talks to the gateway -- whatever host that is (WSL now, mini PC or
Jetson later). This proxy is also where AI overlay (YOLO -> LLaVA) will slot in
later: intercept frames in stream() before they go to the browser.

    ESP32_HOST=192.168.1.164 python app.py     # then open http://localhost:8000
"""
import os
import time

import requests
from flask import Flask, Response, jsonify, send_from_directory

ESP32_HOST = os.environ.get("ESP32_HOST", "192.168.1.164")
PORT = int(os.environ.get("GATEWAY_PORT", "8000"))
STATIC_DIR = os.path.join(os.path.dirname(__file__), "static")

# Must match PART_BOUNDARY in the firmware so the browser parses the relayed
# multipart stream across upstream reconnects.
BOUNDARY = "iot_ai_frame"

app = Flask(__name__, static_folder=None)


@app.route("/")
def index():
    return send_from_directory(STATIC_DIR, "index.html")


def mjpeg_relay():
    """Relay the ESP32's MJPEG stream, reconnecting on any upstream blip.

    The browser sees one continuous multipart stream; if the camera's stream
    ends or the network hiccups, we transparently reopen it and keep yielding
    frames. The shared boundary means concatenated parts parse seamlessly.
    """
    while True:
        try:
            upstream = requests.get(
                f"http://{ESP32_HOST}/stream", stream=True, timeout=10
            )
            for chunk in upstream.iter_content(chunk_size=8192):
                yield chunk
        except GeneratorExit:
            return  # browser disconnected
        except requests.RequestException:
            time.sleep(0.5)  # camera blip -- pause, then reconnect


@app.route("/stream")
def stream():
    return Response(
        mjpeg_relay(),
        content_type=f"multipart/x-mixed-replace;boundary={BOUNDARY}",
    )


@app.route("/snapshot")
def snapshot():
    upstream = requests.get(f"http://{ESP32_HOST}/snapshot", timeout=10)
    return Response(
        upstream.content,
        content_type=upstream.headers.get("Content-Type", "image/jpeg"),
    )


@app.route("/config")
def config():
    return jsonify(esp32_host=ESP32_HOST)


if __name__ == "__main__":
    print(f"[gateway] ESP32_HOST={ESP32_HOST}  serving http://0.0.0.0:{PORT}")
    # threaded so one streaming client doesn't block the page or others.
    app.run(host="0.0.0.0", port=PORT, threaded=True)
