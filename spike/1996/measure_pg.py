#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
# Times PickJob (queries/postgres/queue/queue.sql) with worker filters on the seeded databases.
# Each query runs under EXPLAIN ANALYZE in a rolled-back transaction, 3 times after a warm run; the
# median execution time, the buffers and the plan's scan are printed.
import json, statistics, subprocess, sys

ORDER = """AND NOT EXISTS (
    SELECT 1 FROM jobs AS k
    WHERE k.ordering_key = j.ordering_key AND k.id <> j.id
      AND (k.locked_until > now() OR k.created_at < j.created_at OR (k.created_at = j.created_at AND k.id < j.id))
  )"""
DUE = "j.run_at <= now() AND (j.locked_until IS NULL OR j.locked_until <= now())"

def plain(extra=""):
    return f"""SELECT j.id, j.ordering_key FROM jobs AS j
WHERE {DUE} {extra} {ORDER}
ORDER BY j.run_at, j.id LIMIT 1 FOR UPDATE SKIP LOCKED"""

def lateral(types):
    arr = "ARRAY[" + ",".join(f"'{t}'" for t in types) + "]"
    return f"""SELECT c.id, c.ordering_key FROM unnest({arr}) AS t(type)
CROSS JOIN LATERAL (
  SELECT j.id, j.ordering_key, j.run_at FROM jobs AS j
  WHERE j.type = t.type AND {DUE} {ORDER}
  ORDER BY j.run_at, j.id LIMIT 1 FOR UPDATE SKIP LOCKED
) AS c ORDER BY c.run_at, c.id LIMIT 1"""

def anyof(types):
    return "AND j.type = ANY(ARRAY[" + ",".join(f"'{t}'" for t in types) + "])"

def noneof(types):
    return "AND j.type <> ALL(ARRAY[" + ",".join(f"'{t}'" for t in types) + "])"

RARE = ["identity.email"]
BIG = ["federation.deliver"]
ABSENT = ["voice.absent"]
THREE = ["notification.email", "federation.inbox", "media.process"]
OTHERS = [f"other.t{i}" for i in range(78)] + THREE + RARE + ABSENT  # about the registered types, without the big one

CASES = [
    ("today: no filter", plain()),
    ("include 1 rare type", plain(anyof(RARE))),
    ("include the dominant type", plain(anyof(BIG))),
    ("include 1 absent type (idle poll)", plain(anyof(ABSENT))),
    ("include 3 types", plain(anyof(THREE))),
    ("include 83 types (exclude as list)", plain(anyof(OTHERS))),
    ("exclude the dominant type", plain(noneof(BIG))),
    ("exclude 1 rare type", plain(noneof(RARE))),
    ("lateral: 1 rare type", lateral(RARE)),
    ("lateral: 3 types", lateral(THREE)),
    ("lateral: 83 types", lateral(OTHERS)),
]
INDEX = "CREATE INDEX jobs_type_run_at ON jobs (type, run_at, id)"

def psql(db, sql):
    r = subprocess.run(["docker", "exec", "-i", "spike-1996-pg", "psql", "-X", "-At", "-q", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", db],
                       input=sql, text=True, capture_output=True)
    if r.returncode:
        raise SystemExit(r.stderr + sql)
    return r.stdout

def scans(node, out):
    t = node.get("Node Type", "")
    if "Scan" in t and node.get("Relation Name") == "jobs" and node.get("Alias", "") == "j":
        out.append(f"{t}({node.get('Index Name', '-')})")
    if t == "Sort":
        out.append("Sort")
    for c in node.get("Plans", []):
        scans(c, out)
    return out

def run(db, label, sql):
    times, plan = [], None
    for i in range(4):
        out = psql(db, f"BEGIN; EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) {sql}; ROLLBACK;")
        doc = json.loads(out[out.index("["):out.rindex("]") + 1])[0]
        if i:
            times.append(doc["Execution Time"])
        plan = doc
    p = plan["Plan"]
    buf = p.get("Shared Hit Blocks", 0) + p.get("Shared Read Blocks", 0)
    s = sorted(set(scans(p, [])))
    print(f"| {db} | {label} | {statistics.median(times):,.2f} | {buf:,} | {plan.get('Planning Time', 0):.2f} | {', '.join(s)} |", flush=True)

for db in sys.argv[1:] or ["burst", "mixed"]:
    psql(db, "DROP INDEX IF EXISTS jobs_type_run_at;")
    print(f"\n### {db}: no new index\n\n| db | filter | ms | buffers | plan ms | scan |\n| --- | --- | ---: | ---: | ---: | --- |")
    for label, sql in CASES:
        if not label.startswith("lateral"):
            run(db, label, sql)
    psql(db, INDEX + "; ANALYZE jobs;")
    print(f"\n### {db}: with (type, run_at, id)\n\n| db | filter | ms | buffers | plan ms | scan |\n| --- | --- | ---: | ---: | ---: | --- |")
    for label, sql in CASES:
        run(db, label, sql)
    print(psql(db, "SELECT 'index size ' || pg_size_pretty(pg_relation_size('jobs_type_run_at')) || ', jobs_run_at ' || pg_size_pretty(pg_relation_size('jobs_run_at'));"))
    psql(db, "DROP INDEX jobs_type_run_at;")
