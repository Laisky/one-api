# Dependency refresh and quality acceptance — 2026-09-30

PR #437 consolidates #394 and #383. This revision supersedes the earlier decision to leave legacy CRA advisories unresolved: Air and Berry now use Vite, and all three shipped themes have mandatory full-tree dependency audits.

## Dependency scope

The Go refresh retains Go 1.27.1 and existing import-major paths while updating gRPC to 1.84.0, WebAuthn to 0.18.2, AWS SDK/Bedrock, Google APIs, database drivers and `golang.org/x/*`. Modern retains React 19.3.0, Vite 8.3.1, Vitest 5.0.3 and Axios 1.20.0. Its jest-dom adapter extends Vitest's current matcher interface without disabling strict TypeScript library checks.

Air and Berry retain React 18 and their component APIs, but no longer install or execute `react-scripts`, webpack-dev-server or the CRA-specific JSONPath chain. They use Vite 8.3.1, the corresponding React plugin, standalone ESLint and Node's test runner. The original legacy trees had no Jest test files; the new commands run committed regressions instead of a pass-with-no-tests command.

The shared Vite factory preserves JSX in `.js`, Berry's absolute imports, SVG React components, public assets, API proxying, explicit production build stamps and independent `web/build/air` and `web/build/berry` outputs. The Go static router already serves arbitrary paths from the selected embedded directory, including Vite assets. Development defaults to loopback; set `HOST` explicitly for remote access. Only intentional public server/version constants enter client code, not the server environment.

## Fixed problems and regression coverage

| Problem | Resolution and acceptance |
| --- | --- |
| Axios interceptors swallowed failures | Preserve the rejected error after notification/auth handling; real HTTP regressions cover success, errors, cancellation, JSON bodies and headers. |
| Requests remained busy after rejection | Add `finally` cleanup to previously unprotected loading/searching/submitting paths. An AST regression scans all matching asynchronous handlers rather than selected filenames. |
| Network errors crashed notification helpers | Handle missing responses safely, notify once per error object, suppress cancellation notifications and never log raw Axios credential-bearing objects. |
| Password reset remained disabled on failure | Validate before starting the cooldown, release busy/cooldown state, preserve the email and use Axios query encoding. |
| Failed token deletion removed visible rows | Update rows only after confirmed server success. Preserve records on business/transport errors, support UUID/numeric references and use immutable status updates. |
| Invalid React hook placement | Keep Air pagination hook order stable; acquire Berry navigation inside `useAuth` and redirect from an effect. |
| Legacy build incompatibilities | Convert Day.js/SVG imports, modernize Berry Sass, select Rough.js's published ESM entry for VRender and remove only browser-invalid Semantic UI rules. Strict processing stays enabled. |
| Stale service-worker registration | Retire only the matching old CRA worker without waiting indefinitely for readiness or unregistering other applications. |
| Vitest 5 matcher declarations | Register the original jest-dom matchers through the compatible generic interface; test synchronous/asynchronous return types. |
| Missing quality gates | All three frontend jobs require complete zero-advisory audits. Both legacy jobs also require builds and production/development browser acceptance. Contract tests protect those gates and shared-config path filters. |

## Security acceptance

The previous tree at `bf0222e` had 8 high, 12 moderate and 3 low advisory occurrences in each legacy theme. Removing the vulnerable build chain eliminates these findings rather than suppressing them or forcing incompatible transitive majors.

| Theme | Earlier graph size | Migrated graph size | Critical | High | Moderate | Low |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Air | 1,610 | 605 | 0 | 0 | 0 | 0 |
| Berry | 1,415 | 395 | 0 | 0 | 0 | 0 |
| Modern | 671 | 671 | 0 | 0 | 0 | 0 |

Graph sizes are registry audit dependency totals, not bundle sizes or distinct vulnerability counts. A clean audit is a point-in-time registry result, not a guarantee against undiscovered vulnerabilities.

Air retains `**/geojson-flatten/minimist: 1.2.8` because the chart consumer pins vulnerable 1.2.0. The actual-consumer prototype-pollution regression remains required. The JSONPath/underscore override is removed because its consumer is no longer installed. Tests prevent CRA, webpack-dev-server and JSONPath from silently returning.

The gate audits development and runtime dependencies. It rejects nonzero severities, registry errors, incomplete/truncated reports, contradictory details, duplicate summaries and unsuccessful processes. Registry outages therefore fail instead of appearing clean. Raw reports are retained as CI artifacts.

## Behavioral evidence

Both migration validation jobs passed in [the accepted validation run](https://github.com/Laisky/one-api/actions/runs/36772748385). The original `bf0222e` notification and token-loading code was replayed against the same dependency tree and failed the intended assertions: missing response access and busy state retained after rejection. Fixed code passes the same cases.

The browser harness serves actual built assets with a local failure-injectable API, then repeats through the real Vite development server/proxy. Each theme must recover from password-reset failure, preserve input and retry successfully. Token requests remain failed until an explicit retry gate opens; Air's development StrictMode replay cannot masquerade as a successful user retry. The list must release busy state, remain empty on failure, issue a new request after the visible retry action and show the returned token. Unexpected browser page errors fail the job; screenshots/error records are retained.

The complete PR CI additionally runs the existing Go race/database shards, inventory/coverage checks, govulncheck, static guardrails, Modern tests/build/translations and Modern browser regressions. The final commit's checks, not earlier builds, determine merge readiness. Temporary migration scripts/workflows are absent from the final diff.

## Reproduce

Use Node 24 and Yarn Classic, matching CI. From the repository root:

```sh
for theme in modern air berry; do
  (cd "web/$theme" && yarn install --frozen-lockfile --non-interactive)
  node .github/scripts/frontend-audit.mjs "$theme"
done

(cd web/modern && yarn test --run && yarn build && yarn check:i18n)
(cd web/air && yarn test && yarn build:prod)
(cd web/berry && yarn test && yarn build:prod)

python3 -m pip install playwright==1.57.0 PyYAML==6.0.3
python3 -m playwright install --with-deps chromium
python3 .github/scripts/legacy-browser.py air
python3 .github/scripts/legacy-browser.py berry
python3 .github/scripts/test_workflows.py
python3 .github/scripts/test_frontend_contracts.py
```

Historical negative control, after installing either legacy theme:

```sh
LEGACY_THEME=air \
LEGACY_QUALITY_BASELINE=bf0222eacf17fca529df203e5bb7ee90f0b89171 \
node --test --test-name-pattern='network failures|token loading' \
  .github/scripts/legacy-quality.test.cjs
```

That command must fail on the two historical assertions. Omit `LEGACY_QUALITY_BASELINE` to verify the current implementation.
