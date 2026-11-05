# Application log fidelity and explicit limits

The optional OTLP application-log sink preserves the message and fields supplied
by the application after its chosen Zap encoding. It does not redact text, replace
messages, select diagnostic keys, rename fields, flatten objects, repair invalid
text, or change severity text. Unicode, multiline messages, original errors,
unknown fields, native SDK EventName and structured SDK Body values are retained.
The local file/stdout sinks keep their existing encoding and behavior.

## Encoding and metadata

Zap Object/Array/Namespace fields retain nested maps and lists. Namespace state
continues across With calls, and bound serializers run at binding time rather
than being evaluated again on every log. Reflected values follow Zap's JSON
contract, including exported fields, JSON tags, explicit marshalers and nil values.
The original error field is retained; SDK exception metadata is additive. Bound
error text/type metadata are snapshots, so later logs do not reevaluate or retain
the original bound error. Explicit exception attributes take precedence.
Generated caller metadata is added only when the application did not supply that
key. Arbitrary context-valued application fields are not implicit trace carriers;
SpanContextField/SpanContextFieldFrom are the explicit correlation API.

Native SDK scope name/version/schema/attributes and resource attributes are
preserved in their OTLP protocol namespaces. A record-level service.name does not
replace resource service.name. Attribute.Set inputs already have the SDK's
canonical-map semantics.

### Repeated keys

Zap allows a field key to appear more than once on a record, and it happens in
ordinary use: the process logger binds `host` (the container hostname) and the
gin request logger binds `host` again (the HTTP Host header), so every
request-scoped record carries it twice. OTLP requires each key to appear once in
a record's attributes and in every nested map. The OpenTelemetry common
specification resolves a key set twice by letting the later value overwrite the
earlier one, and that is what both the Zap bridge and the native SDK path do,
at every depth (record attributes, nested maps, maps inside lists, and map
bodies): the first occurrence keeps its position and takes the last value. On a
request record, `host` is therefore the HTTP Host header; the container identity
remains in the local log line.

The result is identical to the OpenTelemetry Go SDK's own deduplication, except
in one case where the pinned SDK (sdk/log v0.22.0) is not conformant -- an
overwriting value whose own map repeats a key -- where the bridge produces the
conformant result. `TestNormalizeMatchesSDKDeduplication` pins both. The bridge
applies the rule itself rather than enabling the SDK's deduplication because the
SDK deduplicates one attribute at a time, which measured about 27% slower on a
request-scoped record; see
`docs/benchmarks/20261104_otlp-app-log-key-collisions.md`.

A record whose key repeated is exported, never dropped, and increments the
`deduplicated` outcome in addition to its other outcomes, so an overwritten value
is visible in metrics. Before this rule, such records were rejected whole with
`dropped_encoding`, which removed every request-scoped line from the export.

OTLP represents signed int64, doubles (including NaN/Inf), booleans, strings,
bytes, maps, lists and empty values. Native unsigned values above MaxInt64 use an
exact decimal string because OTLP has no unsigned integer type. Zap duration
fields retain signed int64 nanoseconds. Zap time attributes retain signed int64
nanoseconds when representable, or exact RFC3339Nano text outside that range
(including zero time); mixed time arrays may therefore contain both types.
Native record/observed timestamps instead accept zero as unset and otherwise
require nonnegative signed UnixNano values for the pinned SDK exporter;
this is narrower than OTLP's uint64 timestamp range.

Reflected JSON integers retain int64 precision or exact text. Decimal/exponent
tokens use double64 only if its JSON round trip has the same numeric value;
otherwise the original token is text. Large-number comparison work is bounded,
with exact text as the fallback. JSON reflection rejects values its encoder cannot
represent, such as cycles, channels and nonfinite JSON numbers. No arbitrary Go
internals are exposed through a formatting fallback. Ordinary JSON-visible strings
are checked for invalid UTF-8 before token decoding. Explicit JSON/Text marshalers
choose their JSON projection and run once; encoding/json's own TextMarshaler
Unicode replacement is part of that selected projection, not reinterpreted by the
bridge. Maps with custom TextMarshaler keys likewise use the complete chosen JSON
projection, avoiding a second key callback to reconstruct field paths. Invalid UTF-8 in native text or ordinary reflected strings, and unpaired
surrogate escapes in explicit JSON output, fail instead of being repaired. Bridge validation and decoded JSON projection have a 256-edge stack/work budget;
this is an application guard, not an OTLP protocol limit. It does not bound arbitrary
application marshaler execution or encoding/json's initial traversal/allocation.

Encoding failures reject the complete OTLP record and increment the fixed
dropped_encoding outcome. Bridge failures return a fixed encoding error; native
SDK processor failures use the SDK error handler. Diagnostics contain no failed
field value. A serializer failure never exports the successfully encoded subset.
The bridge's exported ErrLogEncoding is a shared rejection marker. Its original
identity and fixed message are preserved at the bridge boundary; its stored stack
reflects marker initialization, not the particular rejection site. Rejection
details and application values are not attached to this diagnostic.
Once the bridge knows that a record cannot be encoded, it stops evaluating
further OTLP field serializers; entry and previously bound-state validation run
before field conversion. Namespace overflow also rejects promptly. Other Zap
sinks retain their independent behavior. These short circuits do not impose a
time/memory limit on arbitrary application serializers while they are running.

## Operator limits and overload

The SDK's automatic attribute truncation is disabled, and its deduplication is
replaced by the equivalent rule described under "Repeated keys". The
default LOG_OTLP_MAX_ATTRIBUTES=0 and LOG_OTLP_MAX_ATTRIBUTE_VALUE_BYTES=0 impose
no content limits. An explicitly configured positive attribute-count or text/
binary-leaf byte limit rejects the complete record with dropped_operator_limit;
it never silently removes keys or truncates values. This changes older positive
settings from partial/truncated export to observable whole-record rejection.
Set them to zero to request unrestricted content within the queue resource budget.

The existing LOG_OTLP_QUEUE_SIZE and LOG_OTLP_QUEUE_MAX_MB still bound admission.
Body, EventName, severity text and attribute values participate in byte accounting.
Overload, exporter failures, startup-not-ready and shutdown losses retain explicit
outcome counters. The `emitted` outcome counts Zap-bridge handoffs, not native SDK
log calls; `exported` includes both paths, so these two labels are not a
native-inclusive delivery fraction. These best-effort resource limits do not
prove durable storage; the local sink remains required.

## Explicit application policy

Existing caller-selected logger.Redact and application-selected payload handling
remain application intent. No new automatic redaction option is introduced.
NewPrivacyCore remains a deliberately selected legacy API with its historical
constant-body/field-restriction contract; the production OTLP sink does not select
it. The normal SDK provider does not silently force a private scope or resource.
Applications are responsible for deciding what they log. When full-fidelity OTLP
is enabled, request identity fields (such as UUIDs and names), error text and
other application-selected data can leave the process. Before enabling export,
review collector authentication, transport security, access to queries and
retention. Do not add mandatory field filtering or silent redaction in transport.

## Qualification boundaries

Synthetic tests cover the actual Zap to SDK to bounded batch to HTTP OTLP
protobuf exporter, including original credential-shaped synthetic strings, typed
fields and separate resource/scope/record namespaces. These are fidelity fixtures,
not production payloads or real credentials.

HTTP export acceptance is separate from collector/storage compatibility. In
particular, a backend that flattens namespaces, discards empty values or collides
record keys with resource keys does not satisfy this contract. A compatible
end-to-end collector/storage path must be qualified before release; no source-only
or local HTTP result claims that downstream gate has passed.
