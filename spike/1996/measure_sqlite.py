#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
# Times the SQLite claim (queries/sqlite/queue/queue.sql) with worker filters on 1M jobs, burst distribution.
# Each claim runs in BEGIN IMMEDIATE and rolls back; the median of 5 runs after a warm one.
import os, sqlite3, statistics, sys, tempfile, time, uuid

path = os.path.join(sys.argv[1] if len(sys.argv) > 1 else tempfile.mkdtemp(), "spike.db")
if os.path.exists(path):
    os.remove(path)
db = sqlite3.connect(path, isolation_level=None)
db.execute("PRAGMA journal_mode=WAL")
db.execute("PRAGMA synchronous=NORMAL")
db.execute("""CREATE TABLE jobs (id TEXT PRIMARY KEY, type TEXT NOT NULL, payload BLOB NOT NULL, idempotency_key TEXT, ordering_key TEXT,
  trace_parent TEXT, attempt INTEGER NOT NULL, run_at INTEGER NOT NULL, locked_by TEXT, locked_until INTEGER, last_error TEXT, created_at INTEGER NOT NULL)""")
now = int(time.time() * 1000)
def rows():
    for i in range(1, 1000001):
        t = "federation.deliver" if i <= 900000 else "notification.email" if i <= 940000 else "federation.inbox" if i <= 980000 else "media.process" if i <= 999000 else "identity.email"
        ok = f"actor:{i % 5000}" if 900000 < i <= 980000 and i % 2 == 0 else None
        run = now - 3 * 3600000 + i * 8 if i <= 900000 else now - 3600000 + (i - 900000) * 30
        yield (f"{run:016x}{uuid.uuid4().hex[:16]}", t, b"{}", f"k{i}", ok, 0, run, run)
db.execute("BEGIN")
db.executemany("INSERT INTO jobs (id,type,payload,idempotency_key,ordering_key,attempt,run_at,created_at) VALUES (?,?,?,?,?,?,?,?)", rows())
db.execute("COMMIT")
db.execute("CREATE UNIQUE INDEX jobs_idempotency_key ON jobs (idempotency_key)")
db.execute("CREATE INDEX jobs_run_at ON jobs (run_at)")
db.execute("CREATE INDEX jobs_ordering_key_run_at ON jobs (ordering_key, run_at)")
db.execute("ANALYZE")

ORDER = """AND NOT EXISTS (SELECT 1 FROM jobs AS k WHERE k.ordering_key = j.ordering_key AND k.id <> j.id
  AND (k.locked_until > :now OR k.created_at < j.created_at OR (k.created_at = j.created_at AND k.id < j.id)))"""
DUE = "j.run_at <= :now AND (j.locked_until IS NULL OR j.locked_until <= :now)"
def plain(extra=""):
    return f"SELECT j.id FROM jobs AS j WHERE {DUE} {extra} {ORDER} ORDER BY j.run_at, j.id LIMIT 1"
def inlist(ts):
    return ",".join(f"'{t}'" for t in ts)
def probes(ts):
    # One probe per type in a UNION ALL: SQLite has no LATERAL.
    arms = " UNION ALL ".join(f"SELECT * FROM (SELECT j.id, j.run_at FROM jobs AS j WHERE j.type = '{t}' AND {DUE} {ORDER} ORDER BY j.run_at, j.id LIMIT 1)" for t in ts)
    return f"SELECT id FROM ({arms}) ORDER BY run_at, id LIMIT 1"
def claim(pick):
    return f"UPDATE jobs SET locked_by = 'w', locked_until = :now + 300000, attempt = attempt + 1 WHERE id = ({pick}) RETURNING id"

RARE, BIG, ABSENT = ["identity.email"], ["federation.deliver"], ["voice.absent"]
THREE = ["notification.email", "federation.inbox", "media.process"]
OTHERS = [f"other.t{i}" for i in range(78)] + THREE + RARE + ABSENT
CASES = [
    ("today: no filter", plain()),
    ("include 1 rare type", plain(f"AND j.type IN ({inlist(RARE)})")),
    ("include 1 absent type (idle poll)", plain(f"AND j.type IN ({inlist(ABSENT)})")),
    ("include 3 types", plain(f"AND j.type IN ({inlist(THREE)})")),
    ("exclude the dominant type", plain(f"AND j.type NOT IN ({inlist(BIG)})")),
]
PROBES = [("probes: 1 rare type", probes(RARE)), ("probes: 3 types", probes(THREE)), ("probes: 83 types", probes(OTHERS))]

def run(label, sql):
    ts, rowsback = [], None
    for i in range(6):
        db.execute("BEGIN IMMEDIATE")
        t = time.perf_counter()
        rowsback = db.execute(claim(sql), {"now": now}).fetchall()
        d = (time.perf_counter() - t) * 1000
        db.execute("ROLLBACK")
        if i:
            ts.append(d)
    plan = [r[3] for r in db.execute("EXPLAIN QUERY PLAN " + sql, {"now": now}) if "SCAN" in r[3] or "SEARCH" in r[3]]
    print(f"| {label} | {statistics.median(ts):,.2f} | {len(rowsback)} | {plan[0] if plan else ''} |", flush=True)

print("sqlite", sqlite3.sqlite_version)
print("\nno new index\n\n| filter | ms | rows | first plan step |\n| --- | ---: | ---: | --- |")
for l, s in CASES:
    run(l, s)
db.execute("CREATE INDEX jobs_type_run_at ON jobs (type, run_at, id)")
db.execute("ANALYZE")
print("\nwith (type, run_at, id)\n\n| filter | ms | rows | first plan step |\n| --- | ---: | ---: | --- |")
for l, s in CASES + PROBES:
    run(l, s)
db.close()
