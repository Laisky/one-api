"""Synthetic backend for the opt-in Gateway API cluster regression suite."""

import base64
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading
import time

STATE = {"posts": 0, "cancelled": 0}
LOCK = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    """Handler serves synthetic HTTP, SSE and WebSocket traffic without providers."""

    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        """log_message suppresses request data in the synthetic fixture's logs."""

    def do_POST(self):
        """do_POST counts one admitted synthetic request and returns its count."""
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        with LOCK:
            STATE["posts"] += 1
        self.do_GET()

    def do_GET(self):
        """do_GET returns fixture state or exercises streaming and upgrade transport."""
        if self.path == "/ws":
            key = self.headers["Sec-WebSocket-Key"]
            digest = hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode(), usedforsecurity=False).digest()
            self.send_response(101)
            self.send_header("Upgrade", "websocket")
            self.send_header("Connection", "Upgrade")
            self.send_header("Sec-WebSocket-Accept", base64.b64encode(digest).decode())
            self.end_headers()
            self.wfile.write(b"\x81\x02ok")
            self.wfile.flush()
            self.close_connection = True
            return
        if self.path == "/sse":
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Connection", "close")
            self.end_headers()
            try:
                for index in range(200):
                    self.wfile.write(f"data: {index}\n\n".encode())
                    self.wfile.flush()
                    time.sleep(0.05)
            except (BrokenPipeError, ConnectionResetError):
                with LOCK:
                    STATE["cancelled"] += 1
            self.close_connection = True
            return
        with LOCK:
            body = json.dumps(STATE).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 3000), Handler).serve_forever()
