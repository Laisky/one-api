<!--
Sync Impact Report
Version change: scaffold -> 1.0.0
Modified principles:
- Scaffold principle 1 -> I. API Format Compatibility Is Product Behavior
- Scaffold principle 2 -> II. Minimal Sufficient Change
- Scaffold principle 3 -> III. Trust Boundaries Stay Explicit
- Scaffold principle 4 -> IV. Observable Request Paths
- Scaffold principle 5 -> V. Checks Prove the Change
Added sections:
- Project Constraints
- Development Workflow
Removed sections:
- None
Follow-up TODOs:
- None
-->
# One API Constitution

## Core Principles

### I. API Format Compatibility Is Product Behavior
one-api MUST let users call ChatCompletion API, Response API, or Claude Messages
API formats and transparently convert among these three formats. Any adaptor that
supports one of these request families MUST preserve the shared conversion
contract unless the model or upstream provider makes a combination impossible.
Unsupported combinations MUST be explicit, tested, and surfaced as compatibility
behavior rather than accidental failures.

Rationale: The gateway's core value is format interoperability. A feature that
works for only one request shape without preserving conversion paths fragments
the product.

### II. Minimal Sufficient Change
Contributors MUST understand the touched flow before changing it, then choose the
lowest rung that fully solves the problem: avoid building unneeded behavior,
reuse existing code, prefer the standard library and platform features, avoid new
dependencies, and keep the working diff as small as correctness allows. Bug fixes
MUST address the shared root cause after checking sibling callers. New
abstractions are allowed only when explicitly requested or when the existing
codebase already establishes the pattern.

Rationale: The best maintainable code is often the code never added. Small,
well-placed changes reduce review burden and future defects.

### III. Trust Boundaries Stay Explicit
Inputs from users, upstream providers, environment variables, databases, network
requests, and files MUST be validated before they influence queries, allocations,
authorization, billing, or outbound requests. Secrets MUST NOT appear in logs,
errors, fixtures, UI text, or documentation. Sensitive comparisons MUST use
constant-time behavior, passwords MUST follow OWASP-strength hashing guidance,
and server, database, and API time handling MUST use UTC.

Rationale: one-api is a multi-tenant gateway for external model providers.
Security and billing bugs cross tenant and provider boundaries quickly.

### IV. Observable Request Paths
Request-handling code MUST use the request-scoped logger from context, retrieved
once per function, and MUST emit structured Zap fields with `zap.Error(err)` for
errors. Each error MUST be processed exactly once: either returned with context
or logged, but never both. Debug logging SHOULD be targeted enough to diagnose
format conversion, provider routing, quota, and billing behavior without exposing
secrets.

Rationale: Format conversion and provider routing failures are hard to diagnose
after the fact unless each request carries consistent, structured context.

### V. Checks Prove the Change
Non-trivial logic MUST leave one runnable check that fails if the logic regresses.
Bug fixes and new features MUST update focused tests near the changed behavior,
using `github.com/stretchr/testify/require` in Go tests. Trivial one-line changes
may omit a new test only when existing checks already cover the behavior.

Rationale: Minimal code without a check is unfinished work for the next person.

## Project Constraints

The repository MUST use `yarn` for JavaScript package management to preserve
`yarn.lock`. Python work MUST use a project-local virtual environment when one
exists, and dependencies MUST NOT be installed into the system Python
environment. The Modern frontend is the primary template for new UI work, while
Default, Berry, and Air remain compatibility targets.

All UI text MUST support internationalization through the locale files under
`web/modern/src/i18n/locales/`. CSS MUST avoid `!important` and inline styles.
Go code targets Go 1.26, threads `context.Context` through call chains where
feasible, wraps returned errors with `github.com/Laisky/errors/v2`, and uses
`gorm.io/gorm` for ORM writes while preferring explicit SQL for complex reads.
Manually written code files MUST stay at or below 800 lines unless the file is
generated; cohesive documentation, runbooks, proposals, and evidence notes may
exceed that limit.

## Development Workflow

Before implementing, contributors MUST trace the real end-to-end flow and search
for existing helpers, utilities, standard-library coverage, native platform
features, or installed dependencies that already solve the problem. When a
deliberate simplification has a real ceiling, the code MUST include a `ponytail:`
comment that names the ceiling and the upgrade path.

For code changes, contributors MUST run the smallest relevant check while
iterating and, before handoff when feasible, run the project quality gates:
`go vet ./...`, `go test -race ./...`, and `make build-frontend-modern` or the
equivalent CI checks. Multiple agents may edit concurrently; contributors MUST
preserve unrelated changes and stop only when an irreconcilable conflict blocks
the constitution or feature work.

## Governance

This constitution supersedes informal project practices when they conflict.
Amendments MUST be made through the Spec Kit constitution workflow, include a
Sync Impact Report, and update the semantic version and amendment date in this
file. Governance-only changes MUST NOT modify application source, tests,
deployment files, or unrelated artifacts.

Versioning follows semantic versioning. MAJOR versions remove or redefine
governance in a backward-incompatible way. MINOR versions add or materially
expand principles, constraints, or workflow requirements. PATCH versions clarify
wording without changing required behavior. Each specification, plan, task list,
and code review MUST check compatibility with this constitution before work is
accepted.

**Version**: 1.0.0 | **Ratified**: 2026-08-25 | **Last Amended**: 2026-08-25
