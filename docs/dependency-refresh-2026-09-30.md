# Dependency refresh — 2026-09-30

This refresh consolidates PRs #394 and #383 into #437. The manifests and lockfiles were resolved from package registries, rather than edited to claim versions that had not been installed.

## Compatibility decisions

Go modules are updated within existing import-major paths. The Go 1.27.1 toolchain and the existing module/fork choices remain unchanged. Notable updates include gRPC 1.84.0, WebAuthn 0.18.2, AWS SDK/Bedrock, Google APIs, database drivers, and `golang.org/x/*`.

Modern uses the current stable direct dependencies selected by this refresh, including React 19.3.0, Vite 8.3.1, Vitest 5.0.3 and Axios 1.20.0. The intentional TypeScript 6/7 package aliases are preserved. The local jest-dom adapter registers the original runtime matchers while extending Vitest 5's `Matchers<R, T>` interface. It does not disable strict TypeScript library checks. Existing i18n mocks and browser polyfills are unchanged.

Air and Berry retain their React 18, CRA and existing UI component/chart major lines. They use Axios 1.20.0 and React Router 7.18.4. Berry's router core and DOM bindings are pinned together; upgrading one independently risks installing two incompatible routing contexts. This is not a claim that every legacy package has been migrated to its newest major version.

## Scoped security overrides

Some transitive consumers pin vulnerable patch versions. The following resolutions stay within the same major and are deliberately scoped to those consumers:

| Themes | Resolution | Patched version | Reason |
| --- | --- | --- | --- |
| Air | `**/geojson-flatten/minimist` | 1.2.8 | The chart dependency pins 1.2.0, which can pollute object prototypes. |
| Air, Berry | `**/jsonpath/underscore` | 1.13.8 | The CRA toolchain's jsonpath dependency pins the older 1.13.6 patch. |

Do not replace these with blanket overrides to unrelated major versions. Remove an override only when its actual consumer resolves a patched version without the override, the frozen lockfile install succeeds, the dependency-security regressions pass, and the audit no longer reports the affected package.

The regression harness resolves each dependency from its actual consumer, not an unrelated hoisted package. It verifies prototype-pollution resistance, normal argument parsing, underscore template behavior and JSONPath queries. The original installed minimist was also tested and failed the pollution assertion before the resolution was applied.

## Validation evidence

- [Dependency resolution and initial validation](https://github.com/Laisky/one-api/actions/runs/36764257083): module verification, compilation of all Go test packages, and go vet passed; both legacy builds passed; all three frozen lockfile installs passed. The Modern build exposed the matcher declaration conflict subsequently fixed by this PR.
- [PR compatibility validation](https://github.com/Laisky/one-api/actions/runs/36765722468): Modern tests, strict build and translation checks passed after the matcher fix; both legacy test jobs, Go static guardrails and govulncheck passed. Complete Go/race/database and browser checks are tracked by CI, not inferred from compilation.
- [Security resolution validation](https://github.com/Laisky/one-api/actions/runs/36766207707): the negative control, patched dependency regressions, existing legacy tests, both builds, frozen installs and security audit assertions passed. The temporary validation workflow was not imported into the PR branch.

Both legacy `yarn test` commands run the compatibility and dependency-security regressions through `pretest`, then run the original react-scripts test command. Existing CI path filters include `.github/**`, so edits to these harnesses trigger the relevant checks.

## Residual audit findings

These are Yarn advisory occurrence counts from the validated dependency trees, not counts of distinct vulnerabilities or proof of production exploitability:

| Theme | Critical | High | Moderate | Low |
| --- | ---: | ---: | ---: | ---: |
| Modern | 0 | 0 | 0 | 0 |
| Air | 0 | 8 | 12 | 3 |
| Berry | 0 | 8 | 12 | 3 |

Before the scoped security patch, Air had four critical occurrences through minimist; both legacy themes also reported underscore. Neither package remains in the validated audit findings.

Remaining findings follow the legacy CRA build/development dependency tree: `nth-check`, `postcss`, `serialize-javascript`, `svgo`, `uuid`, `webpack-dev-middleware`, `webpack-dev-server`, and `@tootallnate/once`. They are not hidden by audit exclusions or incompatible major-version overrides. A dedicated legacy toolchain migration is needed to address this remaining debt; this PR does not claim the old themes are vulnerability-free.

## Reproduce checks

```sh
go mod verify
go test -run '^$' ./...
go vet ./...
# The full repository CI also runs Go tests with race detection and databases.

(cd web/modern && yarn install --frozen-lockfile && yarn test --run && yarn build && yarn check:i18n)
(cd web/air && yarn install --frozen-lockfile && CI=true yarn test --watchAll=false --runInBand --passWithNoTests && CI=false yarn build)
(cd web/berry && yarn install --frozen-lockfile && CI=true yarn test --watchAll=false --runInBand --passWithNoTests && CI=false yarn build)

# These commands intentionally retain nonzero audit exits for residual findings.
(cd web/air && yarn audit)
(cd web/berry && yarn audit)
```
