"""Writes the blind input of the LLM judge: the cases of labelled.json without their labels, in a shuffled order."""
import json, random
cases = json.load(open("labelled.json"))
random.Random(2907).shuffle(cases)
blind = [{"id": i, "where": c["description"].split(" Read by")[0], "control": {"en": c["en"]["control"], "ja": c["ja"]["control"]},
          "helper": {"en": c["en"]["text"], "ja": c["ja"]["text"]}} for i, c in enumerate(cases)]
json.dump(blind, open("judge_input.json", "w"), ensure_ascii=False, indent=1)
json.dump({i: c["key"] for i, c in enumerate(cases)}, open("judge_ids.json", "w"), indent=1)
