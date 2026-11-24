// SPDX-License-Identifier: AGPL-3.0-only

package projectiontest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

// ProjectionStoreHarness is a projection.Store on a fresh database with the product schema,
// with the UnitOfWork of the same database.
type ProjectionStoreHarness struct {
	UnitOfWork uow.UnitOfWork
	Store      projection.Store
}

// ProjectionStoreContract is the contract of projection.Store, and of projection.Service on
// it, that every database adapter must pass. It works on poll_tallies, whose columns are text,
// integers, and a nullable integer.
func ProjectionStoreContract(t *testing.T, newHarness func(t *testing.T) ProjectionStoreHarness) {
	t.Helper()
	t.Run("reads, inserts, and deletes rows", func(t *testing.T) {
		t.Parallel()
		projectionStoreRows(t, newHarness(t))
	})
	t.Run("refuses unknown tables and columns", func(t *testing.T) {
		t.Parallel()
		projectionStoreRefuses(t, newHarness(t))
	})
	t.Run("verify reports drift and rebuild corrects it in chunks", func(t *testing.T) {
		t.Parallel()
		projectionServiceDrift(t, newHarness(t))
	})
	t.Run("the verify job runs one projection per job and moves past one that fails", func(t *testing.T) {
		t.Parallel()
		projectionVerifyChain(t, newHarness(t))
	})
}

var tallyColumns = []string{"poll_id", "position", "votes", "voters_count"}

// pollKey is the poll ID for a letter: later letters sort after earlier ones.
func pollKey(letter string) string { return testkit.OrderedID(uint32(letter[0]-'a') + 1) }

// tallyLine is the line readTallies reports for the poll with the letter and the rest of the row.
func tallyLine(letter, rest string) string { return pollKey(letter) + " " + rest }

func tally(letter string, position, votes int64, voters *int64) []any {
	poll := pollKey(letter)
	var v any
	if voters != nil {
		v = *voters
	}
	return []any{poll, position, votes, v}
}

func voters(n int64) *int64 { return &n }

// readTallies returns the rows of poll_tallies, each formatted with fmt.Sprintln, so that the
// integer types of the drivers compare equal.
func readTallies(t *testing.T, h ProjectionStoreHarness, offset, limit int) []string {
	t.Helper()
	var out []string
	err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		out = nil
		cols, rows, err := h.Store.Rows(t.Context(), tx, "poll_tallies", offset, limit)
		if err != nil {
			return err
		}
		if !slices.Equal(cols, tallyColumns) {
			return fmt.Errorf("columns = %v, want %v", cols, tallyColumns)
		}
		for _, r := range rows {
			out = append(out, strings.TrimSpace(fmt.Sprintln(r...)))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Rows = %v", err)
	}
	return out
}

func writeTallies(t *testing.T, h ProjectionStoreHarness, insert bool, rows ...[]any) int64 {
	t.Helper()
	var n int64
	err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		if insert {
			n, err = h.Store.Insert(t.Context(), tx, "poll_tallies", tallyColumns, rows)
		} else {
			n, err = h.Store.Delete(t.Context(), tx, "poll_tallies", tallyColumns, rows)
		}
		return err
	})
	if err != nil {
		t.Fatalf("write = %v", err)
	}
	return n
}

func projectionStoreRows(t *testing.T, h ProjectionStoreHarness) {
	if n := writeTallies(t, h, true, tally("b", 0, 3, nil), tally("a", 1, 2, voters(5)), tally("a", 0, 1, voters(5))); n != 3 {
		t.Fatalf("Insert = %d, want 3", n)
	}
	if n := writeTallies(t, h, true, tally("a", 0, 9, nil)); n != 0 {
		t.Errorf("Insert of a row whose key exists = %d, want 0", n)
	}
	want := []string{tallyLine("a", "0 1 5"), tallyLine("a", "1 2 5"), tallyLine("b", "0 3 <nil>")}
	if got := readTallies(t, h, 0, 10); !slices.Equal(got, want) {
		t.Fatalf("Rows = %q, want %q", got, want)
	}
	if got := readTallies(t, h, 1, 1); !slices.Equal(got, want[1:2]) {
		t.Errorf("Rows(offset 1, limit 1) = %q, want %q", got, want[1:2])
	}
	if got := readTallies(t, h, 3, 10); len(got) != 0 {
		t.Errorf("Rows past the end = %q, want none", got)
	}
	// Rows round-trip through Delete: NULL matches NULL, and a row with another value matches
	// nothing.
	var rows [][]any
	err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		_, rows, err = h.Store.Rows(t.Context(), tx, "poll_tallies", 0, 10)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := writeTallies(t, h, false, tally("a", 1, 2, nil)); n != 0 {
		t.Errorf("Delete of a row that differs in a NULL = %d, want 0", n)
	}
	if n := writeTallies(t, h, false, rows[2], rows[0]); n != 2 {
		t.Errorf("Delete = %d, want 2", n)
	}
	if got := readTallies(t, h, 0, 10); !slices.Equal(got, want[1:2]) {
		t.Errorf("Rows after Delete = %q, want %q", got, want[1:2])
	}
}

func projectionStoreRefuses(t *testing.T, h ProjectionStoreHarness) {
	ctx := t.Context()
	err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, _, err := h.Store.Rows(ctx, tx, "no_such_table", 0, 10)
		return err
	})
	if err == nil {
		t.Error("Rows of an unknown table succeeded")
	}
	err = h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := h.Store.Insert(ctx, tx, "poll_tallies", []string{"poll_id", "position", "votes"}, [][]any{{pollKey("a"), int64(0), int64(1)}})
		return err
	})
	if err == nil {
		t.Error("Insert with a subset of the columns succeeded")
	}
	err = h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := h.Store.Delete(ctx, tx, `poll_tallies"; DROP TABLE users; --`, tallyColumns, [][]any{tally("a", 0, 1, nil)})
		return err
	})
	if err == nil {
		t.Error("Delete from a table name outside the schema succeeded")
	}
}

func projectionServiceDrift(t *testing.T, h ProjectionStoreHarness) {
	ctx := t.Context()
	// The "sources" of the projection are this list; its rebuild empties the table and writes it.
	sources := [][]any{tally("a", 0, 1, nil), tally("a", 1, 2, voters(3)), tally("b", 0, 4, nil), tally("c", 0, 5, nil), tally("d", 0, 6, nil)}
	p := projection.Projection{
		Name: "tallies", Module: "testkit", Tables: []string{"poll_tallies"},
		Rebuild: func(ctx context.Context, u uow.UnitOfWork) error {
			return u.Tx(ctx, func(tx uow.Tx) error {
				_, rows, err := h.Store.Rows(ctx, tx, "poll_tallies", 0, 100)
				if err != nil {
					return err
				}
				if _, err := h.Store.Delete(ctx, tx, "poll_tallies", tallyColumns, rows); err != nil {
					return err
				}
				_, err = h.Store.Insert(ctx, tx, "poll_tallies", tallyColumns, sources)
				return err
			})
		},
	}
	svc := projection.NewService(projection.Options{UnitOfWork: h.UnitOfWork, Store: h.Store, Projections: []projection.Projection{p}, ChunkSize: 2})
	if got, err := svc.Lookup("poll_tallies"); err != nil || got.Name != "tallies" {
		t.Fatalf("Lookup(table) = %v, %v", got.Name, err)
	}
	if _, err := svc.Lookup("nothing"); !errors.Is(err, projection.ErrUnknown) {
		t.Errorf("Lookup(unknown) = %v, want ErrUnknown", err)
	}
	// The table holds one row too many, one wrong row, and lacks three.
	writeTallies(t, h, true, tally("a", 0, 1, nil), tally("a", 1, 9, voters(3)), tally("z", 0, 1, nil))
	held := readTallies(t, h, 0, 100)

	r, err := svc.Verify(ctx, p)
	if err != nil {
		t.Fatalf("Verify = %v", err)
	}
	if !r.Drifted() || len(r.Tables) != 1 {
		t.Fatalf("Verify = %+v, want drift in one table", r)
	}
	d := r.Tables[0]
	if len(d.Extra) != 2 || len(d.Missing) != 4 {
		t.Errorf("Verify found %d extra and %d missing rows, want 2 and 4: %v / %v", len(d.Extra), len(d.Missing), d.Extra, d.Missing)
	}
	if got := readTallies(t, h, 0, 100); !slices.Equal(got, held) {
		t.Errorf("Verify changed the table: %q, want %q", got, held)
	}

	r, err = svc.Rebuild(ctx, p)
	if err != nil {
		t.Fatalf("Rebuild = %v", err)
	}
	if r.Deleted != 2 || r.Inserted != 4 {
		t.Errorf("Rebuild deleted %d and inserted %d rows, want 2 and 4", r.Deleted, r.Inserted)
	}
	want := []string{tallyLine("a", "0 1 <nil>"), tallyLine("a", "1 2 3"), tallyLine("b", "0 4 <nil>"), tallyLine("c", "0 5 <nil>"), tallyLine("d", "0 6 <nil>")}
	if got := readTallies(t, h, 0, 100); !slices.Equal(got, want) {
		t.Errorf("after Rebuild = %q, want %q", got, want)
	}
	if r, err := svc.Verify(ctx, p); err != nil || r.Drifted() {
		t.Errorf("Verify after Rebuild = %+v, %v; want no drift", r, err)
	}
}

// recordingQueue records the jobs enqueued to it, and refuses them under a context that ended.
type recordingQueue struct {
	mu   sync.Mutex
	jobs []queue.NewJob
}

func (q *recordingQueue) Enqueue(ctx context.Context, jobs ...queue.NewJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, jobs...)
	return nil
}

// take returns the jobs enqueued since the last take.
func (q *recordingQueue) take() []queue.NewJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	jobs := q.jobs
	q.jobs = nil
	return jobs
}

func projectionVerifyChain(t *testing.T, h ProjectionStoreHarness) {
	var (
		mu  sync.Mutex
		ran []string
	)
	ok := func(name string) func(context.Context, uow.UnitOfWork) error {
		return func(ctx context.Context, _ uow.UnitOfWork) error {
			mu.Lock()
			defer mu.Unlock()
			ran = append(ran, name)
			return ctx.Err()
		}
	}
	broken := errors.New("broken")
	ps := []projection.Projection{
		{Name: "first", Module: "testkit", Tables: []string{"poll_tallies"}, Rebuild: ok("first")},
		{Name: "failing", Module: "testkit", Tables: []string{"account_search_terms"}, Rebuild: func(context.Context, uow.UnitOfWork) error {
			mu.Lock()
			defer mu.Unlock()
			ran = append(ran, "failing")
			return broken
		}},
		{Name: "last", Module: "testkit", Tables: []string{"scheduled_drafts"}, Rebuild: ok("last")},
	}
	jobs := &recordingQueue{}
	svc := projection.NewService(projection.Options{UnitOfWork: h.UnitOfWork, Store: h.Store, Projections: ps, Jobs: jobs, Logger: slog.New(slog.DiscardHandler)})
	reg := &queue.JobHandlerRegistry{}
	sched := queue.NewScheduler(jobs, testkit.NewClock(time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	svc.RegisterJobs(reg, sched)
	handler, policy, found := reg.Lookup(projection.JobVerify)
	if !found {
		t.Fatalf("no handler of %s", projection.JobVerify)
	}
	if schedules := sched.Schedules(); len(schedules) != 1 || schedules[0].Job.Type != projection.JobVerify || len(schedules[0].Job.Payload) != 0 {
		t.Fatalf("schedules = %+v, want one of %s without a payload", schedules, projection.JobVerify)
	}
	run := func(ctx context.Context, j queue.Job) (queue.Result, []string, []queue.NewJob) {
		t.Helper()
		mu.Lock()
		ran = nil
		mu.Unlock()
		r := handler(ctx, j)
		mu.Lock()
		defer mu.Unlock()
		return r, slices.Clone(ran), jobs.take()
	}
	next := func(t *testing.T, enqueued []queue.NewJob, name string) queue.NewJob {
		t.Helper()
		if len(enqueued) != 1 || enqueued[0].Type != projection.JobVerify || enqueued[0].IdempotencyKey != projection.JobVerify+":run:"+name {
			t.Fatalf("enqueued %+v, want the one job of %q in the run", enqueued, name)
		}
		return enqueued[0]
	}

	// The scheduled job verifies the first projection only and queues the next.
	r, rebuilt, enqueued := run(t.Context(), queue.Job{ID: "run", Type: projection.JobVerify, Attempt: 1})
	if !r.IsDone() || !slices.Equal(rebuilt, []string{"first"}) {
		t.Fatalf("scheduled job = done %v, rebuilt %q; want done, first only", r.IsDone(), rebuilt)
	}
	failing := next(t, enqueued, "failing")

	// A failing projection is retried on its own and queues nothing while it has attempts left.
	r, rebuilt, enqueued = run(t.Context(), queue.Job{ID: "j2", Type: projection.JobVerify, Payload: failing.Payload, Attempt: 1})
	if !r.IsRetry() || !errors.Is(r.Err(), broken) || !slices.Equal(rebuilt, []string{"failing"}) || len(enqueued) != 0 {
		t.Fatalf("failing job, first run = retry %v (%v), rebuilt %q, enqueued %+v; want a retry of it alone", r.IsRetry(), r.Err(), rebuilt, enqueued)
	}
	if policy.MaxAttempts < 2 {
		t.Fatalf("the policy of %s allows %d runs, want a retry", projection.JobVerify, policy.MaxAttempts)
	}

	// Its last run fails, even under a context that ended as a lease does, and the run moves on.
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	r, _, enqueued = run(ended, queue.Job{ID: "j2", Type: projection.JobVerify, Payload: failing.Payload, Attempt: policy.MaxAttempts})
	if !r.IsFail() {
		t.Fatalf("failing job, last run = %+v, want it to fail", r)
	}
	last := next(t, enqueued, "last")

	// The last projection queues nothing.
	r, rebuilt, enqueued = run(t.Context(), queue.Job{ID: "j3", Type: projection.JobVerify, Payload: last.Payload, Attempt: 1})
	if !r.IsDone() || !slices.Equal(rebuilt, []string{"last"}) || len(enqueued) != 0 {
		t.Fatalf("last job = done %v, rebuilt %q, enqueued %+v; want done, last only, nothing queued", r.IsDone(), rebuilt, enqueued)
	}

	// A projection this version lacks ends the run.
	r, rebuilt, enqueued = run(t.Context(), queue.Job{ID: "j4", Type: projection.JobVerify, Payload: []byte(`{"run":"run","projection":"gone"}`), Attempt: 1})
	if !r.IsDone() || len(rebuilt) != 0 || len(enqueued) != 0 {
		t.Fatalf("job of an unknown projection = done %v, rebuilt %q, enqueued %+v; want done and nothing else", r.IsDone(), rebuilt, enqueued)
	}
}
