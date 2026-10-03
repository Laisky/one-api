"""Keep immutable comparison identities authoritative after caller workload options."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


class ManagedArgumentsTests(unittest.TestCase):
    """ManagedArgumentsTests observe actual shell invocation and argparse's last-value behavior."""

    def exercise(self, style):
        """exercise runs real disposable worktrees while compiler and Python stand-ins record arguments."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository, tools, cache = root / 'repository', root / 'tools', root / 'validated cache'
            for path in (repository, tools, cache):
                path.mkdir()
            scripts = repository / 'tests/stream-perf'
            scripts.mkdir(parents=True)
            script = Path(__file__).with_name('compare.sh').resolve()
            shutil.copyfile(script.with_name('cache_tokens.py'), scripts / 'cache_tokens.py')
            subprocess.run(['git', 'init', '-q', str(repository)], check=True, timeout=15)
            subprocess.run(['git', '-C', str(repository), 'add', '.'], check=True, timeout=15)
            subprocess.run(['git', '-C', str(repository), '-c', 'user.name=Fixture',
                            '-c', 'user.email=fixture@example.invalid', 'commit', '-qm', 'fixture'],
                           check=True, timeout=15)
            revision = subprocess.check_output(['git', '-C', str(repository), 'rev-parse', 'HEAD'],
                                               text=True, timeout=15).strip()
            go = tools / 'go'
            go.write_text('#!' + sys.executable + '''
import sys
from pathlib import Path
args = sys.argv[1:]
if args == ['env', 'GOVERSION']:
    print('go1.27.1')
elif args[0] == 'build':
    Path(args[args.index('-o') + 1]).write_text('immutable fixture binary')
else:
    print('go version go1.27.1 linux/amd64')
''')
            runner = tools / 'python3'
            runner.write_text('#!' + sys.executable + '''
import json, os, sys
from pathlib import Path
with open(os.environ['ARGUMENT_LOG'], 'a') as stream:
    stream.write(json.dumps(sys.argv[1:]) + '\\n')
''')
            go.chmod(0o755)
            runner.chmod(0o755)
            output = root / 'output with spaces'
            controlled = {
                'binary': output / 'build/one-api-candidate',
                'baseline': output / 'build/one-api-baseline',
                'driver': output / 'build/stream-perf',
                'token-cache': cache,
                'output': output / 'results',
            }
            short = {'binary': 'bina', 'baseline': 'basel', 'driver': 'dri',
                     'token-cache': 'token-c', 'output': 'out'}
            options = ['--concurrency', '8,64', '--repeats', '5']
            for name in controlled:
                key = '--' + (short[name] if style == 'abbreviated' else name)
                value = str(root / ('unvalidated ' + name))
                options.extend([key + '=' + value] if style == 'equals' else [key, value])
            environment = {**os.environ, 'PATH': str(tools) + os.pathsep + os.environ['PATH'],
                           'TIKTOKEN_CACHE_DIR': str(cache), 'ARGUMENT_LOG': str(root / 'calls.jsonl')}
            result = subprocess.run(['bash', str(script), revision, str(output), *options],
                                    cwd=repository, env=environment, capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stderr)
            calls = [json.loads(line) for line in (root / 'calls.jsonl').read_text().splitlines()]
            measured = next(args for args in calls if Path(args[0]).name == 'run.py')
            parser = argparse.ArgumentParser()
            for name in controlled:
                parser.add_argument('--' + name)
            parser.add_argument('--concurrency')
            parser.add_argument('--repeats', type=int)
            actual = vars(parser.parse_args(measured[1:]))
            for name, expected in controlled.items():
                self.assertEqual(actual[name.replace('-', '_')], str(expected), name)
            self.assertEqual(actual['concurrency'], '8,64')
            self.assertEqual(actual['repeats'], 5)
            validated = next(args for args in calls if Path(args[0]).name == 'cache_tokens.py')
            self.assertIn('--check-only', validated)
            self.assertEqual(validated[validated.index('--cache') + 1], actual['token_cache'])
            self.assertEqual((output / 'build/token-cache-path.txt').read_text().strip(), actual['token_cache'])
            worktrees = subprocess.check_output(['git', '-C', str(repository), 'worktree', 'list', '--porcelain'],
                                                text=True, timeout=15)
            self.assertEqual(worktrees.count('worktree '), 1)

    def test_separate_options_cannot_replace_managed_paths(self):
        """test_separate_options_cannot_replace_managed_paths rejects effective last-value replacement."""
        self.exercise('separate')

    def test_equals_options_cannot_replace_managed_paths(self):
        """test_equals_options_cannot_replace_managed_paths covers argparse's --name=value spelling."""
        self.exercise('equals')

    def test_abbreviations_cannot_replace_managed_paths(self):
        """test_abbreviations_cannot_replace_managed_paths covers accepted unambiguous option prefixes."""
        self.exercise('abbreviated')


if __name__ == '__main__':
    unittest.main()
