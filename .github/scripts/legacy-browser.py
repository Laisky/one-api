"""Exercise actual legacy bundles and Vite's dev proxy with a local API fixture."""
from __future__ import annotations

import argparse
import functools
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import sys
import threading
import time
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import URLError
from urllib.parse import parse_qs, urlsplit
from urllib.request import urlopen

from playwright.sync_api import expect, sync_playwright

from browser_process import process_cleanup

ROOT = Path(__file__).resolve().parents[2]
USER = {"id": 1, "uuid": "10000000-0000-0000-0000-000000000001", "username": "quality", "display_name": "Quality", "role": 100, "status": 1, "group": "default", "quota": 100000, "used_quota": 0}
SITE = {"system_name": "One API quality fixture", "version": "quality-test", "server_address": "http://127.0.0.1", "quota_per_unit": 500000, "display_in_currency": True, "turnstile_check": False, "github_oauth": False, "email_verification": False}
TOKEN = {"id": 42, "uuid": "10000000-0000-0000-0000-000000000042", "name": "quality-token", "key": "fixture-only", "status": 1, "created_time": 1700000000, "accessed_time": 1700000000, "expired_time": -1, "remain_quota": 10000, "used_quota": 0, "unlimited_quota": False, "models": "", "model_limits_enabled": False}


class FixtureHandler(SimpleHTTPRequestHandler):
    """FixtureHandler serves real build files and failure-injectable API responses."""

    def log_message(self, _format: str, *_args: object) -> None:
        """log_message suppresses payloads and noisy expected HTTP failures."""

    def do_GET(self) -> None:
        """do_GET serves the local API or SPA, preserving static 404 responses."""
        parsed = urlsplit(self.path)
        if parsed.path.startswith('/api/'):
            status = 200
            data: object = []
            if parsed.path == '/api/status':
                data = SITE
            elif parsed.path == '/api/user/self':
                data = USER
            elif parsed.path == '/api/reset_password':
                self.server.reset_calls += 1
                assert parse_qs(parsed.query).get('email') == ['a+b@example.test']
                status = 500 if self.server.reset_calls == 1 else 200
            elif parsed.path in ('/api/token/', '/api/token/search'):
                self.server.token_calls += 1
                # Keep every initial request failed, including React StrictMode
                # effect replays. Only an explicit user retry opens this gate.
                status = 500 if self.server.token_fail else 200
                data = [TOKEN]
            payload = json.dumps({"success": status == 200, "message": "fixture failure" if status != 200 else "", "data": data}).encode()
            self.send_response(status)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            return
        candidate = Path(self.translate_path(parsed.path))
        if not candidate.is_file() and not Path(parsed.path).suffix:
            self.path = '/index.html'
        super().do_GET()


    def do_POST(self) -> None:
        """do_POST accepts only the local user-creation fixture and injects one failure."""
        size = int(self.headers.get('Content-Length', '0'))
        assert 0 < size < 4096, 'The fixture accepts only bounded JSON payloads'
        data = json.loads(self.rfile.read(size))
        if urlsplit(self.path).path != '/api/user/':
            self.send_error(404)
            return
        assert data['username'] == 'quality-user'
        assert data['password'] == 'quality-password-for-fixture'
        self.server.user_calls += 1
        status = 500 if self.server.user_calls == 1 else 200
        payload = json.dumps({'success': status == 200, 'message': 'fixture create failure' if status != 200 else '', 'data': {}}).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


def reserve_port() -> int:
    """reserve_port chooses an ephemeral loopback port for the Vite subprocess."""
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        return listener.getsockname()[1]


def exercise(browser, base: str, server, theme: str, evidence: Path, mode: str) -> None:
    """exercise checks real rendering, injected failures, retry, and browser errors."""
    errors: list[str] = []
    context = browser.new_context(viewport={"width": 1280, "height": 900})
    context.route(re.compile(r'^https://'), lambda route: route.abort())
    page = context.new_page()
    page.on('pageerror', lambda error: errors.append(str(error)))
    server.reset_calls = 0
    server.token_calls = 0
    server.token_fail = True
    server.user_calls = 0
    try:
        page.goto(base + '/reset', wait_until='networkidle', timeout=60000)
        email = page.locator('input[name="email"]')
        expect(email).to_be_visible(timeout=30000)
        email.fill('a+b@example.test')
        submit = page.get_by_role('button', name='提交', exact=True)
        with page.expect_response(lambda response: urlsplit(response.url).path == '/api/reset_password'):
            submit.click()
        expect(submit).to_be_enabled(timeout=10000)
        expect(email).to_have_value('a+b@example.test')
        assert server.reset_calls == 1, 'Failed form request must reach the local API exactly once'
        submit.click()
        page.wait_for_function("document.body.innerText.includes('重置邮件发送成功')")
        assert server.reset_calls == 2, 'The failed form must support a successful retry'
        page.screenshot(path=str(evidence / f'{theme}-{mode}-reset.png'), full_page=True)

        context.add_init_script('localStorage.setItem("user", ' + json.dumps(json.dumps(USER)) + ');')
        route = '/token' if theme == 'air' else '/panel/token'
        page.goto(base + route, wait_until='networkidle', timeout=60000)
        page.wait_for_function("document.querySelectorAll('.semi-spin-spinning,.MuiLinearProgress-root').length === 0")
        assert server.token_calls >= 1, 'The token failure must be exercised, not a login redirect'
        expect(page.get_by_text('quality-token', exact=True)).to_have_count(0)
        failed_calls = server.token_calls
        server.token_fail = False
        if theme == 'air':
            page.get_by_role('button', name='默认排序', exact=True).click()
            page.get_by_text('按剩余额度排序', exact=True).click()
        else:
            page.get_by_role('button', name='刷新', exact=True).click()
        expect(page.get_by_text('quality-token', exact=True)).to_be_visible(timeout=15000)
        assert server.token_calls > failed_calls, 'A visible retry must issue a new request'
        assert page.evaluate('document.styleSheets.length') > 0, 'Built CSS must load'
        page.screenshot(path=str(evidence / f'{theme}-{mode}-tokens.png'), full_page=True)
        if theme == 'air':
            page.goto(base + '/user', wait_until='networkidle', timeout=60000)
            page.get_by_role('button', name='添加用户', exact=True).click()
            username = page.locator('input[name="username"]:visible')
            password = page.locator('input[name="password"]:visible')
            username.fill('quality-user')
            password.fill('quality-password-for-fixture')
            create = page.get_by_role('button', name='提交', exact=True)
            with page.expect_response(lambda response: urlsplit(response.url).path == '/api/user/' and response.request.method == 'POST'):
                create.click()
            page.wait_for_function("document.querySelectorAll('.semi-spin-spinning').length === 0")
            expect(username).to_have_value('quality-user')
            assert server.user_calls == 1
            create.click()
            page.wait_for_function("document.body.innerText.includes('用户账户创建成功')")
            assert server.user_calls == 2, 'The failed UI event must support an explicit successful retry'
            page.screenshot(path=str(evidence / f'{theme}-{mode}-user-create.png'), full_page=True)
        assert not errors, f'Unexpected page errors in {theme}/{mode}: {errors}'
    except Exception:
        page.screenshot(path=str(evidence / f'{theme}-{mode}-failure.png'), full_page=True)
        (evidence / f'{theme}-{mode}-failure.html').write_text(page.content())
        raise
    finally:
        (evidence / f'{theme}-{mode}-errors.json').write_text(json.dumps(errors, indent=2))
        context.close()


def main() -> None:
    """main runs production and development acceptance with deterministic cleanup."""
    parser = argparse.ArgumentParser()
    parser.add_argument('theme', choices=['air', 'berry'])
    args = parser.parse_args()
    evidence = Path(os.environ.get('RUNNER_TEMP', '/tmp')) / f'legacy-browser-{args.theme}'
    evidence.mkdir(parents=True, exist_ok=True)
    handler = functools.partial(FixtureHandler, directory=str(ROOT / 'web/build' / args.theme))
    server = ThreadingHTTPServer(('127.0.0.1', 0), handler)
    server.reset_calls = 0
    server.token_calls = 0
    server.token_fail = True
    server.user_calls = 0
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    api = f'http://127.0.0.1:{server.server_port}'
    try:
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch()
            try:
                exercise(browser, api, server, args.theme, evidence, 'production')
                port = reserve_port()
                environment = {**os.environ, 'PORT': str(port), 'PROXY_TARGET': api, 'HOST': '127.0.0.1'}
                with (evidence / 'vite-dev.log').open('w') as output:
                    process = subprocess.Popen(['yarn', 'dev'], cwd=ROOT / 'web' / args.theme, env=environment, stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
                    with process_cleanup(process):
                        base = f'http://127.0.0.1:{port}'
                        for _ in range(120):
                            if process.poll() is not None:
                                raise RuntimeError('Vite development server exited unexpectedly')
                            try:
                                with urlopen(base, timeout=1) as response:
                                    if response.status == 200:
                                        break
                            except (URLError, TimeoutError):
                                time.sleep(0.25)
                        else:
                            raise RuntimeError('Vite development server did not become ready')
                        exercise(browser, base, server, args.theme, evidence, 'development')
                print(f'{args.theme}: production and development browser acceptance passed')
            finally:
                browser.close()
    finally:
        server.shutdown()
        server.server_close()
        worker.join(timeout=5)


if __name__ == '__main__':
    subprocess.run([sys.executable, str(Path(__file__).with_name('test_browser_process.py'))], check=True)
    main()
