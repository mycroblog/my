"""The held-out cases of spike #2907, one sentence each, from two other reviews (897150d69: "Hints that repeat their
labels and the second sentences of empty states are gone"; a2f0041d2: the report panel's hints that "restated" what
its buttons say). A sentence the review removed is R, one it kept is F. Writes heldout.json and its blind input."""
import json, random, re, subprocess, sys
REPO = sys.argv[1]
def show(rev, f): return json.loads(subprocess.run(["git", "show", f"{rev}:{f}"], capture_output=True, text=True, cwd=REPO).stdout)
def sentences(t, loc):
    return [s for s in (re.split(r"(?<=。)", t) if loc == "ja" else re.split(r"(?<=\.) ", t)) if s.strip()]
SPEC = [  # commit, catalog, key, the control it sits under (key), which sentences the review removed (index)
    ("897150d69", "staff_domains", "staff_domains.actions.empty", None, {1}),
    ("897150d69", "staff_domains", "staff_domains.allows.empty", None, {1}),
    ("897150d69", "staff_domains", "staff_domains.lists.empty", None, {1}),
    ("897150d69", "staff_domains", "staff_domains.drafts.empty", None, {1}),
    ("897150d69", "staff_domains", "staff_domains.servers.empty", None, {1}),
    ("897150d69", "staff_audit", "staff_audit.empty", None, {1}),
    ("897150d69", "staff_domains", "staff_domains.list.form.url.hint", "staff_domains.list.form.url", {0}),
    ("897150d69", "staff_domains", "staff_domains.deliveries.account.hint", "staff_domains.deliveries.account", {0}),
    ("897150d69", "staff_domains", "staff_domains.form.domain.hint", "staff_domains.form.domain", set()),
    ("897150d69", "staff_domains", "staff_domains.servers.open.hint", "staff_domains.servers.open", set()),
    ("897150d69", "staff_domains", "staff_domains.list.stop.about", "staff_domains.list.stop", set()),
    ("897150d69", "staff_domains", "staff_domains.fetches.empty", None, set()),
    ("897150d69", "staff_domains", "staff_domains.availability.empty", None, set()),
    ("897150d69", "staff_domains", "staff_domains.deliveries.empty", None, set()),
    ("a2f0041d2", "report", "report.mute_hint", "report.mute", {0, 1}),
    ("a2f0041d2", "report", "report.block_hint", "report.block", {0}),
    ("a2f0041d2", "report", "report.forward_hint", "report.forward", set()),
    ("a2f0041d2", "report", "report.comment_hint", "report.comment", set()),
]
def path(cat, loc):
    out = subprocess.run(["git", "ls-tree", "-r", "--name-only", "HEAD", "web/"], capture_output=True, text=True, cwd=REPO).stdout
    for p in out.split():
        if p.endswith(f"/locales/{loc}/{cat}.json"): return p
cases = []
for commit, cat, key, control, removed in SPEC:
    cs = {loc: show(f"{commit}^", path(cat, loc)) for loc in ("en", "ja")}
    en, ja = sentences(cs["en"][key]["defaultMessage"], "en"), sentences(cs["ja"][key]["defaultMessage"], "ja")
    for i, s in enumerate(en):
        cases.append({"key": f"{key}#{i}", "label": "R" if i in removed else "F",
                      "where": cs["en"][key]["description"].split(" Read by")[0],
                      "control": {loc: (cs[loc].get(control, {}).get("defaultMessage") if control else None) for loc in ("en", "ja")},
                      "whole": {"en": cs["en"][key]["defaultMessage"], "ja": cs["ja"][key]["defaultMessage"]},
                      "sentence": {"en": s, "ja": ja[i] if i < len(ja) else None}})
json.dump(cases, open("heldout.json", "w"), ensure_ascii=False, indent=1)
random.Random(7).shuffle(cases)
json.dump([{"id": i, "where": c["where"], "control": c["control"], "whole_text": c["whole"], "sentence_to_judge": c["sentence"]}
           for i, c in enumerate(cases)], open("heldout_input.json", "w"), ensure_ascii=False, indent=1)
json.dump({i: c["key"] for i, c in enumerate(cases)}, open("heldout_ids.json", "w"), indent=1)
print(len(cases), sum(c["label"] == "R" for c in cases), "R")
