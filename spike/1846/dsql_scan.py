# SPDX-License-Identifier: AGPL-3.0-only
#
# Spike #1846 (not for merge, ADR-0022): scan schema/postgres and queries/postgres for
# constructs that Aurora DSQL does not accept, per the AWS user guide checked on 2026-10-29.
# Run from the repository root: python3 -I spike/1846/dsql_scan.py
import collections
import glob
import re

NON_INDEXABLE = ("bytea", "interval", "json", "jsonb", "timetz")
MAX_INDEXES = 24
MAX_INDEX_COLUMNS = 8
MAX_TABLES = 1000


def statements(paths):
    text = "\n".join(open(p).read() for p in sorted(paths))
    text = re.sub(r"--[^\n]*", "", text)
    return [s.strip() for s in text.split(";") if s.strip()]


def scan_schema():
    tables, cols = {}, collections.defaultdict(dict)
    indexes = collections.Counter()
    findings = []
    for s in statements(glob.glob("schema/postgres/*.sql")):
        m = re.match(r"CREATE TABLE\s+(\w+)\s*\((.*)\)\s*$", s, re.S | re.I)
        if m:
            t, body = m.group(1), m.group(2)
            tables[t] = body
            for cm in re.finditer(r"^\s*(\w+)\s+(\w+)", body, re.M):
                cols[t][cm.group(1)] = cm.group(2).lower()
            indexes[t] += 1 + len(re.findall(r"\bUNIQUE\b", body, re.I))
            for c, typ in cols[t].items():
                if typ in NON_INDEXABLE and (
                    re.search(rf"^\s*{c}\s+{typ}[^,\n]*(PRIMARY KEY|UNIQUE)", body, re.M | re.I)
                    or re.search(rf"(PRIMARY KEY|UNIQUE)\s*\([^)]*\b{c}\b", body, re.I)
                ):
                    findings.append(f"key on {typ} column: {t}.{c}")
            continue
        m = re.match(r"CREATE\s+(UNIQUE\s+)?INDEX\s+(\w+)\s+ON\s+(\w+)\s*(USING\s+\w+\s*)?\((.*?)\)", s, re.S | re.I)
        if m:
            name, t, using, keys = m.group(2), m.group(3), m.group(4), m.group(5)
            indexes[t] += 1
            findings.append(f"CREATE INDEX without ASYNC (Atlas CE output): {name}")
            if using:
                findings.append(f"index method {using.strip()}: {name}")
            if len(keys.split(",")) > MAX_INDEX_COLUMNS:
                findings.append(f"more than {MAX_INDEX_COLUMNS} key columns: {name}")
            for c, typ in cols[t].items():
                if typ in NON_INDEXABLE and re.search(rf"\b{c}\b", keys):
                    findings.append(f"index on {typ} column: {name} ({t}.{c})")
            continue
        findings.append("other statement: " + s[:60].replace("\n", " "))
    if len(tables) > MAX_TABLES:
        findings.append(f"{len(tables)} tables > {MAX_TABLES}")
    for t, n in indexes.items():
        if n > MAX_INDEXES:
            findings.append(f"{t}: {n} indexes > {MAX_INDEXES}")
    print(f"tables: {len(tables)}, most indexes on a table: {indexes.most_common(1)}")
    summary = collections.Counter(f.split(":")[0] for f in findings)
    for k, v in summary.items():
        print(f"  {v:4d} x {k}")
    for f in findings:
        if not f.startswith("CREATE INDEX without ASYNC"):
            print("   -", f)


UNSUPPORTED_SQL = {
    "advisory lock": r"pg_(try_)?advisory",
    "FOR SHARE": r"FOR SHARE\b",
    "SKIP LOCKED": r"SKIP LOCKED",
    "NOWAIT": r"\bNOWAIT\b",
    "LISTEN/NOTIFY": r"\b(LISTEN|pg_notify)\b",
    "TRUNCATE": r"\bTRUNCATE\b",
    "temporary table": r"\bTEMP(ORARY)?\s+TABLE",
    "SAVEPOINT": r"\bSAVEPOINT\b",
    "READ COMMITTED": r"ReadCommitted|READ COMMITTED",
}


def scan_queries():
    files = glob.glob("queries/postgres/**/*.sql", recursive=True)
    files += glob.glob("internal/adapter/**/*.go", recursive=True)
    for label, pat in UNSUPPORTED_SQL.items():
        hits = []
        for p in sorted(files):
            if p.endswith((".sql.go", "_test.go")) or ("postgres" not in p):
                continue
            for i, line in enumerate(open(p), 1):
                if re.search(pat, line) and not line.lstrip().startswith(("--", "//")):
                    hits.append(f"{p}:{i}")
        print(f"{label}: {len(hits)}")
        for h in hits:
            print("   -", h)


if __name__ == "__main__":
    scan_schema()
    scan_queries()
