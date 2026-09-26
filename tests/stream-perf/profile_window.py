"""Fail-closed sustained-window validation with duration-weighted gateway attribution."""
from __future__ import annotations
import math


def number(value, name: str, *, positive: bool = False) -> float:
    """number accepts finite JSON numeric scalars, excluding booleans and invalid negative values."""
    if type(value) not in (float, int) or not math.isfinite(value) or value < 0 or (positive and value == 0):
        raise ValueError('invalid ' + name)
    return float(value)


def counter(value, name: str) -> int:
    """counter requires a nonnegative integer observation rather than coercing corrupted progress."""
    if type(value) is not int or value < 0:
        raise ValueError('invalid ' + name)
    return value


def validated(rows: list[dict]) -> list[dict]:
    """validated checks every raw timestamp and cumulative gateway/progress observation before window selection."""
    observations = []
    for row in rows:
        try:
            item = {'elapsed': number(row['elapsed'], 'sample time'),
                    'cpu': number(row['processes']['gateway']['cpu_seconds'], 'gateway CPU'),
                    'completed': counter(row['upstream']['completed'], 'upstream completed'),
                    'active': counter(row['upstream']['active'], 'upstream active'),
                    'durable': counter(row['durable_requests'], 'durable requests')}
        except (KeyError, TypeError) as error:
            raise ValueError('incomplete profiling sample') from error
        if observations:
            previous = observations[-1]
            if item['elapsed'] <= previous['elapsed'] or any(item[key] < previous[key] for key in ('cpu', 'completed', 'durable')):
                raise ValueError('non-monotonic profiling samples')
        observations.append(item)
    return observations


def half_rates(intervals: list[dict], start: float, seconds: float) -> list[dict]:
    """half_rates uses actual elapsed half-windows, excluding a straddling interval rather than inventing event times."""
    result = []
    for begin, end in ((start, start + seconds / 2), (start + seconds / 2, start + seconds)):
        part = [item for item in intervals if item['start'] >= begin and item['end'] <= end]
        covered = sum(item['seconds'] for item in part)
        result.append({'coverage_seconds': covered,
                       'gateway_cores': sum(item['gateway_cpu_seconds'] for item in part) / covered if covered else None,
                       'upstream_rps': sum(item['upstream_completed'] for item in part) / covered if covered else None})
    return result


def stable_window(rows: list[dict], cores: float, gateway_procs: int, start: float, seconds: float,
                  minimum: float = .5, fraction: float = .8, max_rate_drift: float = .15) -> dict:
    """stable_window requires sustained gateway CPU plus positive, stable throughput and intact counters/coverage.

    Version 2 measures high-load coverage by elapsed duration, not sample count.
    Progress uses upstream completion counters, not client completion timestamps;
    the caller must independently validate final delivery and durable accounting.
    """
    cores = number(cores, 'CPU allowance', positive=True)
    start = number(start, 'window start')
    seconds = number(seconds, 'window duration', positive=True)
    minimum = number(minimum, 'minimum utilization', positive=True)
    fraction = number(fraction, 'required fraction', positive=True)
    max_rate_drift = number(max_rate_drift, 'rate-drift limit')
    if type(gateway_procs) is not int or not 1 <= gateway_procs <= 64 or minimum > 1 or fraction > 1 or max_rate_drift > 1:
        raise ValueError('invalid profiling-window parameters')
    observations = validated(rows)
    intervals = []
    for previous, current in zip(observations, observations[1:]):
        if previous['elapsed'] < start or current['elapsed'] > start + seconds:
            continue
        duration = current['elapsed'] - previous['elapsed']
        cpu = current['cpu'] - previous['cpu']
        used = cpu / duration
        intervals.append({'start': previous['elapsed'], 'end': current['elapsed'], 'seconds': duration,
                          'gateway_cpu_seconds': cpu, 'gateway_cores': used,
                          'machine_utilization': used / cores,
                          'gateway_slot_utilization': used / min(cores, gateway_procs),
                          'upstream_completed': current['completed'] - previous['completed'],
                          'durable_requests': current['durable'] - previous['durable'],
                          'upstream_active': current['active']})
    coverage = sum(item['seconds'] for item in intervals)
    high_duration = sum(item['seconds'] for item in intervals if item['machine_utilization'] >= minimum)
    ratio = high_duration / coverage if coverage else 0.0
    count_ratio = sum(item['machine_utilization'] >= minimum for item in intervals) / len(intervals) if intervals else 0.0
    halves = half_rates(intervals, start, seconds)
    rates = [part['upstream_rps'] for part in halves]
    means = [part['gateway_cores'] for part in halves]
    progress = all(value is not None and value > 0 for value in rates)
    rate_drift = rates[1] / rates[0] - 1 if progress else None
    cpu_drift = abs(means[1] / means[0] - 1) if all(v is not None and v > 0 for v in means) else None
    reasons = []
    if not intervals or coverage < seconds - min(2, seconds * .05):
        reasons.append('incomplete observation coverage')
    if any(item['seconds'] > 2.5 for item in intervals):
        reasons.append('sampling gap exceeds 2.5 seconds')
    if ratio < fraction:
        reasons.append('insufficient gateway-only high-load duration')
    cpu_only_qualified = not reasons
    if any(part['coverage_seconds'] < seconds / 2 - min(2, seconds * .05) for part in halves):
        reasons.append('incomplete half-window coverage')
    if not progress:
        reasons.append('no completed-request progress in both halves')
    elif abs(rate_drift) > max_rate_drift:
        reasons.append('upstream throughput drift exceeds limit')
    return {'rule_version': 2, 'qualified': not reasons, 'cpu_only_qualified': cpu_only_qualified,
            'failure_reasons': reasons, 'intervals': intervals, 'coverage_seconds': coverage,
            'fraction_at_or_above_target': ratio, 'high_load_duration_seconds': high_duration,
            'sample_fraction_at_or_above_target': count_ratio, 'fraction_basis': 'elapsed duration',
            'target_machine_utilization': minimum, 'required_fraction': fraction,
            'gateway_cores_mean': sum(item['gateway_cpu_seconds'] for item in intervals) / coverage if coverage else None,
            'half_window_gateway_cores': means, 'relative_cpu_drift': cpu_drift,
            'half_window_coverage_seconds': [part['coverage_seconds'] for part in halves],
            'half_window_upstream_rps': rates, 'upstream_rate_drift': rate_drift,
            'maximum_upstream_rate_drift': max_rate_drift,
            'upstream_active_peak': max((item['upstream_active'] for item in intervals), default=None),
            'progress_note': 'Upstream completions and durable statistics are proxies, not client completion timestamps. '
                             'An interval crossing the half-window boundary is excluded from half-rate calculations.'}
