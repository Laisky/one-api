#!/usr/bin/env python3
"""Collect separate CPU diagnostics for distinct dashboard windows, never profile an acceptance trial."""
from __future__ import annotations
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime,timezone
import http.client
import json
import os
from pathlib import Path
import sys
import tempfile
import threading
import time
import urllib.request

import dataset
from http_fixture import bootstrap,clone_database,gateway,free_port,get,require,sha256
from measure import proc,quantiles
from run import checkpoint
from workload import Endpoint,query

sys.path.append(str(Path(__file__).resolve().parents[1]/'stream-perf'))
from profile_window import stable_window
from profile_support import cpu_allowance,assert_loopback_listeners,cgroup_stats


def input_windows(rows: int) -> list[Endpoint]:
    """input_windows creates eight valid disjoint seven-day user windows without bypassing application singleflight."""
    windows=[]
    for i in range(8):
        end=dataset.END-i*7*86400;start=end-7*86400
        expected=dataset.dashboard(dataset.rows_in_window(rows,2,start=start,end=end-1),2)
        url=query('/api/user/dashboard',from_date=datetime.fromtimestamp(start,timezone.utc).strftime('%Y-%m-%d'),
                  to_date=datetime.fromtimestamp(end-1,timezone.utc).strftime('%Y-%m-%d'))
        windows.append(Endpoint(f'dashboard-window-{i}',url,2,True,lambda b,expect=expected:dataset.check_dashboard(b,expect)))
    return windows


def profile(binary: Path, seed: Path, token: str, folder: Path, windows: list[Endpoint], output: Path) -> dict:
    """profile verifies all bodies and captures one60s CPU profile inside a95s bounded run with raw process/progress samples."""
    folder.mkdir();clone_database(seed,folder/'one-api.db');output.mkdir()
    allowance=cpu_allowance();summary={'complete':False,'diagnostic_only':True,'binary_sha256':sha256(binary),
      'gateway_procs':3,'client':'one Python process with eight HTTP worker threads','allowance':allowance,
      'warmup':30,'seconds':60,'run_seconds':95,'endpoints':[e.path for e in windows],
      'progress_source':'verified completed HTTP reads; adapted to legacy stable_window upstream slot, not a provider counter',
      'durable_counter':'zero adapter value because this workload only reads; not billing evidence'}
    checkpoint(output/'summary.json',summary)
    barrier=threading.Barrier(9,timeout=90);done=threading.Event();lock=threading.Lock()
    progress={'completed':0};samples=[];captures=[]
    pprof_port=free_port()
    with gateway(binary,folder,pprof_port=pprof_port,gateway_procs=3) as f:
        assert_loopback_listeners(f['pid'],{f['port'],pprof_port})
        started=[0.0]
        def load(index: int) -> list[dict]:
            """load keeps one connection and one date window per worker while validating every complete response."""
            endpoint=windows[index];conn=http.client.HTTPConnection('127.0.0.1',f['port'],timeout=30);result=[]
            try:
                for _ in range(2):
                    status,body,_,_=get(conn,endpoint.path,token);require(status==200,'diagnostic warmup status');endpoint.check(body)
                barrier.wait()
                while not done.is_set() and time.monotonic()-started[0] < 125:
                    status,body,ms,size=get(conn,endpoint.path,token);require(status==200,'diagnostic HTTP status');endpoint.check(body)
                    with lock:progress['completed']+=1
                    result.append({'worker':index,'index':len(result),'latency_ms':ms,'bytes':size,'validated':True})
                return result
            except Exception:
                done.set();barrier.abort();raise
            finally:conn.close()
        def capture() -> None:
            """capture stores only a bounded loopback CPU profile and its actual timing, not payloads or credentials."""
            at=time.monotonic();url=f'http://127.0.0.1:{pprof_port}/debug/pprof/profile?seconds=60'
            opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with opener.open(url,timeout=75) as response:
                content=response.read((128<<20)+1)
            require(0<len(content)<=128<<20,'CPU profile size bound')
            (output/'cpu.pprof').write_bytes(content)
            captures.append({'started_elapsed':at-started[0],'finished_elapsed':time.monotonic()-started[0],
                             'bytes':len(content),'sha256':sha256(output/'cpu.pprof')})
        requests=[]
        try:
            with ThreadPoolExecutor(max_workers=8) as pool,ThreadPoolExecutor(max_workers=1) as profiler:
                futures=[pool.submit(load,i) for i in range(8)]
                deadline=time.monotonic()+90
                while barrier.n_waiting<8 and not barrier.broken:
                    require(time.monotonic()<deadline,'diagnostic warmup deadline');time.sleep(.005)
                require(not barrier.broken,'diagnostic worker failed before start')
                started[0]=time.monotonic();barrier.wait();profile_future=None
                with (output/'samples.jsonl').open('w') as telemetry:
                    for second in range(96):
                        if done.is_set():raise RuntimeError('diagnostic worker stopped early')
                        elapsed=time.monotonic()-started[0]
                        if elapsed>=30 and profile_future is None:profile_future=profiler.submit(capture)
                        with lock:completed=progress['completed']
                        cpu,rss=proc(f['pid']);client_cpu,client_rss=proc(os.getpid())
                        row={'elapsed':elapsed,'processes':{'gateway':{'cpu_seconds':cpu,'rss_bytes':rss},
                              'client':{'cpu_seconds':client_cpu,'rss_bytes':client_rss}},
                              'upstream':{'completed':completed,'active':8},'durable_requests':0,
                              'cgroup':cgroup_stats(Path(allowance['stat_path']))}
                        samples.append(row);telemetry.write(json.dumps(row)+'\n');telemetry.flush()
                        if second<95:time.sleep(max(0,started[0]+second+1-time.monotonic()))
                    done.set()
                    for future in futures:requests.extend(future.result(timeout=35))
                    require(profile_future is not None,'CPU capture did not start');profile_future.result(timeout=20)
            require(len(requests)==progress['completed'],'diagnostic request count mismatch')
            summary.update(complete=True,completed=len(requests),failed=0,dropped=0,latency_ms=quantiles([r['latency_ms'] for r in requests]),
                           window=stable_window(samples,allowance['effective_cores'],3,30,60),captures=captures)
            checkpoint(output/'requests.json',{'samples':requests})
        finally:
            done.set();barrier.abort();checkpoint(output/'summary.json',summary)
    return summary


def main() -> None:
    """main collects two separately identified diagnostic profiles after all acceptance and build/test work has ended."""
    p=argparse.ArgumentParser(description=__doc__)
    for n in ('baseline','candidate','token-cache','output'):p.add_argument('--'+n,type=Path,required=True)
    args=p.parse_args()
    for n in ('baseline','candidate','token_cache','output'):setattr(args,n,getattr(args,n).resolve())
    require(not args.output.exists(),'refusing to overwrite diagnostics');args.output.mkdir(parents=True)
    os.environ['TIKTOKEN_CACHE_DIR']=str(args.token_cache)
    with tempfile.TemporaryDirectory(prefix='web-profiles-') as name:
        root=Path(name);database=bootstrap(args.baseline,root/'seed');seed=dataset.seed(database,200000)
        windows=input_windows(200000)
        for label in ('baseline','candidate'):
            summary=profile(getattr(args,label),database,seed['tokens'][2],root/label,windows,args.output/label)
            print(json.dumps({'variant':label,'complete':summary['complete'],'requests':summary['completed'],
                             'window':{k:v for k,v in summary['window'].items() if k!='intervals'}}),flush=True)


if __name__=='__main__':main()
