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

The cleanup suite now contains 13 tests: the original seven updated for the
stronger group contract, two real descendant cases, and four additional cases
covering exit-before-poll, a bounded forced wait, permission errors at either
signal, and invalid timeouts. Both real descendant cases verify the initial
parent PID and process-group ID, a successful leader exit, descendant termination
by `SIGKILL`, and the disappearance of the reaped process group.

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

Run all cleanup regressions without frontend dependencies:

```sh
python3 .github/scripts/test_browser_process.py
```

On Linux, replay the same descendant tests with the previous helper. The command
must fail on both `Cleanup left the SIGTERM-ignoring descendant alive` assertions:

```sh
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
cp .github/scripts/test_browser_process.py "$scratch/"
git show bae918b8ccad0ac8fd00b2f0a95c5cdaf468e52f:.github/scripts/browser_process.py \
  > "$scratch/browser_process.py"
python3 "$scratch/test_browser_process.py" \
  BrowserProcessTests.test_real_descendant_survives_graceful_leader_exit \
  BrowserProcessTests.test_real_descendant_survives_already_exited_leader
```
