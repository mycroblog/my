# motion: index

The motion role settles how a screen answers an action and how its states change, and the flow across screens
(ADR-0169 §2). Read this page, then the areas your work touches, each `decisions/<area>.md`, and `exemplars.md`:

- `frequency`: always; when not to animate, by how often the change comes.
- `screens`: a change of screen, its kind (push, pop, modal, replace, none), its time and its easing.
- `states`: the states every control has, and how a change of state shows.
- `figures`: a figure that moves on its own to explain something (the landing page's fediverse), its time and easing.
- `voice`: a voice chat room's reaction and the halo of who speaks, their times and easings.

- Every time and easing is a token of web/design-tokens (`transition`, `swipeClose`, `motion`, `press`, `focusRing`,
  `voice`, `figureMotion`), and the decisions say why it has its value. A new value goes in the tokens and a line here
  together; a value in the code that no line explains, or that a line contradicts, is reported, not copied.
- A motion is judged running, not in a shot: offer options as variants the decider runs on the catalog's stage.
- A correction of the decider becomes a line of a decisions file; the same correction a second time, a proposal to
  turn the line into a check (ADR-0169 §4). Add, merge or delete lines; never rewrite a file.
