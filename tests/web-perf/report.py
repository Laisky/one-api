#!/usr/bin/env python3
"""Audit complete Web API experiments using paired run-level observations, not pooled-request significance."""
from __future__ import annotations
import argparse
import json
import math
from pathlib import Path
import statistics

from http_fixture import require
from measure import quantiles

TARGETS={'self-dashboard','self-logs','self-deep','self-cursor'}
CONTROLS={'site-dashboard','admin-logs','admin-stat','user-self'}


def finite(value) -> float:
    """finite rejects nonfinite, boolean and negative evidence instead of converting it into success."""
    require(type(value) in (int,float) and math.isfinite(value) and value>=0,'invalid numeric evidence')
    return float(value)


def compare(summary: dict, *, strict: bool = True) -> dict:
    """compare validates all identities, response/qualification gates and the registered160-cell matrix before deciding."""
    require(summary.get('complete') is True and 'failure_type' not in summary,'experiment is incomplete')
    config=summary['configuration'];repeats=config['repeats'];levels=[int(v) for v in config['concurrency'].split(',')]
    require(repeats>=2,'multiple independent repeats required')
    if strict:
        expected={'rows':200000,'repeats':5,'concurrency':'1,8','target_requests':128,'control_requests':512}
        require(all(config.get(k)==v for k,v in expected.items()),'not the registered confirmation protocol')
    for label in ('baseline','candidate'):
        q=summary['qualification'].get(label,{})
        require(all(q.get(k) is True for k in ('complete','all_eight_endpoints','auth_and_tenant_isolation','cursor_continuation_and_cross_user_rejection','empty_shape')),
                'variant lacks independent HTTP qualification')
        require(all(summary['freshness'].get(label,{}).get(k) is True
                    for k in ('committed_insert','legacy_rows_and_count','all_dashboard_aggregates')),'missing post-write freshness')
        require(summary['schema'][label]['rows']==config['rows'],'fixture row count drift')
        require((('idx_logs_user_created_at_id' in summary['schema'][label]['index_names']) == (label=='candidate')),'wrong migrated index identity')
        if label=='candidate':
            require(summary['schema'][label].get('index_columns')==['user_id','created_at','id'],'wrong migrated index columns')
    seen={}
    for t in summary['trials']:
        key=t['endpoint'],t['concurrency'],t['label'],t['repeat']
        require(key not in seen,'duplicate endpoint trial')
        require(t['endpoint'] in TARGETS|CONTROLS and t['target'] is (t['endpoint'] in TARGETS),'wrong endpoint classification')
        require(t['label'] in summary['binaries'] and t['binary_sha256']==summary['binaries'][t['label']],'mixed binary identity')
        n=config['target_requests'] if t['target'] else config['control_requests']
        require(t['offered']==t['completed']==n and t['failed']==t['dropped']==0,'failed or missing HTTP requests')
        for k in ('p50','p95','p99','max'):finite(t['latency_ms'][k])
        require(list(t['latency_ms'][k] for k in ('p50','p95','p99','max'))==sorted(t['latency_ms'].values()),'unordered quantiles')
        require(finite(t['seconds'])>0 and math.isclose(t['successful_rps'],n/t['seconds'],rel_tol=1e-12),'incorrect throughput')
        finite(t['resources']['gateway']['cpu_ms_per_request']);finite(t['resources']['gateway']['peak_rss_mib'])
        seen[key]=t
    expected={(e,c,l,r) for e in TARGETS|CONTROLS for c in levels for l in ('baseline','candidate') for r in range(repeats)}
    require(set(seen)==expected,'incomplete or unexpected matrix')
    cells=[];reasons=[];benefits=[]
    for e in sorted(TARGETS|CONTROLS):
        for c in levels:
            metrics={}
            for name,path in {'p50_ms':('latency_ms','p50'),'p95_ms':('latency_ms','p95'),
                              'cpu_ms':('resources','gateway','cpu_ms_per_request'),'rss_mib':('resources','gateway','peak_rss_mib'),
                              'rps':('successful_rps',)}.items():
                values=[]
                for label in ('baseline','candidate'):
                    current=[]
                    for repeat in range(repeats):
                        v=seen[e,c,label,repeat]
                        for k in path:v=v[k]
                        current.append(finite(v))
                    values.append(current)
                a,b=values;absolute=[y-x for x,y in zip(a,b)];pct=[(y/x-1)*100 for x,y in zip(a,b)] if all(a) else []
                metrics[name]={'baseline_median':statistics.median(a),'candidate_median':statistics.median(b),
                  'paired_abs_median':statistics.median(absolute),'paired_pct_median':statistics.median(pct) if pct else None,
                  'paired_pct_range':[min(pct),max(pct)] if pct else None,'favorable_pairs':sum(y>x if name=='rps' else y<x for x,y in zip(a,b))}
            cell={'endpoint':e,'concurrency':c,'target':e in TARGETS,'metrics':metrics};cells.append(cell)
            latency=metrics['p95_ms'];rss=metrics['rss_mib']
            if e in TARGETS and latency['paired_pct_median']<=-20 and latency['favorable_pairs']>=4:benefits.append([e,c])
            if e in CONTROLS and latency['paired_pct_median']>10 and latency['paired_abs_median']>5:reasons.append(f'{e}/c{c}: control p95 regression')
            if rss['paired_pct_median']>20:reasons.append(f'{e}/c{c}: RSS regression')
    if len({e for e,c in benefits})<2:reasons.append('fewer than two targets have material repeatable benefit')
    return {'accepted':not reasons,'reasons':reasons,'benefits':benefits,'cells':cells,
            'trials':len(seen),'completed':sum(t['completed'] for t in seen.values()),'raw_samples_verified':False}


def audit(directory: Path, *, strict: bool = True) -> dict:
    """audit binds every full request record and warm-up to the exact summary, then recomputes its quantiles and decision."""
    summary=json.loads((directory/'summary.json').read_text())
    result=compare(summary,strict=strict)
    for t in summary['trials']:
        name=f"{t['label']}-c{t['concurrency']}-r{t['repeat']}-{t['endpoint']}.json"
        raw=json.loads((directory/name).read_text())
        require({k:v for k,v in raw.items() if k not in ('samples','warmups')}==t,'raw/summary discrepancy')
        samples=raw['samples'];require(len(samples)==t['completed'],'missing raw samples')
        require(sorted(s['index'] for s in samples)==list(range(t['completed'])),'duplicate or omitted request index')
        require(all(s['validated'] is True and s['response_bytes']>0 for s in samples),'invalid response observation')
        require(quantiles([finite(s['latency_ms']) for s in samples])==t['latency_ms'],'request quantile mismatch')
        warmups=raw['warmups']
        require(len(warmups)==2*t['concurrency'] and {(w['worker'],w['index']) for w in warmups}=={(i,j) for i in range(t['concurrency']) for j in range(2)},'missing per-worker warmups')
    result['raw_samples_verified']=True
    return result


def main() -> None:
    """main prints an independently recomputed result; a rejected complete experiment remains a legitimate audited outcome."""
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('directory',type=Path);args=p.parse_args()
    print(json.dumps(audit(args.directory),indent=2,allow_nan=False))


if __name__=='__main__':main()
