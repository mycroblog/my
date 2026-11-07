// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

const (
	// DefaultRebuildRate is the most statuses a second a rebuild walks unless the operator says
	// otherwise. Each status takes a few reads and a call of the backend, and the jobs run between
	// the others of the queue, so this leaves room for both (ADR-0149 §10).
	DefaultRebuildRate = 20
	// MaxRebuildRate is the most an operator can ask for.
	MaxRebuildRate = 1000

	// rebuildBatch is how many statuses one JobRebuild covers, which has to end within its lease.
	rebuildBatch = 100
	// rebuildOutageFailures is how many statuses in a row may fail before the job treats it as an
	// outage of the backend, retries, and stays at its place, rather than skipping statuses.
	rebuildOutageFailures = 3
	// rebuildFailuresKept is about how many failures of one rebuild are kept for my search status.
	// The counter goes on.
	rebuildFailuresKept = 100
	// rebuildMessageBytes bounds an error kept in the table.
	rebuildMessageBytes = 500
	// clearTimeout bounds Index.Clear, which only the command line calls and which can take much
	// longer than the call of a job: it empties the whole index.
	clearTimeout = 10 * time.Minute
)

// RebuildMode says whether a rebuild empties the index first.
type RebuildMode string

const (
	// RebuildClear empties the index first, so that documents left behind by a missed deletion or a
	// restored database go.
	RebuildClear RebuildMode = "clear"
	// RebuildKeep leaves the index as it is, to add older statuses without taking search away.
	RebuildKeep RebuildMode = "keep"
)

// RebuildPhase is how far a rebuild is.
type RebuildPhase string

const (
	// RebuildClearing is a rebuild whose index is not known to be empty yet.
	RebuildClearing RebuildPhase = "clearing"
	// RebuildFilling is a rebuild walking the statuses.
	RebuildFilling RebuildPhase = "filling"
	// RebuildFinished is a rebuild that reached the last status.
	RebuildFinished RebuildPhase = "finished"
)

// Rebuild is the progress of the latest rebuild.
type Rebuild struct {
	ID    string
	Mode  RebuildMode
	Phase RebuildPhase
	// After is the last status handled, empty before the first.
	After string
	// Rate is the most statuses a second.
	Rate int
	// Processed counts the statuses handled, failed ones included. Failed counts those that failed.
	Processed, Failed int64
	// LastError is the latest error, empty once the next batch went through without one.
	LastError string
	StartedAt time.Time
	UpdatedAt time.Time
}

// RebuildFailure is a status a rebuild could not write.
type RebuildFailure struct {
	StatusID string
	Message  string
	FailedAt time.Time
}

// RebuildStep moves a rebuild on from where it was read: its FromPhase and FromAfter.
type RebuildStep struct {
	ID        string
	FromPhase RebuildPhase
	FromAfter string
	Phase     RebuildPhase
	After     string
	// Processed and Failed are added to the counters.
	Processed, Failed int64
	LastError         string
	UpdatedAt         time.Time
}

// Repository is what the module keeps in the database: the progress of a rebuild, which is one row.
// searchtest.SearchRepositoryContract is its contract.
type Repository interface {
	// StartRebuild writes r as the rebuild and removes the failures of any other, unless a rebuild is
	// unfinished, when it changes nothing and returns false.
	StartRebuild(ctx context.Context, tx uow.Tx, r Rebuild) (bool, error)
	// FindRebuild returns the rebuild, and false without one.
	FindRebuild(ctx context.Context) (Rebuild, bool, error)
	// StepRebuild applies step, and returns false, changing nothing, when the rebuild has moved on.
	StepRebuild(ctx context.Context, tx uow.Tx, step RebuildStep) (bool, error)
	// SetRebuildRate changes the rate of a rebuild.
	SetRebuildRate(ctx context.Context, tx uow.Tx, id string, rate int) error
	// SetRebuildError records an error of a rebuild that stays where it is.
	SetRebuildError(ctx context.Context, tx uow.Tx, id, message string, at time.Time) error
	// AddRebuildFailures records failures of a rebuild, replacing those of the same statuses.
	AddRebuildFailures(ctx context.Context, tx uow.Tx, id string, failures []RebuildFailure) error
	// ListRebuildFailures returns up to limit failures of a rebuild in the order of the statuses.
	ListRebuildFailures(ctx context.Context, id string, limit int) ([]RebuildFailure, error)
	// DeleteRebuild removes the rebuild and every failure.
	DeleteRebuild(ctx context.Context, tx uow.Tx) error
}

// ErrNoBackend reports a command that needs a backend on a server with search = none (ADR-0149 §10).
var ErrNoBackend = errors.New("search: the server has no search backend")

// ErrRebuildModeDiffers reports a rebuild asked for in the other mode than the unfinished one: the
// resumption would do something else than it says. StartRebuild returns the unfinished rebuild with it.
var ErrRebuildModeDiffers = errors.New("search: an unfinished rebuild has the other mode")

// RebuildOptions says what StartRebuild does for a new rebuild.
type RebuildOptions struct {
	// Keep leaves the index as it is instead of emptying it first.
	Keep bool
	// Rate is the most statuses a second, DefaultRebuildRate when 0. On a resumption it replaces the
	// rate only when not 0.
	Rate int
}

// RebuildStart says what StartRebuild did.
type RebuildStart struct {
	Rebuild Rebuild
	// Resumed reports that an unfinished rebuild goes on, at its place, instead of a new one.
	Resumed bool
	// Cleared reports that this call emptied the index.
	Cleared bool
}

type rebuildPayload struct {
	RebuildID string `json:"rebuild_id"`
	// Cursor is the last status handled, the place of the walk this job goes on from.
	Cursor string `json:"cursor,omitempty"`
}

// StartRebuild starts a rebuild, or resumes the unfinished one at its place (ADR-0149 §10). The
// index is emptied here, in the command and not in a job, so that an engine that does not answer
// shows at once, and a job run twice cannot remove what the walk has already written. An index that
// was emptied is not emptied again: the phase moves on in the database right after. It fails with
// ErrNoBackend with search = none, having written nothing.
func (s *Service) StartRebuild(ctx context.Context, opts RebuildOptions) (RebuildStart, error) {
	if !s.Enabled() {
		return RebuildStart{}, ErrNoBackend
	}
	mode, phase := RebuildClear, RebuildClearing
	if opts.Keep {
		mode, phase = RebuildKeep, RebuildFilling
	}
	rate := opts.Rate
	if rate == 0 {
		rate = DefaultRebuildRate
	}
	candidate := Rebuild{ID: s.d.IDs.NewID(), Mode: mode, Phase: phase, Rate: rate, StartedAt: s.d.Clock.Now()}
	var started bool
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		started, err = s.d.Rebuilds.StartRebuild(ctx, tx, candidate)
		return err
	}); err != nil {
		return RebuildStart{}, err
	}
	r, ok, err := s.d.Rebuilds.FindRebuild(ctx)
	if err != nil {
		return RebuildStart{}, err
	}
	if !ok {
		return RebuildStart{}, errors.New("search: the rebuild was removed while it started")
	}
	out := RebuildStart{Rebuild: r, Resumed: !started}
	if !started {
		if r.Mode != mode {
			return out, ErrRebuildModeDiffers
		}
		if opts.Rate != 0 && opts.Rate != r.Rate {
			if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				return s.d.Rebuilds.SetRebuildRate(ctx, tx, r.ID, opts.Rate)
			}); err != nil {
				return out, err
			}
			out.Rebuild.Rate = opts.Rate
		}
	}
	if r.Phase == RebuildClearing {
		if err := s.clear(ctx); err != nil {
			return out, err
		}
		out.Cleared = true
		out.Rebuild.Phase = RebuildFilling
		if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			_, err := s.d.Rebuilds.StepRebuild(ctx, tx, RebuildStep{
				ID: r.ID, FromPhase: RebuildClearing, FromAfter: r.After, Phase: RebuildFilling, After: r.After,
				UpdatedAt: s.d.Clock.Now(),
			})
			return err
		}); err != nil {
			return out, err
		}
	}
	// Enqueued without an idempotency key: on a resumption the chain may be alive, and whichever job
	// steps the rebuild first goes on while the other finds it has moved on.
	job, err := rebuildJob(r.ID, r.After, time.Time{})
	if err != nil {
		return out, err
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, job) }); err != nil {
		return out, err
	}
	return out, nil
}

// ClearIndex empties the index and ends any rebuild, whose jobs then do nothing, so that it does not
// fill the index again (ADR-0149 §10, §12). It fails with ErrNoBackend with search = none, when the
// server cannot reach the index.
func (s *Service) ClearIndex(ctx context.Context) error {
	if !s.Enabled() {
		return ErrNoBackend
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Rebuilds.DeleteRebuild(ctx, tx) }); err != nil {
		return err
	}
	return s.clear(ctx)
}

func (s *Service) clear(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, clearTimeout)
	defer cancel()
	return s.d.Index.Clear(ctx)
}

// RebuildProgress returns the latest rebuild with up to limit of its failures, and false without one.
// It fails with ErrNoBackend with search = none.
func (s *Service) RebuildProgress(ctx context.Context, limit int) (Rebuild, []RebuildFailure, bool, error) {
	if !s.Enabled() {
		return Rebuild{}, nil, false, ErrNoBackend
	}
	r, ok, err := s.d.Rebuilds.FindRebuild(ctx)
	if err != nil || !ok {
		return Rebuild{}, nil, false, err
	}
	failures, err := s.d.Rebuilds.ListRebuildFailures(ctx, r.ID, limit)
	if err != nil {
		return Rebuild{}, nil, false, err
	}
	return r, failures, true, nil
}

func rebuildJob(id, cursor string, runAt time.Time) (queue.NewJob, error) {
	b, err := json.Marshal(rebuildPayload{RebuildID: id, Cursor: cursor})
	if err != nil {
		return queue.NewJob{}, err
	}
	return queue.NewJob{Type: JobRebuild, Payload: b, RunAt: runAt}, nil
}

// handleRebuild handles the next batch of the walk and enqueues the job of the batch after it, no
// sooner than the rate allows. The walk is one chain of jobs, unlike the fan-outs of the other jobs,
// because the rate holds only if the batches do not run side by side.
//
// A job that does not find the rebuild at its own place has nothing to do: it ran twice, or the rebuild
// was resumed or cleared. Each status goes through syncStatus, like the job of a post, so that the
// jobs enqueued meanwhile and the walk end in the same state, whichever writes last: a status created
// after the walk passed has its own job, and one deleted is read again after it is written (§10).
func (s *Service) handleRebuild(ctx context.Context, j queue.Job) queue.Result {
	var p rebuildPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.RebuildID == "" {
		return queue.Fail(fmt.Errorf("search: payload of %s: %q", JobRebuild, j.Payload))
	}
	r, ok, err := s.d.Rebuilds.FindRebuild(ctx)
	if err != nil {
		return queue.Retry(0, err)
	}
	if !ok || r.ID != p.RebuildID || r.Phase != RebuildFilling || r.After != p.Cursor {
		return queue.Done()
	}
	began := s.d.Clock.Now()
	batch := s.rebuildBatchSize()
	ids, err := s.d.Statuses.SearchableStatusIDs(ctx, r.After, batch)
	if err != nil {
		return queue.Retry(0, err)
	}
	var (
		failures []RebuildFailure
		lastErr  string
		inARow   int
	)
	for _, id := range ids {
		err := s.syncStatus(ctx, id)
		if err == nil {
			inARow = 0
			continue
		}
		if ctx.Err() != nil {
			return queue.Retry(0, ctx.Err())
		}
		lastErr = rebuildMessage(err)
		if inARow++; inARow >= rebuildOutageFailures {
			// Stay here, so that an outage of the engine skips nothing. The job's retries, or a
			// resumption after a dead job, go on from this place.
			if recErr := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				return s.d.Rebuilds.SetRebuildError(ctx, tx, r.ID, lastErr, s.d.Clock.Now())
			}); recErr != nil {
				return queue.Retry(0, recErr)
			}
			return queue.Retry(0, err)
		}
		failures = append(failures, RebuildFailure{StatusID: id, Message: lastErr, FailedAt: s.d.Clock.Now()})
	}
	step := RebuildStep{
		ID: r.ID, FromPhase: RebuildFilling, FromAfter: r.After, Phase: RebuildFilling, After: r.After,
		Processed: int64(len(ids)), Failed: int64(len(failures)), LastError: lastErr, UpdatedAt: s.d.Clock.Now(),
	}
	if len(ids) > 0 {
		step.After = ids[len(ids)-1]
	}
	var next *queue.NewJob
	if len(ids) < batch {
		step.Phase = RebuildFinished
	} else {
		runAt := began.Add(time.Duration(batch) * time.Second / time.Duration(r.Rate))
		job, err := rebuildJob(r.ID, step.After, runAt)
		if err != nil {
			return queue.Fail(err)
		}
		next = &job
	}
	kept := failures
	if r.Failed >= rebuildFailuresKept {
		kept = nil
	}
	return outcome(s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		moved, err := s.d.Rebuilds.StepRebuild(ctx, tx, step)
		if err != nil || !moved {
			return err
		}
		if err := s.d.Rebuilds.AddRebuildFailures(ctx, tx, r.ID, kept); err != nil {
			return err
		}
		if next == nil {
			return nil
		}
		return s.d.Outbox.EnqueueTx(ctx, tx, *next)
	}))
}

func (s *Service) rebuildBatchSize() int {
	if s.d.RebuildBatch > 0 {
		return s.d.RebuildBatch
	}
	return rebuildBatch
}

// rebuildMessage is what the table keeps of an error: short, and valid in every dialect.
func rebuildMessage(err error) string {
	m := err.Error()
	if len(m) > rebuildMessageBytes {
		m = m[:rebuildMessageBytes]
	}
	return strings.ToValidUTF8(m, "")
}
