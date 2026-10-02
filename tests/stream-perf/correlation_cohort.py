"""Decode only sampled gateway goroutine states in two bounded, identity-checked offline passes."""
from __future__ import annotations
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import subprocess
import threading
import time

from correlation_decode import HEADER, MAX_BYTES, select


def sha256(path: Path) -> str:
    """sha256 hashes one file incrementally without retaining a second in-memory copy."""
    digest = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1 << 20), b''):
            digest.update(block)
    return digest.hexdigest()


def decode_cohort(tool: Path, trace: Path, output: Path, timeout: int = 120) -> dict:
    """decode_cohort preserves all markers/GC ranges and full state history for marker-owning goroutines.

    Discovery sees the whole pinned trace before state filtering. No state is
    dropped based on latency, and a changed second pass fails. The retained
    record bound stays 128 MiB; no live workload or profiling is performed here.
    """
    if type(timeout) is not int or not 1 <= timeout <= 300:
        raise ValueError('invalid cohort-decoder deadline')
    if not 0 < trace.stat().st_size <= MAX_BYTES:
        raise ValueError('trace input exceeds the bounded capture contract')
    partial, metadata = output.with_suffix('.partial'), output.with_suffix('.meta.json')
    errors = [output.with_suffix(f'.pass{n}.stderr') for n in (1, 2)]
    if any(path.exists() for path in [output, partial, metadata, *errors]):
        raise FileExistsError('refusing to overwrite cohort evidence')
    output.parent.mkdir(parents=True, exist_ok=True)
    identity, tool_identity = sha256(trace), sha256(tool)
    command = [str(tool.resolve()), '-d=parsed', str(trace.resolve())]
    deadline = time.monotonic() + timeout
    gids, counts, passes = set(), Counter(), []
    total = 0
    with partial.open('x') as records:
        for stage, error_path in enumerate(errors):
            marker_digest = hashlib.sha256()
            marker_count, header_count, states_seen = 0, 0, 0
            first, last, last_written = None, None, None
            with error_path.open('x') as stderr:
                process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=stderr, text=True)
                expired = threading.Event()

                def terminate():
                    """terminate kills a silent decoder at the single end-to-end deadline."""
                    expired.set()
                    try:
                        process.kill()
                    except ProcessLookupError:
                        pass

                remaining = deadline - time.monotonic()
                timer = threading.Timer(max(0, remaining), terminate)
                timer.start()
                try:
                    for line in process.stdout:
                        header = HEADER.match(line.rstrip('\n'))
                        if header is None:
                            continue
                        at = int(header[3])
                        if last is not None and at < last:
                            raise ValueError('non-monotonic decoder headers')
                        first = at if first is None else first
                        last = at
                        header_count += 1
                        # The first pass needs all observation errors/markers but no state objects.
                        event = select(line) if stage or 'Category="oneapi.sse' in line else None
                        if event is None:
                            continue
                        if event['kind'] == 'Log':
                            encoded = json.dumps(event, sort_keys=True, separators=(',', ':')).encode()
                            marker_digest.update(encoded + b'\n')
                            marker_count += 1
                            if stage == 0:
                                gids.add(event['g'])
                                if len(gids) > 256:
                                    raise ValueError('too many sampled gateway goroutines')
                        if stage == 0:
                            continue
                        if event['kind'] == 'StateTransition':
                            states_seen += 1
                            if event['target_g'] not in gids:
                                continue
                        encoded = json.dumps(event, separators=(',', ':')) + '\n'
                        total += len(encoded.encode())
                        if total > MAX_BYTES:
                            raise ValueError('retained cohort output exceeds 128 MiB')
                        records.write(encoded)
                        counts[event['kind']] += 1
                        last_written = at
                    status = process.wait(timeout=5)
                    if status or expired.is_set() or not marker_count:
                        raise RuntimeError('cohort decoder failed, expired or captured no markers')
                finally:
                    timer.cancel()
                    timer.join()
                    if process.poll() is None:
                        process.kill()
                        process.wait(timeout=5)
                    process.stdout.close()
            observed = {'marker_sha256': marker_digest.hexdigest(), 'markers': marker_count,
                        'header_count': header_count, 'first_time': first, 'last_time': last}
            passes.append(observed)
            if stage and passes[0] != observed:
                raise ValueError('decoder passes disagree on trace identity or markers')
            if stage and last_written != last:
                # Explicitly synthetic boundary preserves the original trace end for open state intervals.
                encoded = json.dumps({'kind': 'TraceBoundary', 'time': last, 'g': -1}, separators=(',', ':')) + '\n'
                total += len(encoded.encode())
                if total > MAX_BYTES:
                    raise ValueError('retained cohort output exceeds 128 MiB')
                records.write(encoded)
                counts['TraceBoundary'] += 1
    if sha256(trace) != identity or sha256(tool) != tool_identity:
        raise ValueError('decoder input or tool changed during processing')
    output.hardlink_to(partial)
    partial.unlink()
    result = {'algorithm': 'two-pass-sampled-goroutine-states-v1', 'command': command, 'exit_code': 0,
              'counts': dict(counts), 'selected_gateway_goroutines': sorted(gids), 'passes': passes,
              'states_seen': states_seen, 'states_retained': counts['StateTransition'],
              'all_gc_ranges_retained': True, 'synthetic_boundary': 'TraceBoundary is the last parsed header time, not an observed runtime event.',
              'tool_sha256': tool_identity, 'trace_sha256': identity,
              'records_sha256': sha256(output), 'records_bytes': total}
    with metadata.open('x') as target:
        json.dump(result, target, indent=2, allow_nan=False)
        target.write('\n')
    return result


def main() -> None:
    """main decodes a bounded captured trace without changing or overwriting its original evidence."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--trace-tool', type=Path, required=True)
    parser.add_argument('--trace', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(decode_cohort(args.trace_tool, args.trace, args.output), indent=2))


if __name__ == '__main__':
    main()
