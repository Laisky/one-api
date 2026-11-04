# Frontend dependency advisories — 2026-11-04

The required "Dependency audit" jobs failed for every PR that touched a frontend, because
new advisories were published against packages already in the lockfiles. `main`'s own
runs skip those jobs when no frontend path changes, so the failure was invisible there.
This change clears every advisory in all three shipped themes without weakening the gate.

## Advisories and resolution

| Advisory | Package | Reached through | Severity | Resolution |
|---|---|---|---|---|
| GHSA-mrhx-c7xc-m3w5 | zod 4.6.5 | Modern (direct), `eslint-plugin-react-hooks` | high | 4.6.7 inside the declared ranges; Modern's direct floor raised to `^4.6.7` |
| GHSA-4qv8-2p9j-2685, GHSA-f72v-6w45-q472, GHSA-xjj6-f33q-929c, GHSA-4w7w-5xrc-9h26, GHSA-2m5v-pch6-74hx, GHSA-qrxm-fqhp-jwvm, GHSA-9fm3-97px-rjf6, GHSA-55wc-3pg8-jfw6 | undici 8.11.2 | `jsdom` (tests only) | moderate–high | 8.12.3 inside jsdom's `^8.10.2` |
| GHSA-mrwp-f3q8-3g9p | follow-redirects 1.16.1 | `axios` | high | 1.16.2 inside axios's `^1.16.0` |
| GHSA-g3hv-75wg-m249 | minimatch 10.2.6 | `eslint`, `@eslint/config-array`, `@typescript-eslint/typescript-estree` (lint only) | moderate | scoped `resolutions` to 11.0.4 (see below) |

The in-range packages were re-resolved by removing only their lockfile entries, as the
earlier follow-redirects refresh did. Every other locked version is unchanged.

### Why minimatch is a resolution

The fix shipped only in minimatch 11.0.1, three days before the advisory. There is no
10.x backport, and the latest eslint (10.14.0), `@eslint/config-array` (0.23.5) and
typescript-eslint (8.72.1) still declare `^10.2.x`. The 2026-09-30 refresh deliberately
avoided forcing incompatible transitive majors, so this one was checked for
compatibility before it was forced:

- **API.** 11.0 removes `makeRe()`, `Minimatch#regexp`, `matchOne()`, `AST`,
  `slashSplit()`, the `debug` and `maxExtglobRecursion` options. The installed consumers
  call only `new Minimatch(pattern, { dot: true })`, `.match(path[, partial])`,
  `.negate`, `.pattern` and `minimatch(path, pattern, { dot: true })`, all retained.
- **Behaviour.** `eslint --format json` over each theme's real sources is identical under
  10.2.6 and 11.0.4: the same files linted (Modern 400, Air 70, Berry 141) and the same
  messages.
- **Scope.** The resolutions name each reviewed consumer (`**/eslint/minimatch`, …)
  rather than every minimatch. A regression test fails if any other installed package
  starts depending on minimatch, or if a reviewed consumer starts using a removed API.

Drop the resolutions once eslint and typescript-eslint declare minimatch 11.

## Regression coverage

`.github/scripts/frontend-dependency-advisories.test.cjs` and
`.github/scripts/follow-redirects-security.test.cjs` reproduce each advisory through the
copy its real consumer resolves. Every test failed on the previous lockfiles and passes
now:

| Test | Vulnerable result | Patched result |
|---|---|---|
| zod: an oversized `.max()`-bounded array, set or map fails on its bound | one issue per element (20,000) plus `too_big` | a single `too_big`, no element parsed |
| minimatch: twenty `**/../x/y` portions at `optimizationLevel: 2` | exponential set with quadratic dedupe; does not finish within the 20 s limit | stops at `braceExpandMax` (100,000) in about 2 s |
| undici: a tampered body pinned with an uppercase `SHA256-` SRI token | accepted | rejected; matching digests in every case still accepted |
| follow-redirects: a cross-realm (`node:vm`) `sensitiveHeaders` array | the custom credential follows a cross-origin redirect | stripped; same-origin and unrelated headers unchanged |

Both files run in each theme's `yarn test`, and in every "Dependency audit" CI job before
the registry audit, so they still guard the tree when the registry is unavailable.
`test_frontend_contracts.py` keeps that step present, theme-correct and fail-closed. The
follow-redirects test existed before but was not wired into any test command or job.

## Security acceptance

| Theme | Audited dependencies | Critical | High | Moderate | Low |
| --- | ---: | ---: | ---: | ---: | ---: |
| Modern | 681 | 0 | 0 | 0 | 0 |
| Air | 651 | 0 | 0 | 0 | 0 |
| Berry | 441 | 0 | 0 | 0 | 0 |

Before this change: Modern 9 moderate and 7 high; Air and Berry 6 moderate and 6 high
each. A clean audit is a point-in-time registry result, not a guarantee against
undiscovered vulnerabilities.

## Reproduce

Use Node 24 and Yarn Classic, matching CI. From the repository root:

```sh
for theme in modern air berry; do
  (cd "web/$theme" && yarn install --frozen-lockfile --non-interactive)
  FRONTEND_THEME="$theme" node --test .github/scripts/follow-redirects-security.test.cjs \
    .github/scripts/frontend-dependency-advisories.test.cjs
  node .github/scripts/frontend-audit.mjs "$theme"
done
```
