#!/usr/bin/env python3
"""Exercise real drawer/modal/toast focus boundaries offline in Chromium.

Run with Node22 and uv run --with playwright==1.57.0 python scripts/test-overlay-focus-browser.py.
Use --observe for baseline evidence without requiring the final isolation policy.
"""
from pathlib import Path
import argparse
import json
import shutil
import subprocess
import tempfile
import tarfile
import io

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]


def focus_snapshot(page):
    """focus_snapshot records the active element and its modal or notification boundary."""
    return page.evaluate("""() => {
      const el = document.activeElement;
      return {id: el.id, text: el.getAttribute('aria-label') || el.textContent,
              dialog: el.closest('[role=dialog]')?.getAttribute('aria-labelledby') || null,
              toast: !!el.closest('[data-notification-id]')};
    }""")


def setup_page(browser, bundle, css):
    """setup_page loads the real component fixture with a local browser storage substitute."""
    page = browser.new_page(viewport={"width": 390, "height": 844}, reduced_motion="reduce")
    page.on("pageerror", lambda error: print("PAGEERROR " + str(error), flush=True))
    page.route("**/*", lambda route: route.abort())
    page.set_content('<div id="root"></div>')
    page.evaluate("""() => {
      const storage = new Map([['i18nextLng', 'en']]);
      Object.defineProperty(window, 'localStorage', {value: {
        getItem: key => storage.get(key) ?? null,
        setItem: (key, value) => storage.set(key, String(value)),
        removeItem: key => storage.delete(key),
      }});
    }""")
    page.add_style_tag(content=css + "\n#root > button { margin-top: 160px; }")
    page.add_script_tag(content=bundle)
    expect(page.locator("#open-a")).to_be_visible()
    page.evaluate("window.overlayFixture.notify()")
    expect(page.locator("[data-notification-id]")).to_have_count(1)
    return page


def observe_focus(page, modal):
    """observe_focus traverses actual keyboard focus and attempts a real pointer toast dismissal."""
    page.locator("#open-modal" if modal else "#open-a").click()
    dialog = page.get_by_role("dialog", name="Test modal" if modal else "Drawer A", exact=True)
    expect(dialog).to_be_visible()
    traversal = []
    for _ in range(12):
        page.keyboard.press("Tab")
        traversal.append(focus_snapshot(page))
    for _ in range(6):
        page.keyboard.press("Shift+Tab")
        traversal.append(focus_snapshot(page))
    dismiss = page.locator("[data-notification-dismiss]")
    box = dismiss.bounding_box()
    page.mouse.click(box["x"] + box["width"] / 2, box["y"] + box["height"] / 2)
    page.wait_for_timeout(100)
    return {"traversal": traversal, "toastDismissed": page.locator("[data-notification-id]").count() == 0,
            "dialogStillOpenAfterToastPointer": dialog.count() > 0,
            "announcers": page.locator("[aria-live]").evaluate_all(
                "els => els.map(el => ({live:el.getAttribute('aria-live'), hidden:!!el.closest('[inert],[aria-hidden=true]')}))")}



def assert_focus_inside(page, dialog, steps=10):
    """assert_focus_inside proves forward and reverse keyboard traversal stays in the active modal."""
    for key in ["Tab"] * steps + ["Shift+Tab"] * steps:
        page.keyboard.press(key)
        assert dialog.evaluate("element => element.contains(document.activeElement)"), focus_snapshot(page)


def check_lifetimes(page):
    """check_lifetimes verifies shared inert ownership, both close orders and exact opener restoration."""
    results = {}
    page.locator("#open-a").click()
    page.locator("#open-b").click()
    b = page.get_by_role("dialog", name="Drawer B", exact=True)
    expect(b).to_be_visible()
    assert_focus_inside(page, b)
    page.keyboard.press("Escape")
    expect(b).to_have_count(0)
    expect(page.get_by_role("dialog", name="Drawer A", exact=True)).to_be_visible()
    expect(page.locator("#root")).to_have_attribute("inert", "")
    expect(page.locator("#open-b")).to_be_focused()
    page.keyboard.press("Escape")
    expect(page.get_by_role("dialog")).to_have_count(0)
    expect(page.locator("#open-a")).to_be_focused()
    results["lifoFocusAndEscape"] = True

    page.locator("#open-a").click()
    page.locator("#open-b").click()
    expect(b).to_be_visible()
    page.evaluate("window.overlayFixture.closeA()")
    expect(page.get_by_role("dialog", name="Drawer A", exact=True)).to_have_count(0)
    expect(page.locator("#root")).to_have_attribute("inert", "")
    assert_focus_inside(page, b)
    background = page.locator("#background").bounding_box()
    page.mouse.click(background["x"] + background["width"] / 2, background["y"] + background["height"] / 2)
    expect(page.locator("#background-actions")).to_have_text("0")
    page.keyboard.press("Escape")
    expect(page.get_by_role("dialog")).to_have_count(0)
    expect(page.locator("#root")).not_to_have_attribute("inert", "")
    expect(page.locator("#open-a")).to_be_focused()
    results["nonLifoIsolationAndFocus"] = True

    for _ in range(3):
        page.locator("#open-a").click()
        expect(page.get_by_role("dialog", name="Drawer A", exact=True)).to_be_visible()
        page.evaluate("window.overlayFixture.closeA()")
        expect(page.get_by_role("dialog")).to_have_count(0)
        expect(page.locator("#open-a")).to_be_focused()
        expect(page.locator("#root")).not_to_have_attribute("inert", "")
    page.locator("#open-a").click()
    page.evaluate("window.overlayFixture.unmountDrawers()")
    expect(page.get_by_role("dialog")).to_have_count(0)
    expect(page.locator("#open-a")).to_be_focused()
    expect(page.locator("#root")).not_to_have_attribute("inert", "")
    page.evaluate("window.overlayFixture.mountDrawers()")
    page.locator("#root").evaluate("element => element.setAttribute('inert', 'pre-existing')")
    page.evaluate("window.overlayFixture.openA()")
    expect(page.get_by_role("dialog", name="Drawer A", exact=True)).to_be_visible()
    page.evaluate("window.overlayFixture.closeA()")
    expect(page.get_by_role("dialog")).to_have_count(0)
    expect(page.locator("#root")).to_have_attribute("inert", "pre-existing")
    page.locator("#root").evaluate("element => element.removeAttribute('inert')")
    results["repeatedUnmountAndPreexistingInert"] = True
    return results


def check_nested(page):
    """check_nested verifies mixed Drawer/Radix modal nesting follows the same focus and Escape policy."""
    page.locator("#open-a").click()
    page.locator("#nested-modal").click()
    nested = page.get_by_role("dialog", name="Nested modal", exact=True)
    expect(nested).to_be_visible()
    assert_focus_inside(page, nested)
    page.keyboard.press("Escape")
    expect(nested).to_have_count(0)
    expect(page.get_by_role("dialog", name="Drawer A", exact=True)).to_be_visible()
    expect(page.locator("#nested-modal")).to_be_focused()
    page.keyboard.press("Escape")
    expect(page.get_by_role("dialog")).to_have_count(0)
    expect(page.locator("#open-a")).to_be_focused()

    page.locator("#open-modal").click()
    page.locator("#modal-open-b").click()
    b = page.get_by_role("dialog", name="Drawer B", exact=True)
    expect(b).to_be_visible()
    assert_focus_inside(page, b)
    page.keyboard.press("Escape")
    expect(b).to_have_count(0)
    expect(page.get_by_role("dialog", name="Test modal", exact=True)).to_be_visible()
    expect(page.locator("#modal-open-b")).to_be_focused()
    page.keyboard.press("Escape")
    expect(page.get_by_role("dialog")).to_have_count(0)
    expect(page.locator("#open-modal")).to_be_focused()
    return {"drawerToDialog": True, "dialogToDrawer": True}


def check_toast_without_modal(page):
    """check_toast_without_modal confirms existing toast actions and focus return stay usable."""
    page.locator("#background").focus()
    page.keyboard.press("Tab")
    expect(page.locator("[data-notification-id] button").first).to_be_focused()
    page.keyboard.press("Tab")
    expect(page.locator("[data-notification-dismiss]")).to_be_focused()
    page.locator("[data-notification-dismiss]").click()
    expect(page.locator("[data-notification-id]")).to_have_count(0)
    expect(page.locator("#background")).to_be_focused()
    return True


def check_late_announcements(page):
    """check_late_announcements keeps both announcers mounted/exposed through new nested-modal notifications."""
    polite = page.locator("[data-testid=notifications-announcer-polite]")
    assertive = page.locator("[data-testid=notifications-announcer-assertive]")
    polite.evaluate("element => { element.dataset.retained = 'polite'; }")
    assertive.evaluate("element => { element.dataset.retained = 'assertive'; }")
    page.locator("#open-a").click()
    page.locator("#nested-modal").click()
    expect(page.get_by_role("dialog", name="Nested modal", exact=True)).to_be_visible()
    page.evaluate("window.overlayFixture.notify(); window.overlayFixture.notifyError()")
    expect(polite).to_have_text("Overlay notification")
    expect(assertive).to_have_text("Overlay error")
    expect(polite).to_have_attribute("data-retained", "polite")
    expect(assertive).to_have_attribute("data-retained", "assertive")
    assert polite.evaluate("element => !element.closest('[inert],[aria-hidden=true]')")
    assert assertive.evaluate("element => !element.closest('[inert],[aria-hidden=true]')")
    assert_focus_inside(page, page.get_by_role("dialog", name="Nested modal", exact=True))
    return True


def check_together(page, observe=False):
    """check_together keeps the top drawer accessible when independent owners open in one React event."""
    page.locator("#open-a").focus()
    page.evaluate("window.overlayFixture.openTogether()")
    page.wait_for_timeout(100)
    result = {
        "dialogs": page.locator("[role=dialog]").evaluate_all(
            "els => els.map(el => ({title:el.textContent, ariaHidden:el.getAttribute('aria-hidden'), labelledBy:el.getAttribute('aria-labelledby'), focused:el.contains(document.activeElement)}))"),
        "accessibleB": page.get_by_role("dialog", name="Drawer B", exact=True).count(),
        "focus": focus_snapshot(page),
    }
    if not observe:
        assert result["accessibleB"] == 1, result
        b = page.get_by_role("dialog", name="Drawer B", exact=True)
        assert_focus_inside(page, b)
        page.keyboard.press("Escape")
        expect(b).to_have_count(0)
        a = page.get_by_role("dialog", name="Drawer A", exact=True)
        expect(a).to_be_visible()
        expect(page.locator("#root")).to_have_attribute("inert", "")
        page.wait_for_function("element => element.contains(document.activeElement)", arg=a.element_handle(), timeout=5000)
        assert_focus_inside(page, a)
        page.keyboard.press("Escape")
        expect(page.get_by_role("dialog")).to_have_count(0)
        expect(page.locator("#open-a")).to_be_focused()
        expect(page.locator("#root")).not_to_have_attribute("inert", "")
    return result

def main():
    """main captures native focus behavior and enforces permanent modal isolation regressions."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--observe", action="store_true")
    parser.add_argument("--together-only", action="store_true", help="Probe simultaneous drawer acquisition.")
    parser.add_argument("--source-ref", help="Optional frozen Git source revision for red reproduction.")
    parser.add_argument("--output", type=Path, default=ROOT.parents[1] / ".test-results/pr620-p2-20261113/overlay-browser")
    args = parser.parse_args()
    css = "\n".join(p.read_text() for p in sorted((ROOT.parent / "build/modern/assets").glob("*.css")))
    args.output.mkdir(parents=True, exist_ok=True)
    results = {}
    with tempfile.TemporaryDirectory(prefix="overlay-focus-") as temporary:
        bundle = Path(temporary) / "fixture.js"
        source = ROOT
        if args.source_ref:
            frozen = Path(temporary) / "baseline"
            frozen.mkdir()
            archive = subprocess.check_output(["git", "archive", args.source_ref, "web/modern/src", "web/legacy"], cwd=ROOT.parents[1])
            with tarfile.open(fileobj=io.BytesIO(archive)) as tree:
                tree.extractall(frozen, filter="data")
            source = frozen / "web/modern"
            (source / "node_modules").symlink_to(ROOT / "node_modules", target_is_directory=True)
        options = {"entryPoints": [str(ROOT / "scripts/fixtures/overlay-focus.tsx")],
                   "absWorkingDir": str(source), "alias": {"@": str(source / "src")},
                   "bundle": True, "format": "iife", "platform": "browser", "jsx": "automatic",
                   "define": {"process.env.NODE_ENV": '"production"', "__ONEAPI_BROWSER_RELEASE__": '"test"'},
                   "outfile": str(bundle)}
        subprocess.run([shutil.which("node"), "-e", "require('esbuild').buildSync(" + json.dumps(options) + ")"],
                       cwd=ROOT, check=True)
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(args=["--no-sandbox"])
            results["engine"] = {"name": "chromium", "version": browser.version}
            try:
                if args.together_only:
                    page = setup_page(browser, bundle.read_text(), css)
                    try:
                        results["together"] = check_together(page, args.observe)
                    finally:
                        page.close()
                    print(json.dumps(results, indent=2))
                    return
                for modal in [False, True]:
                    page = setup_page(browser, bundle.read_text(), css)
                    try:
                        result = observe_focus(page, modal)
                        results["modal" if modal else "drawer"] = result
                        if not args.observe:
                            assert not any(entry["toast"] or not entry["dialog"] for entry in result["traversal"]), result
                            assert not result["toastDismissed"], result
                            if not modal:
                                assert result["dialogStillOpenAfterToastPointer"], result
                            assert all(not entry["hidden"] for entry in result["announcers"]), result
                    finally:
                        page.close()
                if not args.observe:
                    for name, check in [("lifetimes", check_lifetimes), ("nested", check_nested),
                                        ("toastWithoutModal", check_toast_without_modal),
                                        ("lateAnnouncements", check_late_announcements),
                                        ("together", check_together)]:
                        page = setup_page(browser, bundle.read_text(), css)
                        try:
                            results[name] = check(page)
                        finally:
                            page.close()
                page = setup_page(browser, bundle.read_text(), css)
                try:
                    page.locator("#open-a").click()
                    page.locator("#open-b").click()
                    expect(page.get_by_role("dialog", name="Drawer B", exact=True)).to_be_visible()
                    page.evaluate("window.overlayFixture.closeA()")
                    expect(page.get_by_role("dialog", name="Drawer A", exact=True)).to_have_count(0)
                    overlap = page.locator("#root").get_attribute("inert") is not None
                    results["nonLifoRootStaysInert"] = overlap
                    if not args.observe:
                        assert overlap, "closing drawer A removed inert while drawer B remained open"
                    page.evaluate("window.overlayFixture.closeB()")
                    expect(page.get_by_role("dialog")).to_have_count(0)
                    results["lastCloseRestoresRoot"] = page.locator("#root").get_attribute("inert") is None
                    if not args.observe:
                        assert results["lastCloseRestoresRoot"]
                finally:
                    page.close()
            finally:
                browser.close()
                (args.output / "results.json").write_text(json.dumps(results, indent=2) + "\n")
    print(json.dumps(results, indent=2))


if __name__ == "__main__":
    main()
