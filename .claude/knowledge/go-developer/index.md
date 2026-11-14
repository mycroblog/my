# go-developer: index

The Go developer role implements the server side of an issue (ADR-0170 §1). Read this page, then the areas your work
touches, each `decisions/<area>.md`, and `exemplars.md`:

- `layers`: the layers, the modules and how they reach each other.
- `schema`: a table, a column, an index, or a change of their data.

- The OpenAPI spec is the contract with the web client: when an issue crosses into it, the spec is fixed first, and
  you implement it as given (ADR-0170 §4).
- The tests that pin the done-when are the Go test developer's, written after your commit from the public interface:
  name what you export, and keep it documented.
- A correction of the decider becomes a line of a decisions file; the same correction a second time, a proposal to
  turn the line into a check (ADR-0170 §6). A line goes in the area it is about, or a new area when none fits. Add,
  merge or delete lines; never rewrite a file.
