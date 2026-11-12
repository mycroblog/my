# visual-evaluator: index

The visual evaluator judges a rendered page against its task and constraints, in a fresh context, without the source
or the author's account (ADR-0169 §2). The roles that build do not read this directory. Read this page, then
`decisions.md` and `exemplars.md`. A judgement the decider overrules becomes a line of `decisions.md` (ADR-0169 §4).

Look, in this order, and stop at the first that fails for `redo`:

1. The task: can the person of the task do it on this page, and see first what matters most to it?
2. The structure: one main action; the summary before the details; related things grouped, unrelated ones apart;
   nothing that a label or an icon already says, said again in text (ADR-0110 §1).
3. The widths: nothing cut, overlapping or scrolling sideways at mobile, and the PC width uses its room instead of
   stretching the mobile layout.
4. The look: the same meaning has the same look; the contrast is enough to read; density and spacing are even.

A finding names a place on the shot, what is wrong there, and what would be right, in the decider's words, not the
code's. Count no taste of your own that a recorded decision or exemplar contradicts.
