"""Behavior tests for isolated immutable builds and explicitly prepared offline caches."""
from pathlib import Path
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest


class CompareScriptTests(unittest.TestCase):
    """CompareScriptTests exercise real worktrees with observable compiler and runner stand-ins."""

    def setUp(self):
        """setUp creates a committed fixture and records external commands without building a gateway."""
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.repository, tools = self.root / 'repository', self.root / 'tools'
        self.repository.mkdir()
        tools.mkdir()
        self.script = Path(__file__).with_name('compare.sh').resolve()
        self.git('init', '-q')
        (self.repository / 'fixture').write_text('committed\n')
        fixture_scripts = self.repository / 'tests/stream-perf'
        fixture_scripts.mkdir(parents=True)
        shutil.copyfile(self.script.with_name('cache_tokens.py'), fixture_scripts / 'cache_tokens.py')
        self.git('add', '.')
        self.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-qm', 'fixture')
        self.revision = self.git('rev-parse', 'HEAD').strip()
        (self.repository / 'fixture').write_text('uncommitted\n')
        go = tools / 'go'
        go.write_text('#!' + sys.executable + '''
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
if args == ['env', 'GOVERSION']:
    print('go1.27.1')
else:
    with open(os.environ['BUILD_LOG'], 'a') as stream:
        stream.write(json.dumps({'args': args, 'toolchain': os.environ.get('GOTOOLCHAIN'),
            'fixture': Path('fixture').read_text(),
            'embed': Path('web/build/index.html').read_text() if Path('web/build/index.html').exists() else None}) + '\\n')
    if args[0] == 'build':
        if os.environ.get('FAIL_BUILD') == '1':
            sys.exit(42)
        Path(args[args.index('-o') + 1]).write_text('fixture binary')
    else:
        print('go version go1.27.1 linux/amd64')
''')
        go.chmod(0o755)
        runner = tools / 'python3'
        runner.write_text('#!' + sys.executable + '''
import json, os, subprocess, sys
from pathlib import Path
args = sys.argv[1:]
with open(os.environ['RUN_LOG'], 'a') as stream:
    stream.write(json.dumps(args) + '\\n')
if args and Path(args[0]).name == 'cache_tokens.py' and os.environ.get('CHECK_REAL_CACHE') == '1':
    if '--check-only' not in args:
        print('test forbids implicit tokenizer downloads', file=sys.stderr)
        sys.exit(99)
    sys.exit(subprocess.call([sys.executable, *args]))
''')
        runner.chmod(0o755)
        self.environment = {**os.environ, 'PATH': str(tools) + os.pathsep + os.environ['PATH'],
                            'BUILD_LOG': str(self.root / 'build.jsonl'), 'RUN_LOG': str(self.root / 'run.jsonl')}
        for name in ('TIKTOKEN_CACHE_DIR', 'CHECK_REAL_CACHE', 'FAIL_BUILD'):
            self.environment.pop(name, None)
        self.output = self.root / 'results'

    def git(self, *args):
        """git runs a bounded command in the fixture repository and returns its output."""
        return subprocess.check_output(['git', '-C', str(self.repository), *args], text=True, timeout=15)

    def run_comparison(self, **environment):
        """run_comparison invokes the actual comparison script with controlled external commands."""
        return subprocess.run(['bash', str(self.script), self.revision, str(self.output), '--repeats', '3'],
                              cwd=self.repository, env={**self.environment, **environment},
                              capture_output=True, text=True, timeout=30)

    def records(self, name):
        """records returns observed command records, treating an unused stand-in as an empty log."""
        path = self.root / name
        return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []

    def assert_worktrees_cleaned(self):
        """assert_worktrees_cleaned requires cleanup without altering the caller's uncommitted edit."""
        self.assertEqual(self.git('worktree', 'list', '--porcelain').count('worktree '), 1)
        self.assertEqual((self.repository / 'fixture').read_text(), 'uncommitted\n')

    def test_immutable_build_orchestration(self):
        """test_immutable_build_orchestration preserves source, toolchain, embeds and overwrite protection."""
        result = self.run_comparison()
        self.assertEqual(result.returncode, 0, result.stderr)
        builds = self.records('build.jsonl')
        self.assertEqual(len(builds), 4)
        self.assertTrue(all(entry['toolchain'] == 'go1.27.1' for entry in builds))
        self.assertEqual([entry['fixture'] for entry in builds[1:]], ['committed\n'] * 3)
        self.assertEqual(builds[1]['embed'], builds[2]['embed'])
        self.assertEqual((self.output / 'build/baseline-commit.txt').read_text().strip(), self.revision)
        preparation = [args for args in self.records('run.jsonl') if Path(args[0]).name == 'cache_tokens.py']
        self.assertEqual(len(preparation), 1)
        self.assertNotIn('--check-only', preparation[0])
        self.assertEqual(preparation[0][-1], str(self.output / 'build/token-cache'))
        self.assert_worktrees_cleaned()
        second = self.run_comparison()
        self.assertNotEqual(second.returncode, 0)
        self.assertIn('Refusing to overwrite', second.stderr)

    def test_prepared_cache_is_reused_read_only(self):
        """test_prepared_cache_is_reused_read_only resolves spaces and relative paths before changing worktrees."""
        cache = self.repository / 'prepared cache'
        cache.mkdir()
        sentinel = cache / 'sentinel'
        sentinel.write_text('do not modify\n')
        result = self.run_comparison(TIKTOKEN_CACHE_DIR='prepared cache')
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.records('run.jsonl')
        preparation = [args for args in calls if Path(args[0]).name == 'cache_tokens.py']
        self.assertEqual(len(preparation), 1)
        self.assertIn('--check-only', preparation[0])
        self.assertEqual(preparation[0][preparation[0].index('--cache') + 1], str(cache))
        measured = next(args for args in calls if Path(args[0]).name == 'run.py')
        self.assertEqual(measured[measured.index('--token-cache') + 1], str(cache))
        self.assertEqual((self.output / 'build/token-cache-path.txt').read_text().strip(), str(cache))
        self.assertFalse((self.output / 'build/token-cache').exists())
        self.assertEqual(sentinel.read_text(), 'do not modify\n')
        self.assert_worktrees_cleaned()

    def test_missing_prepared_cache_fails_without_download_or_build(self):
        """test_missing_prepared_cache_fails_without_download_or_build exercises the real offline validator."""
        cache = self.root / 'missing cache'
        result = self.run_comparison(TIKTOKEN_CACHE_DIR=str(cache), CHECK_REAL_CACHE='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('missing or corrupt tokenizer asset', result.stderr)
        self.assertNotIn('test forbids implicit', result.stderr)
        self.assertFalse(cache.exists())
        self.assertEqual(self.records('build.jsonl'), [])
        self.assertEqual(len(self.records('run.jsonl')), 1)
        self.assert_worktrees_cleaned()

    def test_corrupt_prepared_cache_is_not_repaired(self):
        """test_corrupt_prepared_cache_is_not_repaired rejects corruption without writes or measurements."""
        cache = self.root / 'corrupt cache'
        cache.mkdir()
        import cache_tokens
        name = next(iter(cache_tokens.ASSETS))
        url = f'https://openaipublic.blob.core.windows.net/encodings/{name}.tiktoken'
        filename = hashlib.sha1(url.encode()).hexdigest()
        asset = cache / filename
        asset.write_bytes(b'corrupt dictionary')
        result = self.run_comparison(TIKTOKEN_CACHE_DIR=str(cache), CHECK_REAL_CACHE='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('missing or corrupt tokenizer asset', result.stderr)
        self.assertNotIn('test forbids implicit', result.stderr)
        self.assertEqual(asset.read_bytes(), b'corrupt dictionary')
        self.assertEqual(self.records('build.jsonl'), [])
        self.assertEqual(len(self.records('run.jsonl')), 1)
        self.assert_worktrees_cleaned()

    def test_failed_build_does_not_run_measurements(self):
        """test_failed_build_does_not_run_measurements preserves incomplete evidence and cleans worktrees."""
        result = self.run_comparison(FAIL_BUILD='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(Path(args[0]).name == 'run.py' for args in self.records('run.jsonl')))
        self.assertTrue((self.output / 'build/baseline-commit.txt').exists())
        self.assert_worktrees_cleaned()


if __name__ == '__main__':
    unittest.main()
