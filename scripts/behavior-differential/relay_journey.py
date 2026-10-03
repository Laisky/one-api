#!/usr/bin/env python3
"""Drive relay -> billing -> log end to end and record the result (T19/T20).

Points a channel at mock_upstream.py, issues a real /v1/chat/completions request
through the relay, and reads back the resulting log rows and the quota actually
deducted. This is the leg that proves the refactor did not disturb the billing
hot path, rather than arguing it from static reasoning.

Usage: relay_journey.py <base_url> <mock_upstream_url> <out.json>
"""

import sys

from lib import Client, dump, record


def run(client, mock_url):
    """Execute the relay journey against one server.

    Parameters:
      - client: a lib.Client bound to the server under test.
      - mock_url: base URL of mock_upstream.py.

    Return values:
      - int | None: quota consumed by the relay request, or None if unmeasurable.
    """
    call = client.call
    call("login", "POST", "/api/user/login", {"username": "root", "password": "123456"})

    # Delete pre-existing channels first. Channel selection among equal-priority
    # candidates serving the same model is random, so a leftover channel pointing
    # at an unreachable host turns the relay outcome into a coin flip and the
    # comparison into noise.
    existing = call("relay_list_existing_channels", "GET", "/api/channel/?p=0")
    for channel in ((existing or {}).get("data") or []):
        if channel.get("uuid"):
            call(f'relay_delete_channel_{channel.get("name")}', "DELETE",
                 f'/api/channel/{channel["uuid"]}')

    call("relay_add_channel", "POST", "/api/channel/", {
        "name": "relay-mock-channel", "type": 1, "key": "sk-mock-upstream",
        "base_url": mock_url, "models": "gpt-4o-mini", "group": "default",
        "priority": 0,
    })
    call("relay_add_token", "POST", "/api/token/", {
        "name": "relay-token", "remain_quota": 500000, "expired_time": -1,
        "unlimited_quota": False,
    })
    tokens = call("relay_list_tokens", "GET", "/api/token/?p=0")
    key = None
    try:
        key = tokens["data"][0]["key"]
    except Exception:  # noqa: BLE001
        pass

    before = call("quota_before", "GET", "/api/user/self")
    if key:
        auth = {"Authorization": f"Bearer sk-{key}"}
        call("relay_chat_completion", "POST", "/v1/chat/completions", {
            "model": "gpt-4o-mini",
            "messages": [{"role": "user", "content": "hello"}],
        }, headers=auth)
        call("relay_token_logs", "GET", "/api/token/logs", headers=auth)
    after = call("quota_after", "GET", "/api/user/self")

    call("relay_self_logs", "GET", "/api/log/self?p=0")
    call("relay_all_logs", "GET", "/api/log/?p=0")

    try:
        return before["data"]["quota"] - after["data"]["quota"]
    except Exception:  # noqa: BLE001
        return None


def main():
    if len(sys.argv) != 4:
        print(__doc__)
        return 2
    client = Client(sys.argv[1])
    delta = run(client, sys.argv[2].rstrip("/"))
    result = record(client.steps, {"quota_delta": delta})
    dump(result, sys.argv[3])
    print(f"  billing: quota_delta={delta}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
