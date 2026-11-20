"""Runs the deterministic measures on the current en and ja catalogs: every helper (a key ending in about, hint or
help) whose control (the key without that ending) exists. Prints the counts flagged and the flagged strings."""
import glob, json, re, sys
from heuristics import nofact, overlap
root = sys.argv[1]
cat = {}
for loc in ("en", "ja"):
    cat[loc] = {}
    for f in glob.glob(f"{root}/web/*/locales/{loc}/*.json"):
        cat[loc].update({k: v["defaultMessage"] for k, v in json.load(open(f)).items()})
helpers = [k for k in cat["en"] if re.search(r"[._](about|hint|help)$", k) and re.sub(r"[._](about|hint|help)$", "", k) in cat["en"]]
print("strings", len(cat["en"]), "helpers with a control", len(helpers))
chars = sum(len(cat["en"][k]) + len(cat["en"][re.sub(r"[._](about|hint|help)$", "", k)]) for k in helpers)
print("en characters of the helpers and their controls", chars)
flag = {}
for k in helpers:
    ctl = re.sub(r"[._](about|hint|help)$", "", k)
    c = {loc: {"text": cat[loc].get(k), "control": cat[loc].get(ctl), "heading": None} for loc in ("en", "ja")}
    nf, (ov, nov) = nofact(c, "en"), overlap(c, "en")
    if nf and nov <= 6: flag.setdefault("no-fact&novel<=6", []).append((k, c["en"]["control"], c["en"]["text"]))
    if ov >= 0.5: flag.setdefault("overlap>=0.5", []).append((k, c["en"]["control"], c["en"]["text"]))
for m, xs in flag.items():
    print(f"== {m}: {len(xs)}")
    for x in xs: print("  ", *x, sep=" | ")
