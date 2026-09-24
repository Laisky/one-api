"""Behavior tests for isolated immutable builds without performing expensive gateway builds."""
from pathlib import Path
import json
import os
import subprocess
import tempfile
import unittest


class CompareScriptTests(unittest.TestCase):
    """CompareScriptTests verify toolchain pinning, worktree isolation and evidence overwrite protection."""
    def test_immutable_build_orchestration(self):
        """test_immutable_build_orchestration drives real Git worktrees with observable compiler/runner stand-ins."""
        script = Path(__file__).with_name('compare.sh').resolve()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository, tools = root / 'repository', root / 'tools'
            repository.mkdir()
            tools.mkdir()
            subprocess.run(['git', 'init', '-q', str(repository)], check=True)
            (repository / 'fixture').write_text('committed\n')
            subprocess.run(['git', '-C', str(repository), 'add', '.'], check=True)
            subprocess.run(['git', '-C', str(repository), '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                            'commit', '-qm', 'fixture'], check=True)
            revision = subprocess.check_output(['git', '-C', str(repository), 'rev-parse', 'HEAD'], text=True).strip()
            (repository / 'fixture').write_text('uncommitted\n')
            go = tools / 'go'
            go.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args=sys.argv[1:]
if args == ['env','GOVERSION']:
    print('go1.27.1')
else:
    with open(os.environ['BUILD_LOG'], 'a') as stream:
        stream.write(json.dumps({'args': args, 'toolchain': os.environ.get('GOTOOLCHAIN'),
            'fixture': Path('fixture').read_text(), 'embed': Path('web/build/index.html').read_text() if Path('web/build/index.html').exists() else None})+'\\n')
    if args[0] == 'build': Path(args[args.index('-o')+1]).write_text('fixture binary')
    else: print('go version go1.27.1 linux/amd64')
''')
            # Use an absolute interpreter so the stand-in runner does not intercept the compiler stub itself.
            import sys
            go.write_text(go.read_text().replace('#!/usr/bin/env python3', '#!' + sys.executable))
            go.chmod(0o755)
            runner = tools / 'python3'
            runner.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$RUN_LOG"\n')
            runner.chmod(0o755)
            environment = {**os.environ, 'PATH': str(tools) + os.pathsep + os.environ['PATH'],
                           'BUILD_LOG': str(root / 'build.jsonl'), 'RUN_LOG': str(root / 'run.log')}
            output = root / 'results'
            command = ['bash', str(script), revision, str(output), '--repeats', '3']
            result = subprocess.run(command, cwd=repository, env=environment, capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stderr)
            builds = [json.loads(line) for line in (root / 'build.jsonl').read_text().splitlines()]
            self.assertEqual(len(builds), 4)
            self.assertTrue(all(entry['toolchain'] == 'go1.27.1' for entry in builds))
            self.assertEqual([entry['fixture'] for entry in builds[1:]], ['committed\n'] * 3)
            self.assertEqual(builds[1]['embed'], builds[2]['embed'])
            self.assertEqual((repository / 'fixture').read_text(), 'uncommitted\n')
            self.assertEqual((output / 'build/baseline-commit.txt').read_text().strip(), revision)
            worktrees = subprocess.check_output(['git', '-C', str(repository), 'worktree', 'list', '--porcelain'], text=True)
            self.assertEqual(worktrees.count('worktree '), 1)
            second = subprocess.run(command, cwd=repository, env=environment, capture_output=True, text=True, timeout=30)
            self.assertNotEqual(second.returncode, 0)
            self.assertIn('Refusing to overwrite', second.stderr)


if __name__ == '__main__':
    unittest.main()
