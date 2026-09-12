# GitHub Actions

Maintain two workflow entrypoints. Add ordinary regression tests to the existing
Go/frontend suites, not a new workflow for each provider, bug or pull request.
The filenames are retained so existing workflow URLs and badges still resolve.

| File / display name | Responsibility | Triggers |
| --- | --- | --- |
| `workflows/lint.yml` / **CI** | Tests, static analysis, vulnerability scan, coverage and the aggregate check | All PRs; pushes to `main`, `master`, `test/ci`; merge queues; manual |
| `workflows/ci.yml` / **Build and deploy** | Publish amd64/arm64 images and perform the existing SSH deployment | Existing push branches and delivery exclusions only |

## What CI guarantees

The single full Go run uses race detection, fresh execution, verbose evidence
and coverage: `go test -race -v -cover -coverprofile=coverage.txt -count=1
-timeout 45m ./...`. It includes the DeepSeek, GPT Image and Realtime test
packages previously repeated by standalone workflows. `go vet ./...` runs once.

The Go job retains MySQL 8.4, PostgreSQL 17, all primary/log/baseline DSNs,
secondary database creation, full Git history for pinned old-binary tests,
`ffmpeg`, a 90-minute job budget and both no-skip guards:
`ONEAPI_REQUIRE_DB_BACKENDS=1` and `ONEAPI_REQUIRE_COMPACT_UUID_SUITE=1`.
Scale and replication tiers remain opt-in. Nothing runs with `-short`.

Go checks run for every CI event, including documentation-only PRs, preserving
the previous unfiltered `pr-check` behavior and covering non-Go fixtures and
embedded assets without a fragile Go-only path allowlist. Main-branch pushes
now also receive the live database qualification previously limited to PRs.

The goroutine/context guard, type-aware entity-response analyzer, Realtime
`err113` check and `govulncheck` remain blocking. Frontend tests are selected by
changed theme or shared workflow/build inputs; manual runs test every theme.
Installs use frozen Yarn lockfiles, and Modern also gets a production build.

**CI required** fails on a failed/cancelled check, missing change-detection
output or an unexpectedly skipped mandatory job. Only unaffected frontends
and the unrequested historical replay may be skipped. The coverage PR comment
is informational; tests and the coverage artifact remain mandatory. Forks and
Dependabot run validation without needing secrets or a writable PR token.

Configure branch protection to require **CI required** rather than retired
workflow/job names. This change does not edit repository protection settings.
The old `Go unit tests`, `Go Vet`, `Run Tests`, provider-regression checks and
`Boundary entity-response guardrail` are represented by the consolidated gate.

## Optional historical evidence

Current-code billing regressions always run in the full suite. To additionally
prove that the transcription-index test fails against the original broken
implementation, dispatch **CI** with `historical_control=true`. The job retains
the pinned baseline `6138a94f9749312a9c31c7c2ebdcbea758b04235` and requires both
exit status 1 and the intended assertion failure; a build/network error is not
accepted as a successful negative control. This expensive old-code replay is
manual rather than repeated automatically on every related PR.

## Delivery behavior preserved

Existing push branches, path exclusions, commit-prefix skip rules, image
names/tags, credentials references, concurrency groups and SSH commands remain
unchanged. PR/manual CI runs cannot publish images or deploy. Delivery still
follows its existing independent push policy; this refactor does not introduce
a new deployment approval gate or claim that CI completion gates deployment.

The amd64 publishing build now exports its own cache instead of starting a
second build-only job. Cache export remains non-blocking for deployment.
Separate amd64/arm64 cache scopes prevent the two exports overwriting each
other. The native amd64 build no longer initializes QEMU; arm64 still does.

The Windows/release jobs were unreachable because the original workflow only
accepted branch pushes, while those jobs required tag refs. They are removed,
not silently enabled by adding new release triggers.

## Retired automation

`pr.yml`, `deepseek-regression.yml`, `image-billing-regression.yml` and
`realtime-billing-regression.yml` are absorbed as described above. No
application regression-test source is removed. Full Go logs are retained as
`go-test-evidence-<sha>` for 14 days, replacing provider-specific log artifacts.

`run-frontend-security-update.yml` and its two Python generators were temporary
patch-generation scaffolding restricted to one branch, not an ongoing general
security scanner. Their hardening step expected obsolete non-frozen Yarn and
Node 20 workflow text. They are removed rather than kept as a broken manual
tool. Dependabot, frozen installs and the existing Go vulnerability scan are
unchanged; this refactor does not claim to add a general frontend OSV audit.

## Workflow validation

```sh
python3 -m pip install 'PyYAML==6.0.3'
python3 .github/scripts/test_workflows.py
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -color
```

The contract tests reject duplicate YAML keys, check triggers and preserved
coverage, parse every shell block, and execute the actual aggregate-gate code
against successful, failed, cancelled, skipped and missing-job scenarios.
GitHub Actions runs both the contract tests and actionlint. Application tests,
live database qualification and frontend builds must also pass on the PR;
workflow validation alone is not evidence of an application-test pass.

References: [workflow syntax and permissions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax),
[paths-filter event/permission behavior](https://github.com/dorny/paths-filter),
[Docker GitHub Actions cache scopes and export options](https://docs.docker.com/build/cache/backends/gha/).
