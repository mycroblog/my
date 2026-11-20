"""Scores the LLM judge's verdicts against labelled.json."""
import json, sys
cases = {c["key"]: c for c in json.load(open("labelled.json"))}
ids = json.load(open("judge_ids.json"))
for f in sys.argv[1:]:
    v = {str(x["id"]): x for x in json.load(open(f))}
    tp = fp = fn = tn = 0; wrong = []
    for i, k in ids.items():
        p = v[i]["verdict"] == "R"; r = cases[k]["label"] == "R"
        tp += p and r; fp += p and not r; fn += r and not p; tn += not p and not r
        if p != r: wrong.append(("FP" if p else "FN", cases[k]["en"]["control"], cases[k]["en"]["text"], v[i].get("reason")))
    print(f"{f}: tp={tp} fp={fp} fn={fn} tn={tn} precision={tp/(tp+fp):.2f} recall={tp/(tp+fn):.2f} agreement={(tp+tn)/len(ids):.2f}")
    if "-v" in os_args if (os_args := sys.argv) else []: pass
    for w in wrong: print("   ", *w, sep=" | ")
