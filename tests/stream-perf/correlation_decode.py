"""Decode numeric SSE markers and trace state headers without retaining request payloads or stacks."""
from __future__ import annotations
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import re
import subprocess
import threading

HEADER = re.compile(r'^M=\S+ P=\S+ G=(-?\d+) (\w+) Time=(\d+)(.*)$')
LOG = re.compile(r' Category="oneapi\.sse" Message="(\d+)/(\d+)/(begin|flush_start|flush_end)/(\d+)"$')
STATE = re.compile(r' GoID=(\d+) (\w+)->(\w+) Reason="([^"]*)"$')
MAX_BYTES = 128 << 20
RANGE = re.compile(r' Name="([^"]+)" Scope=(\S+)')


def select(line: str) -> dict | None:
    """select parses only recognized trace headers and fails closed on malformed observation markers."""
    header = HEADER.match(line.rstrip('\n'))
    if not header:
        return None
    gid, kind, at, tail = header.groups()
    base = {'kind': kind, 'time': int(at), 'g': int(gid)}
    if kind == 'Log' and 'Category="oneapi.sse.error"' in tail:
        raise ValueError('gateway observation error recorded in trace')
    if kind == 'Log' and 'Category="oneapi.sse"' in tail:
        match = LOG.search(tail)
        if not match:
            raise ValueError('malformed observation marker')
        request, frame, phase, mono = match.groups()
        if not (0 <= int(request) < 8192 and int(request) % 32 == 0 and 0 <= int(frame) < 2048 and int(mono) > 0 and int(gid) >= 0):
            raise ValueError('invalid observation bounds')
        return {**base, 'request': int(request), 'frame': int(frame), 'phase': phase, 'mono': int(mono)}
    if kind == 'StateTransition' and ' GoID=' in tail:
        match = STATE.search(tail)
        if not match:
            raise ValueError('unsupported goroutine state header')
        target, before, after, reason = match.groups()
        return {**base, 'target_g': int(target), 'before': before, 'after': after, 'reason': reason}
    if kind in ('RangeBegin', 'RangeEnd', 'RangeActive'):
        match = RANGE.search(tail)
        if match and ('stop-the-world' in match[1] or 'mark assist' in match[1]):
            return {**base, 'name': match[1], 'scope': match[2]}
    return None


def decode(tool: Path, trace: Path, output: Path, timeout: int = 120) -> dict:
    """decode streams a pinned Go decoder with a wall deadline and bounded retained output, publishing only complete success."""
    if not 1 <= timeout <= 300:
        raise ValueError('invalid decoder deadline')
    if output.exists() or output.with_suffix('.partial').exists() or output.with_suffix('.meta.json').exists():
        raise FileExistsError('refusing to overwrite decoded evidence')
    count = Counter()
    temporary = output.with_suffix('.partial')
    stderr = output.with_suffix('.stderr')
    output.parent.mkdir(parents=True, exist_ok=True)
    command = [str(tool.resolve()), '-d=parsed', str(trace.resolve())]
    with stderr.open('x') as errors, temporary.open('x') as records:
        process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=errors, text=True)
        expired = threading.Event()
        def stop():
            """stop bounds a silent or stalled decoder rather than waiting indefinitely for its next line."""
            expired.set()
            try:
                process.kill()
            except ProcessLookupError:
                pass
        timer = threading.Timer(timeout, stop)
        timer.start()
        total = 0
        try:
            for line in process.stdout:
                event = select(line)
                if event is None:
                    continue
                count[event['kind']] += 1
                encoded = json.dumps(event, separators=(',', ':')) + '\n'
                total += len(encoded)
                if total > MAX_BYTES:
                    raise ValueError('retained decoder output exceeds 128 MiB')
                records.write(encoded)
            status = process.wait(timeout=5)
            if status or expired.is_set() or not count['Log']:
                raise RuntimeError('decoder failed, expired or captured no markers')
        finally:
            timer.cancel()
            if process.poll() is None:
                process.kill()
                process.wait(timeout=5)
            process.stdout.close()
    # Hard link publication cannot overwrite another collector's result.
    output.hardlink_to(temporary)
    temporary.unlink()
    result = {'command': command, 'exit_code': status, 'counts': dict(count),
            'tool_sha256': hashlib.sha256(tool.read_bytes()).hexdigest(),
            'trace_sha256': hashlib.sha256(trace.read_bytes()).hexdigest(),
            'records_sha256': hashlib.sha256(output.read_bytes()).hexdigest(), 'records_bytes': total}
    with output.with_suffix('.meta.json').open('x') as metadata:
        json.dump(result, metadata, indent=2)
        metadata.write('\n')
    return result


def main() -> None:
    """main saves filtered trace records and their exact decoder/source identities."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--trace-tool', type=Path, required=True)
    parser.add_argument('--trace', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(decode(args.trace_tool, args.trace, args.output), indent=2))


if __name__ == '__main__':
    main()
