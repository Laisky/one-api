#!/usr/bin/env python3
"""Reproduce issue #395 with real SQLite SQL, not mocked latency or HTTP claims.

The before arm mirrors the six model/log.go queries at BASE_COMMIT. The after
arm reads the exact SQL embedded by the production Go implementation. Both run
against the same fixture with fresh aggregate computations (no Redis/results
cache), warm database/OS caches, and alternating measurement order. Native Go,
MySQL and PostgreSQL benchmarks are in model/dashboard_aggregate_bench_test.go.
"""
from __future__ import annotations

import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import platform
import random
import sqlite3
import statistics
import sys
import tempfile
import time
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
BASE_COMMIT = "7cfbc6ebaf6505cadc5c28a69d37676ca2db50da"
SQL_BYTES = (ROOT / "model/dashboard_aggregate.sql").read_bytes()
NEW_SQL = SQL_BYTES.decode().replace("/*MATERIALIZED*/", "MATERIALIZED")
DAY_SQL = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch'))"
SUMS = """count(1) as request_count, sum(quota) as quota,
 sum(prompt_tokens) as prompt_tokens, sum(completion_tokens) as completion_tokens,
 sum(cached_prompt_tokens) as cached_prompt_tokens,
 sum(CASE WHEN cached_prompt_tokens > 0 THEN 1 ELSE 0 END) as cache_hit_count,
 sum(CASE WHEN cached_prompt_tokens > 0 THEN quota ELSE 0 END) as cache_hit_quota"""
DIMS = [
    ("model_name", "model_name", "model_name"),
    ("username, user_id, COALESCE(user_uuid, '') as user_uuid",
     "username, user_id, user_uuid", "username"),
    ("COALESCE(token_name, '') as token_name, username, user_id, COALESCE(user_uuid, '') as user_uuid",
     "token_name, username, user_id, user_uuid", "username, token_name"),
    ("COALESCE(model_name, '') as tool_name", "tool_name", "tool_name"),
    ("COALESCE(username, '') as username, user_id, COALESCE(user_uuid, '') as user_uuid",
     "username, user_id, user_uuid", "username, user_id"),
    ("COALESCE(username, '') as username, user_id, COALESCE(user_uuid, '') as user_uuid, COALESCE(token_name, '') as token_name",
     "username, user_id, user_uuid, token_name", "username, user_id, token_name"),
]
METRICS = ["request_count", "quota", "prompt_tokens", "completion_tokens",
           "cached_prompt_tokens", "cache_hit_count", "cache_hit_quota"]
KEYS = [["day", "model_name"], ["day", "username", "user_id", "user_uuid"],
        ["day", "token_name", "username", "user_id", "user_uuid"], ["day", "tool_name"],
        ["day", "username", "user_id", "user_uuid"],
        ["day", "username", "user_id", "user_uuid", "token_name"]]
STRINGS = {"day", "model_name", "tool_name", "username", "user_uuid", "token_name"}
CASES = {"290k": (290000, 7, False), "1m": (1000000, 30, False),
         "high-cardinality": (100000, 30, True)}
BASE = 1767225600


def check(condition: bool, message: str) -> None:
    """Fail even under python -O; invalid fixtures must never produce evidence."""
    if not condition:
        raise AssertionError(message)


def legacy_queries(start: int, end: int, user: int) -> list[tuple[str, list[int]]]:
    """Return the six original SQL shapes and bound values in response order."""
    queries = []
    for kind, (select, group, order) in enumerate(DIMS):
        metrics = SUMS if kind < 3 else "count(1) as request_count, COALESCE(sum(quota), 0) as quota"
        sql = f"SELECT {DAY_SQL} as day, {select}, {metrics} FROM logs WHERE type = ? AND created_at >= ? AND created_at < ?"
        args = [2 if kind < 3 else 7, start, end]
        if user:
            sql += " AND user_id = ?"
            args.append(user)
        queries.append((sql + f" GROUP BY day, {group} ORDER BY day, {order}", args))
    return queries


def legacy(db: sqlite3.Connection, start: int, end: int, user: int) -> list:
    """Execute and fully fetch the six uncached baseline aggregate queries."""
    return [db.execute(sql, args).fetchall() for sql, args in legacy_queries(start, end, user)]


def optimized_query(start: int, end: int, user: int) -> tuple[str, list[int]]:
    """Bind the same window/scope into the exact production SQL template."""
    return (NEW_SQL.replace("/*USER_FILTER*/", "AND user_id = ?" if user else ""),
            [start, end] + ([user] if user else []))


def optimized(db: sqlite3.Connection, start: int, end: int, user: int) -> list:
    """Execute and fully fetch one optimized aggregate statement."""
    return db.execute(*optimized_query(start, end, user)).fetchall()


def normalized(rows: list, after: bool, nocase: bool = False) -> list[Counter]:
    """Compare DTO-equivalent values, retaining duplicate NULL/empty groups."""
    result = [Counter() for _ in range(6)]
    source = rows if after else [(kind, row) for kind, items in enumerate(rows) for row in items]
    for item in source:
        if after:
            kind, stamp, model, username, user_id, uuid, token, *metrics = item
            data = dict(zip(METRICS, metrics[:7]))
            data.update(day=datetime.fromtimestamp(stamp, timezone.utc).strftime("%Y-%m-%d"),
                        model_name=model, tool_name=model, username=username, user_id=user_id,
                        user_uuid=uuid, token_name=token)
            names = KEYS[kind] + METRICS[:7 if kind < 3 else 2]
            values = [data[name] for name in names]
        else:
            kind, values = item
            names = KEYS[kind] + METRICS[:7 if kind < 3 else 2]
        key = []
        for name, value in zip(names, values):
            if value is None:
                value = "" if name in STRINGS else 0
            # SQL may choose either spelling for a NOCASE group. SQLite's
            # COALESCE(tool_name), unlike its raw column, groups as BINARY.
            if nocase and isinstance(value, str) and name != "tool_name":
                value = value.lower()
            key.append(value)
        result[kind][tuple(key)] += 1
    return result


def schema(db: sqlite3.Connection, nocase: bool = False) -> None:
    """Create a disposable log projection with the existing relevant indexes."""
    collate = "NOCASE" if nocase else "BINARY"
    db.executescript(f"""CREATE TABLE logs (
        user_id BIGINT, user_uuid TEXT COLLATE {collate}, created_at BIGINT, type INTEGER,
        model_name TEXT COLLATE {collate}, username TEXT COLLATE {collate}, token_name TEXT COLLATE {collate},
        quota BIGINT, prompt_tokens BIGINT, completion_tokens BIGINT, cached_prompt_tokens BIGINT,
        content TEXT, metadata TEXT);
        CREATE INDEX idx_created_at_type ON logs(created_at, type);
        CREATE INDEX idx_logs_user_id ON logs(user_id);""")


def verify() -> dict:
    """Differentially check adversarial values, boundaries, scopes and scan plans."""
    combinations = 0
    for nocase in (False, True):
        with sqlite3.connect(":memory:") as db:
            schema(db, nocase)
            rng = random.Random(395)
            names = [None, "", "alice", "bob", "UPPER", "upper"]
            stamps = [-86401, -86400, -1, 0, 1, 86400, BASE-1, BASE, BASE+1, BASE+86399, BASE+86400]
            rows = [(rng.choice([None, 0, 1, 2, 3]), rng.choice(names), rng.choice(stamps),
                     rng.choice([1, 2, 2, 2, 6, 7, 7]), rng.choice(names), rng.choice(names), rng.choice(names),
                     rng.choice([None, 0, -11, 97, 2**40]), rng.choice([0, 11, None]),
                     rng.choice([None, 20]), rng.choice([None, -1, 0, 21]), None, None)
                    for _ in range(2000)]
            db.executemany("INSERT INTO logs VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)", rows)
            for start, end in [(-90000, BASE+86401), (BASE, BASE+86400), (0, 0), (0, 1),
                               (-86400, 0), (-86400, -86400), (BASE+86400, BASE)]:
                for user in (0, 1, 2, 404, -1):
                    before = normalized(legacy(db, start, end, user), False, nocase)
                    after = normalized(optimized(db, start, end, user), True, nocase)
                    check(before == after, f"differential mismatch: {nocase=}, {start=}, {end=}, {user=}")
                    combinations += 1
            plan = db.execute("EXPLAIN QUERY PLAN " + optimized_query(-90000, BASE+86401, 0)[0],
                              [-90000, BASE+86401]).fetchall()
            check(sum("SEARCH logs " in row[-1] or "SCAN logs" in row[-1] for row in plan) == 1,
                  "optimized plan does not access raw logs exactly once")
        db.close()
    return {"window_scope_combinations": combinations, "result_set_comparisons": combinations*6,
            "adversarial_rows_per_collation": 2000, "collations": ["BINARY", "NOCASE"], "passed": True}


def seed(db: sqlite3.Connection, count: int, days: int, distinct: bool) -> dict:
    """Match the Go benchmark distribution and independently tally requests/quota."""
    totals = {scope: {kind: [0, 0] for kind in (2, 7)} for scope in (0, 7)}
    payload = "x"*1024
    for offset in range(0, count, 5000):
        rows = []
        for i in range(offset, min(offset+5000, count)):
            user = 100+(i//10)%100 if i%10 == 0 else 7
            kind = 7 if i%23 == 0 else 2
            quota = 50+i%100
            rows.append((user, f"00000000-0000-4000-8000-{user:012d}", BASE+i*days*86400//count,
                         kind, f"model-or-tool-{(i//19)%17:02d}", f"user-{user:03d}",
                         f"token-{i:09d}" if distinct else f"token-{(i//11)%11:02d}",
                         quota, 100+i%200, 10+i%20, 25 if i%3 == 0 else 0, payload, "{}"))
            for scope in (0, 7):
                if scope == 0 or user == scope:
                    totals[scope][kind][0] += 1
                    totals[scope][kind][1] += quota
        db.executemany("INSERT INTO logs VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)", rows)
    db.commit()
    check(db.execute("SELECT count(*) FROM logs").fetchone()[0] == count, "partial fixture")
    return totals


def benchmark_case(case: str, samples: int, storage: str) -> list[dict]:
    """Measure both arms sequentially with warm-up, equality and alternating order."""
    count, days, distinct = CASES[case]
    results = []
    with tempfile.TemporaryDirectory(prefix="dashboard-395-") as directory:
        path = Path(directory)/"logs.db"
        db = sqlite3.connect(str(path) if storage == "file" else ":memory:")
        try:
            db.execute("PRAGMA journal_mode=WAL")
            db.execute("PRAGMA synchronous=NORMAL")
            db.execute("PRAGMA cache_size=-2000")
            db.execute("PRAGMA temp_store=FILE")
            schema(db)
            expected = seed(db, count, days, distinct)
            db.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchall()
            for user in (0, 7):
                start, end = BASE, BASE+days*86400
                before = legacy(db, start, end, user)
                after = optimized(db, start, end, user)
                check(normalized(before, False) == normalized(after, True), "large-fixture result mismatch")
                for kind, view in ((2, 0), (7, 3)):
                    tally = [sum(row[2] for row in before[view]), sum(row[3] or 0 for row in before[view])]
                    check(tally == expected[user][kind], "independent request/quota tally mismatch")
                output_rows = len(after)
                del before, after
                timings = {"legacy": [], "optimized": []}
                for iteration in range(samples):
                    order = ("legacy", "optimized") if iteration%2 == 0 else ("optimized", "legacy")
                    for arm in order:
                        fn = legacy if arm == "legacy" else optimized
                        began = time.perf_counter_ns()
                        rows = fn(db, start, end, user)
                        elapsed_ms = (time.perf_counter_ns()-began)/1e6
                        timings[arm].append(elapsed_ms)
                        del rows
                query, args = optimized_query(start, end, user)
                plan = db.execute("EXPLAIN QUERY PLAN "+query, args).fetchall()
                old_plan = [row for sql, values in legacy_queries(start, end, user)
                            for row in db.execute("EXPLAIN QUERY PLAN "+sql, values).fetchall()]
                accesses = lambda items: sum("SEARCH logs " in row[-1] or "SCAN logs" in row[-1] for row in items)
                check(accesses(old_plan) == 6 and accesses(plan) == 1, "unexpected physical scan count")
                old_ms, new_ms = statistics.median(timings["legacy"]), statistics.median(timings["optimized"])
                result = {"case": case, "rows": count, "days": days, "distinct_token_per_row": distinct,
                          "scope_user": user, "storage": storage, "audit_payload_bytes": 1024,
                          "fixture_bytes": path.stat().st_size if path.exists() else None,
                          "output_rows": output_rows, "expected_counts_and_quota": expected[user],
                          "samples_ms": timings, "legacy_median_ms": old_ms, "optimized_median_ms": new_ms,
                          "speedup": old_ms/new_ms, "legacy_log_accesses": accesses(old_plan),
                          "optimized_log_accesses": accesses(plan), "optimized_plan": plan}
                results.append(result)
                print(f"{case} user={user}: {old_ms:.2f} -> {new_ms:.2f} ms ({old_ms/new_ms:.2f}x)", file=sys.stderr, flush=True)
        finally:
            db.close()
    return results


def main() -> None:
    """Run verification and optionally emit reproducible JSON measurement evidence."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify-only", action="store_true")
    parser.add_argument("--samples", type=int, default=5)
    parser.add_argument("--storage", choices=["file", "memory"], default="file")
    parser.add_argument("--cases", nargs="+", choices=list(CASES), default=list(CASES))
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    if args.samples < 2:
        parser.error("--samples must be at least 2")
    report: dict[str, Any] = {"base_commit": BASE_COMMIT, "sql_sha256": hashlib.sha256(SQL_BYTES).hexdigest(),
        "python": sys.version, "sqlite": sqlite3.sqlite_version, "platform": platform.platform(),
        "cpu_affinity_count": len(os.sched_getaffinity(0)) if hasattr(os, "sched_getaffinity") else os.cpu_count(),
        "measurement": "SQL execution plus Python row fetch; no application result cache; warm DB/OS cache; alternating order",
        "verification": verify(), "results": []}
    if not args.verify_only:
        for case in args.cases:
            report["results"].extend(benchmark_case(case, args.samples, args.storage))
    text = json.dumps(report, indent=2)+"\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(text)
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
