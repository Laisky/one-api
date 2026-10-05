# Issue 479: dashboard CSRF acceptance

The baseline is `952ae6d2`, including the merged account-session revalidation
fix from PR 522. The retained reproduction commit is `a74f6e5b`.

## Behavioral reproduction

`router/security_csrf_test.go` uses the production API router, a signed
SameSite=Lax session cookie, an isolated SQLite account database, and a local
HTTP billing fixture. Before remediation:

- Cross-site, sibling-site, absent-origin, null-origin, and wrong-scheme PUT
  requests changed the stored account display name.
- A cross-site GET navigation replaced the stored management access token.
- A cross-site GET balance refresh dispatched a request to the local upstream.
- Same-origin and bearer-only profile updates passed. A same-origin balance
  request reached the local fixture, excluding an invalid channel fixture as
  an explanation for absent dispatch.

These are in-process HTTP behavioral reproductions with browser headers and
signed cookies; they are not a claim of a live deployment exploit or a browser
automation run. No production endpoint or paid provider was used.

## Remediation

The ten account/channel actions listed in the API reference now require POST.
Dashboard authentication requires trustworthy provenance before authorizing
unsafe cookie requests. It preserves the existing session-first identity
contract, current-account validation, signed role ceiling, and bearer-only
clients. Logout applies the same provenance check separately.

Trusted origins include scheme and port. An explicitly configured separate
frontend remains supported. Missing provenance fails closed; same-origin
Fetch Metadata and trusted Referer provide browser compatibility fallbacks.
An untrusted Origin cannot fall back to either header. Arbitrary forwarded
headers do not establish trust, and the default localhost configuration is not
trusted as an additional production origin.

Modern, Air, and Berry callers use POST, preserving parameters and response
shapes. This checkout has no Default application source. OAuth provider
callbacks retain their existing state validation and redirect methods.

## Retained coverage and limits

- Production-route tests verify persisted account state, account bindings,
  logout cookies, management token rotation, and GET/HEAD rejection.
- Single and bulk channel probes use a local upstream. Rejected requests make
  zero calls; trusted POST controls dispatch. Background workers are joined
  before fixture cleanup, and notifications cannot access external services.
- Balance refresh verifies both local upstream dispatch and stored balance.
  The bulk balance handler currently performs no refresh at all; its POST
  migration preserves that existing behavior rather than claiming a repaired
  bulk implementation.
- Middleware cases cover origin scheme/port, configured frontend, absent/null
  provenance, sibling sites, duplicate origins, and untrusted forwarded headers.
- Cookie plus Authorization requests cannot borrow the bearer exemption.
- Modern component tests cover method migration for channel actions, account
  settings, and logout. No user-visible strings were added.

The API reference documents the breaking GET-to-POST migration. Headless
management clients should use their access token; scripts intentionally using
cookies must supply a trusted Origin header for mutations.

## Validation

- `go vet ./...` passed.
- `go test -race ./router ./middleware -count=1` passed.
- Modern targeted component suite: six files, 28 tests passed with Vitest 5.0.3.
- `yarn install --frozen-lockfile` followed by `yarn build` passed under Node
  22.22.3; the dependency manifest and lockfile are unchanged.
- Modern locale alignment check passed for all five locales.

The package race run initially exposed unregistered asynchronous channel-log
and latency writes outliving fixture cleanup. The retained fixture now observes
their completed database transactions explicitly before restoring globals;
production worker behavior was not changed as part of this CSRF fix.
