"""Require explicit offline assets and bounded fixture knobs before provisioning a gateway."""
from pathlib import Path
import sqlite3
import subprocess
import tempfile
import unittest
from unittest import mock

import costs
import http_fixture as h


class PreflightTests(unittest.TestCase):
    """PreflightTests prevent invalid controls or missing assets from becoming hidden network/setup work."""
    def test_read_only_asset_validator(self):
        """test_read_only_asset_validator invokes the repository's existing checker in check-only mode."""
        with mock.patch.object(h.subprocess,'run') as call:
            h.check_cache(Path('/provided-cache'))
        args=call.call_args.args[0]
        self.assertEqual(args[-3:],['--cache','/provided-cache','--check-only'])
        self.assertTrue(call.call_args.kwargs['check'])
        self.assertEqual(call.call_args.kwargs['timeout'],30)

    def test_invalid_slots_start_no_process(self):
        """test_invalid_slots_start_no_process rejects booleans, fractions and unbounded profiler controls."""
        for value in (0,65,True,2.5,'3'):
            with mock.patch.object(h.subprocess,'Popen') as start:
                with self.assertRaises(ValueError):
                    with h.gateway(Path('/unused'),Path('/unused'),gateway_procs=value):
                        self.fail('invalid fixture entered')
                start.assert_not_called()

    def test_missing_cache_does_not_start_gateway(self):
        """test_missing_cache_does_not_start_gateway fails before creating private state or starting the API process."""
        with tempfile.TemporaryDirectory() as root:
            directory=Path(root)/'not-created'
            with mock.patch.object(h,'check_cache',side_effect=ValueError('invalid cache')),mock.patch.object(h.subprocess,'Popen') as start:
                with self.assertRaises(ValueError):
                    with h.gateway(Path('/unused'),directory):
                        self.fail('missing cache accepted')
                start.assert_not_called();self.assertFalse(directory.exists())

    def test_insert_cost_counts_committed_rows(self):
        """test_insert_cost_counts_committed_rows checks cost measurement integrity without a production gateway or large data."""
        with tempfile.TemporaryDirectory() as root:
            database=Path(root)/'fixture.db'
            with sqlite3.connect(database) as db:
                db.execute('CREATE TABLE logs(id INTEGER PRIMARY KEY,content TEXT)')
            result=costs.append_cost(database,[{'id':1,'content':'one'},{'id':2,'content':'two'}])
            self.assertEqual(result['rows'],2)
            self.assertGreater(result['seconds'],0)
            with sqlite3.connect(database) as db:self.assertEqual(db.execute('SELECT content FROM logs ORDER BY id').fetchall(),[('one',),('two',)])


if __name__=='__main__':unittest.main()
