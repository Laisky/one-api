"""Shared plumbing for the live behavior-differential gate (T19/T20).

The gate answers one question: does build A behave identically to build B, over
real HTTP, for the same journey? It is deliberately black-box — it knows nothing
about Go types, only about requests and responses — so it can compare two
binaries built from different commits.

See README.md for how the pieces fit together.
"""

import json
import re
import urllib.error
import urllib.request

# Keys whose concrete values legitimately differ between two independently seeded
# servers (generated identifiers, generated credentials, wall-clock stamps). Only
# their PRESENCE and type are compared; every other scalar is compared by value,
# so a genuine contract change still surfaces as a diff.
DYNAMIC_KEYS = {
    "uuid", "user_uuid", "channel_uuid", "token_uuid", "redemption_uuid",
    "log_uuid", "key", "access_token", "created_time", "accessed_time",
    "expired_time", "request_time", "created_at", "updated_at", "used_time",
    "test_time", "balance_updated_time", "message", "trace_id", "request_id",
    "transaction_id", "id", "elapsed_time", "elapsed_time_ms", "expires_at",
    "confirmed_at", "canceled_at",
    # aff_code is a per-user random invite code, regenerated on every creation.
    "aff_code",
}

# Generated credential formats (redemption keys, raw token keys). Normalized by
# format, never by "looks vaguely random", so the blast radius stays small.
_HEX32 = re.compile(r"^[0-9a-f]{32}$")
_UUIDV = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")

# Secret keys that must never appear in any response, on either build.
SECRET_KEYS = ('"password"', '"access_token"', '"totp_secret"', '"verification_code"')


class Client:
    """A cookie-aware JSON HTTP client that records every exchange.

    one-api issues its session cookie with the Secure attribute, so a stock
    cookie jar refuses to store it over plain HTTP (dev servers have no TLS).
    The server does not require Secure inbound, so the session value is captured
    from Set-Cookie and replayed in an explicit Cookie header. Without this the
    whole journey silently runs unauthenticated and every step 401s — which
    compares "equal" between two builds while proving nothing.
    """

    def __init__(self, base_url):
        """Create a client bound to one server.

        Parameters:
          - base_url: server root, e.g. http://127.0.0.1:3000
        """
        self.base = base_url.rstrip("/")
        self.session = None
        self.steps = []

    def _capture_session(self, headers):
        for raw in headers.get_all("Set-Cookie") or []:
            for part in raw.split(";"):
                part = part.strip()
                if part.startswith("session="):
                    self.session = part[len("session="):]

    def call(self, name, method, path, body=None, headers=None):
        """Issue one request and record (name, status, parsed body).

        Parameters:
          - name: stable step identifier; it is the key the differ compares on.
          - method: HTTP verb.
          - path: path beginning with '/'.
          - body: JSON-serializable request body, or None.
          - headers: extra headers; an Authorization header suppresses the cookie.

        Return values:
          - dict | None: the parsed JSON body, so callers can chain on it.
        """
        url = self.base + path
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.session and not (headers or {}).get("Authorization"):
            req.add_header("Cookie", f"session={self.session}")
        for key, value in (headers or {}).items():
            req.add_header(key, value)

        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                status, raw = resp.status, resp.read().decode("utf-8", "replace")
                self._capture_session(resp.headers)
        except urllib.error.HTTPError as err:
            status, raw = err.code, err.read().decode("utf-8", "replace")
            self._capture_session(err.headers)
        except Exception as err:  # noqa: BLE001 - transport failures are data too
            self.steps.append({"name": name, "status": -1, "error": type(err).__name__})
            return None

        try:
            parsed = json.loads(raw)
        except Exception:  # noqa: BLE001
            parsed = {"__raw__": raw[:200]}
        self.steps.append({"name": name, "status": status, "body": parsed})
        return parsed

    def logout(self):
        """Drop the session so subsequent steps exercise unauthenticated paths."""
        self.session = None


def shape(node, path=""):
    """Reduce a JSON document to a sorted list of "key path = value-or-kind".

    Dynamic values collapse to their type name; everything else keeps its literal
    value. Lists contribute their length plus the shape of their first element.

    Parameters:
      - node: parsed JSON value.
      - path: accumulated key path prefix (internal).

    Return values:
      - list[str]: sorted entries, stable across runs of the same build.
    """
    out = []
    if isinstance(node, dict):
        for key in sorted(node.keys()):
            out.extend(shape(node[key], f"{path}.{key}"))
    elif isinstance(node, list):
        out.append(f"{path}[]=len:{len(node)}")
        if node:
            out.extend(shape(node[0], f"{path}[0]"))
    else:
        leaf = path.rsplit(".", 1)[-1].rstrip("]").split("[")[0]
        if leaf in DYNAMIC_KEYS:
            out.append(f"{path}=<{type(node).__name__}>")
        elif isinstance(node, str) and (_HEX32.match(node) or _UUIDV.match(node)):
            out.append(f"{path}=<generated>")
        else:
            out.append(f"{path}={json.dumps(node)}")
    return sorted(out)


def record(steps, extra=None):
    """Build the comparable recording from a client's captured steps.

    Parameters:
      - steps: the Client.steps list.
      - extra: optional dict merged into the recording (e.g. quota_delta).

    Return values:
      - dict: {"steps": {...}, "secret_leaks": [...], **extra}
    """
    out, leaks = {}, []
    for step in steps:
        entry = {"status": step["status"]}
        if step.get("body") is not None:
            entry["shape"] = shape(step["body"])
            blob = json.dumps(step["body"])
            for secret in SECRET_KEYS:
                if secret in blob:
                    leaks.append(f'{step["name"]}: {secret}')
        if "error" in step:
            entry["error"] = step["error"]
        out[step["name"]] = entry
    result = {"steps": out, "secret_leaks": leaks}
    result.update(extra or {})
    return result


def dump(result, path):
    """Write a recording to disk deterministically and print a one-line summary."""
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(result, handle, indent=2, sort_keys=True)
    ok = sum(1 for s in result["steps"].values() if s["status"] == 200)
    print(f"recorded {len(result['steps'])} steps ({ok} x 200) -> {path}; "
          f"secret_leaks={len(result['secret_leaks'])}")
