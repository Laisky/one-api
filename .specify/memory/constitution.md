<!--
Sync Impact Report:
- Version: N/A → 1.0.0
- Modified Principles:
	- [PRINCIPLE_1_NAME] → I. Channel-Parity Billing Discipline
	- [PRINCIPLE_2_NAME] → II. Universal API Conversion Fidelity
	- [PRINCIPLE_3_NAME] → III. Test-Orchestrated Delivery
	- [PRINCIPLE_4_NAME] → IV. Operational Transparency & Responsive UX
	- [PRINCIPLE_5_NAME] → V. Documentation & Access Governance
- Added Sections:
	- Implementation Constraints
	- Development Workflow & Quality Gates
- Removed Sections:
	- None
- Templates Updated:
	- .specify/templates/plan-template.md ✅ updated
	- .specify/templates/spec-template.md ✅ updated
	- .specify/templates/tasks-template.md ✅ updated
- Follow-up TODOs:
	- None
-->
# One API Constitution

## Core Principles

### I. Channel-Parity Billing Discipline

One API MUST deliver identical billing outcomes across every channel and adaptor.

- Pricing data MUST originate from the shared `ModelRatios` map and remain expressed per 1M tokens.
- Cache-write tokens MUST subtract from normal input tokens and cached completions MUST NOT be billed.
- Every change to billing logic MUST ship with synchronized updates to documentation, UI displays, and automated tests.

*Rationale*: Accurate, transparent billing sustains customer trust and enables predictable operations.

### II. Universal API Conversion Fidelity

All protocol conversions (ChatCompletion, Response API, Claude Messages, Gemini) MUST preserve intent, tooling metadata, and quotas.

- Requests routed through the Response fallback MUST conform to conversion handlers and deny unsupported streaming features.
- Context keys defined in `common/ctxkey` MUST gate conversion state and be respected end-to-end.
- Regression tests guarding conversion (`relay/adaptor/openai`, `relay/controller`) MUST remain green before merge.

*Rationale*: Consistent conversions prevent customer regressions and simplify multi-provider integration.

### III. Test-Orchestrated Delivery

Feature work MUST begin with executable tests covering the new behavior and guards for critical edge cases.

- Go services MUST pass `go test -race ./...`; frontend work MUST pass Vitest suites with `jsdom`.
- Regression tests MUST be added or extended whenever billing, security, or conversion logic changes.
- Smoke flows (tenant management, channel routing, billing reports) MUST be validated before release.

*Rationale*: Tests are the contract that protects adapters, billing, and UX from silent regressions.

### IV. Operational Transparency & Responsive UX

Runtime instrumentation MUST surface actionable telemetry and the UI MUST remain accessible on desktop and mobile.

- Server code MUST use `gmw.GetLogger` for structured logging and attach request context to every log line.
- Frontend API requests MUST include the `/api/` prefix and respect responsive layouts, touch targets, and accessibility labels.
- Observability dashboards and pagination controls MUST stay visible and reliable across breakpoints.

*Rationale*: High-signal observability and inclusive UI keep operations debuggable and user friendly.

### V. Documentation & Access Governance

Documentation, configuration, and access policies MUST remain synchronized with product behavior.

- README, manuals, and deployment guides MUST reflect the active feature set and pricing formulas.
- Access controls MUST honor model entitlements (`/api/models/display`) and audit trails.
- Major updates MUST include guidance for operators migrating from upstream forks.

*Rationale*: Clear documentation and entitlement hygiene reduce onboarding risk and ensure compliant adoption.

## Implementation Constraints

- Errors MUST be wrapped with `github.com/Laisky/errors/v2`; context MUST travel through call chains.
- ORM writes MAY use GORM, but high-volume reads MUST use SQL optimized for load.
- Time handling MUST remain in UTC; rate limits and pagination MUST honor server-side enforcement.
- Secrets and keys MUST leverage configuration files or environment variables—never hard-coded in source.

## Development Workflow & Quality Gates

- Plans MUST document Constitution Check outcomes before work starts; violations require explicit justification.
- Every PR MUST include updated tests and, when applicable, documentation or migration notes.
- CI pipelines MUST run Go race tests, Vitest, and linting before merge; failures MUST block release.
- Manual smoke validation MUST cover API routing, billing summaries, and frontend pagination on mobile and desktop.

## Governance

- Constitution amendments REQUIRE explicit rationale, updated templates, and approval from maintainers owning billing and platform domains.
- Versioning follows semantic rules: MAJOR for breaking governance changes, MINOR for new principles/sections, PATCH for clarifications.
- Ratification history MUST remain immutable; `LAST_AMENDED_DATE` updates only when content changes.
- Compliance reviews occur quarterly and before each release branch cut, with findings logged in project docs.

**Version**: 1.0.0 | **Ratified**: 2025-10-09 | **Last Amended**: 2025-10-09
