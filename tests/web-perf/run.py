#!/usr/bin/env python3
"""Run the registered Web API recovery experiment against two immutable local gateway binaries."""
from __future__ import annotations
import argparse
from datetime import datetime,timezone
import json
import os
from pathlib import Path
import platform
import sqlite3
import tempfile
import time

import dataset
from http_fixture import bootstrap,clone_database,gateway,get,require,sha256
import measure
import workload


def checkpoint(path: Path, value: dict) -> None:
    """checkpoint atomically retains partial results without representing an interrupted matrix as complete."""
    temporary=path.with_suffix('.tmp')
    temporary.write_text(json.dumps(value,indent=2,allow_nan=False)+'\n')
    temporary.replace(path)


def schema_evidence(database: Path, expected_index: bool) -> dict:
    """schema_evidence verifies real startup migration and records relevant access plans, not guessed SQL improvements."""
    with sqlite3.connect(database) as db:
        names=[r[1] for r in db.execute("PRAGMA index_list('logs')")]
        require((dataset.INDEX in names) is expected_index,'startup index state mismatch')
        columns=[r[2] for r in db.execute('PRAGMA index_info('+dataset.INDEX+')')]
        if expected_index:
            require(columns==['user_id','created_at','id'],'wrong composite index order')
        sql=('SELECT id FROM logs WHERE user_id=? AND created_at>=? AND created_at<=? AND type!=6 '
             'ORDER BY created_at DESC,id DESC LIMIT 21')
        plan=[list(row) for row in db.execute('EXPLAIN QUERY PLAN '+sql,(2,dataset.START,dataset.END-1))]
        return {'index_names':sorted(names),'index_columns':columns,'plan_sql':sql,'query_plan':plan,
                'rows':db.execute('SELECT count(*) FROM logs').fetchone()[0],
                'sqlite_version':sqlite3.sqlite_version}


def main() -> None:
    """main freezes and validates the complete matrix, retains failures and removes only its private fixture directory."""
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--baseline',type=Path,required=True)
    parser.add_argument('--candidate',type=Path,required=True)
    parser.add_argument('--token-cache',type=Path,required=True)
    parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--rows',type=int,default=200000)
    parser.add_argument('--repeats',type=int,default=5)
    parser.add_argument('--concurrency',default='1,8')
    parser.add_argument('--target-requests',type=int,default=128)
    parser.add_argument('--control-requests',type=int,default=512)
    args=parser.parse_args()
    for name in ('baseline','candidate','token_cache','output'):
        setattr(args,name,getattr(args,name).resolve())
    levels=[int(c) for c in args.concurrency.split(',')]
    require(platform.system()=='Linux','Linux process telemetry is required')
    require(1<=args.repeats<=10 and all(1<=c<=64 for c in levels),'invalid repetitions/concurrency')
    require(len(levels)==len(set(levels)),'duplicate concurrency')
    require(all(c<=n<=100000 and n%c==0 for c in levels for n in (args.target_requests,args.control_requests)),'invalid request dimensions')
    require(not args.output.exists(),'refusing to overwrite evidence')
    args.output.mkdir(parents=True)
    os.environ['TIKTOKEN_CACHE_DIR']=str(args.token_cache)
    summary={'schema_version':1,'complete':False,'started_utc':datetime.now(timezone.utc).isoformat(),
             'configuration':{k:str(v) if isinstance(v,Path) else v for k,v in vars(args).items()},
             'binaries':{label:sha256(getattr(args,label)) for label in ('baseline','candidate')},
             'environment':{'platform':platform.platform(),'cpu_model':next((l.split(':',1)[1].strip() for l in Path('/proc/cpuinfo').read_text().splitlines() if l.startswith('model name')),'unknown'),
              'cpu_max':Path('/sys/fs/cgroup/cpu.max').read_text().strip(),'memory_max':Path('/sys/fs/cgroup/memory.max').read_text().strip(),
              'GOMAXPROCS':2,'database':'fresh file-backed SQLite','rate_limiter':'enabled; GLOBAL_API_RATE_LIMIT=10000000',
              'cursor':'explicit fixture opt-in; product default remains disabled','dashboard_cache':'standalone default, TTL=0',
              'resource_sampling_ms':20,'latency_scope':'request send through complete body read, before JSON/oracle validation',
              'throughput_scope':'includes client validation; not browser paint or server-only capacity'},
             'qualification':{},'schema':{},'freshness':{},'trials':[]}
    checkpoint(args.output/'summary.json',summary)
    try:
        with tempfile.TemporaryDirectory(prefix='oneapi-web-') as name:
            root=Path(name);seed_dir=root/'seed'
            database=bootstrap(args.baseline,seed_dir)
            seed=dataset.seed(database,args.rows)
            summary['dataset']={k:v for k,v in seed.items() if k!='tokens'}
            specs,personal,_=workload.endpoints(args.rows)
            require(len(personal)>1020,'fixture must populate the deep page')
            for repeat in range(args.repeats):
                for level in levels:
                    for label in (('baseline','candidate') if repeat%2==0 else ('candidate','baseline')):
                        folder=root/f'{label}-c{level}-r{repeat}';folder.mkdir()
                        clone_database(database,folder/'one-api.db')
                        with gateway(getattr(args,label),folder) as running:
                            evidence=schema_evidence(folder/'one-api.db',label=='candidate')
                            require(evidence['rows']==args.rows,'fixture changed during startup')
                            summary['schema'].setdefault(label,evidence)
                            key=f'{label}-c{level}-r{repeat}'
                            if label not in summary['qualification']:
                                summary['qualification'][label]=workload.qualify(running['port'],seed['tokens'],args.rows,specs,personal)
                                checkpoint(args.output/'summary.json',summary)
                            for endpoint in specs:
                                record=measure.measure(endpoint,running['port'],seed['tokens'][endpoint.principal],running['pid'],level,
                                    args.target_requests if endpoint.target else args.control_requests)
                                record.update(label=label,repeat=repeat,binary_sha256=summary['binaries'][label],startup_seconds=running['startup_seconds'])
                                filename=f'{key}-{endpoint.name}.json'
                                checkpoint(args.output/filename,record)
                                summary['trials'].append({k:v for k,v in record.items() if k not in ('samples','warmups')})
                                checkpoint(args.output/'summary.json',summary)
                                print(json.dumps({'trial':filename,'p95_ms':record['latency_ms']['p95'],'rps':record['successful_rps'],
                                     'cpu_ms':record['resources']['gateway']['cpu_ms_per_request'],'rss_mib':record['resources']['gateway']['peak_rss_mib']}),flush=True)
                            if repeat==0 and level==levels[0]:
                                summary['freshness'][label]=workload.freshness(running['port'],folder/'one-api.db',seed['tokens'],args.rows)
                                checkpoint(args.output/'summary.json',summary)
            summary['complete']=True
    except Exception as error:
        summary['failure_type']=type(error).__name__
        summary['failure_message']=str(error) if isinstance(error,ValueError) else 'see exception type; private logs intentionally not exported'
        raise
    finally:
        summary['finished_utc']=datetime.now(timezone.utc).isoformat()
        checkpoint(args.output/'summary.json',summary)


if __name__=='__main__':
    main()
