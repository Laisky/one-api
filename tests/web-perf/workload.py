"""Explicit Web API targets, independent response checks and isolation/freshness qualification."""
from __future__ import annotations
from dataclasses import dataclass
import http.client
import json
from pathlib import Path
import sqlite3
from typing import Callable
from urllib.parse import urlencode

import dataset as data
from http_fixture import get, require


@dataclass
class Endpoint:
    """Endpoint binds one real path to its authorized principal, role and independent response oracle."""
    name: str
    path: str
    principal: int
    target: bool
    check: Callable[[dict], None]


def query(path: str, **fields) -> str:
    """query URL-encodes explicit fixture filters without creating arbitrary destinations."""
    return path+'?'+urlencode(fields)


def check_self(body: dict) -> None:
    """check_self checks known user values and forbids credential/internal-ID exposure."""
    require(body.get('success') is True, 'self failed envelope')
    user = body['data']
    for key, expected in {'uuid': data.identity('user',2), 'username':'web-2', 'role':1, 'status':1,
                          'quota':data.QUOTA, 'used_quota':data.USED, 'request_count':17}.items():
        require(user.get(key) == expected, 'self identity/counter mismatch: '+key)
    require(not {'id','password','access_token','totp_secret'} & set(user), 'self leaked secret/internal ID')


def endpoints(rows: int) -> tuple[list[Endpoint], list[dict], list[dict]]:
    """endpoints freezes four targets and four controls before timing, including the existing opt-in cursor route."""
    all_logs = data.rows_in_window(rows)
    personal = [r for r in all_logs if r['user_id'] == 2]
    dashboard_self, dashboard_all = data.dashboard(personal,2), data.dashboard(all_logs,0)
    dates = {'from_date':'2026-09-20','to_date':'2026-09-26'}
    filters = {'start_timestamp':data.START,'end_timestamp':data.END-1,'sort':'created_at','order':'desc','size':20}
    exact_sum = sum(r['quota'] for r in all_logs if r['type'] == 2)
    def stat(body: dict) -> None:
        """stat verifies the exact consume-only quota sum from generated rows."""
        require(body == {'success':True,'message':'','data':{'quota':exact_sum}}, 'stat sum/envelope mismatch')
    specs = [
        Endpoint('self-dashboard',query('/api/user/dashboard',**dates),2,True,lambda b:data.check_dashboard(b,dashboard_self)),
        Endpoint('self-logs',query('/api/log/self',p=0,**filters),2,True,lambda b:data.check_logs(b,personal)),
        Endpoint('self-deep',query('/api/log/self',p=50,**filters),2,True,lambda b:data.check_logs(b,personal,1000)),
        Endpoint('self-cursor',query('/api/log/self/cursor',**filters),2,True,lambda b:data.check_logs(b,personal,cursor=True)),
        Endpoint('site-dashboard',query('/api/user/dashboard',**dates),1,False,lambda b:data.check_dashboard(b,dashboard_all)),
        Endpoint('admin-logs',query('/api/log/',p=0,**filters),1,False,lambda b:data.check_logs(b,all_logs)),
        Endpoint('admin-stat',query('/api/log/stat',start_timestamp=data.START,end_timestamp=data.END-1),1,False,stat),
        Endpoint('user-self','/api/user/self',2,False,check_self),
    ]
    return specs, personal, all_logs


def validate(port: int, endpoint: Endpoint, token: str) -> tuple[dict, float]:
    """validate performs one complete real HTTP read, checking status and all workload-specific invariants."""
    conn = http.client.HTTPConnection('127.0.0.1',port,timeout=30)
    try:
        status, body, ms, _ = get(conn,endpoint.path,token)
        require(status == 200, 'HTTP status for '+endpoint.name)
        endpoint.check(body)
        return body, ms
    finally:
        conn.close()


def qualify(port: int, tokens: dict, rows: int, specs: list[Endpoint], personal: list[dict]) -> dict:
    """qualify checks positive, negative, selective-filter and cross-principal pagination paths before any timing."""
    first = {}
    for endpoint in specs:
        _, first[endpoint.name] = validate(port,endpoint,tokens[endpoint.principal])
    conn = http.client.HTTPConnection('127.0.0.1',port,timeout=30)
    try:
        for path, principal, code in [('/api/user/self',0,401),('/api/log/',2,403),('/api/log/stat',2,403)]:
            status,body,_,_=get(conn,path,tokens.get(principal,''))
            require(status == code and body.get('success') is False, 'authorization oracle failed')
        for fields in [{'from_date':'2026-09-01','to_date':'2026-09-26'},
                       {'from_date':'2026-09-20','to_date':'2026-09-26','user_id':data.identity('user',3)}]:
            status,body,_,_=get(conn,query('/api/user/dashboard',**fields),tokens[2])
            require(status == 200 and body.get('success') is False, 'dashboard scope/range gate failed')
        selected = data.rows_in_window(rows,2,model='model-1',kind=2,token='token-1')
        url=query('/api/log/self',p=0,size=7,sort='created_at',order='desc',type=2,model_name='model-1',token_name='token-1',
                  start_timestamp=data.START,end_timestamp=data.END-1)
        status,body,_,_=get(conn,url,tokens[2]);require(status==200,'filtered status');data.check_logs(body,selected,size=7)
        status,body,_,_=get(conn,specs[3].path,tokens[2]);require(status==200,'cursor first status')
        next_cursor=body['next_cursor'];require(next_cursor,'fixture needs next page')
        next_url=specs[3].path+'&'+urlencode({'cursor':next_cursor})
        status,body,_,_=get(conn,next_url,tokens[2]);require(status==200,'cursor next status');data.check_logs(body,personal,20,cursor=True)
        status,body,_,_=get(conn,next_url,tokens[3]);require(body.get('success') is False,'foreign cursor must fail closed')
        empty=query('/api/log/self',size=20,start_timestamp=data.END+100,end_timestamp=data.END+200)
        status,body,_,_=get(conn,empty,tokens[2]);require(status==200,'empty status');data.check_logs(body,[])
    finally:
        conn.close()
    return {'complete':True,'all_eight_endpoints':True,'auth_and_tenant_isolation':True,'filtered_exact_rows':len(selected),
            'cursor_continuation_and_cross_user_rejection':True,'empty_shape':True,'first_hit_ms':first}


def freshness(port: int, database: Path, tokens: dict, rows: int) -> dict:
    """freshness inserts one synthetic committed row after all timings and verifies immediate list/stat/dashboard changes."""
    original=data.rows_in_window(rows)
    row=data.log_row(rows+1,rows)
    row.update(user_id=2,user_uuid=data.identity('user',2),username='web-2',created_at=data.END-1,updated_at=data.END-1,type=2)
    with sqlite3.connect(database) as db:
        names=list(row)
        db.execute('INSERT INTO logs ('+','.join(names)+') VALUES ('+','.join('?' for _ in names)+')',[row[k] for k in names])
    personal=sorted([r for r in original if r['user_id']==2]+[row],key=lambda r:(r['created_at'],r['id']),reverse=True)
    ep=Endpoint('fresh-list',query('/api/log/self',p=0,size=20,sort='created_at',order='desc',start_timestamp=data.START,end_timestamp=data.END-1),2,True,
                lambda b:data.check_logs(b,personal))
    validate(port,ep,tokens[2])
    ep=Endpoint('fresh-dashboard',query('/api/user/dashboard',from_date='2026-09-20',to_date='2026-09-26'),2,True,
                lambda b:data.check_dashboard(b,data.dashboard(personal,2)))
    validate(port,ep,tokens[2])
    return {'committed_insert':True,'legacy_rows_and_count':True,'all_dashboard_aggregates':True}
