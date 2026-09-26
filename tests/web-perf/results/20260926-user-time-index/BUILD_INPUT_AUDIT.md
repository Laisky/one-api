# PR430: review fixes and reproducible build-input audit

## Decision boundary

This follow-up fixes two genuine acceptance-check omissions and resolves a separate study-identity ambiguity without repeating completed traffic. It does not change the index, query, cache, authentication, API, workload, performance thresholds or saved timings. The prior head `100aab0303d46828e862b613f1a5dd16470cd096` passed full CI 36265304352; a later repair commit requires its own CI.

`report.compare()` now requires every literal-true post-write flag (`committed_insert`, `legacy_rows_and_count`, `all_dashboard_aggregates`) for both variants and the candidate's exact ordered index columns (`user_id`, `created_at`, `id`). These conditions already existed in the published-evidence verifier and actual fixture qualification; the standalone reporter must enforce them too. Its synthetic positive fixture now supplies the full contract. No favorable timings can compensate for missing correctness evidence.

Five new top-level regression tests exercise both strict and nonstrict modes, missing/false/truthy nonboolean flags, missing variant evidence, every wrong column permutation, truncated/extra/duplicate columns, and the raw-directory entry point. On the unchanged original reporter, 78 negative subcases fail their assertions because bad evidence is accepted or raw files are opened before contract rejection. After the repair all 13 locally run tests pass (eight existing fixture/worker/audit tests plus five new tests). These are not 78 independent production bugs. No failure is a compile or missing-dependency error; the raw-path negative control explicitly detects the wrong order of validation.

## The earlier `.gitkeep` finding and the published study are different

| UTC on 2026-09-26 | Record | Scope |
| --- | --- | --- |
| 17:45:53 | [5848442067](https://github.com/Laisky/one-api/pull/430#issuecomment-5848442067) | Earlier completed recovery claim, including self-dashboard P95 changes -70.62%/-55.83%. Its raw data was not recovered here. |
| 17:51:47 | [5848484441](https://github.com/Laisky/one-api/pull/430#issuecomment-5848484441) | Correctly identifies a missing candidate `web/build/.gitkeep` in that earlier study. Keep that study as pre-alignment evidence; no retrospective relabeling. |
| 18:36:11 | [5848803766](https://github.com/Laisky/one-api/pull/430#issuecomment-5848803766) | Preregisters the independently rebuilt binary pair below and a new complete confirmation. |
| 18:36:29.513053–18:51:43.128990 | Current committed manifest and compressed summary | The later 160-trial/51,200-response study in REPORT.md, including self-dashboard -72.76%/-64.76%. |
| This review follow-up | Fresh offline builds and complete input comparison | Reproduces both later binary hashes while retaining the exact same embedded `.gitkeep` in both variants. No HTTP performance traffic is generated. |

Review 4112471386 correctly questions identical-input claims when the empty marker differs, but its earlier-study finding does not describe the binary pair now in the committed manifest. This is established by reproduction, not by assuming an empty asset is harmless. The current report's scoped single-variable claim remains; the older study retains its disqualification and missing-raw boundary.

## Independently reproduced identities

The original source/vendor export from run 36246623576 / artifact 10908065782 is SHA-256 `35a5184ced396534d113949a236492287e7cba0893f1a9a639a9a447a5959c14`. Its public revision `7494f6b15670968f23c25e7fb747a272b0ee8e85` differs from baseline main `c13183989f47d1ee1d797151b81a1e36d5fb48a4` only in the temporary workflow and initial plan; the workflow was already removed. The actual public compare was rechecked.

One recovered source directory, vendor tree, embedded filesystem and Go 1.27.1 toolchain were used for both builds with `-mod=vendor -p=2 -trimpath -buildvcs=false`. The baseline was built before applying only the three Log index tags. The patched full `model/log.go` then matched public blob `0e27f60edd05da81b40df95c2f9fc6aba3c794d0`; its baseline blob is `6718493fbecdd7181eb8709fa1cb5d2fe4c45e3d`.

| Artifact | Rebuilt SHA-256 | Matches current published measurement |
| --- | --- | --- |
| Baseline | `5f32abdcdb53d10d970c7367d41ea9421d23f7f40a76c0035d50b2243783e3fd` | Yes, exactly |
| Index candidate | `b0f1b08c18b18e1dbe864d9ab973ba1324b6da7d3cbc53849b0f2debda1d455a` | Yes, exactly |

Actual `go list -json .` reports `EmbedFiles: ["web/build/.gitkeep"]` for both. Both files are zero bytes, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`. Comparing all 974 non-test repository Go/module/actual-embed inputs per variant yields exactly one difference: `model/log.go`. Vendor contents come from the same hash-verified export, not independently selected dependencies. `build-inputs.json` records these compact observations; the full path/hash inventories and actual Go package/build outputs are in the separately delivered validation archive, not implicitly embedded in this document.

This proves the identity and controlled source inputs of the binary pair used by the later published study. It does not reverify the original raw HTTP bodies, 51,200 request records, cost batches or profiles: the named full Web API raw archive was not present in the working runtime or targeted Library recovery. The committed compressed summary remains unmodified. No new speedup, DDL availability, native-engine performance, browser timing or production rollout is claimed.

## Reproduction and rollout limits

Inspect the baseline/candidate `go list -json .` EmbedFiles from the actual build trees rather than relying on Git's tracked filenames. Preserve ignored embedded markers when making detached worktrees. After a new build, bind complete source/module/embed inventories, toolchain/dependency inputs and exact binary hashes before timing. A mismatch must be recorded and resolved, not excused based on expected runtime relevance. Do not repeat this completed confirmation simply to restore context.

The existing index rollout constraints remain: normal AutoMigrate is not an online-index implementation; qualify locking/write availability, disk headroom, interrupted DDL and rollback coordination on the deployment's actual database and table size. This read-performance PR and its native correctness tests do not certify those operator-specific conditions.
