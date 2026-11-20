"""Deterministic measures of spike #2907, run on labelled.json (and, with --catalogs, on the current catalogs).
overlap:  the share of the control's content words (en) or character bigrams (ja) that the helper says again,
          and how many content words the helper adds that are not the control's.
no-fact:  the helper has no marker of a condition, a limit, a negation, a number or a consequence.
Each measure flags a helper as redundant; the script prints precision and recall against the labels."""
import json, re, sys
STOP = set("""a an the of to in on for and or by with is are be it its this that these those as at from
their them they you your can may what which who whom whose here there one any all""".split())
def stem(w): return re.sub(r"(ies|es|s|ed|ing)$", "", w) if len(w) > 4 else w
def words(t): return {stem(w) for w in re.findall(r"[a-z0-9']+", (t or "").lower()) if w not in STOP}
def bigrams(t):
    t = re.sub(r"[\s、。・()（）「」,.:]+", "", t or "")
    t = re.sub(r"[ぁ-ゖ]", "", t)  # kana particles carry no content
    return {t[i:i + 2] for i in range(len(t) - 1)}
FACT_EN = re.compile(r"\d|\b(if|when|while|unless|until|only|never|always|not|no|none|cannot|can't|nobody|nothing|"
                     r"after|before|still|already|once|even|either|unless|instead|despite|keeps?|refused|deleted|"
                     r"days?|months?|hours?|limit|most|least|up to)\b", re.I)
FACT_JA = re.compile(r"\d|とき|場合|だけ|しか|ない|ません|以外|まで|後|前|ても|のみ|常に|すでに|まだ|日|月|時間|上限|最大|最小")
def overlap(c, loc):
    h, l = c[loc]["text"], " ".join(x for x in (c[loc]["control"], c[loc]["heading"]) if x)
    if loc == "en":
        H, L = words(h), words(l)
    else:
        H, L = bigrams(h), bigrams(l)
    contain = len(H & L) / len(L) if L else 0.0
    novel = len(H - L)
    return contain, novel
def nofact(c, loc):
    return not (FACT_EN if loc == "en" else FACT_JA).search(c[loc]["text"] or "")
MEASURES = {
    "overlap>=0.5":      lambda c, loc: overlap(c, loc)[0] >= 0.5,
    "novel<=3":          lambda c, loc: overlap(c, loc)[1] <= (3 if loc == "en" else 8),
    "no-fact":           nofact,
    "no-fact|overlap":   lambda c, loc: nofact(c, loc) or overlap(c, loc)[0] >= 0.5,
    "no-fact&novel<=6":  lambda c, loc: nofact(c, loc) and overlap(c, loc)[1] <= (6 if loc == "en" else 16),
}
def score(cases, pred, loc):
    tp = fp = fn = tn = 0
    for c in cases:
        p, r = pred(c, loc), c["label"] == "R"
        tp += p and r; fp += p and not r; fn += r and not p; tn += not p and not r
    prec = tp / (tp + fp) if tp + fp else 0; rec = tp / (tp + fn) if tp + fn else 0
    return tp, fp, fn, tn, prec, rec
if __name__ == "__main__":
    cases = json.load(open("labelled.json"))
    for loc in ("en", "ja"):
        print(f"== {loc} ({len(cases)} cases, {sum(c['label']=='R' for c in cases)} R)")
        for name, m in MEASURES.items():
            tp, fp, fn, tn, p, r = score(cases, m, loc)
            print(f"  {name:18} tp={tp:2} fp={fp:2} fn={fn:2} tn={tn:2}  precision={p:.2f} recall={r:.2f}")
    if "-v" in sys.argv:
        for c in cases:
            o = overlap(c, "en")
            print(c["label"], f"{o[0]:.2f} {o[1]:2}", "NF" if nofact(c, "en") else "  ", c["en"]["control"], "|", c["en"]["text"])
