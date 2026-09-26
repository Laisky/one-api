"""Synthetic controls distinguish observed gap arithmetic from unsupported one-way or GC attribution."""
import copy
import unittest

from correlation_analyze import correlate, overlap, union


def fixture():
    """fixture supplies one sampled request with a known runnable interval and delayed client observation."""
    base = 1_000_000_000
    def at(milliseconds):
        """at maps synthetic milliseconds to integer monotonic nanoseconds."""
        return base + int(milliseconds * 1_000_000)
    events = []
    for frame, begin in [(1, 1), (2, 22), (3, 23)]:
        for phase, offset in [('begin', 0), ('flush_start', .2), ('flush_end', .4)]:
            events.append({'kind': 'Log', 'request': 0, 'frame': frame, 'phase': phase, 'g': 10,
                           'mono': at(begin + offset), 'time': at(begin + offset) + 10_000})
    for when, before, after, reason in [(0,'Undetermined','Running',''),(1.6,'Running','Runnable','runtime.Gosched'),
                                      (21.7,'Runnable','Running',''),(35,'Running','Waiting','network')]:
        events.append({'kind':'StateTransition','time':at(when)+10_000,'g':10,'target_g':10,
                       'before':before,'after':after,'reason':reason})
    for kind,when,name,scope in [('RangeBegin',10,'stop-the-world (GC)','Goroutine(20)'),
                                 ('RangeEnd',11,'stop-the-world (GC)','Goroutine(20)'),
                                 ('RangeBegin',16,'GC mark assist','Goroutine(10)'),
                                 ('RangeEnd',18,'GC mark assist','Goroutine(10)')]:
        events.append({'kind':kind,'time':at(when)+10_000,'g':10,'name':name,'scope':scope})
    events.sort(key=lambda e:e['time'])
    requests={'failed':0,'dropped':0,'completed':1,'offered':1,
              'samples':[{'index':0,'observed_event_ns':[at(x) for x in (.5,1.3,30,31,32,33,34)]}]}
    return events,requests


class CorrelationAnalyzeTests(unittest.TestCase):
    """CorrelationAnalyzeTests enforce exact matching, additive interval identities and explicit boundary exclusions."""

    def test_gap_decomposition_and_overlaps(self):
        """test_gap_decomposition_and_overlaps compares independent known intervals without summing GC and scheduler time."""
        events,requests=fixture()
        result=correlate(events,requests,3)
        self.assertEqual(result['adjacent_content_pairs'],2)
        stall=result['pairs'][0]
        self.assertAlmostEqual(stall['client_gap_ms'],28.7)
        self.assertAlmostEqual(stall['gateway_flush_gap_ms'],21)
        self.assertAlmostEqual(stall['downstream_lag_change_ms'],7.7)
        self.assertAlmostEqual(stall['runnable_ms'],20.1)
        self.assertAlmostEqual(stall['goschedrunnable_ms'],20.1)
        self.assertAlmostEqual(stall['stw_overlap_ms'],1)
        self.assertAlmostEqual(stall['mark_assist_overlap_ms'],2)
        self.assertEqual(result['stalls_above_10ms'],1)
        self.assertNotIn('begin_to_client_ms',result['frame_metrics'])

    def test_unknown_epoch_offsets_cancel(self):
        """test_unknown_epoch_offsets_cancel forbids inferred one-way latency and preserves differences after a huge clock shift."""
        events,requests=fixture()
        expected=correlate(events,requests,3)
        for offset in (5_000_000_000_000,-500_000_000):
            shifted=copy.deepcopy(requests)
            shifted['samples'][0]['observed_event_ns']=[n+offset for n in shifted['samples'][0]['observed_event_ns']]
            self.assertEqual(correlate(events,shifted,3),expected)

    def test_client_can_receive_before_flush_returns(self):
        """test_client_can_receive_before_flush_returns preserves negative signed post-flush lag without clipping it."""
        events,requests=fixture()
        result=correlate(events,requests,3,absolute_clock=True)
        self.assertAlmostEqual(result['frame_metrics']['client_minus_flush_return_ms']['min'],-.1)
        shifted=copy.deepcopy(requests)
        shifted['samples'][0]['observed_event_ns']=[n-500_000_000 for n in shifted['samples'][0]['observed_event_ns']]
        with self.assertRaisesRegex(ValueError,'precedes'):
            correlate(events,shifted,3,absolute_clock=True)

    def test_corrupt_inputs_fail_closed(self):
        """test_corrupt_inputs_fail_closed rejects duplicate identities, missing frames, wrong order and mixed goroutines."""
        for kind in ('duplicate_marker','missing_client_frame','wrong_phase_order','changed_g','wrong_client_index','clock_rollback','state_mismatch'):
            with self.subTest(kind=kind):
                events,requests=fixture()
                marks=[e for e in events if e['kind']=='Log']
                if kind=='duplicate_marker':events.append(copy.deepcopy(marks[0]));events.sort(key=lambda e:e['time'])
                if kind=='missing_client_frame':requests['samples'][0]['observed_event_ns'].pop()
                if kind=='wrong_phase_order':marks[0]['mono']=marks[1]['mono']+1
                if kind=='changed_g':marks[1]['g']=999
                if kind=='wrong_client_index':requests['samples'][0]['index']=32
                if kind=='clock_rollback':requests['samples'][0]['observed_event_ns'][2]=1
                if kind=='state_mismatch':next(e for e in events if e.get('after')=='Runnable')['before']='Waiting'
                with self.assertRaises(ValueError):correlate(events,requests,3)

    def test_incomplete_boundary_frame_is_not_imputed(self):
        """test_incomplete_boundary_frame_is_not_imputed excludes incomplete marker/range observations without inventing durations."""
        events,requests=fixture()
        events.append({'kind':'Log','request':0,'frame':0,'phase':'flush_end','g':10,'time':1_000_010_000,'mono':1_000_000_000})
        events.append({'kind':'RangeEnd','time':1_000_020_000,'g':10,'name':'GC mark assist','scope':'Goroutine(10)'})
        events.sort(key=lambda e:e['time'])
        result=correlate(events,requests,3)
        self.assertEqual(result['incomplete_boundary_frames'],1)
        self.assertEqual(result['excluded_runtime_ranges']['unmatched_range_ends'],1)
        self.assertEqual(result['adjacent_content_pairs'],2)

    def test_marker_skew_does_not_invent_state_attribution(self):
        """test_marker_skew_does_not_invent_state_attribution preserves client gaps but excludes poorly aligned state overlays."""
        events,requests=fixture()
        for event in events:
            if event.get('frame')==2:event['mono']-=2_000_000
        result=correlate(events,requests,3)
        self.assertEqual(result['alignment_excluded_pairs'],2)
        self.assertNotIn('runnable_ms',result['pairs'][0])
        self.assertEqual(result['stalls_above_10ms'],1)

    def test_overlapping_global_ranges_are_unioned(self):
        """test_overlapping_global_ranges_are_unioned never double counts overlapping scopes as extra wall time."""
        merged=union([(10,20),(15,25),(30,40)])
        self.assertEqual(merged,[(10,25),(30,40)])
        self.assertEqual(overlap(merged,[10,30],0,100),25/1e6)
