#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
# Load runs with pgbench on a copy of the `burst` database: workers that claim and delete (one statement,
# without the lease write and the ordering-key advisory lock of the real claim), and enqueue throughput.
import re, subprocess

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
TWO = "CREATE INDEX jobs_type_run_at ON jobs (type, run_at, id); ANALYZE jobs;"
TWO2 = "CREATE INDEX jobs_type_run_at ON jobs (type, run_at); ANALYZE jobs;"

CASES = [
    ("today, no filter (claims the 900k head)", "", plain(), [1, 8]),
    ("plain type = email, no new index (900k skipped)", "", plain("AND j.type = 'notification.email'"), [1, 8]),
    ("plain type <> deliver, no new index (900k skipped)", "", plain("AND j.type <> 'federation.deliver'"), [1, 8]),
    ("lateral email, (type, run_at, id)", TWO, lateral(EMAIL), [1, 8]),
    ("lateral 5 types, (type, run_at, id)", TWO, lateral(FIVE), [1, 8]),
    ("lateral 5 types, (type, run_at)", TWO2, lateral(FIVE), [8]),
]

def sh(*a, stdin=None):
    r = subprocess.run(["docker", "exec", "-i", "spike-1996-pg", *a], input=stdin, text=True, capture_output=True)
    if r.returncode:
        raise SystemExit(r.stdout + r.stderr)
    return r.stdout

def psql(db, sql):
    return sh("psql", "-X", "-At", "-q", "-U", "postgres", "-d", db, "-c", sql)

def bench(db, script, clients, secs=20):
    sh("sh", "-c", "cat > /tmp/s.sql", stdin=script)
    o = sh("pgbench", "-U", "postgres", "-n", "-M", "prepared", "-c", str(clients), "-j", str(clients), "-T", str(secs), "-f", "/tmp/s.sql", db)
    lat = re.search(r"latency average = ([\d.]+) ms", o).group(1)
    tps = re.search(r"tps = ([\d.]+)", o).group(1)
    return float(tps), float(lat)

def fresh(setup):
    psql("spike", "DROP DATABASE IF EXISTS c")
    psql("spike", "CREATE DATABASE c TEMPLATE burst")
    if setup:
        sh("psql", "-X", "-q", "-U", "postgres", "-d", "c", "-c", setup)

print("| worker | clients | claims/s | latency ms | jobs left |\n| --- | ---: | ---: | ---: | ---: |")
for label, setup, pick, clients in CASES:
    for n in clients:
        fresh(setup)
        tps, lat = bench("c", claim(pick), n)
        left = psql("c", "SELECT count(*) FROM jobs").strip()
        print(f"| {label} | {n} | {tps:,.0f} | {lat:.2f} | {left} |", flush=True)

print("\n| enqueue | clients | inserts/s | latency ms |\n| --- | ---: | ---: | ---: |")
for label, setup in [("today's indexes", ""), ("+ (type, run_at, id)", TWO), ("+ (type, run_at)", TWO2)]:
    fresh(setup)
    tps, lat = bench("c", ENQ, 8)
    print(f"| {label} | 8 | {tps:,.0f} | {lat:.2f} |", flush=True)
    if setup:
        print("size", psql("c", "SELECT pg_size_pretty(pg_relation_size('jobs_type_run_at'))").strip())
psql("spike", "DROP DATABASE IF EXISTS c")
