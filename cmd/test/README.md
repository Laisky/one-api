# one-api Regression Harness

`go run ./cmd/test` exercises every configured upstream adaptor across the Chat Completions, Response API, and Claude Messages surfaces. For each model the tool fires streaming, non-streaming, and tool-invocation calls, using consistent hyperparameters (`temperature`, `top_p`, `top_k`) to catch regressions before they reach production.

## Environment variables

| Variable               | Required | Default                                                                       | Description                                                                |
| ---------------------- | -------- | ----------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| `API_TOKEN`            | ✅       | _none_                                                                        | One-API token with access to the models under test.                        |
| `API_BASE`             | ❌       | `https://oneapi.laisky.com`                                                   | Base URL for the relay instance. Trailing slash is trimmed automatically.  |
| `ONEAPI_TEST_MODELS`   | ❌       | `gpt-4o-mini,gpt-5-mini,claude-3.5-haiku,gemini-2.5-flash,openai/gpt-oss-20b` | Comma/semicolon/whitespace separated model list.                           |
| `ONEAPI_TEST_VARIANTS` | ❌       | _(all default variants)_                                                      | Optional subset selector (accepts keys, headers, aliases, or type groups). |

> **Tip**: The parser accepts commas, semicolons, line breaks, or plain whitespace, so strings such as `"gpt-4o claude-3"` and multiline lists both work.

## What the harness does

- Sends _eighteen_ requests per model:
  - Chat Completions with `stream=false`, `stream=true`, `tool` + `stream=false`, `tool` + `stream=true`, `structured` + `stream=false`, and `structured` + `stream=true`.
  - Response API with `stream=false`, `stream=true`, `tool` + `stream=false`, `tool` + `stream=true`, `structured` + `stream=false`, and `structured` + `stream=true`.
  - Claude Messages with `stream=false`, `stream=true`, `tool` + `stream=false`, `tool` + `stream=true`, `structured` + `stream=false`, and `structured` + `stream=true`.
- Applies consistent sampling parameters:
  - `temperature = 0.7`
  - `top_p = 0.9`
  - `top_k = 40` (Claude-only, ignored elsewhere)
  - `max_tokens`/`max_output_tokens = 2048`
- Records full request payloads and truncated responses for every failure.
- Uses a shared HTTP client with concurrency (`errgroup`) to keep suites fast.
- Classifies outcomes as **PASS**, **FAIL**, or **SKIP** (unsupported feature combinations).

Streaming responses are captured by accumulating the opening SSE/event payload. If the upstream rejects streaming (`"streaming is not supported"`, HTTP 405, etc.), the harness marks the attempt as `SKIP` instead of failing the whole run.

## Running the suite

```bash
API_TOKEN=sk-... ONEAPI_TEST_MODELS="gpt-4o-mini,claude-3.5-haiku" go run ./cmd/test
```

The command exits **non-zero** when at least one request fails (excluding skips). Unsupported combinations still appear in the report but do not flip the exit code.

### Sample output (trimmed)

```text
=== One-API Regression Matrix ===
┌───────────────┬──────────────────────┬──────────────────────┬────────────────────────────┬────────────────────────────┬──────────────────────┬──────────────────────┬────────────────────────────┬────────────────────────────┬──────────────────────┬──────────────────────┬────────────────────────────┬────────────────────────────┐
│ Model         │ Chat (stream=false) │ Chat (stream=true)   │ Chat Tools (stream=false)  │ Chat Tools (stream=true)   │ Response (stream=false) │ Response (stream=true) │ Response Tools (stream=false) │ Response Tools (stream=true) │ Claude (stream=false) │ Claude (stream=true) │ Claude Tools (stream=false) │ Claude Tools (stream=true) │
├───────────────┼──────────────────────┼──────────────────────┼────────────────────────────┼────────────────────────────┼──────────────────────┼──────────────────────┼────────────────────────────┼────────────────────────────┼──────────────────────┼──────────────────────┼────────────────────────────┼────────────────────────────┤
│ gpt-4o-mini   │ PASS 32ms            │ PASS 41ms            │ PASS 55ms                  │ PASS 59ms                  │ PASS 58ms            │ SKIP stream disabled │ PASS 65ms                   │ SKIP stream disabled       │ PASS 27ms            │ PASS 29ms            │ PASS 45ms                   │ PASS 47ms                   │
│ openai/gpt-oss│ FAIL upstream 400    │ SKIP stream disabled │ FAIL tool unsupported      │ SKIP stream disabled       │ FAIL unknown field   │ SKIP stream disabled │ FAIL tool unsupported       │ SKIP stream disabled       │ PASS 30ms            │ PASS 31ms            │ PASS 52ms                   │ PASS 54ms                   │
└───────────────┴──────────────────────┴──────────────────────┴────────────────────────────┴────────────────────────────┴──────────────────────┴──────────────────────┴────────────────────────────┴────────────────────────────┴──────────────────────┴──────────────────────┴────────────────────────────┴────────────────────────────┘

Totals  | Requests: 24 | Passed: 14 | Failed: 4 | Skipped: 6

Failures:
- openai/gpt-oss-20b · Chat (stream=false) → upstream error payload: ...
- openai/gpt-oss-20b · Chat Tools (stream=false) → tool unsupported by upstream
- openai/gpt-oss-20b · Response (stream=false) → unknown field `messages`
- openai/gpt-oss-20b · Response Tools (stream=false) → tool unsupported by upstream

```

## Logs & troubleshooting

- Every failure log includes:
  - Model, variant, HTTP status, and duration.
  - Truncated request body (max 2 KB) for reproducibility.
  - Truncated response body (max 2 KB) to surface upstream errors.
- Skipped scenarios log at `INFO` with the reason (usually a missing streaming capability).
- Successful requests stay at `INFO` with compact metadata only.

## Extending the harness

- Add new models by setting `ONEAPI_TEST_MODELS` or editing `defaultTestModels`.
- Introduce additional request variants by expanding `requestVariants`—the reporting table and failure summaries update automatically.
- Hyperparameters (`defaultTemperature`, `defaultTopP`, `defaultTopK`, `defaultMaxTokens`) live in `constants.go` for easy tuning.
- The default variant catalog lives in `variants.go`; add new entries there to have them picked up automatically.
- New failure modes should update `isUnsupportedCombination` so that intentional limitations continue to surface as `SKIP` instead of false negatives.

## Exit codes

| Exit code | Meaning                                           |
| --------- | ------------------------------------------------- |
| `0`       | All runs passed or were skipped.                  |
| `1`       | At least one request failed (genuine regression). |

Happy regression hunting!
