#!/usr/bin/env python3
"""Check real form label lookup, click focus, forwarded refs and changing ids offline in Chromium.

Use uv run --with playwright==1.57.0 python scripts/test-form-identity-browser.py.
Supply --source-dir to reproduce against a frozen baseline checkout.
"""
import argparse
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]


def main():
    """main bundles actual form components and checks generated, matching and explicit-id paths."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-dir', type=Path, default=ROOT)
    args = parser.parse_args()
    source = args.source_dir.resolve()
    with tempfile.TemporaryDirectory(prefix="form-identity-browser-") as temporary:
        bundle = Path(temporary) / "fixture.js"
        options = {"entryPoints": [str(ROOT / "scripts/fixtures/form-identity.tsx")],
                   "absWorkingDir": str(source), "alias": {"@": str(source / "src")},
                   "bundle": True, "format": "iife", "platform": "browser",
                   "define": {"process.env.NODE_ENV": '"production"',
                              "__ONEAPI_BROWSER_RELEASE__": '"test"'},
                   "outfile": str(bundle)}
        subprocess.run([shutil.which("node"), "-e",
                        "require('esbuild').buildSync(" + json.dumps(options) + ")"], cwd=source, check=True)
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(args=["--no-sandbox"])
            try:
                page = browser.new_page()
                page.set_default_timeout(3000)
                page.route("http://form.test/", lambda route: route.fulfill(
                    content_type="text/html", body='<div id="root"></div>'))
                page.goto("http://form.test/")
                page.add_script_tag(content=bundle.read_text())
                for label in ["Matched", "Generated", "Name"]:
                    page.get_by_text(label, exact=True).click()
                    if label == "Name":
                        expect(page.locator("#custom-name")).to_be_focused()
                    control = page.get_by_label(label, exact=True)
                    expect(control).to_be_visible()
                    expect(control).to_be_focused()
                    print(label + " label lookup and click focus passed", flush=True)
                name = page.get_by_label("Name", exact=True)
                expect(name).to_have_attribute("id", "custom-name")
                page.get_by_role("button", name="Save Name", exact=True).click()
                expect(name).to_be_focused()
                expect(name).to_have_attribute("aria-invalid", "true")
                message = page.get_by_text("Name is required", exact=True)
                expect(message).to_be_visible()
                message_id = message.get_attribute("id")
                expect(name).to_have_attribute("aria-describedby", "name-hint " + message_id)
                page.evaluate("window.formIdentityFixture.setId('next-name')")
                expect(page.get_by_label("Name", exact=True)).to_have_attribute("id", "next-name")
                expect(message).to_have_attribute("id", message_id)
                page.get_by_text("Name", exact=True).click()
                expect(page.get_by_label("Name", exact=True)).to_be_focused()
                page.evaluate("window.formIdentityFixture.setId(undefined)")
                expect(page.get_by_label("Name", exact=True)).not_to_have_attribute("id", "next-name")
                expect(page.get_by_label("Name", exact=True)).to_have_attribute("aria-describedby", "name-hint " + message_id)
                print(json.dumps({"engine": "chromium", "version": browser.version,
                                  "explicitIdentity": "passed", "matchingHtmlForControl": "passed",
                                  "generatedIdentityControl": "passed", "dynamicIdentity": "passed",
                                  "forwardedRefErrorFocus": "passed", "descriptions": "passed"}))
            finally:
                browser.close()


if __name__ == "__main__":
    main()
