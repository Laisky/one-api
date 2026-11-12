#!/usr/bin/env python3
"""Exercise real MobileDrawer scroll, focus and cleanup offline in installed browser engines.

Build Modern first. Run with uv run --with playwright==1.57.0 python scripts/test-mobile-drawer-browser.py.
The --css-dir option can reuse a qualified build's CSS for a frozen-source red run.
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


def background_position(page) -> float:
    """background_position measures a stable background landmark instead of fixed-body window.scrollY."""
    return page.locator('#opener').evaluate('element => element.getBoundingClientRect().top')


def check_drawer(page, bundle: str, css: str, unsupported: bool, engine: str) -> dict:
    """check_drawer verifies scroll isolation and restoration through repeated and interrupted modal lifetimes."""
    page.set_default_timeout(5000)
    page.route('**/*', lambda route: route.abort())
    page.set_content('<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><div id="root"></div></body></html>')
    page.evaluate("""() => {
      const storage = new Map([['i18nextLng', 'en']]);
      Object.defineProperty(window, 'localStorage', {value: {
        getItem: key => storage.get(key) ?? null,
        setItem: (key, value) => storage.set(key, String(value)),
        removeItem: key => storage.delete(key),
      }});
    }""")
    page.add_style_tag(content=css)
    if unsupported:
        # Model engines which ignore this enhancement, without claiming this is native Safari.
        page.add_style_tag(content='* { overscroll-behavior: auto !important; }')
    page.add_script_tag(content=bundle)
    opener = page.get_by_role('button', name='Open drawer')
    expect(opener).to_be_visible()
    page.evaluate('window.scrollTo(0, 400)')
    page.wait_for_function('window.scrollY === 400')
    opener.evaluate('element => element.focus({preventScroll: true})')
    before = background_position(page)
    page.evaluate('window.drawerFixture.open()')
    dialog = page.get_by_role('dialog', name='Test drawer')
    expect(dialog).to_be_visible()
    expect(page.locator('#root')).to_have_attribute('inert', '')
    expect(dialog.get_by_role('button').first).to_be_focused()
    page.keyboard.press('Tab')
    expect(dialog.get_by_role('link')).to_be_focused()
    page.keyboard.press('Shift+Tab')
    expect(dialog.get_by_role('button').first).to_be_focused()
    if engine == 'chromium':
        # CDP generates native browser touch input; has_touch alone only changes device capabilities.
        session = page.context.new_cdp_session(page)
        session.send('Input.dispatchTouchEvent', {'type': 'touchStart', 'touchPoints': [{'x': 380, 'y': 650}]})
        for y in range(620, 250, -30):
            session.send('Input.dispatchTouchEvent', {'type': 'touchMove', 'touchPoints': [{'x': 380, 'y': y}]})
            page.wait_for_timeout(15)
        session.send('Input.dispatchTouchEvent', {'type': 'touchEnd', 'touchPoints': []})
        session.detach()
        page.wait_for_timeout(180)
        assert abs(background_position(page) - before) < 1, 'touch scrolled the background'
    assert abs(background_position(page) - before) < 1, 'opening shifted the page'
    for x, y in [(380, 500), (150, 70), (150, 450)]:
        page.mouse.move(x, y)
        page.mouse.wheel(0, 350)
        page.wait_for_timeout(180)
        assert abs(background_position(page) - before) < 1, ('background scrolled beneath short drawer', x, y)
    page.evaluate('window.drawerFixture.long()')
    content = dialog.locator('.overflow-y-auto')
    page.mouse.move(150, 450)
    page.mouse.wheel(0, 450)
    page.wait_for_function('document.querySelector("[role=dialog] .overflow-y-auto").scrollTop > 0')
    content.evaluate('element => { element.scrollTop = element.scrollHeight; }')
    page.mouse.wheel(0, 350)
    page.wait_for_timeout(180)
    assert abs(background_position(page) - before) < 1, 'drawer boundary chained to background'
    page.keyboard.press('Escape')
    expect(dialog).to_have_count(0)
    expect(opener).to_be_focused()
    assert page.evaluate('window.scrollY') == 400
    expect(page.locator('#root')).not_to_have_attribute('inert', '')
    # Repeated close/open and an interrupted lifetime must release all locks.
    for _ in range(3):
        page.evaluate('window.drawerFixture.open()')
        expect(dialog).to_be_visible()
        page.evaluate('window.drawerFixture.close()')
        expect(dialog).to_have_count(0)
        assert page.evaluate('window.scrollY') == 400
        expect(opener).to_be_focused()
    page.evaluate('window.drawerFixture.open()')
    expect(dialog).to_be_visible()
    page.evaluate('window.drawerFixture.unmount()')
    expect(dialog).to_have_count(0)
    expect(opener).to_be_focused()
    expect(page.locator('#root')).not_to_have_attribute('inert', '')
    assert page.evaluate('window.scrollY') == 400
    page.mouse.move(380, 500)
    page.mouse.wheel(0, 350)
    page.wait_for_function('window.scrollY > 400')
    return {'scrollIsolation': True, 'drawerScrolling': True, 'focusReturn': True,
            'repeatedClose': True, 'unmountCleanup': True, 'nativeTouchInput': engine == 'chromium'}


def main() -> None:
    """main bundles real source and records native/unsupported-containment results for selected engines."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--engines', nargs='+', choices=['chromium', 'webkit'], default=['chromium'])
    parser.add_argument('--source-dir', type=Path, default=ROOT, help='Optional frozen baseline Modern source directory.')
    parser.add_argument('--css-dir', type=Path, default=ROOT.parent / 'build/modern/assets')
    parser.add_argument('--output', type=Path, default=ROOT / 'test-results/mobile-drawer')
    args = parser.parse_args()
    styles = sorted(args.css_dir.glob('*.css'))
    if not styles:
        parser.error('Build Modern or supply --css-dir.')
    css = '\n'.join(path.read_text() for path in styles)
    source = args.source_dir.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    results = []
    with tempfile.TemporaryDirectory(prefix='mobile-drawer-') as temporary:
        bundle = Path(temporary) / 'fixture.js'
        options = {'entryPoints': [str(ROOT / 'scripts/fixtures/mobile-drawer.tsx')],
                   'absWorkingDir': str(source), 'alias': {'@': str(source / 'src')},
                   'bundle': True, 'format': 'iife', 'platform': 'browser',
                   'define': {'process.env.NODE_ENV': '"production"', '__ONEAPI_BROWSER_RELEASE__': '"test"'},
                   'outfile': str(bundle)}
        subprocess.run([shutil.which('node'), '-e', "require('esbuild').buildSync(" + json.dumps(options) + ')'], cwd=source, check=True)
        with sync_playwright() as playwright:
            for engine in args.engines:
                browser = getattr(playwright, engine).launch(args=['--no-sandbox'] if engine == 'chromium' else [])
                try:
                    for unsupported in [False, True]:
                        page = browser.new_page(viewport={'width': 390, 'height': 844}, has_touch=True, is_mobile=True)
                        try:
                            result = check_drawer(page, bundle.read_text(), css, unsupported, engine)
                            results.append({'engine': engine, 'version': browser.version,
                                            'unsupportedContainment': unsupported, **result})
                        except Exception:
                            page.screenshot(path=str(args.output / f'{engine}-{unsupported}-failure.png'))
                            raise
                        finally:
                            page.close()
                finally:
                    browser.close()
                    (args.output / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
    print(json.dumps(results, indent=2))


if __name__ == '__main__':
    main()
