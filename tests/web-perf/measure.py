"""Bounded persistent-connection Web API load with exact checks and gateway-only resource counters."""
from __future__ import annotations
from concurrent.futures import ThreadPoolExecutor
import hashlib
import http.client
import json
import math
import os
from pathlib import Path
import threading
import time

from http_fixture import get, require
from workload import Endpoint


def proc(pid: int) -> tuple[float, int]:
    """proc samples Linux process CPU seconds and resident bytes without including unrelated child processes."""
    fields=Path(f'/proc/{pid}/stat').read_text().rsplit(')',1)[1].split()
    return ((int(fields[11])+int(fields[12]))/os.sysconf('SC_CLK_TCK'),
            int(Path(f'/proc/{pid}/statm').read_text().split()[1])*os.sysconf('SC_PAGE_SIZE'))


def quantiles(values: list[float]) -> dict:
    """quantiles returns deterministic nearest-rank millisecond summaries and refuses empty or invalid observations."""
    require(bool(values) and all(math.isfinite(v) and v>=0 for v in values),'invalid timing observations')
    ordered=sorted(values)
    return {k:ordered[(len(ordered)*p+99)//100-1] for k,p in [('p50',50),('p95',95),('p99',99),('max',100)]}


def measure(endpoint: Endpoint, port: int, token: str, pid: int, concurrency: int, requests: int) -> dict:
    """measure warms every worker twice, synchronizes timed starts and verifies every HTTP response with the fixture oracle."""
    require(1<=concurrency<=64 and concurrency<=requests<=100000 and requests%concurrency==0,'invalid bounded load')
    barrier=threading.Barrier(concurrency+1,timeout=60)
    finished=threading.Event()
    samples, warmups, errors=[],[],[]
    rss={'gateway':0,'client':0}
    def watch() -> None:
        """watch records independent process RSS peaks every20ms and preserves sampling errors."""
        try:
            while not finished.wait(.02):
                for name, process in [('gateway',pid),('client',os.getpid())]:
                    rss[name]=max(rss[name],proc(process)[1])
        except (OSError,ValueError) as exc:
            errors.append(type(exc).__name__)
    def worker(worker_id: int) -> tuple[list[dict],list[dict]]:
        """worker holds one connection through warmup and measurement; response validation never substitutes for reading bytes."""
        conn=http.client.HTTPConnection('127.0.0.1',port,timeout=30)
        local, warm=[],[]
        try:
            for index in range(2):
                status,body,elapsed,size=get(conn,endpoint.path,token)
                require(status==200,'warm-up HTTP status')
                endpoint.check(body)
                warm.append({'worker':worker_id,'index':index,'latency_ms':elapsed,'response_bytes':size})
            socket=conn.sock
            require(socket is not None,'persistent connection unavailable')
            barrier.wait()
            for index in range(requests//concurrency):
                status,body,elapsed,size=get(conn,endpoint.path,token)
                require(conn.sock is socket,'measured connection unexpectedly replaced')
                require(status==200,'measured HTTP status')
                endpoint.check(body)
                local.append({'index':worker_id*(requests//concurrency)+index,'worker':worker_id,
                              'latency_ms':elapsed,'response_bytes':size,'validated':True})
            return local,warm
        except Exception:
            barrier.abort()
            raise
        finally:
            conn.close()
    monitor=threading.Thread(target=watch,daemon=True)
    with ThreadPoolExecutor(max_workers=concurrency) as pool:
        futures=[pool.submit(worker,i) for i in range(concurrency)]
        # Wait until every worker has completed both verified warmups before counters/start time.
        try:
            deadline=time.monotonic()+60
            while barrier.n_waiting<concurrency and not barrier.broken:
                require(time.monotonic()<deadline,'worker warmup deadline')
                time.sleep(.001)
            require(not barrier.broken,'worker qualification failed before timed barrier')
            g0,rss['gateway']=proc(pid)
            c0,rss['client']=proc(os.getpid())
            start=time.perf_counter()
            monitor.start()
            barrier.wait()
            for f in futures:
                observed,warm=f.result(timeout=600)
                samples.extend(observed);warmups.extend(warm)
            seconds=time.perf_counter()-start
            g1,gpeak=proc(pid);c1,cpeak=proc(os.getpid())
        finally:
            barrier.abort()
            finished.set()
            if monitor.ident is not None:
                monitor.join(timeout=5)
    require(not errors,'resource sampling failed')
    require(len(samples)==requests and sorted(s['index'] for s in samples)==list(range(requests)),'missing request observations')
    return {'endpoint':endpoint.name,'path':endpoint.path,'principal':endpoint.principal,'target':endpoint.target,
            'concurrency':concurrency,'offered':requests,'completed':len(samples),'failed':0,'dropped':0,
            'seconds':seconds,'successful_rps':requests/seconds,'latency_ms':quantiles([s['latency_ms'] for s in samples]),
            'resources':{'gateway':{'cpu_seconds':g1-g0,'cpu_ms_per_request':(g1-g0)*1000/requests,
                          'peak_rss_mib':max(gpeak,rss['gateway'])/2**20},
                         'client':{'cpu_seconds':c1-c0,'peak_rss_mib':max(cpeak,rss['client'])/2**20}},
            'warmups':warmups,'samples':sorted(samples,key=lambda s:s['index'])}
