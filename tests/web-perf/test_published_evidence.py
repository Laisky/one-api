"""Regression controls for the published Web API experiment's provenance and operational gates."""
import copy
import gzip
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).with_name('results') / '20260926-user-time-index'
SPEC = importlib.util.spec_from_file_location('web_index_audit', ROOT / 'verify.py')
audit = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(audit)


class PublishedEvidenceTests(unittest.TestCase):
    """PublishedEvidenceTests prevent incomplete or relabeled measurements from appearing accepted."""
    def setUp(self):
        """setUp loads fresh immutable summary and manifest values, never connecting to a gateway."""
        self.summary = json.loads(gzip.decompress((ROOT / 'summary.json.gz').read_bytes()))
        self.manifest = json.loads((ROOT / 'manifest.json').read_text())

    def test_complete_summary(self):
        """test_complete_summary reproduces the saved outcome without claiming raw requests were locally present."""
        result = audit.verify(ROOT)
        self.assertEqual(result['accepted'], self.manifest['accepted'])
        self.assertEqual(result['completed'], 51200)
        self.assertEqual(result['trials'], 160)
        self.assertFalse(result['raw_samples_verified'])

    def test_file_checksum(self):
        """test_file_checksum rejects corruption before parsing or recalculating candidate benefits."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'manifest.json').write_text(json.dumps(self.manifest))
            for name in self.manifest['files']:
                (root / name).write_bytes((ROOT / name).read_bytes())
            path = root / 'summary.json.gz'
            path.write_bytes(path.read_bytes() + b'corrupt')
            with self.assertRaises(ValueError):
                audit.verify(root)

    def test_identity_and_scope(self):
        """test_identity_and_scope rejects mixed binaries, datasets, principal changes and missing freshness checks."""
        mutations = (
            lambda s: s['binaries'].update(candidate='unmeasured'),
            lambda s: s['dataset'].update(logical_sha256='different'),
            lambda s: s['trials'][0].update(principal=99),
            lambda s: s['trials'][0].update(path='/api/other'),
            lambda s: s['freshness']['candidate'].update(legacy_rows_and_count=False),
            lambda s: s['schema']['candidate'].update(index_columns=['created_at', 'user_id', 'id']),
            lambda s: s['trials'].reverse(),
        )
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                summary = copy.deepcopy(self.summary)
                mutation(summary)
                with self.assertRaises(ValueError):
                    audit.validate_provenance(summary, self.manifest)

    def test_resource_arithmetic(self):
        """test_resource_arithmetic rejects made-up per-request CPU and invalid client observations."""
        for mutation in (
            lambda t: t['resources']['gateway'].update(cpu_ms_per_request=123456),
            lambda t: t['resources']['client'].update(cpu_seconds=float('nan')),
            lambda t: t['resources']['gateway'].update(peak_rss_mib=True),
        ):
            summary = copy.deepcopy(self.summary)
            mutation(summary['trials'][0])
            with self.assertRaises(ValueError):
                audit.validate_provenance(summary, self.manifest)

    def test_missing_matrix_and_qualification(self):
        """test_missing_matrix_and_qualification disallows completed-looking partial studies and missing auth checks."""
        for mutation in (
            lambda s: s.update(complete=False), lambda s: s['trials'].pop(),
            lambda s: s['trials'].append(copy.deepcopy(s['trials'][0])),
            lambda s: s['qualification']['candidate'].update(auth_and_tenant_isolation=False),
        ):
            summary = copy.deepcopy(self.summary)
            mutation(summary)
            with self.assertRaises(ValueError):
                audit.report.compare(summary)

    def test_control_latency_not_excused(self):
        """test_control_latency_not_excused preserves rejection even when personal endpoints improve."""
        for trial in self.summary['trials']:
            if trial['label'] == 'candidate' and trial['endpoint'] == 'admin-logs':
                trial['latency_ms'] = dict.fromkeys(('p50', 'p95', 'p99', 'max'), 100000)
        self.assertFalse(audit.report.compare(self.summary)['accepted'])

    def test_memory_regression_not_excused(self):
        """test_memory_regression_not_excused keeps the 20-percent RSS guard independent from target latency gains."""
        for trial in self.summary['trials']:
            if trial['label'] == 'candidate':
                trial['resources']['gateway']['peak_rss_mib'] = 100000
        self.assertFalse(audit.report.compare(self.summary)['accepted'])

    def test_equal_latency_is_not_a_material_benefit(self):
        """test_equal_latency_is_not_a_material_benefit requires the preregistered magnitude and repeatability."""
        for trial in self.summary['trials']:
            trial['latency_ms'] = dict.fromkeys(('p50', 'p95', 'p99', 'max'), 100)
        self.assertFalse(audit.report.compare(self.summary)['accepted'])


if __name__ == '__main__':
    unittest.main()
