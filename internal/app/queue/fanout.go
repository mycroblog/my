// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"errors"
	"strings"
)

// Sizes of a fan-out. docs/architecture/job-system.md says how FanOutSize was chosen against the
// lease of a job.
const (
	// FanOutSize is how many keys one job of a fan-out covers at most.
	FanOutSize = 1000
	// FanOutRanges is how many ranges one dispatching job enqueues, with the job that goes on from
	// there, so that dispatching stays short however many keys there are.
	FanOutRanges = 100
	// FanOutBatch is how many keys one transaction of a fan-out job covers.
	FanOutBatch = 100
)

// KeyRange is a part of an ordered walk: the keys after After, up to and including Until; "" for no
// bound on that side.
type KeyRange struct {
	After string `json:"after,omitempty"`
	Until string `json:"until,omitempty"`
}

// FanOutStep says what one job of a fan-out does, embedded in the payload of its type. The zero
// value is the first job. With a Range it handles the keys of that range; otherwise it dispatches
// from Cursor.
type FanOutStep struct {
	Cursor string    `json:"cursor,omitempty"`
	Range  *KeyRange `json:"range,omitempty"`
}

// Root reports whether this is the first job of the fan-out, enqueued by what started it.
func (s FanOutStep) Root() bool { return s.Cursor == "" && s.Range == nil }

// StepKey returns the idempotency key of the job of step in the fan-out whose first job has the key
// base, so that a dispatching job run again enqueues nothing that is still pending.
func StepKey(base string, step FanOutStep) string {
	switch {
	case step.Range != nil:
		return base + ":range:" + step.Range.After
	case step.Cursor != "":
		return base + ":cursor:" + step.Cursor
	}
	return base
}

// KeyPage reads a page of the keys of a walk in key order, starting after the cursor after ("" for
// the first). next is the cursor of the following page, "" at the end. A page may hold fewer keys
// than limit, or none, when it leaves some out, and it still advances the walk.
type KeyPage func(ctx context.Context, after string, limit int) (keys []string, next string, err error)

// ErrFanOutPage reports a page that does not advance, which would walk forever.
var ErrFanOutPage = errors.New("queue: a page of a fan-out returned its own cursor")

// FanOut splits a walk over more keys than one job should handle into jobs of up to FanOutSize keys
// each, so that a job that runs out of its lease repeats only its own part. Handling a key twice
// must be safe: a range reaches past its end when keys were removed meanwhile. A key added meanwhile
// falls in the range of its place, or in the last range, which has no end.
type FanOut struct {
	Page KeyPage
	// Descending says that the cursors of Page go down, as for the newest first.
	Descending bool
	// Size is the keys of one job (FanOutSize when 0), Batch the keys of one call of Handle
	// (FanOutBatch when 0), and Ranges the ranges of one dispatching job (FanOutRanges when 0).
	Size, Batch, Ranges int
	// Handle handles a batch of the keys.
	Handle func(ctx context.Context, keys []string) error
	// Enqueue enqueues the jobs of steps in one transaction.
	Enqueue func(ctx context.Context, steps []FanOutStep) error
}

// Run runs the job of step. The first job handles all the keys itself when they fit in one job.
func (f FanOut) Run(ctx context.Context, step FanOutStep) error {
	if step.Range != nil {
		return f.Walk(ctx, *step.Range)
	}
	ranges, next, err := f.split(ctx, step.Cursor)
	if err != nil {
		return err
	}
	if step.Root() && next == "" && len(ranges) == 1 {
		return f.Walk(ctx, ranges[0])
	}
	return f.dispatch(ctx, ranges, next)
}

// Dispatch runs a job known to have more keys than fit in one: it enqueues the jobs from step without
// handling any key itself, unless step is a range.
func (f FanOut) Dispatch(ctx context.Context, step FanOutStep) error {
	if step.Range != nil {
		return f.Walk(ctx, *step.Range)
	}
	ranges, next, err := f.split(ctx, step.Cursor)
	if err != nil {
		return err
	}
	return f.dispatch(ctx, ranges, next)
}

// dispatch enqueues the jobs of ranges, and the job that dispatches from next unless it is "".
func (f FanOut) dispatch(ctx context.Context, ranges []KeyRange, next string) error {
	steps := make([]FanOutStep, 0, len(ranges)+1)
	for i := range ranges {
		steps = append(steps, FanOutStep{Range: &ranges[i]})
	}
	if next != "" {
		steps = append(steps, FanOutStep{Cursor: next})
	}
	return f.Enqueue(ctx, steps)
}

// split reads the boundaries of up to Ranges ranges starting after cursor, and returns the cursor to
// go on from, "" when the last range reaches the end.
func (f FanOut) split(ctx context.Context, cursor string) ([]KeyRange, string, error) {
	size, count := f.Size, f.Ranges
	if size <= 0 {
		size = FanOutSize
	}
	if count <= 0 {
		count = FanOutRanges
	}
	var ranges []KeyRange
	after := cursor
	for len(ranges) < count {
		_, next, err := f.Page(ctx, after, size)
		if err != nil {
			return nil, "", err
		}
		if next != "" && next == after {
			return nil, "", ErrFanOutPage
		}
		ranges = append(ranges, KeyRange{After: after, Until: next})
		if next == "" {
			return ranges, "", nil
		}
		after = next
	}
	return ranges, after, nil
}

// Walk handles the keys of r, a batch at a time.
func (f FanOut) Walk(ctx context.Context, r KeyRange) error {
	batch := f.Batch
	if batch <= 0 {
		batch = FanOutBatch
	}
	if f.Size > 0 {
		batch = min(batch, f.Size)
	}
	after := r.After
	for {
		keys, next, err := f.Page(ctx, after, batch)
		if err != nil {
			return err
		}
		if next != "" && next == after {
			return ErrFanOutPage
		}
		if len(keys) > 0 {
			if err := f.Handle(ctx, keys); err != nil {
				return err
			}
		}
		if next == "" || f.reached(next, r.Until) {
			return nil
		}
		after = next
	}
}

// reached reports whether the cursor next is at or past the end of a range.
func (f FanOut) reached(next, until string) bool {
	if until == "" {
		return false
	}
	c := strings.Compare(next, until)
	if f.Descending {
		return c <= 0
	}
	return c >= 0
}

// PageByLastKey makes a KeyPage from a read that returns up to limit keys in ascending order, whose
// last key is the cursor of the next page.
func PageByLastKey(read func(ctx context.Context, after string, limit int) ([]string, error)) KeyPage {
	return func(ctx context.Context, after string, limit int) ([]string, string, error) {
		keys, err := read(ctx, after, limit)
		if err != nil || len(keys) < limit {
			return keys, "", err
		}
		return keys, keys[len(keys)-1], nil
	}
}
