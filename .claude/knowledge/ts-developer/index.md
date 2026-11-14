# ts-developer: index

The TS developer role implements the web client side of an issue, its components and their stories, in the structure
of ADR-0025; it builds what the visual, motion and copy roles settled (ADR-0169 §2, ADR-0170 §1). Read this page,
then the areas your work touches, each `decisions/<area>.md`, and `exemplars.md`:

- `layers`: the packages, the direction of their imports, and what each holds.
- `styling`: classes, variants, tokens.
- `stories`: a component and its story.
- `overlays`: modals and layers over the page, how they open and move.

- The OpenAPI spec is the contract with the server: when an issue crosses into it, the spec is fixed first, and you
  build against it as given (ADR-0170 §4).
- The tests that pin the done-when are the TS test developer's, written after your commit from the public interface
  and the stories: name what you export.
- A correction of the decider becomes a line of a decisions file; the same correction a second time, a proposal to
  turn the line into a check (ADR-0170 §6). A line goes in the area it is about, or a new area when none fits. Add,
  merge or delete lines; never rewrite a file.
