"""Deterministic, non-production Web API fixture and independent response oracles."""
from __future__ import annotations
from collections import defaultdict
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import secrets
import sqlite3
import uuid

from http_fixture import require

END = 1790467200  # 2026-09-27T00:00:00Z; dashboard to_date remains inclusive.
START = END - 7 * 86400
ORIGIN = END - 90 * 86400
INDEX = 'idx_logs_user_created_at_id'
USERS = 32
QUOTA = 123456789
USED = 9876
METRICS = ('RequestCount', 'Quota', 'PromptTokens', 'CompletionTokens', 'CachedPromptTokens', 'CacheHitCount', 'CacheHitQuota')


def identity(kind: str, number: int) -> str:
    """identity returns a stable synthetic UUID without encoding any production identifier."""
    return str(uuid.uuid5(uuid.NAMESPACE_URL, f'one-api-web-perf/{kind}/{number}'))


def log_row(index: int, rows: int = 200000) -> dict:
    """log_row defines each fixture row independently of database queries and index layout."""
    user = 2 if index % 5 else 3 + (index // 5) % 30
    at = ORIGIN + (index-1) * (END-ORIGIN) // rows
    kind = 6 if index % 19 == 0 else 7 if index % 17 == 0 else 2
    prefix = f'fixture-{index}: hello 世界 |'
    return {'id': index, 'user_id': user, 'uuid': identity('log', index), 'user_uuid': identity('user', user),
            'created_at': at, 'type': kind, 'content': prefix + 'x' * (1024-len(prefix.encode())),
            'username': f'web-{user}', 'token_name': f'token-{index%3}', 'token_uuid': identity('token', user*3+index%3),
            'model_name': f'model-{index%4}', 'origin_model_name': f'model-{index%4}',
            'quota': index%997+1, 'prompt_tokens': index%251+1, 'completion_tokens': index%127+1,
            'channel_id': 0, 'channel_uuid': None, 'request_id': f'fixture-request-{index}', 'trace_id': '',
            'updated_at': at, 'elapsed_time': index%200, 'is_stream': index%2 == 0,
            'system_prompt_reset': False, 'cached_prompt_tokens': 10 if index%4 == 0 else 0, 'metadata': '{}'}


def public_row(row: dict) -> dict:
    """public_row independently describes the complete external log DTO, including omitted and nullable fields."""
    return {k: v for k, v in row.items() if k not in ('id', 'user_id', 'channel_id', 'metadata')}


def seed(database: Path, rows: int) -> dict:
    """seed populates a real migrated schema; ephemeral authentication values never leave the private fixture."""
    require(1000 <= rows <= 1000000, 'fixture rows outside bounds')
    tokens = {u: secrets.token_hex(16) for u in range(1, USERS+1)}
    digest = hashlib.sha256()
    with sqlite3.connect(database) as connection:
        connection.execute('DELETE FROM logs')
        connection.execute('DELETE FROM users WHERE id != 1')
        for user in range(1, USERS+1):
            role = 100 if user == 1 else 1
            values = (identity('user', user), f'web-{user}', f'Fixture user {user}', role, 1, tokens[user],
                      QUOTA, USED, 17, f'fixture-aff-{user}', ORIGIN, ORIGIN, '!fixture-password-login-disabled')
            columns = 'uuid,username,display_name,role,status,access_token,quota,used_quota,request_count,aff_code,created_at,updated_at,password'
            if user == 1:
                connection.execute('UPDATE users SET '+','.join(c+'=?' for c in columns.split(','))+' WHERE id=1', values)
            else:
                connection.execute('INSERT INTO users (id,'+columns+') VALUES ('+','.join('?' for _ in range(14))+')', (user,*values))
        names = list(log_row(1, rows))
        statement = 'INSERT INTO logs ('+','.join(names)+') VALUES ('+','.join('?' for _ in names)+')'
        for start in range(1, rows+1, 2000):
            batch = [log_row(i, rows) for i in range(start, min(rows+1, start+2000))]
            for row in batch:
                digest.update(json.dumps(row, sort_keys=True, separators=(',', ':')).encode())
                digest.update(b'\n')
            connection.executemany(statement, [[row[n] for n in names] for row in batch])
        connection.execute('PRAGMA wal_checkpoint(PASSIVE)') if not connection.in_transaction else None
    with sqlite3.connect(database) as connection:
        connection.execute('PRAGMA wal_checkpoint(TRUNCATE)')
    return {'tokens': tokens, 'rows': rows, 'logical_sha256': digest.hexdigest()}


def rows_in_window(rows: int, user: int = 0, *, start: int = START, end: int = END-1,
                   model: str = '', kind: int = 0, token: str = '') -> list[dict]:
    """rows_in_window filters the deterministic row generator without consulting SQL implementation or response data."""
    result = []
    # Derive an enclosing index range; apply explicit predicates to preserve boundary correctness.
    lo = max(1, (start-ORIGIN)*rows//(END-ORIGIN))
    hi = min(rows, (end-ORIGIN+1)*rows//(END-ORIGIN)+2)
    for i in range(lo, hi+1):
        row = log_row(i, rows)
        if (start <= row['created_at'] <= end and row['type'] != 6 and
                (not user or row['user_id'] == user) and (not kind or row['type'] == kind) and
                (not model or row['model_name'] == model) and (not token or row['token_name'] == token)):
            result.append(row)
    return sorted(result, key=lambda r: (r['created_at'],r['id']), reverse=True)


def dashboard(logs: list[dict], user: int) -> dict:
    """dashboard builds all six response views directly from fixture rows, preserving every count and quota field."""
    buckets = {name: {} for name in ('logs', 'user_logs', 'token_logs', 'tool_logs', 'tool_user_logs', 'tool_token_logs')}
    for row in logs:
        if row['type'] not in (2,7):
            continue
        day = datetime.fromtimestamp(row['created_at'], timezone.utc).strftime('%Y-%m-%d')
        owner = {'Day': day, 'Username': row['username'], 'user_uuid': row['user_uuid']}
        views = [('logs', {'Day': day, 'ModelName': row['model_name']}), ('user_logs', owner),
                 ('token_logs', {**owner, 'TokenName': row['token_name']})] if row['type'] == 2 else [
                 ('tool_logs', {'Day': day, 'ToolName': row['model_name']}), ('tool_user_logs', owner),
                 ('tool_token_logs', {**owner, 'TokenName': row['token_name']})]
        for name, keys in views:
            key = tuple(keys.items())
            fields = METRICS if row['type'] == 2 else ('RequestCount','Quota')
            value = buckets[name].setdefault(key, {**keys, **dict.fromkeys(fields,0)})
            value['RequestCount'] += 1
            value['Quota'] += row['quota']
            if row['type'] == 2:
                value['PromptTokens'] += row['prompt_tokens']
                value['CompletionTokens'] += row['completion_tokens']
                value['CachedPromptTokens'] += row['cached_prompt_tokens']
                if row['cached_prompt_tokens'] > 0:
                    value['CacheHitCount'] += 1
                    value['CacheHitQuota'] += row['quota']
    return {**{k: list(v.values()) or None for k,v in buckets.items()},
            'total_quota': QUOTA * (1 if user else USERS), 'used_quota': USED * (1 if user else USERS),
            'status': 'Active' if user else 'All Active'}


def canonical_dashboard(value: dict) -> dict:
    """canonical_dashboard compares complete group multisets without assuming unspecified database tie ordering."""
    return {k: sorted(v, key=lambda r: json.dumps(r, sort_keys=True)) if isinstance(v, list) else v for k,v in value.items()}


def check_dashboard(body: dict, expected: dict) -> None:
    """check_dashboard verifies envelope and all aggregates, not only HTTP200 or overall request counts."""
    require(body.get('success') is True and body.get('message') == '', 'dashboard failed envelope')
    require(canonical_dashboard(body.get('data', {})) == canonical_dashboard(expected), 'dashboard oracle mismatch')


def check_logs(body: dict, rows: list[dict], offset: int = 0, size: int = 20, *, cursor: bool = False) -> None:
    """check_logs verifies complete ordered rows, exact legacy totals or bounded cursor-count semantics."""
    require(body.get('success') is True and body.get('message') == '', 'logs failed envelope')
    require(body.get('data') == [public_row(r) for r in rows[offset:offset+size]], 'log rows/order/ownership mismatch')
    if not cursor:
        require(body.get('total') == len(rows), 'legacy exact count mismatch')
        return
    require(body.get('version') == 1 and body.get('has_more') is (offset+size < len(rows)), 'cursor boundary mismatch')
    require(isinstance(body.get('next_cursor'), str), 'missing cursor token')
    require(bool(body['next_cursor']) is body['has_more'], 'invalid cursor continuation')
    count = body.get('count', {})
    require(type(count.get('cached')) is bool and type(count.get('as_of')) is int and count['as_of'] > 0, 'invalid count provenance')
    quality, value = count.get('quality'), count.get('value')
    if quality == 'exact':
        require(value == len(rows), 'false exact cursor count')
    elif quality == 'lower_bound':
        require(type(value) is int and 10000 <= value <= len(rows), 'false bounded count')
    else:
        require(quality == 'unavailable' and value is None, 'invalid count envelope')
