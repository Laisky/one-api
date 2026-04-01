# Practical Implementation Plan: Replacing `bufio.Scanner` for Large Streaming JSON Payloads

## Table of Contents

- [Practical Implementation Plan: Replacing `bufio.Scanner` for Large Streaming JSON Payloads](#practical-implementation-plan-replacing-bufioscanner-for-large-streaming-json-payloads)
  - [Table of Contents](#table-of-contents)
  - [1. Summary](#1-summary)
    - [1.1 Problem](#11-problem)
    - [1.2 Goal](#12-goal)
    - [1.3 Important Scope Decision](#13-important-scope-decision)
  - [2. Current Repository Reality](#2-current-repository-reality)
    - [2.1 Shared infrastructure](#21-shared-infrastructure)
    - [2.2 Current handler patterns](#22-current-handler-patterns)
      - [Pattern A: Parse OpenAI-compatible chunks and optionally rewrite them](#pattern-a-parse-openai-compatible-chunks-and-optionally-rewrite-them)
      - [Pattern B: Faithful SSE forwarding with light parsing](#pattern-b-faithful-sse-forwarding-with-light-parsing)
      - [Pattern C: Full streaming format conversion](#pattern-c-full-streaming-format-conversion)
    - [2.3 Non-standard streaming shapes](#23-non-standard-streaming-shapes)
  - [3. Design Principles](#3-design-principles)
  - [4. Non-Goals](#4-non-goals)
  - [5. Proposed Architecture](#5-proposed-architecture)
    - [5.1 Introduce a streaming line reader first](#51-introduce-a-streaming-line-reader-first)
    - [Behavior requirements](#behavior-requirements)
    - [Why not a full SSE event reader first](#why-not-a-full-sse-event-reader-first)
    - [5.2 Heartbeat wrapper for the new reader](#52-heartbeat-wrapper-for-the-new-reader)
    - [Practical concurrency rule](#practical-concurrency-rule)
    - [5.3 Small-path and large-path contract](#53-small-path-and-large-path-contract)
    - [Small path](#small-path)
    - [Large path](#large-path)
      - [Large path type 1: Raw forwarding is acceptable](#large-path-type-1-raw-forwarding-is-acceptable)
      - [Large path type 2: Limited field extraction is enough](#large-path-type-2-limited-field-extraction-is-enough)
      - [Large path type 3: Full schema rewrite is required](#large-path-type-3-full-schema-rewrite-is-required)
    - [5.4 Logging and error classification](#54-logging-and-error-classification)
  - [6. Handler Classification and Migration Strategy](#6-handler-classification-and-migration-strategy)
    - [6.1 Class 1: Low-risk adopters](#61-class-1-low-risk-adopters)
    - [Candidate handlers](#candidate-handlers)
    - [Why first](#why-first)
    - [6.2 Class 2: OpenAI-compatible typed chunk handlers](#62-class-2-openai-compatible-typed-chunk-handlers)
    - [Candidate handlers](#candidate-handlers-1)
    - [Constraints](#constraints)
    - [6.3 Class 3: Full format conversion handlers](#63-class-3-full-format-conversion-handlers)
    - [Candidate handlers](#candidate-handlers-2)
    - [Constraints](#constraints-1)
    - [6.4 Class 4: Non-standard streaming](#64-class-4-non-standard-streaming)
    - [Candidate handlers](#candidate-handlers-3)
    - [Strategy](#strategy)
  - [7. Recommended Rollout Plan](#7-recommended-rollout-plan)
    - [Phase 0: Test fixtures and acceptance tests](#phase-0-test-fixtures-and-acceptance-tests)
    - [Phase 1: Infrastructure only](#phase-1-infrastructure-only)
    - [Phase 2: Low-risk handler migrations](#phase-2-low-risk-handler-migrations)
    - [Phase 3: OpenAI-compatible typed chunk handlers](#phase-3-openai-compatible-typed-chunk-handlers)
    - [Phase 4: Full conversion handlers](#phase-4-full-conversion-handlers)
    - [Phase 5: Cleanup](#phase-5-cleanup)
  - [8. Detailed Guidance for Large-Path Support](#8-detailed-guidance-for-large-path-support)
    - [8.1 Response API direct stream](#81-response-api-direct-stream)
    - [8.2 OpenAI chat completion stream](#82-openai-chat-completion-stream)
    - [8.3 Anthropic native stream](#83-anthropic-native-stream)
    - [8.4 Anthropic to OpenAI conversion](#84-anthropic-to-openai-conversion)
    - [8.5 OpenAI or Response API to Claude SSE conversion](#85-openai-or-response-api-to-claude-sse-conversion)
    - [8.6 Gemini stream conversion](#86-gemini-stream-conversion)
  - [9. Testing Strategy](#9-testing-strategy)
    - [9.1 Reader tests](#91-reader-tests)
    - [9.2 Heartbeat tests](#92-heartbeat-tests)
    - [9.3 Handler regression tests](#93-handler-regression-tests)
    - [9.4 Large-line test design](#94-large-line-test-design)
  - [10. Risks and Mitigations](#10-risks-and-mitigations)
  - [11. Recommended First PR](#11-recommended-first-pr)
    - [Include](#include)
    - [Do not include](#do-not-include)
    - [Success criteria](#success-criteria)
  - [12. Final Recommendation](#12-final-recommendation)

## 1. Summary

### 1.1 Problem

The current streaming pipeline still relies on `bufio.Scanner` to read upstream SSE streams line by line. That design fails when a single `data:` line becomes very large, which can happen when upstream providers include base64-encoded image or file data inside a JSON chunk.

The current scanner configuration is:

- Initial buffer: `64 KB`
- Maximum token size: `64 MB`

This avoids many failures, but it does not solve the real problem:

1. `bufio.Scanner` must hold the entire line before returning it.
2. The scanner buffer can grow to the maximum and stay large for the lifetime of the stream.
3. Most handlers then call `json.Unmarshal`, which creates a second full in-memory copy of the payload.
4. A payload larger than the configured limit still fails with `bufio.ErrTooLong`.

### 1.2 Goal

Replace the scanner-based line reader with a streaming reader that:

- Removes the hard dependency on a full in-memory line.
- Preserves current handler behavior for normal streams.
- Supports adapter-specific handling for oversized `data:` lines.
- Keeps heartbeat, cancellation, and logging semantics aligned with the current codebase.

### 1.3 Important Scope Decision

This plan does **not** start by introducing a fully generic SSE event abstraction plus a fully generic JSON path extractor.

That approach is theoretically clean, but it does not match the current repository architecture closely enough. Many handlers are still line-oriented and several conversion paths need fully parsed typed chunks rather than raw event forwarding.

The first practical step is:

1. Replace `bufio.Scanner` with a streaming **line reader** that preserves current handler semantics.
2. Keep the existing small-chunk `json.Unmarshal` path for normal traffic.
3. Add **adapter-specific** oversized-line handling only where needed.

## 2. Current Repository Reality

The implementation must be grounded in how streaming works today.

### 2.1 Shared infrastructure

- Scanner configuration lives in `common/helper/scanner.go`.
- Heartbeat support is implemented by `common/render/heartbeat.go`.
- Scanner-specific error classification lives in `common/render/stream_log.go`.

### 2.2 Current handler patterns

The repository does not have one universal streaming pattern.

#### Pattern A: Parse OpenAI-compatible chunks and optionally rewrite them

Examples:

- `relay/adaptor/openai/main.go`
- `relay/adaptor/openai_compatible/unified_streaming.go`

These handlers:

- Read one line at a time.
- Normalize the `data:` prefix.
- `json.Unmarshal` the chunk into `ChatCompletionsStreamResponse`.
- Update usage, token accounting, reasoning extraction, and tool-call state.
- Optionally pass the parsed chunk to `StreamRewriteHandler`.

This is important because `StreamRewriteHandler` expects a fully parsed typed chunk, not raw JSON bytes.

#### Pattern B: Faithful SSE forwarding with light parsing

Examples:

- `relay/adaptor/openai/main.go` `ResponseAPIDirectStreamHandler`
- `relay/adaptor/anthropic/main.go` `ClaudeNativeStreamHandler`

These handlers still parse some fields for usage or local bookkeeping, but the downstream wire format is close to the upstream wire format.

These are the best first adopters for a new line reader.

#### Pattern C: Full streaming format conversion

Examples:

- `relay/adaptor/anthropic/main.go` `StreamHandler`
- `relay/adaptor/openai_compatible/claude_convert.go`
- `relay/adaptor/gemini/main.go`

These handlers do not simply forward upstream bytes. They synthesize new downstream chunks in a different schema.

For these handlers, a generic tee-and-forward design is not enough. If a large payload appears inside a field that must be transformed, the handler needs a provider-specific slow path.

### 2.3 Non-standard streaming shapes

Some upstreams are not standard SSE in practice.

Example:

- Ollama behaves more like JSON Lines and should remain on a separate implementation path.

## 3. Design Principles

1. Preserve current line-oriented semantics first.
2. Keep small chunks on the existing parse path.
3. Add oversized handling per adapter, not via a premature generic abstraction.
4. Avoid changing downstream behavior for normal streams.
5. Preserve the current "honest proxy" behavior around terminal events such as `[DONE]`.
6. Preserve request cancellation and heartbeat behavior.
7. Keep old scanner-based code available until replacements are proven in production-like tests.

## 4. Non-Goals

The following are explicitly out of scope for the first implementation wave:

1. A universal SSE event model for all providers.
2. A universal JSON-path extraction engine for every streaming format.
3. Migrating every adapter in one PR.
4. Replacing Ollama with the same reader used for SSE providers.
5. Deleting `HeartbeatScanner` and scanner-specific utilities in the first phase.

## 5. Proposed Architecture

### 5.1 Introduce a streaming line reader first

The first reusable component should be a **line reader**, not a full SSE event reader.

Reason:

- Current handlers are line-oriented.
- `event:` lines and `data:` lines are often processed separately.
- A line reader is enough to remove the scanner hard limit.
- It minimizes migration cost and reduces regression risk.

Suggested package layout:

```text
common/
  sse/
    line_reader.go
    line_reader_test.go
```

Suggested API shape:

```go
package sse

type LineKind int

const (
    LineKindBlank LineKind = iota
    LineKindComment
    LineKindEvent
    LineKindData
    LineKindOther
)

type Line struct {
    Kind      LineKind
    Prefix    string
    Oversized bool

    // Small is populated when the full line fits in the internal buffer.
    Small []byte

    // Large is populated only for oversized data lines.
    // The caller must fully consume or discard it before calling Next again.
    Large io.Reader
}

type LineReader struct {
    // internal fixed-size bufio.Reader-backed implementation
}

func NewLineReader(r io.Reader, bufSize int) *LineReader

func (r *LineReader) Next() (Line, error)
```

### Behavior requirements

1. Blank lines must be preserved as blank lines.
2. Comment lines beginning with `:` must be recognized.
3. `event:` lines must be readable without special aggregation.
4. `data:` lines that fit in memory should be returned as small lines.
5. Oversized `data:` lines should expose a streaming `io.Reader`.
6. The first version only needs streaming support for oversized `data:` lines.
7. Oversized non-`data:` lines may return an explicit error if needed, because AI upstreams do not realistically send huge `event:` or comment lines.

### Why not a full SSE event reader first

The current code frequently processes one line at a time and maintains local state for `event:` lines separately. A full event aggregator would be a larger semantic change than required for this problem.

We can revisit a true event abstraction later if the line reader proves stable.

### 5.2 Heartbeat wrapper for the new reader

Suggested package layout:

```text
common/
  render/
    heartbeat_line_reader.go
    heartbeat_line_reader_test.go
```

The new wrapper should mirror the current responsibilities of `HeartbeatScanner`:

- Flush headers early.
- Send `:\n` heartbeat comments while waiting for upstream data.
- Stop on client cancellation.
- Track `heartbeats_sent` and `heartbeat_write_error`.

Suggested API shape:

```go
type HeartbeatLineReader struct {
    // similar state to HeartbeatScanner
}

func NewHeartbeatLineReader(c *gin.Context, reader *sse.LineReader, interval time.Duration) *HeartbeatLineReader

func (h *HeartbeatLineReader) Next() (sse.Line, error)

func (h *HeartbeatLineReader) Close()

func (h *HeartbeatLineReader) HeartbeatsSent() int

func (h *HeartbeatLineReader) HeartbeatWriteErr() error
```

### Practical concurrency rule

Do not introduce a background pipe-based copy path unless it is required by a specific handler.

A simpler model is preferred:

1. While `Next()` is waiting for the next line, the wrapper may emit heartbeats.
2. Once `Next()` returns a line, the caller owns that line.
3. If the line is oversized and exposes `Large io.Reader`, the caller consumes it directly.
4. No heartbeat is needed while the caller is actively reading and forwarding a large line, because real downstream traffic is already flowing.

This preserves the current "caller goroutine performs downstream writes" model and avoids unnecessary complexity.

### 5.3 Small-path and large-path contract

We should adopt a dual-path strategy:

### Small path

For lines below a configured threshold:

- Read the full line into memory.
- Reuse the existing `json.Unmarshal` logic.
- Keep current state machines and rewrite hooks unchanged.

This path should handle the overwhelming majority of requests.

### Large path

For oversized `data:` lines:

- Do **not** force one generic implementation on every adapter.
- Choose a strategy based on the handler class.

#### Large path type 1: Raw forwarding is acceptable

For handlers that can safely forward the upstream payload:

- Write `data: `.
- Stream-copy the payload to the downstream writer.
- Write `\n\n`.

Optional local parsing may be skipped or reduced if it is not required for correctness.

#### Large path type 2: Limited field extraction is enough

For handlers that need only a few fields such as:

- `delta.content`
- `finish_reason`
- `usage`
- tool-call argument deltas

Implement a **narrow extractor** for that specific provider format.

Important:

- Do not build a generic JSON-path extractor before we have a concrete adopter.
- A targeted token-walking helper is acceptable if it is scoped to one payload schema.

#### Large path type 3: Full schema rewrite is required

For handlers that must rebuild the downstream chunk in a different shape:

- Anthropic stream to OpenAI chunk
- OpenAI or Response API stream to Claude SSE
- Gemini stream to OpenAI chunk

The handler needs a provider-specific streaming rewrite path.

This is the hardest class and should be migrated later.

### 5.4 Logging and error classification

The current logging helpers are scanner-specific. The replacement should preserve the same operational visibility, but remove scanner-only assumptions.

Required fields:

- `stream_error`
- `stream_termination`
- `heartbeats_sent`
- `heartbeat_write_error`
- client context state when present

The new classification should distinguish at least:

- downstream cancellation
- downstream deadline exceeded
- malformed upstream line
- unexpected reader error

The scanner-specific `scanner_max_token_size` field should be retired only after all scanner-based handlers are migrated.

## 6. Handler Classification and Migration Strategy

### 6.1 Class 1: Low-risk adopters

These are the best first migrations because they are close to faithful forwarding.

### Candidate handlers

- `relay/adaptor/openai/main.go` `ResponseAPIDirectStreamHandler`
- `relay/adaptor/anthropic/main.go` `ClaudeNativeStreamHandler`
- simple proxy-style handlers such as Cloudflare if they do not depend on complex rewrite logic

### Why first

- They already preserve most upstream semantics.
- They need limited parsing for usage or local bookkeeping.
- They provide good validation that the new line reader is correct without immediately touching the most complex rewrite paths.

### 6.2 Class 2: OpenAI-compatible typed chunk handlers

### Candidate handlers

- `relay/adaptor/openai/main.go` `StreamHandler`
- `relay/adaptor/openai_compatible/unified_streaming.go`

### Constraints

These handlers depend on parsed `ChatCompletionsStreamResponse` chunks for:

- reasoning extraction
- token accounting
- usage updates
- `StreamRewriteHandler`

Large-line support for these handlers must not silently break:

- incremental quota enforcement
- response text accumulation
- tool-call assembly
- `HandleChunk` rewrite semantics

If the large payload contains fields required by those features, the handler must have an explicit extractor before oversized support is enabled.

### 6.3 Class 3: Full format conversion handlers

### Candidate handlers

- `relay/adaptor/anthropic/main.go` `StreamHandler`
- `relay/adaptor/openai_compatible/claude_convert.go`
- `relay/adaptor/gemini/main.go`

### Constraints

These handlers synthesize a different downstream schema. For large payloads, they need provider-specific logic.

Examples:

- Anthropic tool delta handling depends on `input_json_delta`.
- Claude conversion depends on reasoning, text deltas, signatures, and tool-call deltas.
- Gemini may need to rebuild downstream `image_url` content from `inline_data`.

These should migrate only after the reader and heartbeat layers are stable.

### 6.4 Class 4: Non-standard streaming

### Candidate handlers

- Ollama

### Strategy

Keep on a separate `JSONLinesReader` or equivalent implementation. Do not force it through the SSE reader path.

## 7. Recommended Rollout Plan

### Phase 0: Test fixtures and acceptance tests

Before changing handlers, add shared tests and fixtures that simulate:

1. Standard small SSE streams.
2. Oversized `data:` lines using a small internal buffer such as `4 KB` or `8 KB` to trigger the large path cheaply.
3. Mixed `event:` and `data:` lines.
4. Client cancellation during a blocked read.
5. Upstream EOF without terminal `[DONE]`.

Acceptance criteria:

- Existing small-stream behavior remains byte-for-byte compatible where expected.
- The large path is exercised without needing enormous test payloads.

### Phase 1: Infrastructure only

Implement:

- `common/sse/line_reader.go`
- `common/render/heartbeat_line_reader.go`
- tests for both

Do not migrate adapters yet.

Acceptance criteria:

- Correct handling of blank, comment, `event:`, and `data:` lines.
- Oversized `data:` lines stream successfully.
- No race conditions under `go test -race`.

### Phase 2: Low-risk handler migrations

Migrate:

- `ResponseAPIDirectStreamHandler`
- `ClaudeNativeStreamHandler`

Why:

- They provide the fastest feedback loop.
- They validate `event:` tracking and light parsing.

Acceptance criteria:

- Downstream SSE output remains compatible with existing tests.
- Heartbeats still work.
- Oversized upstream `data:` lines no longer fail because of scanner limits.

### Phase 3: OpenAI-compatible typed chunk handlers

Migrate:

- `relay/adaptor/openai/main.go` `StreamHandler`
- `relay/adaptor/openai_compatible/unified_streaming.go`

Approach:

1. Keep the existing full-parse small path.
2. Add a guarded oversized slow path.
3. Only enable the slow path once it can preserve:
   - text accumulation
   - reasoning extraction
   - usage handling
   - tool-call deltas
   - rewrite handler behavior

Acceptance criteria:

- Existing chat completion and response bridge tests still pass.
- Incremental quota tracking behavior is unchanged for supported large cases.

### Phase 4: Full conversion handlers

Migrate:

- Anthropic streaming conversion
- OpenAI or Response API to Claude SSE conversion
- Gemini streaming conversion

Approach:

- Implement provider-specific oversized handling.
- Do not attempt to force a generic extractor if it makes the code harder to reason about.

Acceptance criteria:

- Existing conversion tests still pass.
- Large payloads in rewritten schemas are supported without scanner-size failures.

### Phase 5: Cleanup

Only after all required handlers have migrated:

- retire `common/helper/scanner.go`
- retire `HeartbeatScanner`
- simplify logging helpers

## 8. Detailed Guidance for Large-Path Support

### 8.1 Response API direct stream

This is a strong early target.

Current behavior:

- tracks `event:` lines separately
- forwards raw SSE events
- optionally parses chunks for local bookkeeping

Practical oversized strategy:

1. Preserve `event:` lines as small lines.
2. For small `data:` lines, keep current parsing.
3. For oversized `data:` lines:
   - forward the raw payload downstream
   - skip internal parsing if needed
   - do not break downstream wire format

This path can provide immediate value with relatively low risk.

### 8.2 OpenAI chat completion stream

This is the highest-volume path, but it is not the easiest migration.

Current behavior depends on parsed chunks for:

- text accumulation
- reasoning extraction
- incremental token accounting
- tool-call handling
- `StreamRewriteHandler`

Practical oversized strategy:

- Keep small-path behavior unchanged.
- Add oversized support only after a targeted extractor exists for the fields needed by local logic.
- Do not silently forward raw large chunks if that would break quota tracking or rewrite handlers.

### 8.3 Anthropic native stream

This is another strong early target because downstream output is close to upstream output.

Current behavior:

- forwards `data:` payloads directly
- parses usage fields locally

Practical oversized strategy:

- forward large `data:` payloads directly
- parse usage only when feasible
- prefer correctness of downstream stream delivery over local optional metadata

### 8.4 Anthropic to OpenAI conversion

This handler is stateful and tool-call aware.

Large-path support must preserve:

- content block start and stop semantics
- `input_json_delta`
- usage accumulation
- finish reasons

This requires a dedicated slow path. Do not treat it as a generic extractor problem.

### 8.5 OpenAI or Response API to Claude SSE conversion

This handler emits Claude-native SSE events and maintains local state for:

- text blocks
- thinking blocks
- signature deltas
- tool-use blocks
- usage

A large payload inside any of those fields requires a dedicated rewrite path.

### 8.6 Gemini stream conversion

Gemini can emit large `inline_data` parts that are converted into OpenAI-style `image_url` content.

This is not a pass-through case. The downstream JSON shape changes, so large-path handling must be provider-specific.

If this path is not needed immediately, it should be migrated after the core reader infrastructure is proven elsewhere.

## 9. Testing Strategy

### 9.1 Reader tests

Add unit tests for:

1. Small `data:` lines.
2. Oversized `data:` lines.
3. `event:` lines.
4. Comment lines.
5. Blank lines.
6. EOF without trailing newline.
7. CRLF handling if needed by upstream compatibility.

### 9.2 Heartbeat tests

Mirror the current heartbeat coverage:

1. headers are flushed early
2. heartbeats are sent while upstream is idle
3. no heartbeats are sent when data is flowing
4. client cancellation stops processing
5. heartbeat diagnostics are recorded

### 9.3 Handler regression tests

For each migrated handler:

1. keep all existing small-stream tests
2. add a large-line test that triggers the oversized path
3. verify exact downstream SSE framing
4. verify terminal event behavior remains honest
5. verify race-safety under `go test -race`

### 9.4 Large-line test design

Avoid huge memory-heavy tests in unit suites.

Instead:

- configure the line reader buffer to a small size in tests
- use payloads of a few hundred kilobytes or a few megabytes
- verify that the oversized path is used

This gives deterministic coverage without creating fragile test runtime costs.

## 10. Risks and Mitigations

| Risk                                            | Impact                                | Mitigation                                                                                |
| ----------------------------------------------- | ------------------------------------- | ----------------------------------------------------------------------------------------- |
| We over-generalize too early                    | Large refactor with unclear ownership | Start with a line reader, not a universal event model                                     |
| Oversized support breaks typed rewrite handlers | Incorrect downstream chunks           | Keep small-path logic unchanged and require adapter-specific slow paths                   |
| Heartbeat refactor introduces race conditions   | Flaky tests or production instability | Preserve caller-goroutine writes and cover with race tests                                |
| Incremental quota tracking becomes inaccurate   | Billing or quota regressions          | Do not enable oversized support for those handlers until required fields can be extracted |
| Migration order is too aggressive               | Hard-to-debug regressions             | Start with low-risk faithful-forwarding handlers                                          |
| We delete scanner utilities too early           | Cross-adapter regressions             | Keep old and new paths side by side until all target handlers are migrated                |

## 11. Recommended First PR

The first PR should be intentionally small and practical.

### Include

1. `common/sse/line_reader.go`
2. `common/sse/line_reader_test.go`
3. `common/render/heartbeat_line_reader.go`
4. `common/render/heartbeat_line_reader_test.go`
5. one low-risk handler migration:
   - preferably `ResponseAPIDirectStreamHandler`
   - optionally `ClaudeNativeStreamHandler` in the same PR if the change stays readable

### Do not include

1. OpenAI chat completion stream migration
2. Anthropic to OpenAI conversion migration
3. Claude conversion migration
4. Gemini conversion migration
5. scanner cleanup or code deletion

### Success criteria

The PR is successful if:

1. it removes scanner-size failures for at least one real streaming path
2. it preserves current behavior for existing small-stream tests
3. it introduces a reusable reader primitive that later handlers can adopt

## 12. Final Recommendation

The repository should move away from `bufio.Scanner` for streaming JSON payloads, but it should do so in a way that matches the current code structure.

The most practical plan is:

1. build a streaming line reader
2. add a heartbeat wrapper for that reader
3. migrate low-risk handlers first
4. migrate typed and rewritten handlers only when each one has a clear oversized slow path

This plan solves the real memory and hard-limit problem without turning the first implementation into a large speculative framework rewrite.
