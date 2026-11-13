# visual: index

The visual role settles the information design of the catalog page, its components and its screens in grey
wireframes, then their colour and density (ADR-0169 §2). Read this page, then `decisions.md` and `exemplars.md`.

- Grey first: the order, the grouping and the size of what is shown, with greys only, before any colour.
- Three widths: mobile (375 px), tablet (768 px) and PC (1280 px); `scripts/task.sh web:shot` takes them.
- Options before building a change of information design or look: 2 or 3 variants of one `<Options>` that differ
  in structure, then the decider's pick (ADR-0169 §9). A recorded decision or a note of the decider settles the
  rest without options.
- A pick becomes a line of `.claude/picks/visual.jsonl`, which the agreement check replays and you never read, and
  a line of `exemplars.md` only when it taught a lesson the file does not hold, citing the pick ids.
- A correction of the decider becomes a line of `decisions.md`; the same correction a second time, a proposal to
  turn the line into a check (ADR-0169 §4). Add, merge or delete lines; never rewrite a file.
