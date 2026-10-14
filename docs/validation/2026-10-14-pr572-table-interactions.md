# PR 572 table interaction acceptance — 2026-10-14

## Qualified source

Production runtime and React regression source: `3166fac4bf7cddcaf0d906fe021f2755c62bf490`.
Final browser driver and production build: `81c5d98b065f4d94605c9c1f06a0684c6e875d73`.

The latter differs only in the runbook and browser-driver menu lifecycle synchronization. The hosted qualification explicitly checked that every other tracked file, including runtime, React tests, dependencies, configuration and existing workflows, remained identical before reusing the earlier full-suite results. This acceptance note is documentation-only and does not change build inputs.

## Passed qualification

- Node 24.21.0 with locked Yarn dependencies.
- Complete Modern frontend suite: **125 test files, 1,063 passing tests, one pre-existing skip**.
- Full lint: **zero errors, 128 existing warnings**; all five locale checks passed.
- TypeScript compilation and `make build-frontend-modern` passed with the unchanged production release-input guard.
- Actual compiled application browser acceptance: **12 of 12 cases passed** across Channels, Users, Tokens, Redemptions, MCP Servers and Logs at 1280px and 390px.
- Browser cases verify initial request sort parameters, reload persistence, visible row ordering and indicators, single/forward/reverse selection, all-matching exclusions, stale-anchor prevention, account isolation, unsupported saved-column fallback and mobile padded-label activation. No browser exceptions, external requests or API writes were observed.

Full-suite and negative-control receipts: [run 38510777943](https://github.com/Laisky/one-api/actions/runs/38510777943), artifact `pr572-release-qualification`. That run passed the complete frontend suite, lint and build but failed the browser-driver menu interaction; it is not described as an overall passing run.

Final browser/build and source-parity receipts: [run 38511433859](https://github.com/Laisky/one-api/actions/runs/38511433859), artifact `pr572-browser-acceptance`. The browser driver now waits for the real dropdown open/close lifecycle; no feature assertions were removed. Results and screenshots cover all twelve page/viewport combinations.

## Reproduction and repair evidence

The real release guard rejected the nine unlisted first-party files and passed after only the reviewed manifest was updated. Six unsupported-column and sixteen identical-row view-change regressions failed against the previous runtime and passed after repair; see [run 38509761949](https://github.com/Laisky/one-api/actions/runs/38509761949).

Four Suspense/concurrent-render cases failed against the render-time anchor reset and passed with commit-phase reset. A focused native mobile negative control recorded two trusted checkbox click events for one activation, with an empty resulting selection and no API or JavaScript failure. Owning the single/range toggle and preventing default native activation fixes that path without shrinking the mobile hit area. The final twelve-case browser pass includes clicks inside the padded label area.

The normal final-head PR CI/security checks remain separately recorded on GitHub. These frontend receipts do not claim execution of the optional full Go/database qualification, a production backend test, image deployment, or live customer interaction. Existing CI/security/deployment controls were not modified. Release and rollback follow the [table interaction runbook](../table-sort-and-range-selection.md).
