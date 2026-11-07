# SPDX-License-Identifier: AGPL-3.0-only
"""Replays merged pull requests against hypothetical package layouts and reports which Go tests the Go test cache
would run again (spike of #2454; never merged).

  python3 spike/testsplit/replay.py FILEGRAPH ALLTESTS PRS APPGRAPH

FILEGRAPH is the output of spike/testsplit (main.go), ALLTESTS `go test -json ./...`, PRS
`gh pr list --state merged --json number,files`, APPGRAPH the module DAG {module: [modules it imports]}.
"""
import collections
import json
import re
import statistics
import sys

MOD = "github.com/mycroblog/my/"
files = {f["path"]: f for f in json.load(open(sys.argv[1]))}
prs = json.load(open(sys.argv[3]))
app = {k: set(v) for k, v in json.load(open(sys.argv[4])).items()}

# --- module DAG ------------------------------------------------------------------------------------------------------
def closure(m, seen=None):
    seen = set() if seen is None else seen
    for d in app.get(m, ()):
        if d not in seen:
            seen.add(d)
            closure(d, seen)
    return seen
CLO = {m: closure(m) for m in app}
MODULES = set(app)

def top(mods):
    """The module of a set: the one that depends on all the others, 'x' when none does, 'base' when empty."""
    mods = set(mods)
    if not mods:
        return "base"
    for t in mods:
        if mods - {t} <= CLO[t]:
            return t
    return "x"

def direct_modules(deps):
    out = set()
    for d in deps:
        m = re.match(r"internal/(?:app|domain)/([a-z]+)/", d)
        if m and m.group(1) in MODULES:
            out.add(m.group(1))
    return out

# --- test costs ------------------------------------------------------------------------------------------------------
cost = collections.defaultdict(float)  # (pkg, Test) -> seconds
for line in open(sys.argv[2]):
    try:
        e = json.loads(line)
    except ValueError:
        continue
    if e.get("Action") in ("pass", "fail") and e.get("Test") and "/" not in e["Test"]:
        cost[(e["Package"], e["Test"])] = e.get("Elapsed", 0.0)

def dirof(path):
    return path.rsplit("/", 1)[0] if "/" in path else "."

def pkgof(path):
    f = files.get(path)
    return f["pkg"][len(MOD):] if f and f["pkg"].startswith(MOD) else (f["pkg"] if f else dirof(path))

AGG_DIRS = {"internal/adapter/db/sqlite", "internal/adapter/db/postgres", "internal/adapter/db/sqlite/sqlitegen",
            "internal/adapter/db/postgres/postgresgen", "pkg/testkit"}

def module_of_file(path):
    f = files.get(path)
    if f is None:
        m = re.match(r"schema/(?:sqlite|postgres)/\d+-([a-z]+)\.sql$", path)
        if m:
            name = {"supporter": "supporters"}.get(m.group(1), m.group(1))
            return name if name in MODULES else "base"
        return None
    d = dirof(path)
    if d.endswith(("sqlitegen", "postgresgen")) or d.startswith("internal/adapter/db/") and path.endswith("_test.go") is False and False:
        pass
    if d.endswith(("sqlitegen", "postgresgen")):
        # Generated code refers to no module; its query file names one (queries/<dialect>/<name>.sql).
        base = path.rsplit("/", 1)[1].split(".")[0]
        for m in sorted(MODULES, key=len, reverse=True):
            if base.startswith(m) or base.startswith(m.rstrip("s")):
                return m
        return "base"
    return top(direct_modules(f["deps"] or []))

# --- layouts ---------------------------------------------------------------------------------------------------------
def make_layout(split_db, fix_upward, split_platform):
    """Returns (group(path) for non-test files, test units [(unit id, cost, deps, own files)])."""
    mod_cache = {}

    def mod(path):
        if path not in mod_cache:
            mod_cache[path] = module_of_file(path)
        return mod_cache[path]

    def group(path):
        d = dirof(path)
        if path.startswith("schema/") and path.endswith(".sql"):
            return ("schema", mod(path)) if split_db else ("schema", "")
        if split_db and (d in AGG_DIRS or d == "schema"):
            if d == "schema":
                return ("schema", "base")
            return (d, mod(path))
        return (pkgof(path), "")

    units = []
    by_pkg = collections.defaultdict(list)
    for p, f in files.items():
        if f["test"]:
            by_pkg[f["pkg"]].append(f)
    for pkg, tfs in by_pkg.items():
        short = pkg[len(MOD):] if pkg.startswith(MOD) else pkg
        d = dirof(tfs[0]["path"])
        split_here = (split_db and d in AGG_DIRS) or (split_platform and d == "internal/platform")
        if not split_here:
            deps = set()
            own = set()
            for f in tfs:
                deps |= set(f["deps"] or [])
                own.add(f["path"])
            c = sum(cost.get((pkg, fn), 0.0) for f in tfs for fn in (f.get("testFns") or []))
            units.append(((short, ""), c, deps, own, (short, "")))
            continue
        per = collections.defaultdict(lambda: [0.0, set(), set()])
        helpers = set()
        for f in tfs:
            for fn in f.get("testFns") or []:
                fdeps = set((f.get("fnDeps") or {}).get(fn) or [])
                m = top({mod(x) for x in fdeps if dirof(x) in AGG_DIRS and mod(x) not in (None, "base", "x")} | direct_modules(fdeps))
                u = per[m]
                u[0] += cost.get((pkg, fn), 0.0)
                u[1] |= fdeps
                u[2].add(f["path"])
        for m, (c, deps, own) in per.items():
            units.append(((short, m), c, deps, own, (d, m) if split_db and d in AGG_DIRS else (short, "")))
    return group, units

def evaluate(name, split_db=False, fix_upward=False, split_platform=False):
    group, units = make_layout(split_db, fix_upward, split_platform)
    # group graph
    gdeps = collections.defaultdict(set)
    for p, f in files.items():
        if f["test"]:
            continue
        g = group(p)
        for d in f["deps"] or []:
            h = group(d)
            if h == g:
                continue
            if fix_upward and g[1] not in ("", "base", "x") and h[0] == g[0] and h[1] not in ("", "base", "x") \
                    and h[1] not in CLO.get(g[1], set()):
                continue  # a lower module's file using a higher module's helper: assumed moved to base
            gdeps[g].add(h)
    memo = {}

    def reach(g):
        if g in memo:
            return memo[g]
        memo[g] = {g}
        out = {g}
        for h in gdeps.get(g, ()):
            out |= reach(h)
        memo[g] = out
        return out

    unit_reach = []
    for uid, c, deps, own, home in units:
        r = set(reach(home))
        for d in deps:
            h = group(d)
            if fix_upward and uid[1] not in ("", "base", "x") and h[1] not in ("", "base", "x") \
                    and h[0] in AGG_DIRS and h[1] not in CLO.get(uid[1], set()) and h[1] != uid[1]:
                continue
            r |= reach(h)
        unit_reach.append((uid, c, r, own))
    total = sum(c for _, c, _, _ in unit_reach)
    results = []
    for pr in prs:
        changed = [x["path"] for x in pr["files"]]
        if not any(p.endswith((".go", ".sql")) or p in ("go.mod", "go.sum") for p in changed):
            continue
        if any(p in ("go.mod", "go.sum") for p in changed):
            results.append(1.0)
            continue
        cg = set()
        for p in changed:
            if p.endswith(".go") or p.endswith(".sql") or p in files:
                g = group(p)
                if split_db and p.endswith("models.go"):
                    # Per-module generated models: the modules whose queries or schema the PR changed.
                    ms = {module_of_file(q) for q in changed if q.endswith((".sql", ".sql.go"))} - {None}
                    for m in ms or MODULES:
                        cg.add((g[0], m))
                    continue
                cg.add(g)
        spent = sum(c for _, c, r, own in unit_reach if (r & cg) or (own & set(changed)))
        results.append(spent / total if total else 0)
    results.sort()
    n = len(results)
    print(f"{name:52} PRs {n:3}  mean {100*statistics.mean(results):5.1f}%  median {100*statistics.median(results):5.1f}%"
          f"  p75 {100*results[3*n//4]:5.1f}%  none-run {sum(1 for r in results if r < 0.01):3}  total {total:6.0f} s")

evaluate("H0  today's packages")
evaluate("H1  DB aggregates split by module", split_db=True)
evaluate("H1' + lower modules no longer use higher ones' helpers", split_db=True, fix_upward=True)
evaluate("H2  + internal/platform tests split by module", split_db=True, fix_upward=True, split_platform=True)


def ideal():
    """The floor that the module DAG sets: every test of module m runs when, and only when, m or a module m imports
    changes (a cross-module test, 'x' or 'base', whenever any module changes); the rest of the tests as in H0."""
    group, units = make_layout(True, True, True)
    total = sum(c for _, c, _, _, _ in units)
    res = []
    for pr in prs:
        changed = [x["path"] for x in pr["files"] if x["path"].endswith((".go", ".sql"))]
        if not changed:
            continue
        cm = set()
        other = False
        for p in changed:
            m = re.match(r"internal/(?:app|domain)/([a-z]+)/", p)
            mf = module_of_file(p) if dirof(p) in AGG_DIRS or p.startswith("schema/") else None
            if m and m.group(1) in MODULES:
                cm.add(m.group(1))
            elif mf and mf in MODULES:
                cm.add(mf)
            else:
                other = True
        spent = 0
        for uid, c, deps, own, home in units:
            um = uid[1]
            if um in MODULES:
                if cm & ({um} | CLO[um]):
                    spent += c
            elif cm or other:
                spent += c
        res.append(spent / total)
    res.sort()
    n = len(res)
    print(f"{'floor of the module DAG (other paths run everything)':52} PRs {n:3}  mean {100*statistics.mean(res):5.1f}%  median {100*statistics.median(res):5.1f}%")


ideal()


def diagnose(split_db=True, fix_upward=True, split_platform=True, n=15):
    """The test units that cost the most across the replayed PRs, with how often they run."""
    group, units = make_layout(split_db, fix_upward, split_platform)
    gdeps = collections.defaultdict(set)
    for p, f in files.items():
        if f["test"]:
            continue
        g = group(p)
        for d in f["deps"] or []:
            h = group(d)
            if h != g:
                gdeps[g].add(h)
    memo = {}

    def reach(g):
        if g not in memo:
            memo[g] = {g}
            out = {g}
            for h in gdeps.get(g, ()):
                out |= reach(h)
            memo[g] = out
        return memo[g]

    rows = []
    gopr = [pr for pr in prs if any(x["path"].endswith((".go", ".sql")) for x in pr["files"])]
    for uid, c, deps, own, home in units:
        r = set(reach(home))
        for d in deps:
            r |= reach(group(d))
        hits = 0
        for pr in gopr:
            cg = {group(x["path"]) for x in pr["files"] if x["path"].endswith((".go", ".sql"))}
            if r & cg:
                hits += 1
        rows.append((c * hits / len(gopr), c, hits, uid, len(r)))
    rows.sort(reverse=True)
    for exp, c, hits, uid, nr in rows[:n]:
        print(f"  expected {exp:6.1f} s  cost {c:6.1f} s  runs in {hits:2}/{len(gopr)} PRs  reaches {nr:4} groups  {uid}")


if len(sys.argv) > 5 and sys.argv[5] == "diagnose":
    diagnose()
