#!/usr/bin/env python3
"""Drive the management-API surface over real HTTP and record the result (T19).

Covers every handler site in Appendix A of
docs/proposals/20260714_boundary-response-dtos.md (user/token/channel/redemption/
log) plus error paths, against a running server. Unlike the in-process Go harness
(controller/behavior_differential_test.go), this exercises the real router,
middleware, session auth and database, so it catches anything that lives outside
the handler layer.

Usage: journey.py <base_url> <out.json>
Requires the server's root account to still be the bootstrap root/123456.
"""

import sys

from lib import Client, dump, record


def run(client):
    """Execute the journey against one server.

    Parameters:
      - client: a lib.Client bound to the server under test.

    Return values: none; steps accumulate on the client.
    """
    call = client.call

    # U1 SetupLogin — the funnel shared by password login, all OAuth providers
    # and passkey. Error variants first so a failed login cannot clobber the
    # session established below.
    call("U1b_login_wrong_password", "POST", "/api/user/login",
         {"username": "root", "password": "wrong-password"})
    call("U1c_login_malformed", "POST", "/api/user/login", {"username": 12345})
    call("U1_login_root", "POST", "/api/user/login",
         {"username": "root", "password": "123456"})

    me = call("U5_get_self", "GET", "/api/user/self")
    root_uuid = ((me or {}).get("data") or {}).get("uuid")

    # Channel: C1-C4
    call("C0_add_channel", "POST", "/api/channel/", {
        "name": "e2e-channel", "type": 1, "key": "sk-e2e-fake-key",
        "base_url": "https://example.invalid", "models": "gpt-4o,gpt-4o-mini",
        "group": "default", "model_mapping": "", "priority": 0,
    })
    channels = call("C1_get_all_channels", "GET", "/api/channel/?p=0")
    call("C2_search_channels", "GET", "/api/channel/search?keyword=e2e")
    channel_uuid = _first_uuid(channels)
    if channel_uuid:
        call("C3_get_channel", "GET", f"/api/channel/{channel_uuid}")
        call("C4_update_channel", "PUT", "/api/channel/",
             {"uuid": channel_uuid, "name": "e2e-channel-renamed"})
    call("C3e_get_channel_bad_uuid", "GET", "/api/channel/does-not-exist")

    # Redemption: R1-R4
    call("R0_add_redemption", "POST", "/api/redemption/",
         {"name": "e2e-redemption", "quota": 100, "count": 1})
    redemptions = call("R1_get_all_redemptions", "GET", "/api/redemption/?p=0")
    call("R2_search_redemptions", "GET", "/api/redemption/search?keyword=e2e")
    redemption_uuid = _first_uuid(redemptions)
    if redemption_uuid:
        call("R3_get_redemption", "GET", f"/api/redemption/{redemption_uuid}")
        call("R4_update_redemption", "PUT", "/api/redemption/",
             {"uuid": redemption_uuid, "name": "e2e-redemption-renamed", "status": 1})
    call("R3e_get_redemption_bad_uuid", "GET", "/api/redemption/does-not-exist")

    # Token: T1-T9
    call("T4_add_token", "POST", "/api/token/",
         {"name": "e2e-token", "remain_quota": 500000, "expired_time": -1,
          "unlimited_quota": True})
    tokens = call("T1_get_all_tokens", "GET", "/api/token/?p=0")
    call("T2_search_tokens", "GET", "/api/token/search?keyword=e2e")
    token_uuid = _first_uuid(tokens)
    token_key = None
    try:
        token_key = tokens["data"][0]["key"]
    except Exception:  # noqa: BLE001
        pass
    if token_uuid:
        call("T3_get_token", "GET", f"/api/token/{token_uuid}")
        call("T6_update_token", "PUT", "/api/token/",
             {"uuid": token_uuid, "name": "e2e-token-renamed", "status": 1})
    call("T3e_get_token_bad_uuid", "GET", "/api/token/does-not-exist")
    call("T7_admin_get_all_tokens", "GET", "/api/admin/tokens/?p=0")
    call("T8_admin_search_tokens", "GET", "/api/admin/tokens/search?keyword=e2e")
    if token_uuid:
        call("T9_admin_get_token", "GET", f"/api/admin/tokens/{token_uuid}")

    # TokenAuth surface: T5 ConsumeToken (an API-client contract) and L3.
    if token_key:
        auth = {"Authorization": f"Bearer sk-{token_key}"}
        call("T5_consume_token", "POST", "/api/token/consume",
             {"add_used_quota": 10, "add_reason": "e2e"}, headers=auth)
        call("L3_get_token_logs", "GET", "/api/token/logs", headers=auth)
        call("X_get_self_by_token", "GET", "/api/user/get-by-token", headers=auth)
        call("T5e_consume_token_malformed", "POST", "/api/token/consume",
             {"add_used_quota": "not-a-number"}, headers=auth)
    call("T5e_consume_token_no_auth", "POST", "/api/token/consume", {"add_used_quota": 1})

    # User admin surface: U2-U4, U6
    call("U2_get_all_users", "GET", "/api/user/?p=0")
    call("U3_search_users", "GET", "/api/user/search?keyword=root")
    if root_uuid:
        call("U4_get_user", "GET", f"/api/user/{root_uuid}")
    call("U4e_get_user_bad_uuid", "GET", "/api/user/does-not-exist")

    call("U7_create_user", "POST", "/api/user/",
         {"username": "e2e-user", "password": "e2e-password-123",
          "display_name": "E2E User"})
    call("U7e_create_user_malformed", "POST", "/api/user/", {"username": 999})
    call("U7e_create_user_short_password", "POST", "/api/user/",
         {"username": "e2e-user2", "password": "x"})
    users = call("U2b_list_users", "GET", "/api/user/?p=0")
    target_uuid = None
    try:
        for user in users["data"]:
            if user.get("username") == "e2e-user":
                target_uuid = user["uuid"]
    except Exception:  # noqa: BLE001
        pass
    if target_uuid:
        call("U6_manage_user_disable", "POST", "/api/user/manage",
             {"uuid": target_uuid, "action": "disable"})
        call("U6b_manage_user_enable", "POST", "/api/user/manage",
             {"uuid": target_uuid, "action": "enable"})
    call("U6e_manage_user_bad_uuid", "POST", "/api/user/manage",
         {"uuid": "does-not-exist", "action": "disable"})

    # UpdateSelf, including the omitted-vs-empty display_name distinction.
    call("U8_update_self_display_name", "PUT", "/api/user/self",
         {"display_name": "Root Renamed"})
    call("U8b_update_self_omit_display_name", "PUT", "/api/user/self",
         {"username": "root"})
    call("U8c_update_self_clear_display_name", "PUT", "/api/user/self",
         {"display_name": ""})
    call("U8e_update_self_malformed", "PUT", "/api/user/self", {"display_name": 12345})

    # D3: legacy integer-id inbound refs must be refused identically on both
    # builds (strict-in has rejected them since 99c5ed01).
    call("D3_update_token_legacy_int_id", "PUT", "/api/token/",
         {"id": 1, "name": "legacy-int-id", "status": 1})

    # Log: L1-L5
    call("L1_get_all_logs", "GET", "/api/log/?p=0")
    call("L2_get_user_logs", "GET", "/api/log/self?p=0")
    call("L4_search_all_logs", "GET", "/api/log/search?keyword=")
    call("L5_search_user_logs", "GET", "/api/log/self/search?keyword=")

    # Unauthenticated / insufficient-role paths.
    client.logout()
    call("E_unauth_get_all_users", "GET", "/api/user/?p=0")
    call("E_unauth_get_all_channels", "GET", "/api/channel/?p=0")
    call("E_unauth_get_self", "GET", "/api/user/self")
    call("E_unauth_get_all_logs", "GET", "/api/log/?p=0")


def _first_uuid(listing):
    try:
        return listing["data"][0]["uuid"]
    except Exception:  # noqa: BLE001
        return None


def main():
    if len(sys.argv) != 3:
        print(__doc__)
        return 2
    client = Client(sys.argv[1])
    run(client)
    dump(record(client.steps), sys.argv[2])
    return 0


if __name__ == "__main__":
    sys.exit(main())
