// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

const (
	// JobEraseDeletedAccounts erases the data of the deleted local accounts (data-model §8,
	// FR-008). Its payload is the cursor of the run (none for the first job of a run).
	JobEraseDeletedAccounts = "account.erase_deleted"
	// ErasureInterval is the period of the erasure's schedule.
	ErasureInterval = time.Hour
	// ErasureDelay is how long a deleted account's data stays before the erasure: the Delete of
	// the actor reaches its followers with the account's key and its follows, and its delivery
	// retries end within about four days (federation.md §7.3).
	ErasureDelay = 7 * 24 * time.Hour
	// ErasureBatch is the number of rows (statuses, for the statuses) that one erasure statement
	// deletes; erasureMaxBatches bounds the transactions of one job, and a job that used them up
	// enqueues the next with its cursor.
	ErasureBatch      = 100
	erasureMaxBatches = 50
	// erasureAccounts is the number of accounts that one listing returns.
	erasureAccounts = 20
)

// ErasureTarget is a deleted local account whose data the erasure removes, with the user who
// logged in as it.
type ErasureTarget struct {
	AccountID string
	UserID    string
}

// ErasureStep is the outcome of one batch of a DataEraser.
type ErasureStep struct {
	// Next is the cursor of the next batch, meaningful only while Done is false.
	Next string
	// Done reports that the eraser has nothing more to erase in this run.
	Done bool
	// Kept reports that the batch left data that a later run erases: the statuses that the
	// moderation records still need (ADR-0038).
	Kept bool
}

// DataEraser erases one module's part of a deleted local account's data (data-model §8). Each
// call is one short transaction that deletes at most about limit rows, starting from cursor (""
// for the first batch of a run), and is safe to repeat; external I/O such as blob deletions runs
// outside that transaction. The content, graph, timeline, notification, and identity modules
// implement it; the composition root hands them to the erasure in that order, except that the
// content module's media files go after the notifications.
type DataEraser interface {
	EraseAccountData(ctx context.Context, target ErasureTarget, cursor string, limit int) (ErasureStep, error)
}

// ErasureRepository is the part of the account module's persistence that the erasure uses. The
// database adapters' account repositories implement it and pass testkit.AccountErasureContract.
type ErasureRepository interface {
	// ListAccountsToErase returns, outside any transaction, up to limit local accounts in ID
	// order after the account ID after that were deleted at or before deletedBefore and are not
	// erased yet.
	ListAccountsToErase(ctx context.Context, deletedBefore time.Time, after string, limit int) ([]ErasureTarget, error)
	// EraseProfilesAndKeys deletes the profile revisions of an account with their sources and
	// fields, its profile head, and its keys with their private halves and revocations.
	EraseProfilesAndKeys(ctx context.Context, tx uow.Tx, accountID string) error
	// InsertErasure records that the account is erased; a second record does nothing.
	InsertErasure(ctx context.Context, tx uow.Tx, accountID string, at time.Time) error
	// AccountErased reports whether the account is erased.
	AccountErased(ctx context.Context, accountID string) (bool, error)
}

// HeldAccounts lists the accounts whose erasure a retention hold stops (ADR-0038);
// *moderation.Service implements it.
type HeldAccounts interface {
	HeldAccountIDs(ctx context.Context) ([]string, error)
}

// ErasureDeps are the ports of the erasure.
type ErasureDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository ErasureRepository
	// Erasers erase the other modules' data, in order.
	Erasers []DataEraser
	// Holds lists the held accounts, read at each run; nil holds none.
	Holds HeldAccounts
	// Jobs enqueues the next job of a run, outside any transaction.
	Jobs  queue.JobQueue
	Clock kernel.Clock
	// Delay is how long after its deletion an account is erased; zero means ErasureDelay.
	Delay time.Duration
	// Batch is the number of rows per statement; zero means ErasureBatch.
	Batch int
}

// Eraser runs JobEraseDeletedAccounts (data-model §8): a local account deleted longer than the
// delay ago, that no retention hold preserves, loses its statuses (each recorded as deleted with
// origin account_deletion first), its favourites, reactions, bookmarks, pins, boosts, follows, blocks, mutes,
// its timeline and notification rows, its uploads with their blobs, its user's email addresses,
// credentials, sessions, tokens, preferences, and registration application, and its profile
// revisions and keys. The account's row with its URI and domain, its username, its lifecycle,
// and the moderation records stay. A status that a report or an action points at keeps its rows
// and its media files until the moderation purge erases that record, and the account is
// recorded as erased only when nothing is left.
type Eraser struct{ d ErasureDeps }

// NewEraser returns the erasure on d.
func NewEraser(d ErasureDeps) *Eraser {
	if d.Delay <= 0 {
		d.Delay = ErasureDelay
	}
	if d.Batch <= 0 {
		d.Batch = ErasureBatch
	}
	return &Eraser{d: d}
}

// ErasureCursor is where a run of the erasure goes on: after the last account that it
// finished, and inside the account it was erasing when its job used up its transactions. It is
// the payload of JobEraseDeletedAccounts (none for the first job of a run).
type ErasureCursor struct {
	// After is the last account that the run finished.
	After string `json:"after,omitempty"`
	// Account is the account that the run was erasing, "" for none; the fields below are about it.
	Account string `json:"account,omitempty"`
	// Eraser is the index in ErasureDeps.Erasers of the eraser that the run was at, or
	// len(Erasers) when only the profile and keys were left.
	Eraser int `json:"eraser,omitempty"`
	// Next is that eraser's cursor.
	Next string `json:"next,omitempty"`
	// Kept reports that the erasers before kept data of the account.
	Kept bool `json:"kept,omitempty"`
}

// RegisterJobs registers the JobEraseDeletedAccounts handler and its hourly schedule.
func (e *Eraser) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobEraseDeletedAccounts, e.handle)
	sched.Add(queue.Schedule{Name: JobEraseDeletedAccounts, Every: ErasureInterval, Job: queue.NewJob{Type: JobEraseDeletedAccounts}})
}

func (e *Eraser) handle(ctx context.Context, j queue.Job) queue.Result {
	var cur ErasureCursor
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &cur); err != nil {
			return queue.Fail(fmt.Errorf("account: erasure payload %s: %w", j.Payload, err))
		}
	}
	res, err := e.Run(ctx, cur)
	if err != nil {
		return queue.Retry(0, err)
	}
	if !res.More || e.d.Jobs == nil {
		return queue.Done()
	}
	b, err := json.Marshal(res.Next)
	if err != nil {
		return queue.Fail(err)
	}
	// The time keeps a continuation of the same cursor from colliding with the running job.
	key := "erase_deleted:" + res.Next.After + ":" + res.Next.Account + ":" + strconv.FormatInt(e.d.Clock.Now().UnixMilli(), 10)
	if err := e.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobEraseDeletedAccounts, Payload: b, IdempotencyKey: key}); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// ErasureRun is the outcome of one run of the erasure.
type ErasureRun struct {
	// Erased counts the accounts recorded as erased.
	Erased int
	// More reports that the run used up its transactions; the next run continues from Next.
	More bool
	Next ErasureCursor
}

// Run erases the accounts to erase from the cursor (the zero value for the first job of a run),
// in a bounded number of short transactions. A run that uses them up inside an account returns
// a cursor inside it, so that the next one goes on from there instead of passing over the kept
// data again. It is safe to repeat.
func (e *Eraser) Run(ctx context.Context, cur ErasureCursor) (ErasureRun, error) {
	var held []string
	if e.d.Holds != nil {
		var err error
		if held, err = e.d.Holds.HeldAccountIDs(ctx); err != nil {
			return ErasureRun{Next: cur}, fmt.Errorf("account: retention holds: %w", err)
		}
	}
	before := e.d.Clock.Now().Add(-e.d.Delay)
	budget := erasureMaxBatches
	out := ErasureRun{Next: ErasureCursor{After: cur.After}}
	for {
		targets, err := e.d.Repository.ListAccountsToErase(ctx, before, out.Next.After, erasureAccounts)
		if err != nil {
			return out, err
		}
		for _, t := range targets {
			if slices.Contains(held, t.AccountID) {
				out.Next.After = t.AccountID
				continue
			}
			// The progress of the cursor holds only for the account it names.
			at := ErasureCursor{Account: t.AccountID}
			if cur.Account == t.AccountID {
				at = cur
			}
			at.After = out.Next.After
			next, erased, err := e.erase(ctx, t, at, &budget)
			if err != nil {
				return out, fmt.Errorf("account: erase %s: %w", t.AccountID, err)
			}
			if next != nil {
				out.More, out.Next = true, *next
				return out, nil
			}
			if erased {
				out.Erased++
			}
			out.Next.After = t.AccountID
		}
		if len(targets) < erasureAccounts {
			return out, nil
		}
		if budget <= 0 {
			out.More = true
			return out, nil
		}
	}
}

// erase runs the erasers on one account from the cursor at, within the budget of
// transactions. It returns the cursor to go on from when the budget ran out first, and
// whether it recorded the account as erased (nothing is kept for later).
func (e *Eraser) erase(ctx context.Context, t ErasureTarget, at ErasureCursor, budget *int) (*ErasureCursor, bool, error) {
	for ; at.Eraser < len(e.d.Erasers); at.Eraser, at.Next = at.Eraser+1, "" {
		er := e.d.Erasers[at.Eraser]
		for {
			if *budget <= 0 {
				return &at, false, nil
			}
			*budget--
			b, err := er.EraseAccountData(ctx, t, at.Next, e.d.Batch)
			if err != nil {
				return nil, false, err
			}
			at.Kept = at.Kept || b.Kept
			if b.Done {
				break
			}
			at.Next = b.Next
		}
	}
	if *budget <= 0 {
		return &at, false, nil
	}
	*budget--
	now := kernel.Normalize(e.d.Clock.Now())
	err := e.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.d.Repository.EraseProfilesAndKeys(ctx, tx, t.AccountID); err != nil {
			return err
		}
		if at.Kept {
			return nil
		}
		return e.d.Repository.InsertErasure(ctx, tx, t.AccountID, now)
	})
	if err != nil {
		return nil, false, err
	}
	return nil, !at.Kept, nil
}
