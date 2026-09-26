"""Independently validate request, usage and sustained-window evidence before correlating frames."""
from __future__ import annotations
import hashlib
import math

from profile_window import stable_window

METRICS = ('ttft_ms', 'done_ms', 'total_ms', 'scheduled_total_ms', 'max_inter_content_gap_ms')


def require(condition: bool, message: str) -> None:
    """require rejects invalid observations even when Python assertions are disabled."""
    if not condition:
        raise ValueError(message)


def check_inputs(summary: dict, requests: dict, rows: list[dict], trace: bytes) -> dict:
    """check_inputs recomputes all request quantiles and the recorded CPU/progress window without trusting success flags."""
    config = summary['configuration']
    require(summary['complete'] is True and summary['sustained_protocol_qualified'] is True,
            'diagnostic did not complete and qualify')
    require(config['mode'] == 'trace' and 1 <= config['requests'] <= 8192 and 1 <= config['chunks'] <= 1024,
            'unsupported diagnostic configuration')
    require({k: v for k, v in requests.items() if k != 'samples'} == summary['requests'], 'request/summary mismatch')
    require(requests['failed'] == requests['dropped'] == 0 and
            requests['offered'] == requests['completed'] == config['requests'], 'incomplete request delivery')
    samples = requests['samples']
    require(sorted(x['index'] for x in samples) == list(range(config['requests'])), 'request index mismatch')
    require(all(not x.get('error') for x in samples), 'failed request sample')
    for metric in METRICS:
        values = sorted(x[metric] for x in samples)
        require(all(type(x) in (int, float) and math.isfinite(x) and x >= 0 for x in values), 'invalid request measurement')
        quantiles = {key: values[(len(values)*percent+99)//100-1]
                     for key, percent in (('p50', 50), ('p95', 95), ('p99', 99), ('max', 100))}
        require(quantiles == requests[metric], 'request percentile mismatch: ' + metric)
    require(requests['seconds'] > 0 and math.isclose(requests['successful_rps'],
            len(samples)/requests['seconds'], rel_tol=1e-12), 'throughput mismatch')
    require(summary['usage']['requests'] == len(samples) and summary['usage']['used_quota'] > 0,
            'durable usage mismatch')
    qualification = summary['qualification']
    require(all(qualification[key] == 0 for key in ('normal', 'crlf', 'fragmented')) and
            all(qualification[key] == 8 for key in ('missing-done', 'wrong-content', 'malformed')) and
            qualification['cancellation'] == '8/8 producers released within 5 seconds', 'qualification mismatch')
    captures = summary['profile_captures']
    require(len(captures) == 1 and captures[0]['file'] == 'runtime.trace', 'trace capture identity mismatch')
    capture = captures[0]
    require(capture['sha256'] == hashlib.sha256(trace).hexdigest() and capture['bytes'] == len(trace), 'trace bytes mismatch')
    require(config['warmup'] <= capture['started_elapsed'] < capture['finished_elapsed'] <=
            config['warmup'] + config['seconds'] and 1 <= config['trace_seconds'] <= 10,
            'trace outside observation')
    require(summary['trace_window_within_observation'] is True and
            summary['profile_window_completed_while_load_alive'] is True, 'incomplete live trace window')
    window = summary['window']
    require(window['qualified'] is True and window['intervals'], 'no qualified interval evidence')
    start = window['intervals'][0]['start']
    require(config['warmup'] <= start <= config['warmup'] + 2.5, 'recorded window does not follow warm-up')
    # The captured start may be a fraction after the nominal warm-up. Reconstruct
    # every retained interval and field; never replace the original decision with
    # an alternative selected window that happened to pass.
    rebuilt = stable_window(rows, summary['allowance']['effective_cores'], config['gateway_procs'],
                            start, config['seconds'])
    require(rebuilt == window, 'recorded window differs from raw counters')
    nominal = stable_window(rows, summary['allowance']['effective_cores'], config['gateway_procs'],
                           config['warmup'], config['seconds'])
    return {'request_samples': len(samples), 'recorded_window_reproduced': True,
            'nominal_window_qualified': nominal['qualified'], 'nominal_coverage_seconds': nominal['coverage_seconds'],
            'nominal_gateway_cores_mean': nominal['gateway_cores_mean'],
            'qualification_and_positive_usage_checked': True,
            'financial_ledger_audit': False}
