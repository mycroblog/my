"""Builds the labelled cases of spike #2907 from the catalogs as they were before three reviews that removed
helper texts as redundant ("Explanations that only restate their label are gone", 0712ccfa5; "the color tokens'
lines that restated their names are gone", c4a7a717c; "Hints that repeat their labels ... are gone", 897150d69).
A helper the review removed is R (redundant); one it read and kept is F (fine). Writes labelled.json."""
import json, subprocess, sys
REPO = sys.argv[1] if len(sys.argv) > 1 else "."
def show(rev, f):
    return json.loads(subprocess.run(["git", "show", f"{rev}:{f}"], capture_output=True, text=True, cwd=REPO).stdout)
def msg(cat, k): return cat.get(k, {}).get("defaultMessage")
cases = []
def add(commit, path, keys_of, label_of, heading_of=lambda k, c: None):
    before = {l: show(f"{commit}^", path.format(l)) for l in ("en", "ja")}
    after = show(commit, path.format("en"))
    for k in keys_of(before["en"]):
        lk = label_of(k)
        cases.append({
            "key": k, "commit": commit, "label": "R" if k not in after else "F",
            "en": {"text": msg(before["en"], k), "control": msg(before["en"], lk), "heading": heading_of(k, before["en"])},
            "ja": {"text": msg(before["ja"], k), "control": msg(before["ja"], lk), "heading": heading_of(k, before["ja"])},
            "description": before["en"][k].get("description", ""),
        })
add("0712ccfa5", "web/app-features/locales/{}/staff_settings.json",
    lambda c: [k for k in c if k.startswith("staff_settings.setting.") and k.endswith(".about")],
    lambda k: k[: -len(".about")])
add("c4a7a717c", "web/app-features/locales/{}/staff_theme.json",
    lambda c: [k for k in c if k.startswith("staff_theme.token.") and k.endswith(".about")],
    lambda k: k[: -len(".about")])
add("897150d69", "web/app-features/locales/{}/staff_domains.json",
    lambda c: ["staff_domains.form.public_comment.hint", "staff_domains.form.private_note.hint"],
    lambda k: k[: -len(".hint")])
json.dump(cases, open("labelled.json", "w"), ensure_ascii=False, indent=1)
print(len(cases), "cases:", sum(c["label"] == "R" for c in cases), "R,", sum(c["label"] == "F" for c in cases), "F")
