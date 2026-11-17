---
name: visual-agreement
description: Check how often the visual evaluator picks what the decider picked among live options (ADR-0169 §7), by replaying the recorded picks with the outcome hidden. Use when the decider asks whether the evaluator agrees with them, or asks for the agreement check.
---

# Visual agreement

Run it only when the decider asks; it is not a gate.

1. `scripts/visual-agreement.sh scenes` lists each recorded pick with its options in name order and the directory of
   its shots, the outcome hidden. With no pick recorded it says so: report that and stop.
2. For each scene, start a fresh `visual-evaluator` with the shots of every variant and the options' names. Never
   pass the outcome, the record of the pick in `.claude/picks/visual.jsonl`, or your guess. Run up to four at
   once. A scene whose shots are missing is skipped and reported.
3. `scripts/visual-agreement.sh score <id>=<variant> ...` with the variant each evaluator named on its first line.

Report the share it prints, and for each miss the evaluator's reason next to the decider's. A miss is a signal for
the knowledge of the visual or the evaluator role (ADR-0169 §4): propose the line, the decider decides.
