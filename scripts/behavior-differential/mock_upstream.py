#!/usr/bin/env python3
"""A deterministic OpenAI-compatible upstream for the relay differential.

Returns a fixed chat-completion with a fixed usage block, so the quota deducted
by the relay is identical on every run and on every build under comparison. A
real provider would make the billing numbers non-deterministic and the
comparison meaningless.

Usage: mock_upstream.py <port>
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

RESPONSE = {
    "id": "chatcmpl-mock-0001",
    "object": "chat.completion",
    "created": 1700000000,
    "model": "gpt-4o-mini",
    "choices": [{
        "index": 0,
        "message": {"role": "assistant", "content": "mock response"},
        "finish_reason": "stop",
    }],
    "usage": {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
}


class Handler(BaseHTTPRequestHandler):
    """Answers every relayed request with RESPONSE."""

    def _send(self, payload):
        body = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):  # noqa: N802 - BaseHTTPRequestHandler API
        self.rfile.read(int(self.headers.get("Content-Length") or 0))
        self._send(RESPONSE)

    def do_GET(self):  # noqa: N802 - serves /v1/models probes
        self._send({"object": "list", "data": [{"id": "gpt-4o-mini"}]})

    def log_message(self, *args):
        """Silence per-request stderr noise."""
        return


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", int(sys.argv[1])), Handler).serve_forever()
