"""Negative controls for deterministic fixture oracles, paired decisions and persistent load workers."""
import copy
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import threading
import unittest

import dataset as d
from http_fixture import require
import measure
import report
from workload import Endpoint


class FixtureTests(unittest.TestCase):
    """FixtureTests require independent exact rows, full aggregation and fail-closed response validation."""
    def test_row_determinism_and_bounds(self):
        """test_row_determinism_and_bounds keeps endpoints, UUIDs, density and payload size reproducible."""
        self.assertEqual(d.log_row(1),d.log_row(1))
        self.assertEqual(len(d.log_row(1)['content'].encode()),1024)
        self.assertEqual(d.log_row(1)['created_at'],d.ORIGIN)
        self.assertLess(d.log_row(200000)['created_at'],d.END)
        self.assertEqual(sum(d.log_row(i)['user_id']==2 for i in range(1,101)),80)
        self.assertNotIn('id',d.public_row(d.log_row(1)))

    def test_manual_aggregate_oracle(self):
        """test_manual_aggregate_oracle verifies independent count, cache and tool accounting with hand-computed values."""
        a=d.log_row(1);a.update(created_at=d.START,type=2,quota=7,prompt_tokens=11,completion_tokens=13,cached_prompt_tokens=3)
        b=copy.deepcopy(a);b.update(quota=17,cached_prompt_tokens=0)
        tool=copy.deepcopy(a);tool.update(type=7,quota=31)
        ignored=copy.deepcopy(a);ignored.update(type=6,quota=999)
        out=d.dashboard([a,b,tool,ignored],2)
        v=out['logs'][0]
        self.assertEqual({k:v[k] for k in d.METRICS},dict(RequestCount=2,Quota=24,PromptTokens=22,CompletionTokens=26,CachedPromptTokens=3,CacheHitCount=1,CacheHitQuota=7))
        self.assertEqual(out['tool_logs'][0]['Quota'],31)
        d.check_dashboard({'success':True,'message':'','data':out},out)
        bad=copy.deepcopy(out);bad['token_logs'][0]['Quota']+=1
        with self.assertRaises(ValueError):d.check_dashboard({'success':True,'message':'','data':bad},out)

    def test_log_mutations_and_count_semantics(self):
        """test_log_mutations_and_count_semantics rejects wrong ownership, missing fields, order and false totals."""
        rows=[d.log_row(9),d.log_row(8)]
        good={'success':True,'message':'','data':[d.public_row(r) for r in rows],'total':2}
        d.check_logs(good,rows)
        for change in (lambda b:b.update(total=1),lambda b:b['data'].reverse(),lambda b:b['data'][0].update(user_uuid='foreign'),lambda b:b['data'][0].pop('content')):
            bad=copy.deepcopy(good);change(bad)
            with self.assertRaises(ValueError):d.check_logs(bad,rows)
        good.update(version=1,has_more=False,next_cursor='',count={'value':2,'quality':'exact','as_of':1,'cached':False})
        d.check_logs(good,rows,cursor=True)
        good['count'].update(value=0)
        with self.assertRaises(ValueError):d.check_logs(good,rows,cursor=True)

    def test_inclusive_window_and_provisional_exclusion(self):
        """test_inclusive_window_and_provisional_exclusion retains equality at supplied bounds without provisional rows."""
        a=d.log_row(180001)
        rows=d.rows_in_window(200000,start=a['created_at'],end=a['created_at'])
        self.assertEqual([r['id'] for r in rows],[a['id']])
        a=d.log_row(180006);self.assertEqual(a['id']%19,0)
        self.assertEqual(d.rows_in_window(200000,start=a['created_at'],end=a['created_at']),[])


def evidence() -> dict:
    """evidence creates a small complete synthetic matrix whose arithmetic is independent of saved measurements."""
    s={'complete':True,'configuration':{'rows':200000,'repeats':5,'concurrency':'1,8','target_requests':128,'control_requests':512},
       'binaries':{'baseline':'base','candidate':'new'},'qualification':{},'schema':{},'freshness':{},'trials':[]}
    for label in s['binaries']:
        s['qualification'][label]=dict.fromkeys(('complete','all_eight_endpoints','auth_and_tenant_isolation','cursor_continuation_and_cross_user_rejection','empty_shape'),True)
        s['schema'][label]={'rows':200000,'index_names':[d.INDEX] if label=='candidate' else [],
                            'index_columns':['user_id','created_at','id'] if label=='candidate' else []}
        s['freshness'][label]=dict.fromkeys(('committed_insert','legacy_rows_and_count','all_dashboard_aggregates'),True)
        for e in report.TARGETS|report.CONTROLS:
            for c in (1,8):
                for r in range(5):
                    n=128 if e in report.TARGETS else 512
                    ms=50 if label=='candidate' and e in report.TARGETS else 100
                    s['trials'].append({'label':label,'endpoint':e,'target':e in report.TARGETS,'concurrency':c,'repeat':r,
                        'binary_sha256':s['binaries'][label],'offered':n,'completed':n,'failed':0,'dropped':0,'seconds':1,'successful_rps':n,
                        'latency_ms':dict.fromkeys(('p50','p95','p99','max'),ms),
                        'resources':{'gateway':{'cpu_ms_per_request':2,'peak_rss_mib':100}}})
    return s


class AuditTests(unittest.TestCase):
    """AuditTests prevent incomplete, incorrect or harmful performance data from earning acceptance."""
    def test_complete_and_repeat_consistent(self):
        """test_complete_and_repeat_consistent accepts known benefits but rejects equal performance."""
        s=evidence();out=report.compare(s);self.assertTrue(out['accepted']);self.assertEqual(out['completed'],51200)
        for t in s['trials']:t['latency_ms']=dict.fromkeys(('p50','p95','p99','max'),100)
        self.assertFalse(report.compare(s)['accepted'])

    def test_corrupted_matrix(self):
        """test_corrupted_matrix exercises omissions, duplicates, binary drift, bad requests and failed qualification."""
        changes=(lambda s:s.update(complete=False),lambda s:s['trials'].pop(),lambda s:s['trials'].append(copy.deepcopy(s['trials'][0])),
                 lambda s:s['trials'][0].update(binary_sha256='other'),lambda s:s['trials'][0].update(failed=1),
                 lambda s:s['qualification']['baseline'].update(complete=False),lambda s:s['trials'][0]['latency_ms'].update(p95=float('nan')))
        for change in changes:
            s=evidence();change(s)
            with self.assertRaises(ValueError):report.compare(s)

    def test_control_latency_and_memory_gates(self):
        """test_control_latency_and_memory_gates forbids target gains from excusing control or resident-memory regressions."""
        for field in ('latency','rss'):
            s=evidence()
            for t in s['trials']:
                if t['label']=='candidate' and t['endpoint']=='admin-logs':
                    if field=='latency':t['latency_ms']=dict.fromkeys(('p50','p95','p99','max'),130)
                    else:t['resources']['gateway']['peak_rss_mib']=130
            self.assertFalse(report.compare(s)['accepted'])


class WorkerTests(unittest.TestCase):
    """WorkerTests use a real local HTTP server to inspect warmups, connection reuse and request identities."""
    def test_verified_persistent_workers(self):
        """test_verified_persistent_workers requires two pre-barrier warmups per independently persistent connection."""
        counts={};lock=threading.Lock()
        class Handler(BaseHTTPRequestHandler):
            """Handler records each connection's request count and emits a deterministic tiny response."""
            protocol_version='HTTP/1.1'
            def do_GET(self):
                """do_GET serves the synthetic local API and records the socket identity."""
                with lock:counts[self.client_address]=counts.get(self.client_address,0)+1
                body=b'{"success":true}'
                self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
            def log_message(self,*args):
                """log_message suppresses synthetic access logs, never real gateway logging."""
        server=ThreadingHTTPServer(('127.0.0.1',0),Handler)
        worker=threading.Thread(target=server.serve_forever,daemon=True);worker.start()
        try:
            endpoint=Endpoint('fixture','/api/test',1,True,lambda b:require(b=={'success':True},'wrong fixture'))
            r=measure.measure(endpoint,server.server_port,'fixture-only',os.getpid(),2,8)
            self.assertEqual(r['completed'],8);self.assertEqual(sorted(counts.values()),[6,6])
            self.assertEqual(len(r['warmups']),4);self.assertEqual([v['index'] for v in r['samples']],list(range(8)))
            self.assertEqual(r['latency_ms'],measure.quantiles([v['latency_ms'] for v in r['samples']]))
        finally:
            server.shutdown();server.server_close();worker.join(timeout=5)


if __name__=='__main__':unittest.main()
