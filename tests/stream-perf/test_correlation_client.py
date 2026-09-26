"""Independent controls for client-side state attribution without cross-clock assumptions."""
import copy
import unittest

from correlation_client import combine
from correlation_decode import select
from test_correlation_analyze import fixture


def paired_fixture(state='Runnable'):
    """paired_fixture supplies a known client interval on a deliberately different trace-clock epoch."""
    gateway, requests = fixture()
    client = []
    times = requests['samples'][0]['observed_event_ns']
    for frame in (1, 2, 3):
        client.append({'kind':'Log', 'time':times[frame]+5_000_000_000, 'mono':times[frame], 'phase':'observe',
                       'request':0, 'frame':frame, 'g':30})
    for ms, before, after, reason in [(0,'Undetermined','Running',''),(2,'Running',state,'network' if state == 'Waiting' else 'preempted'),
                                     (29,state,'Running',''),(35,'Running','Waiting','network')]:
        client.append({'kind':'StateTransition','time':6_000_000_000+int(ms*1e6),'g':30,'target_g':30,
                       'before':before,'after':after,'reason':reason})
    client.sort(key=lambda e:e['time'])
    return gateway, client, requests


class ClientCorrelationTests(unittest.TestCase):
    """ClientCorrelationTests separate runnable wait, network wait, skew and missing observations."""

    def test_runnable_client_is_distinct_from_gateway(self):
        """test_runnable_client_is_distinct_from_gateway uses known nonidentical intervals in each process."""
        gateway, client, requests = paired_fixture()
        result = combine(gateway, client, requests, 3)
        self.assertFalse(result['absolute_cross_process_comparison'])
        self.assertEqual(result['matched_adjacent_pairs'], 2)
        self.assertEqual(result['client_gap_exceeds_gateway_gap_by_5ms']['client_runnable_at_least_80pct'], 1)
        row = result['pairs'][0]
        self.assertAlmostEqual(row['client_gap_ms'], 28.7)
        self.assertAlmostEqual(row['client_runnable_ms'], 27)
        self.assertAlmostEqual(row['runnable_ms'], 20.1)
        self.assertNotIn('one_way_latency_ms', row)

    def test_network_wait_is_not_runnable(self):
        """test_network_wait_is_not_runnable preserves the runtime's reason without asserting network causality."""
        result = combine(*paired_fixture('Waiting'), 3)
        self.assertEqual(result['usable_stalls']['client_runnable_at_least_80pct'], 0)
        self.assertEqual(result['usable_stalls']['client_network_waiting_at_least_80pct'], 1)

    def test_independent_epoch_offsets_do_not_change_intervals(self):
        """test_independent_epoch_offsets_do_not_change_intervals shifts client and trace clocks independently."""
        gateway, client, requests = paired_fixture()
        expected = combine(gateway, client, requests, 3)
        for offset in (5_000_000_000_000, -500_000_000):
            shifted = copy.deepcopy(requests)
            shifted['samples'][0]['observed_event_ns'] = [v+offset for v in shifted['samples'][0]['observed_event_ns']]
            changed = copy.deepcopy(client)
            for event in changed:
                event['time'] += 700_000_000
                if 'mono' in event:
                    event['mono'] += offset
            self.assertEqual(combine(gateway, changed, shifted, 3), expected)

    def test_invalid_client_frames_fail_closed(self):
        """test_invalid_client_frames_fail_closed rejects duplicate, foreign, changed-clock and changed-goroutine markers."""
        for mutation in ('duplicate','request','phase','clock','goroutine'):
            gateway, client, requests = paired_fixture()
            marker = next(e for e in client if e['kind']=='Log')
            if mutation == 'duplicate':
                client.append(copy.deepcopy(marker));client.sort(key=lambda e:e['time'])
            elif mutation == 'request': marker['request']=32
            elif mutation == 'phase': marker['phase']='begin'
            elif mutation == 'clock': marker['mono']+=1
            else: marker['g']=99
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                combine(gateway, client, requests, 3)

    def test_boundary_and_skew_exclusions_are_explicit(self):
        """test_boundary_and_skew_exclusions_are_explicit never imputes an unseen client marker or reliable alignment."""
        gateway, client, requests = paired_fixture()
        missing = [e for e in client if not(e.get('kind')=='Log' and e.get('frame')==1)]
        result = combine(gateway, missing, requests, 3)
        self.assertEqual(result['unmatched_trace_boundary_pairs'], 1)
        marker = next(e for e in client if e.get('frame')==2)
        marker['time'] -= 2_000_000
        client.sort(key=lambda e:e['time'])
        result = combine(gateway, client, requests, 3)
        self.assertEqual(result['client_alignment_excluded_pairs'], 2)
        self.assertNotIn('client_runnable_ms',result['pairs'][0])
        self.assertEqual(result['usable_stalls']['pairs'], 0)

    def test_decoder_numeric_client_schema(self):
        """test_decoder_numeric_client_schema validates the numeric-only bounded observer channel."""
        row = select('M=1 P=0 G=30 Log Time=100 Category="oneapi.sse.client" Message="32/7/observe/900"')
        self.assertEqual(row['phase'],'observe');self.assertEqual(row['request'],32)
        for message in ('32/7/observe/0','1/7/observe/900','32/2048/observe/900','0/0/payload/900'):
            with self.assertRaises(ValueError):
                select(f'M=1 P=0 G=30 Log Time=100 Category="oneapi.sse.client" Message="{message}"')
