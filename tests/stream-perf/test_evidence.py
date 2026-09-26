"""Regression tests for the standalone historical evidence verifier."""
import csv
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


class EvidenceTests(unittest.TestCase):
    """EvidenceTests keep the committed evidence independently auditable without weakening live comparison gates."""
    def test_committed_evidence(self):
        """test_committed_evidence executes the verifier against all preserved measured records."""
        source = Path(__file__).parent / 'results/20260924'
        result = subprocess.run([sys.executable, str(source / 'verify.py')], capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('Validated all 142 run-level records.', result.stdout)
        self.assertIn('not a fresh correctness qualification', result.stdout)

    def test_corruption_and_whole_cell_omission(self):
        """test_corruption_and_whole_cell_omission rejects altered hashes, missing planned cells and unequal billing."""
        source = Path(__file__).parent / 'results/20260924'
        for mode in ('hash', 'missing-cell', 'billing'):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                target = Path(directory)
                for filename in ('verify.py', 'manifest.json', 'runs.csv'):
                    shutil.copyfile(source / filename, target / filename)
                if mode == 'hash':
                    with (target / 'runs.csv').open('a') as stream:
                        stream.write('\n')
                else:
                    with (target / 'runs.csv').open(newline='') as stream:
                        reader = csv.DictReader(stream)
                        fields, rows = reader.fieldnames, list(reader)
                    if mode == 'missing-cell':
                        rows = [row for row in rows if not (row['experiment'] == 'main-comparison'
                                and row['profile'] == 'paced' and row['concurrency'] == '64')]
                    else:
                        rows[1]['used_quota'] = str(int(rows[1]['used_quota']) + 1)
                    with (target / 'runs.csv').open('w', newline='') as stream:
                        writer = csv.DictWriter(stream, fields)
                        writer.writeheader()
                        writer.writerows(rows)
                    manifest = json.loads((target / 'manifest.json').read_text())
                    manifest['runs_csv_sha256'] = hashlib.sha256((target / 'runs.csv').read_bytes()).hexdigest()
                    (target / 'manifest.json').write_text(json.dumps(manifest))
                result = subprocess.run([sys.executable, str(target / 'verify.py')], capture_output=True, text=True, timeout=10)
                self.assertNotEqual(result.returncode, 0)
                expected = {'hash': 'hash mismatch', 'missing-cell': 'incomplete experiment', 'billing': 'paired durable billing differs'}
                self.assertIn(expected[mode], result.stderr)


if __name__ == '__main__':
    unittest.main()
