"""Bounded, credential-free local pprof capture for diagnostic-only runs."""
from __future__ import annotations
import hashlib
import ipaddress
import os
from pathlib import Path
import time
import urllib.parse
import urllib.request

MAX_CAPTURE_BYTES = 128 << 20


class NoRedirect(urllib.request.HTTPRedirectHandler):
    """NoRedirect prevents a local profiling endpoint from redirecting capture to a different service."""

    def redirect_request(self, request, fp, code, message, headers, new_url):
        """redirect_request rejects redirects rather than changing the verified fixture destination."""
        return None


def capture(url: str, path: Path, timeout: float, maximum_bytes: int = MAX_CAPTURE_BYTES) -> dict:
    """capture atomically saves a size/time-bounded loopback profile and returns its identity and capture interval.

    On failure, the .partial file remains visibly incomplete; it is never renamed
    to a completed profile. Existing evidence is never overwritten.
    """
    parsed = urllib.parse.urlsplit(url)
    try:
        loopback = ipaddress.ip_address(parsed.hostname or '').is_loopback
    except ValueError:
        loopback = False
    if parsed.scheme != 'http' or not loopback or parsed.username is not None or parsed.password is not None or parsed.fragment:
        raise ValueError('profile capture requires an unauthenticated literal loopback HTTP URL')
    if not 0 < timeout <= 600 or type(maximum_bytes) is not int or not 0 < maximum_bytes <= MAX_CAPTURE_BYTES:
        raise ValueError('invalid capture bounds')
    if path.exists():
        raise FileExistsError('refusing to overwrite a completed profile')
    partial = path.with_suffix(path.suffix + '.partial')
    started = time.monotonic()
    total, digest = 0, hashlib.sha256()
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with partial.open('xb') as output, opener.open(url, timeout=timeout) as response:
        while True:
            if time.monotonic() - started > timeout:
                raise TimeoutError('bounded profile capture exceeded its deadline')
            block = response.read(min(65536, maximum_bytes - total + 1))
            if not block:
                break
            if total + len(block) > maximum_bytes:
                raise ValueError('profile exceeded the capture byte limit')
            output.write(block)
            digest.update(block)
            total += len(block)
    if not total:
        raise ValueError('profile response was empty')
    finished = time.monotonic()
    os.link(partial, path)  # Publish atomically without ever replacing another writer's result.
    partial.unlink()
    return {'file': path.name, 'bytes': total, 'sha256': digest.hexdigest(),
            'started_monotonic': started, 'finished_monotonic': finished,
            'wall_seconds': finished - started, 'maximum_bytes': maximum_bytes}
