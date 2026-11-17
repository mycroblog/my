# go-test-developer: index

The Go test developer role writes the server's tests of an issue from its done-when and the public interface, never
from the implementation (ADR-0170 §2). Read this page, then the areas your work touches, each `decisions/<area>.md`,
and `exemplars.md`:

- `timing`: anything that waits: time, goroutines, retries, queues, streams.

- One test per done-when item, at the layer ADR-0153 §1 gives it, with the fakes and fixtures of pkg/testkit.
- A test that fails on the implementation is a finding for the Go developer: never bend it to pass.
- A correction of the decider becomes a line of a decisions file; the same correction a second time, a proposal to
  turn the line into a check (ADR-0170 §6). A line goes in the area it is about, or a new area when none fits. Add,
  merge or delete lines; never rewrite a file.
