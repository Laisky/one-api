# OneAPI browser errors

This integration captures browser errors only on `https://oneapi.laisky.com` using `@sentry/react` 11.7.0. Modern uses React 19; Air and Berry use React 18. `THEME=default` resolves to Modern in the current Go theme validation. All supported themes share one policy and bootstrap. Existing Go/OpenTelemetry attribute allowlists, backend alerts, authentication, provider behavior and release controls are preserved.

## Configuration and activation

The deployment owner supplies an approved OneAPI JavaScript project's public ingest DSN and environment through the existing protected root-admin `BrowserSentryConfig` option. Its JSON has exactly `dsn` and `environment`; the latter is `production`, `staging` or `development`. The HTTPS DSN requires a lowercase 32-character public key, approved Sentry ingest hostname and numeric project path. Passwords, ports, query strings, fragments and additional fields are rejected. Release is build-derived and cannot be overridden by this setting. Empty/whitespace clears the setting. There is no separate enabled flag.

The anonymous, no-store `/api/browser-config` endpoint exposes only this validated public setting or null. The browser requests it once with credentials omitted, no referrer, no redirects, same-origin delivery and a 2048-byte streamed response limit. Configuration and lazy SDK loading each have a separate one-second deadline. Invalid/missing settings, unsupported hosts/protocols/ports, unavailable fetch/streams, failed SDK loading/initialization and late imports settle disabled. Disabled reporting adds no root hooks and uses a transparent boundary; native application failures still reach the browser error dispatcher. There is no retry, background polling or buffered error replay.

Reporting readiness settles before application modules are dynamically loaded and mounted. Enabled global handlers cover import/bootstrap, uncaught callback and unhandled rejection errors. React 19 root hooks and the shared React 18/19 boundary retain component stacks. The same exception object is deduplicated across manual, boundary and global capture. A failed root retains the previous blank-root behavior. SDK failures cannot throw into application code, and diagnostic fallback logs contain fixed strings only.

The separately approved prerequisite is the correct OneAPI project/public configuration. The last authorized inventory found no OneAPI project. Do not reuse blog configuration. This work creates no project, key, token or access grant and changes no actual runtime setting. An operator must review code deployment and current runtime/image/rollback qualification before production rollout. A real smoke event additionally requires explicit approval after the correct project/configuration exists.

Saving or clearing the runtime option requires page reload after the existing replicated-option propagation interval. Existing tabs retain their frozen configuration until reloaded. Configuration changes require no frontend rebuild or container restart. No DSN, application credential or upload token is embedded in these builds.

## Diagnostic privacy

The application uses the standard SDK transport and default browser error integrations, excluding `BrowserSession`. Useful messages, ordinary/custom exception types, complete normal stacks including lazy/vendor frames and function names, component stacks, technical contexts, safe breadcrumbs and available Debug IDs are retained. HTTP/navigation breadcrumbs retain safe URL paths, methods and status; console breadcrumbs retain static diagnostics while omitting arbitrary additional strings and objects. Sensitive input/editor DOM breadcrumbs are dropped. No tracing sampler, replay integration, log or metric shipping is enabled, and client reports are disabled. Ingest fetch options omit credentials and referrer.

The bounded sanitizer uses data descriptors rather than invoking getters, rejects cycles/depth excess, and gives core error/stack metadata a separate budget from arbitrary extras. It removes users/personal metadata, auth headers/cookies, credentials and key/token aliases, request/response bodies, callback arguments, prompts/completions/history, MCP tool arguments/results, local-storage secrets, frame variables and source context. URL handling removes userinfo, query strings and fragments, including URLs inside messages/component stacks. Known private response/JSON errors are annotated at their origin without changing their identity, message, stack, class or UI behavior; exact, prefixed, linked-cause and SDK-truncated copies are replaced with controlled diagnostic summaries. Attachments are cleared before delivery. Sanitization failure drops the event.

Handled HTTP/auth/validation/quota/provider replies and cancellations are not automatically captured as incidents. Narrow request catches report unexpected browser transport/decoding failures while preserving normal UI callbacks. If an expected private error escapes to a global handler, it can still produce a sanitized actionable diagnostic. There is no duplicate server-side Sentry, OpenTelemetry or backend-log capture.

Collector IP/geo retention, allowed origins, retention/access controls and source-fetch settings are optional owner policies, not new basic activation requirements. Browser code sets no user identity/IP. Any collector/security policy change remains separately authorized. Sentry Verify SSL concerns outbound hooks/source fetching and does not disable the browser's TLS validation.

## Build identity and private maps

Vite embeds `one-api-{theme}@sha256:<64 lowercase hex>` from reviewed frontend/build inputs, dependency locks, compiler runtime/platform/architecture and explicit public build values. This is an input fingerprint, not a Git SHA. The checked-in `web/legacy/browser-release-inputs.json` includes each supported theme and shared helpers. Its guard rejects unlisted resolved first-party modules, private/outside paths and symlinks before loading; it verifies the same byte snapshot again before emitting artifacts. Update and review this allowlist when adding/removing relevant source. Runtime config does not change the fingerprint. Record source SHA, compiler inputs and each emitted release together.

Normal production builds produce no source maps. This revision preserves SDK Debug ID metadata if present; it does not claim the normal builds generate IDs or perform an upload. A future separately reviewed private-map pipeline must produce maps and the deployed JavaScript from the same build. A separate rebuilt map artifact is insufficient. Use the official Vite plugin/CLI's supported Debug ID pairing or exact release/URL correlation, validate actual pairs and byte identity, quarantine maps outside served/image paths with restrictive permissions, and fail packaging if any map or inline source map remains. Hidden maps alone are still publicly accessible if served. Keep optional source fetching under the owner's reviewed policy.

An approved secret-store upload token belongs only to the private upload process; never place it in browser variables, public configuration, Docker arguments, copied image files, Git or logs. Maps/upload credentials are not prerequisites for basic browser capture. No upload, credential configuration or real telemetry test occurs here.

## Qualification and rollback

Use supported Node24 and each theme's locked Yarn dependencies. Run the shared configuration/readiness/privacy contracts, real SDK memory-transport tests, mounted React 18/19 boundary/root/global tests, new private-origin and request-behavior tests, relevant full frontend suites, compiler/lint checks and all three normal builds serially. Test events stay in memory. Record exact source heads and distinguish source/build acceptance from actual container/browser/production qualification. Do not repeat unrelated completed backend suites unless source changes justify them.

This isolated revision starts from combined qualified source `4082041192b3a2d52fff8c9dfd75360a06b2ab16`, preserving newer remote main and the #568 Gemini accounting tests (blob `d5e1084763e204724bd45075330260b8e5b1eb0c`). Historical [qualification run](https://github.com/Laisky/one-api/actions/runs/38448817944) passed combined and test-only controller gates. Its evidence qualifies that backend source, not the new frontend bytes. Existing PR #561 and #568 histories/readiness remain untouched. Other billing/adaptor/dependency work stays isolated.

Only the new feature branch is eligible for a Draft PR. Main/master/test/ci pushes can publish or deploy and are prohibited here; do not merge, deploy, change release controls or activate collector settings. Chat's merged but runtime-unqualified changes and independently held backend changes must not enter a release implicitly. Production qualification requires an approved current image/digest, routing/configuration, exact artifact identity, behavior and rollback process. Historical workflow success does not establish current running state.

Capture rollback clears `BrowserSentryConfig`, permits propagation and reloads pages. Existing tabs require reload. Code rollback uses a separately approved known-good image/artifact and configuration-preserving process. Missing production facts are reported as remaining rollout gates, not guessed from stale dev checkouts. This task does not execute rollout or rollback.

## Official references

- [React browser setup and errors](https://docs.sentry.io/platforms/javascript/guides/react/)
- [React boundaries/root hooks](https://docs.sentry.io/platforms/javascript/guides/react/features/error-boundary/)
- [SDK options and data collection](https://docs.sentry.io/platforms/javascript/guides/react/configuration/options/)
- [Pinned official React capture implementation](https://github.com/getsentry/sentry-javascript/blob/11.7.0/packages/react/src/error.ts)
- [Pinned browser default integrations](https://github.com/getsentry/sentry-javascript/blob/11.7.0/packages/browser/src/sdk.ts)
- [Private Vite source-map upload](https://docs.sentry.io/platforms/javascript/guides/react/sourcemaps/uploading/vite/)

Compatibility is verified against the installed official 11.7.0 source/types and offline SDK tests. SDK11 uses supported dataCollection and integration selection; legacy autoSessionTracking/sendDefaultPii flags are not used. No custom envelope projection suppresses normal SDK diagnostics.

## Maintaining reviewed browser release inputs

Every file under each supported theme's `src` and `public` directories, including tests and JSON fixtures, must be explicitly reviewed in `web/legacy/browser-release-inputs.json`. Shared helpers and build configuration remain explicitly listed too. New file contents are never automatically approved by builds or CI.

Run the dependency-free check from the repository root before opening a PR:

```sh
node .github/scripts/browser-release-inputs.mjs check
node --test .github/scripts/browser-release-inputs.test.mjs
```

For an intentional new input, inspect the file and explicitly name it:

```sh
node .github/scripts/browser-release-inputs.mjs add modern web/modern/src/path/to/reviewed-file.ts
```

For a deleted or renamed input, explicitly remove its old path and add the new path in the same change:

```sh
node .github/scripts/browser-release-inputs.mjs remove modern web/modern/src/path/to/deleted-file.ts
```

The command changes only the selected theme's requested entries and preserves other theme lists. Name shared `web/legacy` inputs explicitly for every affected theme. It rejects private paths, traversal, cross-theme paths and symlinks when adding inputs; `check` rejects unlisted or deleted files and runs the same production guard for all three themes. Removal changes the list only, so removing a still-present source/public file continues to fail the check.

Review the JSON diff, rerun `check`, and run the affected theme's tests and production build. The mandatory CI `changes` job runs this check without installing frontend dependencies. Full browser/build qualification remains available through the non-deploy CI workflow. Never regenerate approval during the Docker build or rerun delivery just to validate this manifest: delivery publishes production image tags and can deploy.
