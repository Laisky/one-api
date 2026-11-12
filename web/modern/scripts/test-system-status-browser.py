#!/usr/bin/env python3
"""Exercise delayed real Axios responses, logout and React status races offline in Chromium."""
from pathlib import Path
import json
import shutil
import subprocess
import tempfile

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]


def complete(route, name):
    """complete fulfills an intercepted status request using the real backend envelope."""
    route.fulfill(json={"success": True, "data": {"system_name": name, "quota_per_unit": "1000"}})


def main():
    """main bundles real React consumers and tests delayed success through logout and forced refresh."""
    with tempfile.TemporaryDirectory(prefix="system-status-browser-") as temporary:
        bundle = Path(temporary) / "fixture.js"
        options = {"entryPoints": [str(ROOT / "scripts/fixtures/system-status.tsx")],
                   "absWorkingDir": str(ROOT), "alias": {"@": str(ROOT / "src")},
                   "bundle": True, "format": "iife", "platform": "browser",
                   "define": {"process.env.NODE_ENV": '"production"',
                              "__ONEAPI_BROWSER_RELEASE__": '"test"'},
                   "outfile": str(bundle)}
        subprocess.run([shutil.which("node"), "-e",
                        "require('esbuild').buildSync(" + json.dumps(options) + ")"], cwd=ROOT, check=True)
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(args=["--no-sandbox"])
            try:
                page = browser.new_page()
                pending = []
                page.route("http://status.test/", lambda route: route.fulfill(
                    content_type="text/html", body='<div id="root"></div>'))
                page.route("**/api/status*", lambda route: pending.append(route))
                page.goto("http://status.test/")
                page.add_script_tag(content=bundle.read_text())
                expect(page.locator("#loading")).to_have_text("true")
                page.wait_for_timeout(100)
                assert len(pending) == 1
                page.get_by_role("button", name="Logout").click()
                complete(pending.pop(0), "Stale before logout")
                expect(page.locator("#loading")).to_have_text("false")
                page.wait_for_timeout(100)
                expect(page.locator("#name")).to_have_text("")
                keys = ["status", "system_name", "logo", "footer_html", "chat_link",
                        "quota_per_unit", "display_in_currency"]
                assert page.evaluate("(keys) => keys.every(key => localStorage.getItem(key) === null)", keys)
                page.get_by_role("button", name="Refresh").click()
                page.wait_for_timeout(100)
                assert len(pending) == 1
                older = pending.pop(0)
                page.get_by_role("button", name="Refresh").click()
                page.wait_for_timeout(100)
                assert len(pending) == 1
                complete(pending.pop(0), "Latest")
                expect(page.locator("#name")).to_have_text("Latest")
                complete(older, "Older")
                page.wait_for_timeout(100)
                expect(page.locator("#name")).to_have_text("Latest")
                expect(page.locator("#loading")).to_have_text("false")
                assert page.evaluate("localStorage.getItem('system_name')") == "Latest"
                page.get_by_role("button", name="Logout").click()
                expect(page.locator("#name")).to_have_text("")
                assert page.evaluate("(keys) => keys.every(key => localStorage.getItem(key) === null)", keys)
                print(json.dumps({"engine": "chromium", "version": browser.version,
                                  "logoutDelayedResponse": "passed", "latestReactState": "passed",
                                  "mountedClear": "passed"}))
            finally:
                browser.close()


if __name__ == "__main__":
    main()
