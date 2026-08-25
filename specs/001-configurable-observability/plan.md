# Implementation Plan: Configurable Observability

**Branch**: `001-configurable-observability` | **Date**: 2026-08-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-configurable-observability/spec.md`

## Summary

Remove the existing database-backed request tracing implementation from backend request handling and redesign relay observability around OpenTelemetry middleware plus structured request logs. The implementation must not keep database tracing controls, middleware gates, helper checks, request-path write/update calls, or compatibility branches for the deleted DB trace path. OpenTelemetry middleware is registered for relay request paths and uses default or no-op behavior when no exporter or collector is configured. Relay processing records synchronized OpenTelemetry events and structured log entries for request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and errors. GenAI inference spans must follow the OpenTelemetry GenAI span convention exactly for fields already known from normal relay flow, including `CLIENT` span kind, `{gen_ai.operation.name} {gen_ai.request.model}` span naming, standard attribute names, and no invented values. Tracing must not parse, re-serialize, or inspect the client request body only to populate optional GenAI fields. JSON log output remains opt-in and must emit `severity_text`, valid OpenTelemetry `trace_id` and `span_id` when available, the same cheaply known GenAI fields, OneAPI routing fields, and sanitized bounded prompt previews.

## Technical Context

**Language/Version**: Go 1.26.3 for backend service; TypeScript/React frontend remains out of scope for this feature.

**Primary Dependencies**: Existing `github.com/Laisky/go-utils/v6/log`, `github.com/Laisky/zap`, `github.com/Laisky/gin-middlewares/v7`, `github.com/gin-gonic/gin`, and OpenTelemetry packages already present in the repository.

**Storage**: No new storage. Database-backed trace request-path persistence is removed. Historical trace rows, if any, are not migrated, deleted, or hidden by this feature.

**Testing**: Go tests with focused packages around config, logger, tracing helpers, middleware, and relay request handling.

**Target Platform**: Production Linux/containerized web service deployments.

**Project Type**: Web service/API gateway with backend-managed operational observability.

**Performance Goals**: Removing DB-backed tracing eliminates new database trace writes for 100% of normal API requests; tracing does not add client request parsing or serialization solely for observability; JSON logging adds no new network dependency; prompt preview logging bounds each large prompt to at most 203 preview characters after sanitization.

**Constraints**: Reuse existing libraries where possible; remove old DB trace logic rather than preserving a switch; always register OTel relay middleware; align trace data exactly with OpenTelemetry GenAI inference client spans for already-known values; do not parse client requests solely for tracing; do not emit input-detail or content attributes without explicit sanitized enablement; do not adapt frontend or public trace interfaces; do not expose secrets in logs; emitted `trace_id` and `span_id` must be OpenTelemetry-compliant when present.

**Scale/Scope**: Runtime request-path observability only. No schema redesign, trace UI changes, trace history migration, new external observability backend, or database-backed tracing compatibility layer.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **API Format Compatibility Is Product Behavior**: PASS. The plan preserves ChatCompletion, Response API, and Claude Messages behavior and does not touch conversion adapters.
- **Minimal Sufficient Change**: PASS. The approach deletes obsolete DB tracing behavior and reuses existing logger, middleware, and OpenTelemetry dependencies.
- **Trust Boundaries Stay Explicit**: PASS. New log fields come from validated OpenTelemetry span context and sanitized bounded prompt previews.
- **Observable Request Paths**: PASS. Request-scoped logging remains context-based and adds standard trace correlation plus relay event fields.
- **Checks Prove the Change**: PASS. Focused tests cover primary request flow, JSON logs, OTel ID extraction, prompt preview truncation, no-exporter behavior, and relay timing emission.

## Project Structure

### Documentation (this feature)

```text
specs/001-configurable-observability/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── observability-config.md
└── checklists/
    └── requirements.md
```

### Source Code (repository root)

```text
common/
├── config/
├── logger/
└── tracing/

middleware/
└── tracing.go

relay/
├── adaptor/
└── controller/

main.go
docs/
└── arch/
```

**Structure Decision**: Implement the change inside existing backend observability modules and relay request handling. Keep frontend templates and public trace routes unchanged because the feature intentionally avoids new UI/API compatibility behavior for removed DB tracing.

## Complexity Tracking

No constitution violations are expected.

## Phase 0: Research

See [research.md](./research.md). All planning unknowns are resolved.

## Phase 1: Design & Contracts

See [data-model.md](./data-model.md), [contracts/observability-config.md](./contracts/observability-config.md), and [quickstart.md](./quickstart.md).

## Post-Design Constitution Check

- **API Format Compatibility Is Product Behavior**: PASS. Design changes are limited to backend observability and request-scoped logging.
- **Minimal Sufficient Change**: PASS. The old DB trace path is removed instead of generalized, and existing libraries stay central.
- **Trust Boundaries Stay Explicit**: PASS. Invalid or missing OTel context does not create fabricated IDs, and prompt previews are sanitized before emission.
- **Observable Request Paths**: PASS. OTel relay events and synchronized request logs provide structured timing evidence.
- **Checks Prove the Change**: PASS. Quickstart and planned tests cover the new behavior before implementation handoff.
