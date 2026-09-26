"""Fail-closed controls for the independent correlation input audit."""
import copy
import hashlib
import unittest

from correlation_audit import check_inputs, METRICS
from profile_window import stable_window


def fixture():
    """fixture creates complete synthetic request/counter/capture evidence with known quantiles."""
    rows = [{'elapsed':float(i), 'processes':{'gateway':{'cpu_seconds':float(i*3)}},
             'upstream':{'completed':i,'active':1}, 'durable_requests':i} for i in range(92)]
    samples = [{'index':i, **{k:float(i+1) for k in METRICS}} for i in range(4)]
    requests = {'offered':4,'completed':4,'failed':0,'dropped':0,'seconds':2.0,'successful_rps':2.0,
                **{k:{'p50':2.,'p95':4.,'p99':4.,'max':4.} for k in METRICS}, 'samples':samples}
    trace = b'synthetic trace'
    summary = {'complete':True,'sustained_protocol_qualified':True,
        'configuration':{'mode':'trace','requests':4,'chunks':3,'warmup':30,'seconds':60,'trace_seconds':5,'gateway_procs':3},
        'requests':{k:v for k,v in requests.items() if k!='samples'},'usage':{'requests':4,'used_quota':100},
        'qualification':{'normal':0,'crlf':0,'fragmented':0,'missing-done':8,'wrong-content':8,'malformed':8,
                         'cancellation':'8/8 producers released within 5 seconds'},
        'profile_captures':[{'file':'runtime.trace','sha256':hashlib.sha256(trace).hexdigest(),
                             'bytes':len(trace),'started_elapsed':45.,'finished_elapsed':50.}],
        'trace_window_within_observation':True,'profile_window_completed_while_load_alive':True,
        'allowance':{'effective_cores':4.},'window':stable_window(rows,4.,3,30.,60.)}
    return summary,requests,rows,trace


class CorrelationAuditTests(unittest.TestCase):
    """CorrelationAuditTests validate raw samples and counters instead of only accepting metadata success flags."""

    def test_valid_evidence_is_reproduced(self):
        """test_valid_evidence_is_reproduced independently derives the recorded and nominal window."""
        result=check_inputs(*fixture())
        self.assertEqual(result['request_samples'],4)
        self.assertTrue(result['recorded_window_reproduced'])
        self.assertTrue(result['nominal_window_qualified'])
        self.assertFalse(result['financial_ledger_audit'])

    def test_corruption_cannot_be_hidden_by_complete(self):
        """test_corruption_cannot_be_hidden_by_complete exercises each major raw-evidence trust boundary."""
        for failure in ('quantile','duplicate_index','sample_error','usage','qualification','clock_rollback',
                        'window_mean','trace_hash','trace_bytes','capture_outside','not_alive','rps'):
            with self.subTest(failure=failure):
                s,q,r,t=fixture()
                if failure=='quantile':q['samples'][0]['ttft_ms']=100.
                if failure=='duplicate_index':q['samples'][0]['index']=1
                if failure=='sample_error':q['samples'][0]['error']='failed'
                if failure=='usage':s['usage']['requests']=3
                if failure=='qualification':s['qualification']['normal']=1
                if failure=='clock_rollback':r[40]['processes']['gateway']['cpu_seconds']=0
                if failure=='window_mean':s['window']['gateway_cores_mean']=4.
                if failure=='trace_hash':t=b'wrong trace'
                if failure=='trace_bytes':s['profile_captures'][0]['bytes']=1
                if failure=='capture_outside':s['profile_captures'][0]['finished_elapsed']=91.
                if failure=='not_alive':s['profile_window_completed_while_load_alive']=False
                if failure=='rps':q['successful_rps']=3;s['requests']['successful_rps']=3
                with self.assertRaises(ValueError):check_inputs(s,q,r,t)
