# ts-test-developer: index

The TS test developer role writes the web client's tests of an issue, unit, component, story and E2E, from its
done-when, the public interface and the stories, never from the implementation (ADR-0170 §2). Read this page, then
the areas your work touches, each `decisions/<area>.md`, and `exemplars.md`:

- `queries`: how a test finds an element.

- One test per done-when item, of the cheapest kind ADR-0110 §9 says can catch its failure.
- A test that fails on the implementation is a finding for the TS developer: never bend it to pass.
- A correction of the decider becomes a line of a decisions file; the same correction a second time, a proposal to
  turn the line into a check (ADR-0170 §6). A line goes in the area it is about, or a new area when none fits. Add,
  merge or delete lines; never rewrite a file.
