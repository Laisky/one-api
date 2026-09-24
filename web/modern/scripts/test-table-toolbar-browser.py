#!/usr/bin/env python3
"""Check actual table layouts in offline Chromium with a synthetic Axios adapter.

Prerequisites: a locked `yarn install`, `yarn build`, Python Playwright, and Chromium.
Run: python scripts/test-table-toolbar-browser.py --browser /usr/bin/chromium
No live backend, real account, network access, or new production dependency is used.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]


def load_fixture(page, bundle: str, css: str, language: str, theme: str, fixture_page: str) -> None:
    """load_fixture injects compiled real components into an offline document with test-only storage."""
    page.route("**/*", lambda route: route.abort())
    page.set_content('<!doctype html><html><head><meta charset="utf-8"></head><body><div id="root"></div></body></html>')
    page.evaluate("""options => {
      const storage = new Map([['one-api-theme', options.theme], ['i18nextLng', options.language]]);
      Object.defineProperty(window, 'localStorage', {value: {
        getItem: key => storage.get(key) ?? null,
        setItem: (key, value) => storage.set(key, String(value)),
        removeItem: key => storage.delete(key),
        clear: () => storage.clear(),
      }});
      window.toolbarFixture = {language: options.language, page: options.page};
    }""", {"language": language, "theme": theme, "page": fixture_page})
    page.add_style_tag(content=css)
    page.add_script_tag(content=bundle)
    page.get_by_role("checkbox").first.wait_for()


def geometry(toolbar) -> dict:
    """geometry returns actual toolbar dimensions and verifies visible controls do not overlap or overflow."""
    return toolbar.evaluate("""element => {
      const bounds = element.getBoundingClientRect();
      const controls = [...element.querySelectorAll('button')].filter(e => {
        const b = e.getBoundingClientRect();
        return b.width && b.height;
      }).map(e => {
        const b = e.getBoundingClientRect();
        return {left: b.left, top: b.top, right: b.right, bottom: b.bottom, width: b.width, height: b.height};
      });
      const overflow = controls.some(b => b.left < bounds.left - 1 || b.right > bounds.right + 1);
      const overlap = controls.some((a, index) => controls.slice(index + 1).some(b =>
        Math.min(a.right, b.right) - Math.max(a.left, b.left) > 1 &&
        Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 1));
      return {width: bounds.width, height: bounds.height, overflow, overlap,
        minimumTarget: Math.min(...controls.map(b => Math.min(b.width, b.height)))};
    }""")



def check_confirmation(page) -> None:
    """check_confirmation verifies the real menu/dialog flow sends only confirmed synthetic UUIDs."""
    first = page.get_by_role("checkbox", name="Select OpenAI Production", exact=True)
    first.check()
    page.get_by_role("button", name="Actions", exact=True).click()
    page.get_by_role("menuitem", name="Reset selected models", exact=True).click()
    dialog = page.get_by_role("dialog", name="Reset selected models", exact=True)
    expect(dialog).to_contain_text("the 1 selected records")
    assert page.evaluate("window.toolbarCalls.filter(call => call.url.endsWith('/reset_models')).length") == 0
    dialog.get_by_role("button", name="Cancel", exact=True).click()
    expect(first).to_be_checked()
    page.get_by_role("button", name="Actions", exact=True).click()
    page.get_by_role("menuitem", name="Reset selected models", exact=True).click()
    page.get_by_role("dialog").get_by_role("button", name="Confirm", exact=True).click()
    expect(page.get_by_role("region", name="Selected action results")).to_contain_text("1 selected: 1 succeeded")
    calls = page.evaluate("window.toolbarCalls.filter(call => call.url.endsWith('/reset_models'))")
    assert len(calls) == 1 and calls[0]["data"] == {
        "selection": {"mode": "ids", "ids": ["018fcf6d-c484-7000-8000-000000000001"]}
    }, calls
    expect(page.get_by_role("button", name="Actions", exact=True)).to_have_count(0)


def main() -> None:
    """main compiles the fixture, checks both table renderers, and writes screenshots and geometry evidence."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", default=shutil.which("chromium") or shutil.which("google-chrome"))
    parser.add_argument("--node", default=shutil.which("node"))
    parser.add_argument("--output", type=Path, default=ROOT / "test-results" / "table-toolbar")
    parser.add_argument("--source-dir", type=Path, default=ROOT, help="Optional frozen baseline Modern source directory.")
    parser.add_argument("--baseline", action="store_true", help="Measure the old layout without asserting the new contract.")
    args = parser.parse_args()
    if not args.browser or not args.node:
        parser.error("Node.js and a Chromium executable are required.")
    source = args.source_dir.resolve()
    styles = sorted((source.parent / "build" / "modern" / "assets").glob("*.css"))
    if not styles:
        parser.error("Build the Modern frontend first (`yarn build`).")
    css = "\n".join(path.read_text() for path in styles)
    args.output.mkdir(parents=True, exist_ok=True)
    results = []
    with tempfile.TemporaryDirectory(prefix="table-toolbar-") as temporary:
        bundle_path = Path(temporary) / "fixture.js"
        build_options = {
            "entryPoints": [str(ROOT / "scripts/fixtures/table-toolbar.tsx")],
            "absWorkingDir": str(source), "alias": {"@": str(source / "src")},
            "bundle": True, "format": "iife", "platform": "browser",
            "define": {"process.env.NODE_ENV": '"production"'}, "outfile": str(bundle_path),
        }
        subprocess.run([args.node, "-e", "require('esbuild').buildSync(" + json.dumps(build_options) + ")"], cwd=source, check=True)
        bundle = bundle_path.read_text()
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(executable_path=args.browser, args=["--no-sandbox"])
            try:
                cases = [(width, lang, theme, kind)
                         for width in [320, 390, 640, 888, 1280]
                         for lang in ["en", "zh", "fr", "es", "ja"]
                         for theme in ["light", "dark"]
                         for kind in ["channels", "redemptions"]]
                if args.baseline:
                    cases = [(width, "en", "light", "channels") for width in [888, 1280]]
                for width, language, theme, kind in cases:
                    page = browser.new_page(viewport={"width": width, "height": 900}, has_touch=width < 640)
                    errors = []
                    page.on("pageerror", lambda error: errors.append(str(error)))
                    labels = json.loads((ROOT / f"src/i18n/locales/{language}/table-selection.json").read_text())["table_selection"]
                    load_fixture(page, bundle, css, language, theme, kind)
                    name = f"{kind}-{width}-{language}-{theme}"
                    if args.baseline:
                        selection = page.locator('div[aria-label="Row selection"]')
                        container = selection.locator("..")
                        dimensions = container.evaluate("""el => {
                          const a = el.children[0].getBoundingClientRect(), b = el.children[1].getBoundingClientRect();
                          return {width: a.width, height: b.bottom - a.top};
                        }""")
                        results.append({"case": name, "idle": dimensions})
                        page.screenshot(path=str(args.output / f"baseline-{name}.png"), animations="disabled")
                        page.close()
                        continue
                    toolbar = page.get_by_role("group", name=labels["toolbar"], exact=True)
                    toolbar.wait_for()
                    page.get_by_role("button", name=labels["controls"], exact=True).wait_for(state="visible")
                    # API responses are asynchronous; wait until the fixture's initial load settles.
                    page.wait_for_function("!document.querySelector('.table-toolbar__selection button').disabled")
                    idle = geometry(toolbar)
                    assert not idle["overflow"] and not idle["overlap"], (name, idle)
                    assert idle["minimumTarget"] >= (44 if width < 640 else 24), (name, idle)
                    assert idle["height"] <= (44 if idle["width"] > 640 else 96), (name, idle)
                    if kind == "channels":
                        assert page.get_by_role("button", name=labels["actions"], exact=True).count() == 0
                    selector = page.get_by_role("button", name=labels["controls"], exact=True)
                    selector.click()
                    page.get_by_role("menuitem", name=labels["all_pages"], exact=True).click()
                    selected = geometry(toolbar)
                    assert not selected["overflow"] and not selected["overlap"], (name, selected)
                    assert selected["height"] == idle["height"], (name, idle, selected)
                    if kind == "channels":
                        action = page.get_by_role("button", name=labels["actions"], exact=True)
                        action.click()
                        menu = page.get_by_role("menu", name=labels["selected_actions"], exact=True)
                        menu.wait_for()
                        bounds = menu.bounding_box()
                        assert bounds and bounds["x"] >= 0 and bounds["x"] + bounds["width"] <= width + 1, (name, bounds)
                        if language == "en" and width in [390, 888, 1280] and theme == "light":
                            page.screenshot(path=str(args.output / f"{name}-actions.png"), animations="disabled")
                        page.keyboard.press("Escape")
                        assert action.evaluate("el => el === document.activeElement"), name
                        # Opening menus must never execute a record mutation.
                        assert page.evaluate("window.toolbarCalls.every(call => !call.url.endsWith('reset_models'))"), name
                    if language == "en" and width in [390, 888, 1280] and theme == "light":
                        page.screenshot(path=str(args.output / f"{name}-selected.png"), animations="disabled")
                    selector.click()
                    page.get_by_role("menuitem", name=labels["clear"], exact=True).click()
                    if kind == "channels":
                        assert page.get_by_role("button", name=labels["actions"], exact=True).count() == 0
                    if (width, language, theme, kind) == (1280, "en", "light", "channels"):
                        check_confirmation(page)
                    assert not errors, (name, errors)
                    results.append({"case": name, "idle": idle, "selected": selected})
                    page.close()
            finally:
                browser.close()
    (args.output / "geometry.json").write_text(json.dumps(results, indent=2) + "\n")
    print(f"Passed {len(results)} offline Chromium layout cases; evidence: {args.output}")


if __name__ == "__main__":
    main()
