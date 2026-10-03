"""Replay a saved diagnostic window without substituting a more favorable observation interval."""
from __future__ import annotations

from profile_window import number, stable_window


def replay_window(summary: dict, rows: list[dict]) -> dict:
    """replay_window reproduces every saved field and reports whether its original start was actually recorded.

    New captures persist their exact original origin. Legacy captures can only be
    reproduced at the declared nominal origin or the previous first-sample
    convention. Matching is all-or-nothing, including the saved failure reasons;
    these witnesses never recover an unrecorded exact origin or promote a result.
    """
    config, saved = summary['configuration'], summary['window']
    nominal = number(config['warmup'], 'nominal warm-up')
    duration = number(config['seconds'], 'window duration', positive=True)
    if 'window_started_elapsed' in summary:
        origins = [('recorded', number(summary['window_started_elapsed'], 'recorded window start'))]
        exact = True
    else:
        if not saved.get('intervals'):
            raise ValueError('legacy window has no recorded intervals to replay')
        first = number(saved['intervals'][0]['start'], 'first retained interval')
        origins = [('nominal', nominal), ('first_retained_interval', first)]
        exact = False
    matches = []
    for name, start in origins:
        if not nominal <= start <= nominal + 2.5:
            if exact:
                raise ValueError('recorded window start is outside the bounded warm-up interval')
            continue
        replay = stable_window(rows, summary['allowance']['effective_cores'], config['gateway_procs'], start, duration)
        if replay == saved:
            matches.append({'convention': name, 'start': start})
    if not matches:
        raise ValueError('recorded window differs from raw counters at its permitted origin')
    return {'all_recorded_fields_reproduced': True, 'original_start_recorded': exact,
            'reproduction_witnesses': matches, 'recorded_qualified': saved['qualified'],
            'qualification_changed': False}
