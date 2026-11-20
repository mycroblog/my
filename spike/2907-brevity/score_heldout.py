"""Scores the judge's verdicts on the held-out sentences, and the deterministic measures on them."""
import json, sys
from heuristics import nofact, overlap
cases = {c["key"]: c for c in json.load(open("heldout.json"))}
ids = json.load(open("heldout_ids.json"))
def report(name, pred):
    tp = fp = fn = tn = 0; wrong = []
    for i, k in ids.items():
        c = cases[k]; p = pred(i, c); r = c["label"] == "R"
        tp += p and r; fp += p and not r; fn += r and not p; tn += not p and not r
        if p != r: wrong.append(("FP" if p else "FN", c["sentence"]["en"]))
    print(f"{name}: tp={tp} fp={fp} fn={fn} tn={tn} precision={tp/(tp+fp) if tp+fp else 0:.2f} recall={tp/(tp+fn):.2f}")
    for w in wrong: print("   ", *w)
as_case = lambda c, loc: {loc: {"text": c["sentence"][loc], "control": c["control"][loc], "heading": None}}
report("no-fact (en)", lambda i, c: nofact(as_case(c, "en"), "en"))
report("no-fact|overlap (en)", lambda i, c: nofact(as_case(c, "en"), "en") or overlap(as_case(c, "en"), "en")[0] >= 0.5)
for f in sys.argv[1:]:
    v = {str(x["id"]): x["verdict"] for x in json.load(open(f))}
    report(f, lambda i, c: v[i] == "R")
