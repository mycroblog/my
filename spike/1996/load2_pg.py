#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
# A second round of load_pg.py. Rates come from the jobs actually deleted, since pgbench counts a transaction
# that claimed nothing, and the runs are short enough not to run out of the worker's jobs. Enqueue is repeated
# in rotating order, because one run per index shape was too noisy to compare.
import re, subprocess, statistics

ORDER = """AND NOT EXISTS (SELECT 1 FROM jobs AS k WHERE k.ordering_key = j.ordering_key AND k.id <> j.id
 AND (k.locked_until > now() OR k.created_at < j.created_at OR (k.created_at = j.created_at AND k.id < j.id)))"""
DUE = "j.run_at <= now() AND (j.locked_until IS NULL OR j.locked_until <= now())"

def plain(extra=""):
    return f"SELECT j.id FROM jobs AS j WHERE {DUE} {extra} {ORDER} ORDER BY j.run_at, j.id LIMIT 1 FOR UPDATE SKIP LOCKED"

def lateral(types):
    arr = "ARRAY[" + ",".join(f"'{t}'" for t in types) + "]"
    return f"""SELECT c.id FROM unnest({arr}) AS t(type) CROSS JOIN LATERAL (
 SELECT j.id, j.run_at FROM jobs AS j WHERE j.type = t.type AND {DUE} {ORDER}
 ORDER BY j.run_at, j.id LIMIT 1 FOR UPDATE SKIP LOCKED) AS c ORDER BY c.run_at, c.id LIMIT 1"""

def claim(pick):
    return f"WITH p AS ({pick}) DELETE FROM jobs AS d USING p WHERE d.id = p.id;\n"

ENQ = """INSERT INTO jobs (id, type, payload, idempotency_key, attempt, run_at, created_at)
VALUES (uuidv7(), 'notification.email', '\\x7b7d'::bytea, gen_random_uuid()::text, 0, now() + interval '1 hour', now());
"""
EMAIL = ["notification.email"]
FIVE = ["notification.email", "federation.inbox", "media.process", "identity.email", "voice.absent"]
NONE = ""
TRI = "CREATE INDEX jobs_type_run_at ON jobs (type, run_at, id); ANALYZE jobs;"
BI = "CREATE INDEX jobs_type_run_at ON jobs (type, run_at); ANALYZE jobs;"

def sh(*a, stdin=None):
    r = subprocess.run(["docker", "exec", "-i", "spike-1996-pg", *a], input=stdin, text=True, capture_output=True)
    if r.returncode:
        raise SystemExit(r.stdout + r.stderr)
    return r.stdout

def psql(db, sql):
    return sh("psql", "-X", "-At", "-q", "-U", "postgres", "-d", db, "-c", sql)

def bench(db, script, clients, secs):
    sh("sh", "-c", "cat > /tmp/s.sql", stdin=script)
    o = sh("pgbench", "-U", "postgres", "-n", "-M", "prepared", "-c", str(clients), "-j", str(clients), "-T", str(secs), "-f", "/tmp/s.sql", db)
    return float(re.search(r"tps = ([\d.]+)", o).group(1)), float(re.search(r"latency average = ([\d.]+) ms", o).group(1))

def fresh(setup):
    psql("spike", "DROP DATABASE IF EXISTS c")
    psql("spike", "CREATE DATABASE c TEMPLATE burst")
    if setup:
        sh("psql", "-X", "-q", "-U", "postgres", "-d", "c", "-c", setup)

print("| worker | clients | seconds | jobs deleted | jobs/s |\n| --- | ---: | ---: | ---: | ---: |")
for label, setup, pick, n, secs in [
    ("lateral email, (type, run_at, id)", TRI, lateral(EMAIL), 8, 5),
    ("lateral email, (type, run_at)", BI, lateral(EMAIL), 8, 5),
    ("lateral 5 types, (type, run_at, id)", TRI, lateral(FIVE), 8, 5),
    ("lateral 5 types, (type, run_at)", BI, lateral(FIVE), 8, 5),
    ("today, no filter", NONE, plain(), 8, 5),
]:
    fresh(setup)
    bench("c", claim(pick), n, secs)
    left = int(psql("c", "SELECT count(*) FROM jobs"))
    print(f"| {label} | {n} | {secs} | {1000000 - left:,} | {(1000000 - left) / secs:,.0f} |", flush=True)

shapes = [("today's indexes", NONE), ("+ (type, run_at, id)", TRI), ("+ (type, run_at)", BI)]
res = {l: [] for l, _ in shapes}
for r in range(3):
    for l, s in shapes[r:] + shapes[:r]:
        fresh(s)
        psql("c", "CHECKPOINT")
        res[l].append(bench("c", ENQ, 8, 15)[0])
print("\n| enqueue, 8 clients | inserts/s, 3 runs | median |\n| --- | --- | ---: |")
for l, _ in shapes:
    print(f"| {l} | {', '.join(f'{x:,.0f}' for x in res[l])} | {statistics.median(res[l]):,.0f} |", flush=True)
psql("spike", "DROP DATABASE IF EXISTS c")
