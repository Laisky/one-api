#!/usr/bin/env python3
"""Exercise persisted sorting and Shift selection in the actual production bundle.

Run after the locked Modern production build. The driver serves only local build
assets and intercepts every API request with synthetic records. No real account,
backend, provider, or telemetry endpoint is used. Requires Python Playwright.
"""
from __future__ import annotations

import argparse
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import threading
from urllib.parse import parse_qs, urlsplit

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]
CASES = [
    ('channels', '/channels', '/api/channel/', 'name', 'Name', 'id'),
    ('users', '/users', '/api/user/', 'username', 'Username', ''),
    ('tokens', '/tokens', '/api/token/', 'name', 'Name', 'id'),
    ('redemptions', '/redemptions', '/api/redemption/', 'name', 'Name', ''),
    ('mcp', '/mcps', '/api/mcp_servers', 'name', 'Name', 'id'),
    ('logs', '/logs', '/api/log/', 'quota', 'Cost', 'created_at'),
]
INIT = """(() => {
  const id = sessionStorage.getItem('fixture-account') || 'fixture-a';
  const user = {id, uuid: id, username: id, role: 100, status: 1,
                quota: 500000000, used_quota: 0, group: 'default'};
  localStorage.setItem('auth-storage', JSON.stringify({version: 2,
    state: {user, token: null, isAuthenticated: true}}));
  localStorage.setItem('user', JSON.stringify(user));
  localStorage.setItem('i18nextLng', 'en');
  localStorage.setItem('oneapi_page_size', '10');
  localStorage.setItem('one-api-theme', 'light');
  window.__fixtureSelectionClicks = [];
  document.addEventListener('click', (event) => {
    const checkbox = event.target instanceof Element && event.target.closest('[role="checkbox"]');
    if (checkbox) window.__fixtureSelectionClicks.push({label: checkbox.getAttribute('aria-label'),
      shift: event.shiftKey, trusted: event.isTrusted, detail: event.detail});
  }, true);
})();"""


class AssetHandler(SimpleHTTPRequestHandler):
    """AssetHandler serves local production assets with a same-origin SPA fallback."""

    def do_GET(self) -> None:
        """do_GET serves known assets or the entry document for an application route."""
        path = Path(self.translate_path(urlsplit(self.path).path))
        if not path.is_file():
            self.path = '/index.html'
        super().do_GET()

    def log_message(self, _format: str, *args: object) -> None:
        """log_message suppresses repetitive local asset request logging."""


def storage_key(table: str, account: str = 'fixture-a') -> str:
    """storage_key returns the production key for a synthetic list and account."""
    principal = json.dumps([account, 100], separators=(',', ':'))
    return 'oneapi_table_sort_v1:' + json.dumps([table, principal], separators=(',', ':'))


def fixture_rows() -> list[dict]:
    """fixture_rows returns non-sensitive records shared by the six real list pages."""
    return [{
        'id': index, 'uuid': f'018fcf6d-c484-7000-8000-{index:012}',
        'name': f'Row {index:02}', 'username': f'Row {index:02}', 'display_name': f'Row {index:02}',
        'key': f'synthetic-fixture-key-{index}', 'role': 1, 'status': 1, 'type': 1,
        'models': 'gpt-4o', 'model_name': 'synthetic-model', 'group': 'default',
        'created_time': 1780000000 + index, 'created_at': 1780000000000 + index,
        'accessed_time': 1780000000, 'expired_time': -1, 'unlimited_quota': False,
        'quota': index * 500000, 'remain_quota': index * 500000, 'used_quota': 0,
        'balance': 100, 'priority': 0, 'weight': 0, 'response_time': 100,
        'base_url': 'https://fixture.invalid', 'protocol': 'http', 'auth_type': 'none',
        'tool_count': index, 'elapsed_time': index, 'prompt_tokens': index, 'completion_tokens': index,
        'channel': index, 'channel_name': f'Row {index:02}', 'token_name': f'Row {index:02}',
    } for index in range(1, 16)]


def install_routes(context, origin: str, calls: list, violations: list) -> None:
    """install_routes intercepts all APIs and refuses every non-local request."""
    rows = fixture_rows()
    endpoints = {case[2] for case in CASES}

    def route_request(route) -> None:
        """route_request responds to a synthetic GET or records a prohibited request."""
        request = route.request
        parsed = urlsplit(request.url)
        if f'{parsed.scheme}://{parsed.netloc}' != origin:
            violations.append(f'external:{parsed.netloc}')
            route.abort()
            return
        if not parsed.path.startswith('/api/'):
            route.continue_()
            return
        if request.method != 'GET':
            violations.append(f'write:{request.method}:{parsed.path}')
            route.abort()
            return
        query = {key: values[-1] for key, values in parse_qs(parsed.query).items()}
        calls.append({'path': parsed.path, 'query': query})
        data: object = []
        if parsed.path in endpoints:
            field = query.get('sort') or ('created_at' if parsed.path == '/api/log/' else 'id')
            ordered = sorted(rows, key=lambda row: row.get(field, 0), reverse=query.get('order', 'desc') == 'desc')
            page, size = int(query.get('p', '0')), int(query.get('size', '10'))
            data = ordered[page * size:(page + 1) * size]
            if parsed.path == '/api/mcp_servers':
                data = [{'server': row, 'tool_count': row['tool_count']} for row in data]
        elif parsed.path == '/api/status':
            data = {'system_name': 'One API Test', 'quota_per_unit': 500000,
                    'display_in_currency': True, 'log_cursor_enabled': False}
        elif parsed.path == '/api/notice':
            data = ''
        elif parsed.path in ['/api/models/display', '/api/browser/config']:
            data = {}
        route.fulfill(status=200, content_type='application/json',
                      body=json.dumps({'success': True, 'data': data, 'total': len(rows)}))

    context.route('**/*', route_request)


def ready(page) -> None:
    """ready waits for rendered rows and the end of the actual loading state."""
    page.locator('[role="checkbox"][aria-label^="Select Row"]').first.wait_for()
    page.wait_for_function("""() => {
      const c = document.querySelector('[role="checkbox"][aria-label^="Select Row"]');
      return c && !c.disabled && !document.querySelector('[aria-busy="true"]');
    }""")


def row_checks(page):
    """row_checks returns only visible record selection controls, not select-page controls."""
    return page.locator('[role="checkbox"][aria-label^="Select Row"]')


def selected_names(page) -> list[str]:
    """selected_names reads the actual accessible selected state in current display order."""
    return row_checks(page).evaluate_all("elements => elements.filter(e => e.getAttribute('aria-checked') === 'true').map(e => e.getAttribute('aria-label'))")


def selection_command(page, name: str) -> None:
    """selection_command drives a real selection menu and its requested command."""
    page.locator('.table-toolbar__selection button').click()
    page.get_by_role('menuitem', name=name, exact=True).click()


def sort_request_check(calls: list, endpoint: str, field: str, direction: str) -> None:
    """sort_request_check verifies every first-load list request used the restored sort."""
    relevant = [call['query'] for call in calls if call['path'] == endpoint]
    assert relevant, (endpoint, calls)
    for query in relevant:
        assert query.get('sort', '') == field, query
        if field:
            assert query.get('order') == direction, query


def choose_sort(page, field: str, label: str, mobile: bool) -> None:
    """choose_sort clicks an actual header or chooses the actual mobile sort field."""
    if mobile:
        page.locator('.mobile-table-sort select').select_option(field)
    else:
        page.get_by_role('columnheader').get_by_role('button', name=label, exact=True).click()
    ready(page)


def flip_sort(page, label: str, mobile: bool) -> None:
    """flip_sort toggles the real direction control without changing the chosen field."""
    if mobile:
        page.locator('.mobile-table-sort button').click()
    else:
        page.get_by_role('columnheader').get_by_role('button', name=label, exact=True).click()
    ready(page)


def check_case(browser, origin: str, case: tuple, width: int, output: Path) -> dict:
    """check_case verifies real reload, visible ordering, selection ranges, and principal isolation."""
    table, path, endpoint, field, label, default_by = case
    mobile = width < 768
    calls, violations, errors = [], [], []
    context = browser.new_context(viewport={'width': width, 'height': 1000}, has_touch=mobile)
    context.add_init_script(INIT)
    install_routes(context, origin, calls, violations)
    page = context.new_page()
    page.on('pageerror', lambda error: errors.append(str(error)))
    key = storage_key(table)
    try:
        page.goto(origin + path)
        ready(page)
        sort_request_check(calls, endpoint, default_by, 'desc')
        choose_sort(page, field, label, mobile)
        flip_sort(page, label, mobile)
        chosen = page.evaluate('key => JSON.parse(localStorage.getItem(key))', key)
        assert chosen['sortBy'] == field and chosen['sortOrder'] in ['asc', 'desc'], chosen
        calls.clear()
        page.reload()
        ready(page)
        sort_request_check(calls, endpoint, field, chosen['sortOrder'])
        if mobile:
            expect(page.locator('.mobile-table-sort select')).to_have_value(field)
        else:
            expect(page.get_by_role('columnheader').filter(has=page.get_by_role('button', name=label, exact=True))).to_have_attribute(
                'aria-sort', 'ascending' if chosen['sortOrder'] == 'asc' else 'descending')
        labels = row_checks(page).evaluate_all("elements => elements.map(e => e.getAttribute('aria-label'))")
        expected = [f'Select Row {index:02}' for index in (range(1, 11) if chosen['sortOrder'] == 'asc' else range(15, 5, -1))]
        assert labels == expected, (labels, expected)
        row_checks(page).nth(0).click()
        assert selected_names(page) == labels[:1], ('ordinary click', selected_names(page))
        row_checks(page).nth(4).click(modifiers=['Shift'])
        assert selected_names(page) == labels[:5]
        assert urlsplit(page.url).path == path and 'id=' not in urlsplit(page.url).query
        selection_command(page, 'Clear selection')
        row_checks(page).nth(4).click()
        row_checks(page).nth(0).click(modifiers=['Shift'])
        assert selected_names(page) == labels[:5]
        selection_command(page, 'Select all pages')
        row_checks(page).nth(0).click()
        row_checks(page).nth(4).click(modifiers=['Shift'])
        assert selected_names(page) == labels[5:]
        expect(page.get_by_role('status').filter(has_text='All matching pages selected')).to_contain_text('10 items (5 excluded)')
        selection_command(page, 'Clear selection')
        if mobile:
            hit_area = page.locator('.mobile-record__selection').first
            hit_area.click(position={'x': 2, 'y': 2})
            assert selected_names(page) == labels[:1], ('padding click', selected_names(page))
            hit_area.click(position={'x': 2, 'y': 2})
            assert selected_names(page) == []
        row_checks(page).nth(0).click()
        flip_sort(page, label, mobile)
        row_checks(page).nth(4).click(modifiers=['Shift'])
        target = row_checks(page).nth(4).get_attribute('aria-label')
        assert selected_names(page) == [target], selected_names(page)
        page.screenshot(path=str(output / f'{table}-{width}.png'), full_page=True)
        # Changing the principal uses defaults, then restores the first account's
        # last preference without copying it into the second account's storage.
        saved = page.evaluate('key => JSON.parse(localStorage.getItem(key))', key)
        page.evaluate("sessionStorage.setItem('fixture-account', 'fixture-b')")
        calls.clear()
        page.reload()
        ready(page)
        sort_request_check(calls, endpoint, default_by, 'desc')
        assert page.evaluate('key => localStorage.getItem(key)', storage_key(table, 'fixture-b')) is None
        page.evaluate("sessionStorage.setItem('fixture-account', 'fixture-a')")
        calls.clear()
        page.reload()
        ready(page)
        sort_request_check(calls, endpoint, saved['sortBy'], saved['sortOrder'])
        # A syntactically valid but removed column must not reach the API.
        page.evaluate("key => localStorage.setItem(key, JSON.stringify({sortBy:'removed_column',sortOrder:'asc'}))", key)
        calls.clear()
        page.reload()
        ready(page)
        sort_request_check(calls, endpoint, default_by, 'desc')
        assert not errors, errors
        assert not violations, violations
        return {'table': table, 'width': width, 'passed': True,
                'checks': ['first-request sort', 'reload', 'indicator', 'visible order', 'forward range',
                           'reverse range', 'all-matching exclusions', 'stale anchor', 'account isolation', 'removed column'],
                'browser_errors': errors, 'prohibited_requests': violations}
    except Exception:
        page.screenshot(path=str(output / f'failure-{table}-{width}.png'), full_page=True)
        (output / f'failure-{table}-{width}.json').write_text(json.dumps({'calls': calls, 'errors': errors, 'violations': violations, 'selection_clicks': page.evaluate('window.__fixtureSelectionClicks'), 'selected': selected_names(page)}, indent=2))
        raise
    finally:
        context.close()


def main() -> None:
    """main serves a local compiled app, checks six lists at two widths, and closes all resources."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--browser', default=None)
    parser.add_argument('--table', choices=[case[0] for case in CASES])
    parser.add_argument('--width', type=int, choices=[390, 1280])
    parser.add_argument('--output', type=Path, default=ROOT / 'test-results/table-interactions')
    args = parser.parse_args()
    build = ROOT.parent / 'build/modern'
    if not (build / 'index.html').is_file():
        parser.error('Build the Modern production frontend first.')
    args.output.mkdir(parents=True, exist_ok=True)
    server = ThreadingHTTPServer(('127.0.0.1', 0), partial(AssetHandler, directory=str(build)))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    results = []
    try:
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(executable_path=args.browser)
            try:
                for width in ([args.width] if args.width else [1280, 390]):
                    for case in (case for case in CASES if not args.table or case[0] == args.table):
                        results.append(check_case(browser, f'http://127.0.0.1:{server.server_port}', case, width, args.output))
                        print(f'PASS {case[0]} at {width}px', flush=True)
            finally:
                browser.close()
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)
        (args.output / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
    print(f'Passed {len(results)} production-bundle browser cases.')


if __name__ == '__main__':
    main()
