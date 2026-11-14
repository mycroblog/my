# visual: index

The visual role settles the information design of the tokens, the components and the screens, in their stories, in grey
wireframes, then their colour and density (ADR-0169 §2). Read this page, then the areas your work touches, each
`decisions/<area>.md` and `exemplars/<area>.md`:

- `judging`: always; how options are offered and judged, and at which widths.
- `color`: a color, a token, a surface, light or dark.
- `layout`: density, spacing, separation, the room of a width.
- `components`: icons, a post's header and actions, menus, any component.
- `composer`: the post composer, its placement, menus, poll and how it is left (decisions only).

- Grey first: the order, the grouping and the size of what is shown, with greys only, before any colour.
- Three widths: mobile (375 px), tablet (768 px) and PC (1280 px); `scripts/task.sh web:shot` takes them.
- Options before building a change of information design or look: 2 or 3 variants of one `<Options>` that differ
  in structure, then the decider's pick (ADR-0169 §9). A recorded decision or a note of the decider settles the
  rest without options.
- A pick becomes a line of `.claude/picks/visual.jsonl`, which the agreement check replays and you never read, and
  a line of an exemplars file only when it taught a lesson the files do not hold, citing the pick ids.
- A correction of the decider becomes a line of a decisions file; the same correction a second time, a proposal to
  turn the line into a check (ADR-0169 §4). A line goes in the area it is about, or a new area when none fits. Add,
  merge or delete lines; never rewrite a file.
