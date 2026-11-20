---
name: review-sessions
description: Review the sessions since the last review through the four builder roles (ADR-0169 §4, §6) - what the decider said again, chose among options, or judged unlike a reviewer - and propose, by one pull request, new decision lines for each role's knowledge and the promotion of repeated ones to a check. Use when the decider asks for the review of sessions, the retrospective, or to learn from the recent sessions.
---

# Review the sessions

Run it only when the decider asks; nothing runs it on a schedule.

1. `scripts/review-sessions.sh digest` writes the signals since the last run to `.tmp/review-sessions/run` of the
   main checkout, one file per session and `picks.md`, and prints their counts and paths. When it says there is
   nothing to read, run `mark`, report that, and stop. The digest quotes the decider: it never leaves `.tmp`.
2. Split the files into parts of at most about 100 KB (a larger file by line ranges) and start one fresh
   `general-purpose` reader per part, up to four at once, each writing its answer to a file of the scratchpad you
   name (a subagent's report may not reach you). Give each its paths, the knowledge of the four builder
   roles (`.claude/knowledge/visual`, `motion`, `copy`, and for implementation `ts-developer`, `go-developer` and
   their test roles), the line format of each decisions file's header, and this task: for each role, find where the
   decider corrected or said again what an agent did, chose among options, or judged unlike a reviewer (the
   evaluator, the walker, a review pass; `picks.md` marks a pick the evaluator differs from), and return
   - a new line in that role's format, its area file named, for a correction or choice no line holds;
   - for a line that holds it already: the line, and whether this is the same correction again (missed +1, and a
     proposal to turn it into a check) or work that followed it unasked (held +1);
   - a disagreement with a reviewer: the line for the role at fault, the evaluator's or walker's knowledge included.
   Each cites its signals as `session <first 8 of the id> <date>`, a note or pick id, or an issue or PR number,
   and paraphrases in English: never the decider's words, never a decision the decider has not taken. A decision
   an Accepted ADR already holds, a question, or an instruction for one task is no line.
3. Merge the readers' answers: one line per decision, the same decision from several sessions cited once with all
   its sources. A second correction of a recorded line is a promotion: the copy role's becomes a word line of
   `decisions.md` (a rule of the copy lint) when its words can be matched; any other a check or tool, written as the
   draft of an issue (the body of `/new-issue`'s task) in the pull request.
4. When nothing is left to propose, run `mark`, report that, and stop: open nothing.
5. In a worktree on `review/sessions-<YYYY-MM-DD>` from `origin/main`, add the lines and change the counts, adding,
   merging or deleting lines and never rewriting a file (ADR-0169 §4); a file over its limit is consolidated in the
   same way. `scripts/agentkit.sh pr open` with a body that lists, per role, each line added or changed with its
   sources, then each proposed check with its draft issue, and says the decider accepts or drops each. Then `mark`.

Report the pull request, the counts of proposals by role, and the point the next run reads from.
