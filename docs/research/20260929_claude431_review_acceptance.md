# Claude PR 431: resumed review and acceptance

Reviewed base: `efd4ec2a326deb2f2b6eb8a72084adc51d6a7b90`.
This follow-up retains the multi-cloud catalog and request/receipt fixes from that
commit. It addresses the remaining inline review findings and the actual failure
in repository CI run `36506709366`, rather than treating earlier targeted tests
as full-project acceptance.

## Pricing decision remains settled

The owner approved Claude-equivalent Bedrock Sonnet 5.5 defaults: USD per million
tokens = input 2, output 10, cache read 0.20, five-minute write 2.50, one-hour write
4. Published Bedrock IDs are sufficient for catalog registration. Operator
channel overrides remain authoritative. There is no additional tariff-approval
gate, and no claim of independent AWS invoice reconciliation.

## Fixes and acceptance boundaries

| Finding | Repair | Regression coverage |
| --- | --- | --- |
| Existing tool-name round-trip fixture omitted Claude termination events | Supply both `content_block_stop` and `message_stop`, retaining the original tool-name assertions and strict production stream parser. | Complete stream finishes exactly once; remove the block stop, message stop, or final message delta and require an error with no success marker. |
| Shared converter emitted `finish_reason: ""` for incremental events | Leave finish reason unset until the event contains an actual stop reason. | Text/tool starts and deltas, pings, usage-only deltas, and a real terminal tool call. |
| Newly retained Claude fields leaked through shared Chat DTOs | Copy-on-write sanitization at ordinary Chat, native Messages conversion, Responses fallback, and MCP serialization boundaries. Strip only Claude-specific `output_config`, opaque thinking siblings and block binding from shared Chat DTOs; preserve portable type/budget and caller-owned input. | Real strict HTTP fixtures for all four paths, provider-specific Claude positive controls, nil/value DTOs, nested ownership, and unchanged explicit custom raw passthrough. |
| Complete HTTP 200 JSON admission errors retained the full reservation | A private, typed proof distinguishes a complete receipt-free admission rejection from uncertain paid work. All three client entry points use an owned synchronous refund before retry is allowed. | Real HTTP and SQLite user/token/cost accounting on Anthropic and Azure, duplicate refund/audit/retry reset, canceled clients, failed credits, durable intent replay, and forged-error negative controls. |

### Admission rejection is not a blanket free-error rule

The classifier accepts only a top-level `error` envelope with no usage object
(absent or JSON null), complete read/JSON validation/close, and a documented
admission error: invalid request, authentication, billing, permission, not found,
request too large, or rate limit. HTTP 200 is allowed for a status-rewriting
intermediary; a conflicting status is not accepted as refund proof.

Internal errors, overload, timeout, unknown error types, unknown/empty usage
objects, explicit counters (including zero), partial or malformed receipts,
read/close errors, and streaming failures do **not** qualify. Observed receipts
remain billable; uncertain work retains the existing labelled reservation
policy. The proof is not constructed from a client field, matching error string,
or an arbitrary HTTP status.

This classification is a gateway accounting policy based on documented admission
semantics, not a provider-invoice measurement. It does not change SDK transport
error handling or claim all third-party services report failures identically.

### Refund ownership and recovery

A verified admission refund uses the existing `QuotaRefund` intent and atomic
user/token credit implementation. The request claims ownership before any write,
uses a bounded detached context, and reconciles the provisional log and request
cost before permitting replay. Duplicate error handling and retry reset cannot
issue a second credit. An unconfirmed refund blocks replay and zero-cost logging.
Persisted pending intents use existing recovery; enqueue failures retain an audit
ID for reconciliation. Secondary log/cost reconciliation after a recovery-worker
credit remains an operator reconciliation boundary, not a claim of a distributed
transaction across every accounting record.

## Reproduce-first and full-project checks

On the unmodified reviewed base, the existing repository CI reproduced the
incomplete tool-name fixture. New tests also reproduced six incremental-finish
cases, fourteen JSON admission cases, two OpenRouter field-isolation cases, and
twelve real database admission-refund cases. Internal/unknown-error controls
retained charges as intended. The OpenAI positive control was corrected to
recognize its existing native Responses DTO instead of wrongly requiring a
shared Chat DTO; that test assumption was not counted as a production defect.

The resumed local checks use the repository's actual Go 1.27.1 compiler and
verified module sources, not a stubbed DTO or downgraded toolchain. Tokenizer
assets and the full repository are required for broad suites; an offline missing
asset is not reported as a source-code regression or a passing test.

Run on the exact candidate tree:

```sh
go test -race -count=1 ./relay/adaptor/... ./relay/controller ./relay/pricing ./relay
go vet ./...
go build -o /tmp/one-api .
```

Final acceptance additionally requires the normal repository CI, including its
complete package and model race/coverage shards. The PR records the observed
final commit and CI run; passing an earlier tree, an isolated test, or an export
workflow does not satisfy that gate. No test is skipped to obtain a green result.
No paid inference, deployment, automatic merge, dependency update, database
migration, or production CI-workflow change is included.

## Primary references

- Anthropic error taxonomy and envelope: https://platform.claude.com/docs/en/api/errors
- Claude stream event order and usage: https://platform.claude.com/docs/en/build-with-claude/streaming
- Sonnet 5.5 request contract: https://platform.claude.com/docs/en/models/sonnet-5-5/migration-guide
- Multi-cloud catalog and previous acceptance: `20260928_anthropic_model_catalog.md`
