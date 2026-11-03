// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

const (
	// JobPruneRemoteProfiles shrinks the profile history of the remote accounts that nothing local
	// relates to (ADR-0034, FR-803). Its payload is the cursor of the run (none for the first job).
	JobPruneRemoteProfiles = "account.prune_remote_profiles"
	// PruneInterval is the period of the prune's schedule.
	PruneInterval = 24 * time.Hour
	// PruneBatch is the number of accounts one prune transaction examines; pruneMaxBatches
	// bounds the batches of one job, and a job that used them up enqueues the next with its
	// cursor.
	PruneBatch      = 100
	pruneMaxBatches = 10
	// HandleVerificationRetention is how long a check of a remote handle is kept; the newest of an
	// account always stays, because it rebuilds account_handles.
	HandleVerificationRetention = 90 * 24 * time.Hour
	// handleVerificationBatch is the number of checks one statement deletes;
	// handleVerificationMaxBatches bounds the batches of one run, and the next run continues.
	handleVerificationBatch      = 1000
	handleVerificationMaxBatches = 20
)

// ProfilePruneRepository is the part of the account module's persistence that the prune uses.
// The database adapters' account repositories implement it and pass testkit.RetentionContract.
type ProfilePruneRepository interface {
	// ListRemoteAccountsWithPastProfiles returns, outside any transaction, up to limit remote
	// accounts in ID order after the account ID after that keep a profile revision other than
	// the current one, or a snapshot that neither the head nor the current revision points to.
	ListRemoteAccountsWithPastProfiles(ctx context.Context, after string, limit int) ([]string, error)
	// PrunePastProfiles deletes the profile revisions of a remote account other than the current
	// one, with their fields, and the snapshots that neither the head nor the current revision
	// points to. An account without a head keeps everything.
	PrunePastProfiles(ctx context.Context, tx uow.Tx, accountID string) error
	// DeleteHandleVerificationsBefore deletes at most limit handle checks made before t, except the
	// newest of each account, and returns how many it deleted.
	DeleteHandleVerificationsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error)
}

// RemoteRelations tells whether something local relates to a remote account: a follow either
// way, or a status of it that is stored. The composition root implements it with the graph and
// content modules, which import this one.
type RemoteRelations interface {
	RelatedToLocal(ctx context.Context, accountID string) (bool, error)
}

// PruneDeps are the ports of the prune.
type PruneDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository ProfilePruneRepository
	Relations  RemoteRelations
	Clock      kernel.Clock
	// Jobs enqueues the next job of a run, outside any transaction.
	Jobs queue.JobQueue
	// Batch is the number of accounts per transaction; zero means PruneBatch.
	Batch int
}

// ProfilePruner runs JobPruneRemoteProfiles (ADR-0034): a remote account that no local account
// follows or is followed by, and whose statuses are all gone, keeps only its current profile
// revision and the snapshots its head and that revision point to. The account's row, its URI,
// and its keys stay. A later fetch adds revisions again as usual.
type ProfilePruner struct{ d PruneDeps }

// NewProfilePruner returns the prune on d.
func NewProfilePruner(d PruneDeps) *ProfilePruner {
	if d.Batch <= 0 {
		d.Batch = PruneBatch
	}
	return &ProfilePruner{d: d}
}

// prunePayload is the payload of JobPruneRemoteProfiles: the cursor of the run.
type prunePayload struct {
	After string `json:"after,omitempty"`
}

// RegisterJobs registers the JobPruneRemoteProfiles handler and its daily schedule.
func (p *ProfilePruner) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobPruneRemoteProfiles, p.handle)
	sched.Add(queue.Schedule{Name: JobPruneRemoteProfiles, Every: PruneInterval, Job: queue.NewJob{Type: JobPruneRemoteProfiles}})
}

func (p *ProfilePruner) handle(ctx context.Context, j queue.Job) queue.Result {
	var pl prunePayload
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("account: prune payload %s: %w", j.Payload, err))
		}
	}
	next, _, err := p.Run(ctx, pl.After)
	if err != nil {
		return queue.Retry(0, err)
	}
	if next == "" || p.d.Jobs == nil {
		return queue.Done()
	}
	b, err := json.Marshal(prunePayload{After: next})
	if err != nil {
		return queue.Fail(err)
	}
	if err := p.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobPruneRemoteProfiles, Payload: b, IdempotencyKey: "prune_remote_profiles:" + next}); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// purgeHandleVerifications deletes the checks of remote handles past their retention, for every
// remote account whether or not something local relates to it.
func (p *ProfilePruner) purgeHandleVerifications(ctx context.Context) error {
	before := p.d.Clock.Now().Add(-HandleVerificationRetention)
	for range handleVerificationMaxBatches {
		var n int64
		err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = p.d.Repository.DeleteHandleVerificationsBefore(ctx, tx, before, handleVerificationBatch)
			return err
		})
		if err != nil {
			return err
		}
		if n < handleVerificationBatch {
			return nil
		}
	}
	return nil
}

// Run prunes the remote accounts after the account ID after ("" for the first), in a bounded
// number of batches of one short transaction each. It returns how many accounts it pruned and
// the cursor to continue from, "" when it reached the end. It is safe to repeat.
func (p *ProfilePruner) Run(ctx context.Context, after string) (next string, pruned int, err error) {
	if after == "" {
		if err := p.purgeHandleVerifications(ctx); err != nil {
			return after, 0, err
		}
	}
	for range pruneMaxBatches {
		ids, err := p.d.Repository.ListRemoteAccountsWithPastProfiles(ctx, after, p.d.Batch)
		if err != nil {
			return after, pruned, err
		}
		// The relations are other modules' reads; they run before the transaction, which stays
		// short and only deletes.
		var unrelated []string
		for _, id := range ids {
			related, err := p.d.Relations.RelatedToLocal(ctx, id)
			if err != nil {
				return after, pruned, err
			}
			if !related {
				unrelated = append(unrelated, id)
			}
		}
		if len(unrelated) > 0 {
			err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				for _, id := range unrelated {
					if err := p.d.Repository.PrunePastProfiles(ctx, tx, id); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return after, pruned, err
			}
		}
		pruned += len(unrelated)
		if len(ids) < p.d.Batch {
			return "", pruned, nil
		}
		after = ids[len(ids)-1]
	}
	return after, pruned, nil
}
