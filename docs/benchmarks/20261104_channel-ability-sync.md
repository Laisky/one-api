# Channel abilities: in-place sync vs. destructive rebuild (PR #610, 2026-11-04)

## Why this was measured

Every channel save used to delete and recreate all of the channel's `abilities` rows. That
lifted active relay-time suspensions (`suspend_until`), which was the review's P1 on PR #610.
The fix (`model/ability_sync.go`, `syncAbilitiesWithDB`) reconciles the rows in place
instead. It deletes only dropped `(group, model)` rows, inserts only new ones, and updates
routing flags only when they drifted.

This is a correctness fix, not an opt-in optimization. It changes the database work of
every channel save, so the before/after cost is measured here on all three supported
engines.

## Environment

- CPU: AMD Ryzen 7 5700G (16 threads), Linux 6.8, Go 1.27.3.
- SQLite: file database under `t.TempDir()`, one connection.
- MySQL 8.0.46 (Docker `mysql:8.0`, default configuration) on 127.0.0.1.
- PostgreSQL 16.15 (Docker `postgres:16-alpine`, default configuration) on 127.0.0.1.
- Shapes: `2x1` is 2 models in 1 group (2 abilities), `50x2` is 100 abilities, and
  `300x3` is 900 abilities.

The host also runs unrelated containers, and every engine is fsync-bound, so absolute
times are noisy (±20–90%). Treat the direction and the significance (`p`) as the result,
not the absolute milliseconds.

## 1. The abilities write alone (`BenchmarkChannelAbilityProjection`)

Both arms run in one harness on one tree, for a save that leaves models and groups
unchanged (a testing model, name or pricing save):

- `rebuild` is the old behaviour: `deleteAbilitiesWithDB` then `addAbilitiesWithDB`.
- `sync` is the new `syncAbilitiesWithDB`.

Raw data: [`projection-arms.txt`](20261104_channel-ability-sync/projection-arms.txt).

| Engine / shape | rebuild | sync | Δ time | Δ B/op |
|---|---:|---:|---:|---:|
| SQLite 2x1 | 14.27 ms | 0.42 ms | −97% | −34% |
| SQLite 50x2 | 38.88 ms | 2.22 ms | −94% | −16% |
| SQLite 300x3 | 50.73 ms | 15.70 ms | −69% | −4% |
| MySQL 2x1 | 18.63 ms | 7.82 ms | −58% | −38% |
| MySQL 50x2 | 36.87 ms | 9.51 ms | −74% | −46% |
| MySQL 300x3 | 112.65 ms | 21.03 ms | −81% | −42% |
| PostgreSQL 2x1 | 7.07 ms | 3.22 ms | −54% | −33% |
| PostgreSQL 50x2 | 9.72 ms | 6.05 ms | −38% | −66% |
| PostgreSQL 300x3 | 50.06 ms | 14.31 ms | −71% | −70% |
| **geomean** | 26.95 ms | 5.70 ms | **−79%** | **−42%** |

Every row is significant at p=0.008 (n=5, Mann–Whitney U, `benchstat`). An unchanged save
now writes no ability rows at all, where the rebuild rewrote all of them.

## 2. The whole save path (`BenchmarkChannelTestingModelSave`)

This benchmarks `Channel.UpdateWithContext`, alternating SKIP and auto-select. That
includes the row lock, the new upstream-columns read, `testing_model` resolution, the
abilities write and cache invalidation. It uses only APIs that exist on both trees, so
the same file ran on the baseline (`4978421`, the PR head before this follow-up) and on
the fixed tree.

The authoritative comparison is **interleaved**: after, before, after, before, each run
with `-count=3` and a `sync` between runs, giving n=6 per arm. Raw data:
[`save-before.txt`](20261104_channel-ability-sync/save-before.txt) and
[`save-after.txt`](20261104_channel-ability-sync/save-after.txt).

| Engine / shape | before | after | Δ time (p) | Δ B/op |
|---|---:|---:|---:|---:|
| SQLite 2x1 | 10.11 ms | 8.41 ms | ~ (p=0.240) | +2.0% |
| SQLite 50x2 | 13.85 ms | 11.03 ms | ~ (p=0.093) | −4.2% |
| SQLite 300x3 | 77.67 ms | 29.77 ms | **−62%** (p=0.004) | −1.7% |
| MySQL 2x1 | 47.54 ms | 40.10 ms | ~ (p=0.240) | +0.7% |
| MySQL 50x2 | 76.52 ms | 48.43 ms | ~ (p=0.065) | −23% |
| MySQL 300x3 | 170.29 ms | 67.00 ms | **−61%** (p=0.002) | −26% |
| PostgreSQL 2x1 | 30.83 ms | 19.35 ms | **−37%** (p=0.009) | +1.7% |
| PostgreSQL 50x2 | 38.39 ms | 19.77 ms | **−48%** (p=0.002) | −41% |
| PostgreSQL 300x3 | 91.81 ms | 60.83 ms | ~ (p=0.065) | −53% |
| **geomean** | 44.84 ms | 27.28 ms | **−39%** | **−19%** |

No configuration is significantly slower. The small-channel saves allocate about 5–6%
more objects, from the two extra SELECTs (upstream columns and the existing abilities),
with no measurable time cost.

### A rejected sequential run

The first comparison used [`save-before-sequential.txt`](20261104_channel-ability-sync/save-before-sequential.txt)
and [`save-after-sequential.txt`](20261104_channel-ability-sync/save-after-sequential.txt).
In it, the "after" run executed straight after the projection benchmark, whose `rebuild`
arm writes up to 900 rows per iteration on every engine. That run showed PostgreSQL 2x1
at +145% and 50x2 at +47%, and SQLite (a fresh temp file each run) also got noisier.

The interleaved run above reversed both PostgreSQL results with significance. The
statement count also rules out a code cause: a testing-model-only save now issues one
statement fewer than before (+2 SELECT, −1 DELETE, −1 INSERT). The sequential run is
kept for transparency. It measures disk write-back pressure left by the preceding
benchmark, not this change.

## Reproduce

```bash
MYSQL_DSN='root:…@tcp(127.0.0.1:3306)/oneapi?charset=utf8mb4&parseTime=True&loc=UTC' \
PG_DSN='postgres://postgres:…@127.0.0.1:5432/oneapi?sslmode=disable' \
go test ./model/ -run '^$' -bench 'BenchmarkChannelAbilityProjection|BenchmarkChannelTestingModelSave' -benchmem -count=5
```

The server DSNs are administrative connections. Each benchmark creates a randomly named
`oneapi_probe_bench_*` database, migrates its own tables there, and drops only that
database during cleanup. The database user needs CREATE DATABASE and DROP DATABASE
privileges. Use a disposable server; an interrupted process can leave its isolated
database behind.
