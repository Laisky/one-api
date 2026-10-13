# OneAPI browser error monitoring

This change adds browser error capture only to https://oneapi.laisky.com. It does not change Go/OpenTelemetry telemetry, its attribute allowlists, release workflows, or production rollout controls. SDK 11.7.0 supports the shipped React 18 and 19 themes. There are no configured project identifiers, real DSNs, upload credentials, test events, or server-side Sentry instrumentation.

## Supported themes and disabled behavior

The checkout contains Modern, Berry and Air packages, which Docker builds sequentially. Although AGENTS.md still mentions Default, `common/config/config.go` lists only these three valid themes; `main.go:isThemeValid` redirects `THEME=default` to Modern for compatibility. Default therefore uses the same Modern integration, not a separate obsolete entry point.

All three entry points initialize the same shared policy before application imports. Modern additionally uses React 19 root error hooks; the shared reporting boundary covers render failures in React 18 and 19. All routers use declarative BrowserRouter/Routes; there is no independent data-router errorElement that bypasses the root boundary. GlobalHandlers covers uncaught errors and unhandled rejected promises. Reports of the same exception object are deduplicated. Enabled boundary failure renders a blank root, matching the previous root-crash outcome; no new user-facing fallback text is introduced.

Without all validated settings, or on a different hostname/protocol/port, the reporter stays disabled, adds no root callbacks, and uses a transparent wrapper. SDK initialization/capture/transport failures cannot throw into application code; diagnostics use fixed strings only. Partially initialized handlers cannot send because the final transport is gated on successful client initialization.

## Public deployment configuration: separate approval gate

The Sentry organization currently has no OneAPI project. A project owner must separately approve/create the correct OneAPI JavaScript project and its public DSN. Do not reuse the blog project's settings. No project, key, token or access grant is created by this PR.

Only explicitly provided public build variables enable capture:

| Variable                | Required value                                                                                  |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| VITE_SENTRY_ENABLED     | Literal `true`; default disabled                                                                |
| VITE_SENTRY_DSN         | Approved OneAPI public DSN, HTTPS numeric Sentry ingest project URL; no password/query/fragment |
| VITE_SENTRY_ENVIRONMENT | `production`, `staging`, or `development`                                                       |
| VITE_SENTRY_RELEASE     | `one-api@` followed by the exact 40-character lowercase Git commit SHA                          |

Vite substitutes these values into each theme at build time. Docker accepts the same four public build arguments in the frontend stage; absent arguments keep every theme disabled. Its current delivery workflow supplies none, so this PR alone cannot enable production capture. Runtime container environment changes cannot alter already-built JavaScript: enabling/disabling needs a separately approved rebuild.

No SENTRY_AUTH_TOKEN belongs in VITE_* variables, Docker build arguments, .env files copied into images, JavaScript, Git, logs or public artifacts. No release workflow or production configuration is changed here.

The browser gate requires exactly HTTPS oneapi.laisky.com on the default port. It excludes chat, mcp, cv, sso, chat2, local development and other hosts even when configuration is accidentally shared.

## Privacy and diagnostic limits

The SDK uses no default integrations, only GlobalHandlers. It has no tracing sampler or tracing/replay/network/console integrations, zero breadcrumbs, explicit disabled dataCollection categories, and log/metric/transaction drop guards. The final envelope transport permits only a projected error event, with minimal headers; it drops attachments, sessions, logs, metrics, traces, replays and client reports, including internal SDK errors that bypass beforeSend. Fetch uses credentials:omit and referrerPolicy:no-referrer.

Projection retains only bounded known exception categories, a fixed redacted error value, validated deployment identifiers, fixed site/theme tags, event identity/time, and up to 3 exceptions with up to 30 static stack locations each. It discards all arbitrary messages, user metadata, request URLs/headers/cookies/bodies, extra/context/tags/fingerprints, breadcrumbs, source lines, frame variables and function names. This excludes API keys, auth tokens, prompts, completions, chat history, local-storage secrets and MCP tool arguments/results.

Static frame filenames must be HTTPS first-party assets, have a known generated filename shape, and exactly match an immutable snapshot of at most 40 module/script/preload asset URLs loaded at initialization. Queries/fragments are removed; credentials and unsafe source/third-party paths are rejected. Later lazy-loaded chunks are deliberately omitted from stack diagnostics unless already preloaded. Exception text and component stacks are never sent. These constraints reduce issue detail/grouping; changing the allowlist requires privacy tests and review.

Network ingestion necessarily exposes the connection IP to the receiving service. Before any real event, the project owner must verify Sentry-side IP/PII handling, retention, access control and server-side scrubbing. Browser code does not set user identity/IP, but cannot guarantee the service's retention policy.

## Private source maps and release correlation: separate approval gate

Production builds currently generate no source maps by default. Do not turn on legacy GENERATE_SOURCEMAP for a publicly shipped build without a reviewed private upload-and-exclusion pipeline. Hidden source maps suppress the bundle link; they do not prevent public map access.

Before a source-map rollout, separately review a private build step using the same release SHA and identical artifact bytes for all three theme outputs. Generate maps only in that private staging area. Use a least-privilege upload token supplied by the approved secret store to the private upload process; never put it in build arguments or browser configuration. Use classic release plus exact retained HTTPS asset filename correlation for the approved OneAPI project. The privacy projection deliberately drops debug_meta and frame debug_id, so DebugID pairing is unsupported without a future reviewed policy extension. Upload matching private artifacts using this classic release/URL correlation, then delete/exclude every *.map before Docker copies web/build or any static artifact is published. Fail packaging if any map remains, verify no embedded sourceMappingURL/data-URL map is present, and deny *.map at the public serving layer as defense in depth. Keep maps/source artifacts private under an approved retention policy.

Do not run an upload, credential command, production build publication or synthetic real telemetry event as part of this task. A real event may be approved only after the intended project/configuration and privacy prerequisites exist. First use mocked transport tests; an approved staged smoke check must inspect the received redacted event and release correlation without user data.

## Validation and rollout

Run Node >=22.12 (dev validation uses 22.22.3; SDK11 rejects the server's default Node 22.7) or CI Node24 with Yarn. All theme test commands include shared policy tests, mounted React boundary/router tests, and real SDK11 global/rejection/internal-event tests using mocked final transport. Modern tests also mount React19 root hooks. No test can send a real Sentry request.

After the final source commit, record the exact SHA and run the three theme test suites/builds sequentially, go vet ./..., and go test -race ./... with bounded Go concurrency. Existing common/telemetry privacy tests remain unchanged and run in the Go suite. Full database qualification has its own isolated database prerequisites; skipped backends must be reported honestly. Record acceptance results and any baseline/tooling limits in the Draft PR, not by changing deployment controls.

Only the feature branch is pushed for a separate Draft PR. main/master/test/ci pushes can publish images and main can deploy; none is pushed or merged here. Before an eventual rollout, project/configuration/privacy/source-map gates, required exact-head CI, and production authorization must all pass.

Rollback by leaving VITE_SENTRY_ENABLED unset/false and rebuilding the same supported themes through the approved delivery process, or reverting this feature in a reviewed release. No runtime switch can alter an existing bundle. Preserve the normal application behavior and existing telemetry allowlists.

## Official references checked 2026-10-13

- [React setup](https://docs.sentry.io/platforms/javascript/guides/react/)
- [React error hooks and boundaries](https://docs.sentry.io/platforms/javascript/guides/react/features/error-boundary/)
- [SDK configuration and dataCollection](https://docs.sentry.io/platforms/javascript/guides/react/configuration/options/)
- [Vite private source-map upload](https://docs.sentry.io/platforms/javascript/guides/react/sourcemaps/uploading/vite/)

The installed 11.7.0 types/source and real SDK mock-transport tests confirm behavior. SDK11 removed legacy sendDefaultPii/autoSessionTracking/enableLogs flags; this implementation uses supported dataCollection, integration selection and final sending guards instead.
