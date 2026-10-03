#!/usr/bin/env python3
"""Measure additive-index build, storage and append costs separately from HTTP response acceptance."""
from __future__ import annotations
import argparse
import json
import os
from pathlib import Path
import sqlite3
import tempfile
import time

import dataset
from http_fixture import bootstrap,clone_database,gateway,require,sha256
from run import checkpoint,schema_evidence


def inspect(database: Path) -> dict:
    """inspect records external SQLite storage and query plans with its own engine version explicitly labeled."""
    with sqlite3.connect(database) as db:
        info={'engine':'Python SQLite inspection, not the native Go runtime','version':sqlite3.sqlite_version,
              'page_size':db.execute('PRAGMA page_size').fetchone()[0],
              'page_count':db.execute('PRAGMA page_count').fetchone()[0],
              'freelist_count':db.execute('PRAGMA freelist_count').fetchone()[0]}
        try:
            pages=db.execute('SELECT count(*),coalesce(sum(pgsize),0) FROM dbstat WHERE name=?',(dataset.INDEX,)).fetchone()
            info['index_pages'],info['index_bytes']=pages
        except sqlite3.OperationalError:
            info['index_bytes']=None
        plans={}
        for name,sql,args in [
            ('self-page','SELECT * FROM logs WHERE type!=6 AND user_id=? AND created_at>=? AND created_at<=? ORDER BY created_at DESC LIMIT 20',(2,dataset.START,dataset.END-1)),
            ('self-deep','SELECT * FROM logs WHERE type!=6 AND user_id=? AND created_at>=? AND created_at<=? ORDER BY created_at DESC LIMIT 20 OFFSET 1000',(2,dataset.START,dataset.END-1)),
            ('self-count','SELECT count(*) FROM logs WHERE type!=6 AND user_id=? AND created_at>=? AND created_at<=?',(2,dataset.START,dataset.END-1)),
            ('admin-page','SELECT * FROM logs WHERE type!=6 AND created_at>=? AND created_at<=? ORDER BY created_at DESC LIMIT 20',(dataset.START,dataset.END-1)),
        ]:
            plans[name]={'sql':sql,'args':args,'plan':[list(r) for r in db.execute('EXPLAIN QUERY PLAN '+sql,args)]}
        info['query_plans']=plans
        return info


def append_cost(database: Path, rows: list[dict]) -> dict:
    """append_cost times one committed batch of synthetic full rows after generating and validating the payload separately."""
    require(bool(rows),'empty insert cost batch')
    columns=list(rows[0]);values=[[r[k] for k in columns] for r in rows]
    sql='INSERT INTO logs ('+','.join(columns)+') VALUES ('+','.join('?' for _ in columns)+')'
    with sqlite3.connect(database) as db:
        before=db.execute('SELECT count(*) FROM logs').fetchone()[0]
        journal=db.execute('PRAGMA journal_mode').fetchone()[0]
        synchronous=db.execute('PRAGMA synchronous').fetchone()[0]
        started=time.perf_counter()
        db.executemany(sql,values);db.commit()
        elapsed=time.perf_counter()-started
        after=db.execute('SELECT count(*) FROM logs').fetchone()[0]
        require(after-before==len(rows),'insert batch accounting mismatch')
        return {'rows':len(rows),'seconds':elapsed,'rows_per_second':len(rows)/elapsed,
                'journal_mode':journal,'synchronous':synchronous,'scope':'Python SQLite bulk INSERT+COMMIT, not gateway writer end-to-end'}


def main() -> None:
    """main runs five alternating fresh-file cost pairs without concurrent HTTP performance traffic."""
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--baseline',type=Path,required=True);p.add_argument('--candidate',type=Path,required=True)
    p.add_argument('--token-cache',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
    args=p.parse_args()
    for name in ('baseline','candidate','token_cache','output'):setattr(args,name,getattr(args,name).resolve())
    require(not args.output.exists(),'refusing to overwrite cost evidence')
    args.output.mkdir(parents=True);os.environ['TIKTOKEN_CACHE_DIR']=str(args.token_cache)
    result={'complete':False,'rows':200000,'insert_rows':5000,'pairs':[],'binaries':{k:sha256(getattr(args,k)) for k in ('baseline','candidate')}}
    try:
        with tempfile.TemporaryDirectory(prefix='web-costs-') as directory:
            root=Path(directory);database=bootstrap(args.baseline,root/'seed');seed=dataset.seed(database,200000)
            result['dataset_sha256']=seed['logical_sha256']
            additional=[dataset.log_row(i,200000) for i in range(200001,205001)]
            for repeat in range(5):
                for label in (('baseline','candidate') if repeat%2==0 else ('candidate','baseline')):
                    dest=root/f'{repeat}-{label}';dest.mkdir();clone_database(database,dest/'one-api.db')
                    with gateway(getattr(args,label),dest) as running:
                        schema=schema_evidence(dest/'one-api.db',label=='candidate')
                        startup=running['startup_seconds']
                    before=inspect(dest/'one-api.db')
                    cost=append_cost(dest/'one-api.db',additional)
                    result['pairs'].append({'label':label,'repeat':repeat,'startup_seconds':startup,'schema':schema,
                                            'before':before,'append':cost,'after':inspect(dest/'one-api.db')})
                    checkpoint(args.output/'summary.json',result)
            # Direct DDL measurement is labeled separately; actual application migration was timed above.
            for repeat in range(5):
                file=root/f'ddl-{repeat}.db';clone_database(database,file)
                with sqlite3.connect(file) as db:
                    started=time.perf_counter()
                    db.execute('CREATE INDEX '+dataset.INDEX+' ON logs(user_id,created_at,id)');db.commit()
                    elapsed=time.perf_counter()-started
                result.setdefault('external_ddl',[]).append({'repeat':repeat,'seconds':elapsed,'storage':inspect(file)})
            result['complete']=True
    finally:
        checkpoint(args.output/'summary.json',result)
    print(json.dumps({'complete':result['complete'],'pairs':len(result['pairs'])}))


if __name__=='__main__':main()
