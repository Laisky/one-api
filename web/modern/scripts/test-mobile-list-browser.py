#!/usr/bin/env python3
"""Measure real mobile list pages offline; build Modern first, then run this script.

Uses the existing toolbar fixture loader, production CSS, real React pages, and
synthetic Axios responses. No live backend or production credentials are used.
"""
from __future__ import annotations

import argparse
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('toolbar_browser', ROOT / 'scripts/test-table-toolbar-browser.py')
assert spec and spec.loader
loader = importlib.util.module_from_spec(spec)
spec.loader.exec_module(loader)


def record_geometry(record) -> dict:
    """record_geometry measures visible controls and content, excluding deliberately hidden accessible labels."""
    return record.evaluate("""element => {
      const bounds = element.getBoundingClientRect();
      const visible = [...element.querySelectorAll('*')].filter(node => {
        const box = node.getBoundingClientRect();
        return box.width > 1 && box.height > 1 && !node.closest('.sr-only');
      });
      const overflow = visible.filter(node => {
        const box = node.getBoundingClientRect();
        return box.left < bounds.left - 1 || box.right > bounds.right + 1;
      }).map(node => ({tag: node.tagName, className: node.className}));
      const buttons = visible.filter(node => node.matches('button:not([role="checkbox"])'));
      const sizes = buttons.map(node => {
        const box = node.getBoundingClientRect();
        return Math.min(box.width, box.height);
      });
      const label = element.querySelector('.mobile-record__selection')?.getBoundingClientRect();
      const checkbox = element.querySelector('[role="checkbox"]')?.getBoundingClientRect();
      return {height: bounds.height, width: bounds.width, overflow,
        minimumTarget: sizes.length ? Math.min(...sizes) : null,
        checkboxTarget: label ? Math.min(label.width, label.height) : null,
        checkboxGlyph: checkbox ? Math.max(checkbox.width, checkbox.height) : null};
    }""")


def check_mobile(page, name: str, output: Path, screenshot: bool) -> dict:
    """check_mobile checks density, actual label hit targets, disclosures, focus, and no unintended writes."""
    records = page.locator('[data-mobile-record]')
    expect(records.first).to_be_visible()
    record = records.first
    idle = record_geometry(record)
    assert not idle['overflow'], (name, idle)
    assert idle['height'] <= 360, (name, idle)
    assert idle['minimumTarget'] is None or idle['minimumTarget'] >= 43.9, (name, idle)
    assert idle['checkboxTarget'] >= 43.9 and idle['checkboxGlyph'] <= 20.1, (name, idle)
    assert not record_geometry(records.nth(1))['overflow'], (name, 'long record overflow')
    expect(record.locator('.mobile-record__actions')).to_be_hidden()
    if screenshot:
        page.screenshot(path=str(output / f'{name}-idle.png'), animations='disabled')
    # Tap the label padding, outside the 20px visual checkbox. This proves the
    # 44px hit area actually activates the checkbox, rather than only looking larger.
    record.locator('.mobile-record__selection').tap(position={'x': 3, 'y': 3})
    expect(record.get_by_role('checkbox')).to_be_checked()
    record.locator('.mobile-record__selection').tap(position={'x': 3, 'y': 3})
    expect(record.get_by_role('checkbox')).not_to_be_checked()
    details = record.locator('.mobile-record__detail-trigger')
    if details.count():
        details.click()
        expect(record.locator('.mobile-record__details')).to_be_visible()
        assert not record_geometry(record)['overflow'], (name, 'details overflow')
        details.click()
    trigger = record.locator('.mobile-record__action-trigger')
    trigger.click()
    expect(record.locator('.mobile-record__actions')).to_be_visible()
    expanded = record_geometry(record)
    assert not expanded['overflow'], (name, expanded)
    assert expanded['minimumTarget'] >= 43.9, (name, expanded)
    if screenshot:
        record.scroll_into_view_if_needed()
        page.screenshot(path=str(output / f'{name}-actions.png'), animations='disabled')
    record.locator('.mobile-record__actions button').first.focus()
    page.keyboard.press('Escape')
    expect(trigger).to_be_focused()
    expect(trigger).to_have_attribute('aria-expanded', 'false')
    assert page.evaluate("window.toolbarCalls.every(call => !call.method || call.method === 'get')"), name
    # Shared toolbar must stay contained after selection, including localized labels.
    toolbar = page.locator('.table-toolbar')
    toolbar_size = loader.geometry(toolbar)
    assert not toolbar_size['overflow'] and not toolbar_size['overlap'], (name, toolbar_size)
    assert page.evaluate('document.documentElement.scrollWidth <= window.innerWidth + 1'), name
    return {'case': name, 'idle': idle, 'actions': expanded, 'toolbar': toolbar_size}


def main() -> None:
    """main compiles the real-page fixture and writes measured geometry and reviewable screenshots."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--browser', default=None, help='Optional Chromium executable; otherwise use Playwright Chromium.')
    parser.add_argument('--source-dir', type=Path, default=ROOT)
    parser.add_argument('--output', type=Path, default=ROOT / 'test-results/mobile-list')
    parser.add_argument('--baseline', action='store_true', help='Measure old Users cards with their own built CSS; do not assert the new contract.')
    args = parser.parse_args()
    source = args.source_dir.resolve()
    node = shutil.which('node')
    if not node:
        parser.error('Node.js is required.')
    styles = sorted((source.parent / 'build/modern/assets').glob('*.css'))
    if not styles:
        parser.error('Build the Modern frontend before running this test.')
    css = '\n'.join(path.read_text() for path in styles)
    args.output.mkdir(parents=True, exist_ok=True)
    results = []
    with tempfile.TemporaryDirectory(prefix='mobile-list-') as temporary:
        bundle_path = Path(temporary) / 'fixture.js'
        options = {
            'entryPoints': [str(ROOT / 'scripts/fixtures/mobile-list.tsx')],
            'absWorkingDir': str(source), 'alias': {'@': str(source / 'src')},
            'bundle': True, 'format': 'iife', 'platform': 'browser',
            'define': {'process.env.NODE_ENV': '"production"'}, 'outfile': str(bundle_path),
        }
        subprocess.run([node, '-e', "require('esbuild').buildSync(" + json.dumps(options) + ')'], cwd=source, check=True)
        bundle = bundle_path.read_text()
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(executable_path=args.browser, args=['--no-sandbox'])
            try:
                cases = [(width, lang, theme, kind)
                         for width in [320, 360, 390, 430, 640, 768, 1280]
                         for lang in ['en', 'zh', 'fr', 'es', 'ja']
                         for theme in ['light', 'dark']
                         for kind in ['users', 'channels', 'redemptions']]
                if args.baseline:
                    cases = [(390, 'en', 'light', 'users')]
                for width, language, theme, kind in cases:
                    name = f'{kind}-{width}-{language}-{theme}'
                    page = browser.new_page(viewport={'width': width, 'height': 844}, has_touch=width < 768, reduced_motion='reduce')
                    errors = []
                    page.on('pageerror', lambda error: errors.append(str(error)))
                    try:
                        loader.load_fixture(page, bundle, css, language, theme, kind)
                        page.wait_for_function("!document.querySelector('.table-toolbar__selection button').disabled")
                        if args.baseline:
                            checkbox = page.get_by_role('checkbox').first
                            card = checkbox.locator('xpath=ancestor::div[contains(@class,"shadow-sm")][1]')
                            measurement = card.bounding_box()
                            assert measurement
                            results.append({'case': name, 'idle': {'height': measurement['height'], 'width': measurement['width']}})
                            page.screenshot(path=str(args.output / f'{name}-before.png'), animations='disabled')
                        elif width < 768:
                            results.append(check_mobile(page, name, args.output, width == 390 and language == 'en'))
                        else:
                            expect(page.get_by_role('table')).to_be_visible()
                            expect(page.locator('[data-mobile-record]')).to_have_count(0)
                            results.append({'case': name, 'desktopTable': True})
                        assert not errors, (name, errors)
                    except Exception:
                        page.screenshot(path=str(args.output / f'{name}-failure.png'), animations='disabled')
                        (args.output / f'{name}-failure.html').write_text(page.content())
                        raise
                    finally:
                        page.close()
            finally:
                browser.close()
                (args.output / 'geometry.json').write_text(json.dumps(results, indent=2) + '\n')
    print(f'Passed {len(results)} offline Chromium cases; evidence: {args.output}')


if __name__ == '__main__':
    main()
