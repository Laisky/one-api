# Archived Proposals

Proposals in this directory are **closed**: the work they describe has shipped (or, where
noted, was deliberately dropped). They are kept as historical design records — the rationale,
the invariants, and the acceptance criteria behind code that is now in `main` — and are no
longer plans of record. Active proposals live one level up in [`docs/proposals/`](../).

File and line anchors inside these documents were accurate when the document was written and
may have drifted since; treat the code as the source of truth.

| Proposal | Area | Outcome |
| --- | --- | --- |
| [Channel Hidden Models](20260421_channel-hidden-models.md) | channel config, abilities, model discovery | Implemented (`Channel.HiddenModels`) |
| [Async/Sync Races in the Relay & Billing Path](20260608_relay-billing-async-sync-race-fixes.md) | relay, billing, `common/relayctx` | Implemented (P1–P5; guarded by `.ast-grep/rules`) |
| [Upstream API Provider Expansion](20260627_upstream-provider-expansion.md) | channel types, adaptors, pricing | **Partial** — DeepInfra shipped; HuggingFace, Perplexity, SambaNova, VoyageAI dropped |
| [Time-of-Day Pricing Windows](20260629_time-of-day-pricing-windows.md) | pricing, billing, model display | Implemented (`relay/pricing` time windows) |
| [External UUIDv7 Resource Identifiers](20260703_external-uuid-identifiers.md) | data model, management API, auth, frontend | Implemented (strict-out UUID contract) |
| [Boundary Response DTOs](20260714_boundary-response-dtos.md) | management API serialization, `dto/` | Implemented (`noentityresponse` analyzer enforces it) |
| [Automatic Compact UUID Storage](20260715_compact-uuid-storage.md) | storage, migrations, indexes | Implemented (`compact_uuid_storage_v1`) |
| [Incremental External UUID Backfill](20260715_incremental-uuid-backfill.md) | model init, data migration | Implemented (v3 remediation) |
| [Stateful Responses Conversion Across API Formats](20260719_stateful-responses-format-conversion.md) | relay routing, Responses API, `relay/state` | Implemented (ST-001–ST-023; §14 tracks per-task closure) |

Live operational documentation derived from this work lives in [`docs/arch/`](../../arch/),
[`docs/manuals/`](../../manuals/) and [`docs/ops/`](../../ops/).
