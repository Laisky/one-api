# Browser process-group cleanup follow-up

This follow-up to PR #437 addresses review comment `4149475652` against
`bae918b8ccad0ac8fd00b2f0a95c5cdaf468e52f`.

## Defect and scope

`Popen.wait()` reaps only the `yarn dev` leader. The old helper sent `SIGKILL`
only after a leader timeout, so a descendant ignoring `SIGTERM` could survive
when the leader exited normally or had already exited before cleanup. Both
scenarios fail the new real-process regressions against the old helper. The
original seven cleanup tests passed that helper and did not detect this leak.

`stop_process` now sends `SIGKILL` to the owned process group after the graceful
leader wait whether it succeeds or times out. It waits again only when polling
shows that the leader still needs reaping. Expected process-group exit races
remain harmless; permission errors and a final wait timeout still propagate.
The caller must use `start_new_session=True`. This helper covers descendants
remaining in that group, not processes that deliberately escape the session.
There is no change to production application code or dependency versions.

## Regression coverage

The descendant follow-up established 13 tests (retained in the current suite): the original seven updated for the
stronger group contract, two real descendant cases, and four additional cases
covering exit-before-poll, a bounded forced wait, permission errors at either
signal, and invalid timeouts. Both real descendant cases verify the initial
parent PID and process-group ID, a successful leader exit, descendant termination
by `SIGKILL`, and the disappearance of the reaped process group.

The real descendant fixture allows a five-second graceful leader wait so
scheduler and interpreter-shutdown latency under parallel load do not turn the
normal-exit scenario into an unintended timeout scenario. The zero leader exit
assertion is retained, and separate timeout regressions still require bounded
graceful and forced waits. Parallel stress exposed the original 200-millisecond
fixture budget as too short; this changes the test budget, not helper defaults.

The descendant tests use Linux's child-subreaper API only in the standalone test
process, restore the original setting, and independently kill and reap fixtures
on failure. This keeps historical negative-control runs from leaking live
processes or zombies. Those two tests are Linux-specific and run on the Ubuntu
CI runners; the helper itself remains POSIX-only as before.

The existing Air and Berry browser entry point runs this suite before browser
acceptance, so no new workflow or optional check is needed. Final PR acceptance
still requires the complete CI result for the updated head, not the older green
run. See [the UI-boundary acceptance contract](dependency-ui-boundaries-2026-09-30.md).

## Reproduce

Run all 24 cleanup regressions with Python 3.11+ and no frontend dependencies:

```sh
python3 .github/scripts/test_browser_process.py
```

On Linux, replay the same descendant tests with the previous helper. The command
must fail on both `Cleanup left the SIGTERM-ignoring descendant alive` assertions:

```sh
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
cp .github/scripts/test_browser_process.py .github/scripts/test_browser_cleanup_failures.py "$scratch/"
git show bae918b8ccad0ac8fd00b2f0a95c5cdaf468e52f:.github/scripts/browser_process.py \
  > "$scratch/browser_process.py"
python3 "$scratch/test_browser_process.py" \
  BrowserProcessTests.test_real_descendant_survives_graceful_leader_exit \
  BrowserProcessTests.test_real_descendant_survives_already_exited_leader
```

## Interruption and concurrent failure follow-up

The helper at `cb0c87745364587a0a2c83e6b89a40307eeb4d8d` passes those
13 tests, but an interruption during its graceful wait skips the forced phase.
A new real-child test reproduces the live, unreaped child; a separate signal-call
test confirms that SIGKILL was never attempted. Their teardown is independent
of the helper, so the negative controls do not leave processes behind.

Cleanup now attempts the graceful phase, forced group signal, and final leader
reaping independently. Unexpected errors and interruptions are retained until
those bounded attempts finish. One failure is re-raised unchanged; multiple
failures use Python 3.11's `BaseExceptionGroup`, including cancellations. At most
two waits use the supplied finite, positive timeout; NaN and infinities are
rejected before signaling. Missing groups remain an expected exit race.

The actual browser runner uses `process_cleanup` around readiness and browser
execution. If the task fails and cleanup succeeds, the original task error is
re-raised unchanged. If both fail, both exceptions remain in a group, with the
task failure first. A cleanup-only failure is still a failed acceptance run.
This does not hide errors, log request payloads, or retry cleanup indefinitely.
It cannot guarantee cleanup after SIGKILL of the harness, loss of signal
permission, or a descendant deliberately leaving its owned process group.

Eleven additional cases cover interruption, TERM/KILL errors, multiple failures,
nonfinite budgets, success/failure/cancellation in the cleanup scope, the actual
runner's use of that scope, and interrupted reaping of a real child. The existing
entry point imports this suite, so both legacy browser CI jobs run all 24 tests.
Eight full local runs at concurrency four passed **192/192 test executions**,
with no skipped cases on Linux. The final tests still produce two assertion
failures against `cb0c877`, while its original 13 tests pass. These local results
do not replace the final-head frontend, backend, browser, and security CI gates.

To replay the two interruption failures using the final tests:

```sh
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
cp .github/scripts/test_browser_cleanup_failures.py "$scratch/"
git show cb0c87745364587a0a2c83e6b89a40307eeb4d8d:.github/scripts/browser_process.py \
  > "$scratch/browser_process.py"
python3 "$scratch/test_browser_cleanup_failures.py" \
  BrowserCleanupFailureTests.test_interrupted_wait_still_kills_and_reaps \
  BrowserCleanupFailureTests.test_real_child_is_reaped_after_interrupted_wait
```

The request-fixture JSDoc and Modern DOM-mock/translation documentation were also
completed. Their comment-free parsed source was compared before and after the
edit; these documentation changes do not alter test behavior or relax checks.
