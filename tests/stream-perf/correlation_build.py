"""Generate a byte-pinned diagnostic-only Go overlay; never edit tracked production files."""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path

PINS = {
    'common/render/render.go': 'fcbe67c4ddee77ccd884aa4100e2d66ca0bbec08',
    'tests/stream-perf/main.go': '63aedab2ce54ca8e17234bfa7d6fbb2218fb4f56',
    'tests/stream-perf/protocol.go': '80f6e72dc3db58831804347c526c9a428ebdfc06',
}
IMPORT = '\n\t"github.com/Laisky/one-api/tests/stream-perf/observation"\n'


def blob(data: bytes) -> str:
    """blob computes the Git identity of an exact source file without consulting mutable refs."""
    return hashlib.sha1(b'blob ' + str(len(data)).encode() + b'\0' + data).hexdigest()


def replace_once(text: str, old: str, new: str) -> str:
    """replace_once rejects structural drift instead of guessing a new instrumentation position."""
    if text.count(old) != 1:
        raise ValueError('diagnostic overlay anchor is absent or ambiguous')
    return text.replace(old, new, 1)


def overlay_sources(sources: dict[str, bytes]) -> dict[str, str]:
    """overlay_sources instruments only pinned files; returns copies and never writes the originals."""
    if set(sources) != set(PINS) or any(blob(sources[name]) != pin for name, pin in PINS.items()):
        raise ValueError('unsupported source revision; review anchors and behavior before updating pins')
    texts = {name: data.decode() for name, data in sources.items()}
    for name in texts:
        texts[name] = replace_once(texts[name], 'import (', 'import (' + IMPORT)
    name = 'common/render/render.go'
    texts[name] = replace_once(texts[name], '''func StringData(c *gin.Context, str string) {
\tstr = strings.TrimPrefix''', '''func StringData(c *gin.Context, str string) {
\tvar observed observation.Span
\tif c.Request != nil {
\t\tframe := c.GetInt(observation.StateKey)
\t\tobserved = observation.Begin(c.Request.Context(), c.Request.Header.Get(observation.Header), frame)
\t\tif observed.Tracked() { c.Set(observation.StateKey, frame+1) }
\t}
\tstr = strings.TrimPrefix''')
    texts[name] = replace_once(texts[name], '''c.Render(-1, common.CustomEvent{Data: "data: " + str})
\tc.Writer.Flush()''', '''c.Render(-1, common.CustomEvent{Data: "data: " + str})
\tobserved.BeforeFlush()
\tc.Writer.Flush()
\tobserved.AfterFlush()''')
    name = 'tests/stream-perf/main.go'
    texts[name] = replace_once(texts[name], '\tr := runLoad(o, client, key)', '''
	clientTrace, traceErr := observation.StartClientTrace(os.Getenv(observation.ClientTraceEnvironment))
	if traceErr != nil { return traceErr }
	var traceCloseErr error
	r := func() result {
		defer func() { traceCloseErr = clientTrace.Close() }()
		return runLoad(o, client, key)
	}()
	if traceCloseErr != nil { return traceCloseErr }''')
    texts[name] = replace_once(texts[name], 'type sample struct {', '''type sample struct {
\tObservedEventNS []int64 `json:"observed_event_ns,omitempty"`
\tobserve bool''')
    texts[name] = replace_once(texts[name], 'func execute(o options) error {', '''func execute(o options) error {
\tif o.mode != "mock" && (o.requests > observation.MaxRequests || o.chunks > 1024) {
\t\treturn fmt.Errorf("correlation overlay allows at most 8192 requests and 1024 chunks")
\t}''')
    texts[name] = replace_once(texts[name], '\ts.Index = j.index', '\ts.Index = j.index\n\ts.observe = observation.Selected(j.index)')
    texts[name] = replace_once(texts[name], '\treq.Header.Set("Accept", "text/event-stream")', '''\treq.Header.Set("Accept", "text/event-stream")
\tif s.observe { req.Header.Set(observation.Header, strconv.Itoa(j.index)) }''')
    name = 'tests/stream-perf/protocol.go'
    texts[name] = replace_once(texts[name], '\t\tpayload := strings.Join(data, "\\n")', '''\t\tpayload := strings.Join(data, "\\n")
\t\tif s.observe {
\t\t\tvar observationErr error
\t\t\ts.ObservedEventNS, observationErr = observation.ObserveClient(s.ObservedEventNS, s.Index)
\t\t\tif observationErr != nil { return observationErr }
\t\t}''')
    return texts


def prepare(repository: Path, output: Path) -> dict:
    """prepare creates an exclusive overlay directory after validating every immutable input."""
    repository, output = repository.resolve(), output.resolve()
    sources = {name: (repository / name).read_bytes() for name in PINS}
    transformed = overlay_sources(sources)
    output.mkdir(parents=True, exist_ok=False, mode=0o700)
    replacements, entries = {}, {}
    for name, text in transformed.items():
        target = output / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text)
        replacements[str(repository / name)] = str(target)
        entries[name] = {'original_git_blob': PINS[name], 'overlay_sha256': hashlib.sha256(text.encode()).hexdigest()}
    descriptor = {'Replace': replacements}
    (output / 'overlay.json').write_text(json.dumps(descriptor, indent=2) + '\n')
    manifest = {'schema_version': 2, 'diagnostic_only': True, 'sources': entries,
                'client_trace': 'explicit loopback listener; same-process numeric observation markers only',
                'request_stride': 32, 'max_requests': 8192, 'max_events_per_request': 2048,
                'clock': 'Linux CLOCK_MONOTONIC; verify epoch identity or restrict comparisons to per-process intervals',
                'render_semantics': 'StringData only; call/flush order and data bytes unchanged',
                'limitations': 'Instrumentation overhead exists. Flush return is not packet arrival. Not an A/B speedup.'}
    (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    return manifest


def main() -> None:
    """main generates a source overlay for separately named diagnostic gateway and driver binaries."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repository', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(prepare(args.repository, args.output), indent=2))


if __name__ == '__main__':
    main()
