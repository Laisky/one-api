"""Keep supplementary diagnostics separate from acceptance, with independently checkable windows."""
from datetime import datetime, timezone
from urllib.parse import parse_qs, urlsplit
import unittest
from unittest import mock

import dataset
import diagnose


class DiagnosticWindowTests(unittest.TestCase):
    """DiagnosticWindowTests preserve valid, disjoint keys and exact oracle binding without running a load."""

    def test_distinct_valid_seven_day_windows(self):
        """test_distinct_valid_seven_day_windows prevents accidental singleflight coalescing or an overlong ordinary-user range."""
        with mock.patch.object(dataset, 'rows_in_window', return_value=[]) as source:
            windows = diagnose.input_windows(200000)
        self.assertEqual(len(windows), 8)
        self.assertEqual(len({e.path for e in windows}), 8)
        expected = dataset.dashboard([], 2)
        for index, endpoint in enumerate(windows):
            parsed = parse_qs(urlsplit(endpoint.path).query)
            begin = datetime.strptime(parsed['from_date'][0], '%Y-%m-%d').replace(tzinfo=timezone.utc).timestamp()
            last = datetime.strptime(parsed['to_date'][0], '%Y-%m-%d').replace(tzinfo=timezone.utc).timestamp()
            end = dataset.END-index*7*86400
            self.assertEqual(begin, end-7*86400)
            self.assertEqual(last, end-86400)
            self.assertEqual(endpoint.principal, 2)
            self.assertEqual(source.call_args_list[index], mock.call(200000, 2, start=end-7*86400, end=end-1))
            endpoint.check({'success': True, 'message': '', 'data': expected})
            with self.assertRaises(ValueError):
                endpoint.check({'success': True, 'message': '', 'data': {}})

    def test_each_window_keeps_its_own_oracle(self):
        """test_each_window_keeps_its_own_oracle rejects a late-bound closure that validates every date against the final window."""
        values = [{'sentinel': i} for i in range(8)]
        with mock.patch.object(dataset, 'rows_in_window', return_value=[]), \
             mock.patch.object(dataset, 'dashboard', side_effect=values):
            windows = diagnose.input_windows(200000)
        with mock.patch.object(dataset, 'check_dashboard') as check:
            for i, endpoint in enumerate(windows):
                endpoint.check({'response': i})
                self.assertEqual(check.call_args, mock.call({'response': i}, values[i]))


if __name__ == '__main__':
    unittest.main()
