# Live behavior-differential gate

Answers one question: **does this build behave identically to that build, over
real HTTP?**

Point it at two running servers built from different commits, drive the same
journey against both, and diff the recordings. It is black-box — it knows only
requests and responses — so the two servers may be arbitrarily far apart in
source.

This was written for the boundary-response-DTO refactor
([proposal](../../docs/proposals/20260714_boundary-response-dtos.md), rows
T19/T20), whose acceptance bar was "architecture changed, behavior didn't". It
is not specific to that change: any refactor claiming observable-behavior
invariance can use it.

## Relationship to the in-process Go harness

[`controller/behavior_differential_test.go`](../../controller/behavior_differential_test.go)
does the same thing in-process for the same endpoints, runs in CI, and needs no
servers — **prefer it**. This gate is the complement it cannot be:

| | Go harness (CI) | This gate (manual) |
| --- | --- | --- |
| Router, middleware, real auth | bypassed (handlers called directly) | exercised |
| Session cookies, real DB | no | yes |
| Relay → billing → log | no | yes (`relay_journey.py`) |
| Two binaries at once | no | yes |

Use this when a change could plausibly affect anything *outside* the handler
layer, or when you want an independent check that the in-process harness is not
lying to you.

## Requirements

Python 3 standard library only. No pip install.

## Running it

Build or check out the two versions you want to compare. A worktree at the
"before" commit is the easy way:

```sh
git worktree add /tmp/before <before-commit>
```

Start both servers on different ports with **separate, empty** databases (fresh
state matters — leftover rows change list lengths and produce false diffs):

```sh
# candidate (this tree)
SQLITE_PATH=/tmp/after.db  SESSION_SECRET=213 go run main.go --port 3000 &
# baseline (the "before" tree)
cd /tmp/before && SQLITE_PATH=/tmp/before.db SESSION_SECRET=213 go run main.go --port 3001 &
```

Both must still have the bootstrap `root` / `123456` account, which one-api
creates automatically on an empty database.

### Management surface (T19)

```sh
cd scripts/behavior-differential
python3 journey.py http://127.0.0.1:3001 /tmp/before.json   # baseline
python3 journey.py http://127.0.0.1:3000 /tmp/after.json    # candidate
python3 compare.py /tmp/before.json /tmp/after.json
```

### Relay, billing and logs (T20)

```sh
python3 mock_upstream.py 3002 &
python3 relay_journey.py http://127.0.0.1:3001 http://127.0.0.1:3002 /tmp/relay_before.json
python3 relay_journey.py http://127.0.0.1:3000 http://127.0.0.1:3002 /tmp/relay_after.json
python3 compare.py /tmp/relay_before.json /tmp/relay_after.json
```

`compare.py` exits non-zero on any difference, so it can gate a script.

## What is compared, and what is not

Recordings keep each step's **status code** and a canonical **shape**: every key
path with its literal value.

Values that legitimately differ between two independently seeded servers —
generated UUIDs, generated keys, wall-clock timestamps, `aff_code` — collapse to
a type marker (`lib.DYNAMIC_KEYS`). Everything else is compared **by value**, so
a changed field still fails. Two extra assertions run regardless of agreement:

- **secret leaks** — `password` / `access_token` / `totp_secret` /
  `verification_code` appearing anywhere fails, even if both builds leak it;
- **billing** — `quota_delta` from the relay journey must match exactly.

## Verifying the gate itself

Over-normalization is the failure mode here: normalize too much and everything
"agrees" while proving nothing. The same is true of a journey that silently runs
unauthenticated — every step 401s, both builds agree, and the result looks green.
(That is not hypothetical; it is why `lib.Client` replays the session cookie by
hand — one-api marks it `Secure`, so a stock cookie jar drops it over plain HTTP.)

So check the differ can fail:

```sh
python3 compare.py --self-test /tmp/after.json
```

It injects a status change, a leaked `id` key, a dropped step and a secret leak,
and asserts each is caught.

And sanity-check that the journey did real work — `journey.py` prints how many
steps returned 200, and non-200s should be only the intentional error cases. If
almost everything is 401, the recording is worthless no matter how well it
compares.
