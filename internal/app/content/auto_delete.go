// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

const (
	// JobAutoDeleteStatuses deletes the local posts past their authors' automatic deletion
	// policies (FR-118). Its payload is the cursor of the run, an account ID (none for the first
	// job of a run).
	JobAutoDeleteStatuses = "content.auto_delete_statuses"
	// AutoDeleteInterval is the period of the job's schedule.
	AutoDeleteInterval = time.Hour
	// AutoDeleteBatch is the number of posts one transaction deletes, and AutoDeletePerAccount the
	// most posts of one account that one run deletes: each deletion sends a Delete to the
	// author's followers, so a policy that finds many old posts spreads their deletion, and the
	// deliveries, over the hourly runs (ADR-0004). autoDeleteAccountsPerJob bounds the accounts of
	// one job; a job that used them up enqueues the next with its cursor.
	AutoDeleteBatch          = 20
	AutoDeletePerAccount     = 100
	autoDeleteAccountsPerJob = 20
	// AutoDeletePreviewCap is the most posts that a preview counts, and AutoDeletePreviewExamples
	// the posts it shows: the count stays cheap enough to repeat as the user edits a policy.
	AutoDeletePreviewCap      = 1000
	AutoDeletePreviewExamples = 3
)

// AutoDeletePreview is what a policy would delete now: Count posts up to AutoDeletePreviewCap,
// More when there are more, and the oldest of them.
type AutoDeletePreview struct {
	Count    int
	More     bool
	Examples []content.Status
}

// AutoDeleteRepository is the part of the content module's persistence that the automatic
// deletion of old posts uses. The database adapters' status repositories implement it and pass
// testkit.AutoDeleteContract.
type AutoDeleteRepository interface {
	// FindAutoDeletePolicy returns the saved policy of the account, or ErrNotFound.
	FindAutoDeletePolicy(ctx context.Context, accountID string) (content.AutoDeletePolicy, error)
	// SaveAutoDeletePolicy inserts or replaces the policy of p.AccountID with its kept
	// visibilities.
	SaveAutoDeletePolicy(ctx context.Context, tx uow.Tx, p content.AutoDeletePolicy) error
	// ListEnabledAutoDeletePolicies returns up to limit enabled policies in account ID order
	// after the account ID after.
	ListEnabledAutoDeletePolicies(ctx context.Context, after string, limit int) ([]content.AutoDeletePolicy, error)
	// ListStatusesToAutoDelete returns, oldest first, up to limit local statuses of p.AccountID
	// that are not deleted, were published before before, and that none of p's exceptions keeps.
	ListStatusesToAutoDelete(ctx context.Context, tx uow.Tx, p content.AutoDeletePolicy, before time.Time, limit int) ([]content.Status, error)
}

// AutoDeleteAccounts finds the accounts whose policies run; *account.Service implements it.
type AutoDeleteAccounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
}

// AutoDeleteDeps are the ports of the automatic deletion.
type AutoDeleteDeps struct {
	UnitOfWork  uow.UnitOfWork
	Repository  Repository
	AutoDeletes AutoDeleteRepository
	Accounts    AutoDeleteAccounts
	// Jobs enqueues the next job of a run, outside any transaction.
	Jobs  queue.JobQueue
	Clock kernel.Clock
	// Batch and PerAccount override AutoDeleteBatch and AutoDeletePerAccount when positive.
	Batch, PerAccount int
}

// AutoDelete is the automatic deletion of local accounts' own old posts (FR-118): the policy
// that each account sets, and the hourly job that carries the enabled ones out. The job deletes
// a post as its author would (a deletion with origin auto_delete, which takes it out of every
// projection and sends a Delete to where the post went), so it is safe to repeat: a deleted post
// is not listed again. It skips the accounts that are not active (deactivated, or scheduled for
// deletion or deleted), whose posts stay until they are active again.
type AutoDelete struct{ d AutoDeleteDeps }

// NewAutoDelete returns the automatic deletion on d.
func NewAutoDelete(d AutoDeleteDeps) *AutoDelete {
	if d.Batch <= 0 {
		d.Batch = AutoDeleteBatch
	}
	if d.PerAccount <= 0 {
		d.PerAccount = AutoDeletePerAccount
	}
	return &AutoDelete{d: d}
}

// AutoDeletePolicy returns the account's policy, the default one if it has never saved one.
func (a *AutoDelete) AutoDeletePolicy(ctx context.Context, accountID string) (content.AutoDeletePolicy, error) {
	p, err := a.d.AutoDeletes.FindAutoDeletePolicy(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return content.DefaultAutoDeletePolicy(accountID), nil
	}
	return p, err
}

// UpdateAutoDeletePolicy replaces the account's policy with p and returns it as saved. It fails
// with a *content.AutoDeletePolicyError for an invalid value.
func (a *AutoDelete) UpdateAutoDeletePolicy(ctx context.Context, accountID string, p content.AutoDeletePolicy) (content.AutoDeletePolicy, error) {
	p.AccountID = accountID
	p.UpdatedAt = kernel.Normalize(a.d.Clock.Now())
	p, err := p.Normalize()
	if err != nil {
		return content.AutoDeletePolicy{}, err
	}
	err = a.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return a.d.AutoDeletes.SaveAutoDeletePolicy(ctx, tx, p)
	})
	if err != nil {
		return content.AutoDeletePolicy{}, err
	}
	return p, nil
}

// autoDeletePayload is the payload of JobAutoDeleteStatuses: the cursor of the run.
type autoDeletePayload struct {
	After string `json:"after,omitempty"`
}

// PreviewAutoDeletePolicy returns what p would delete of the account's posts now if it were
// enabled, without saving it. It fails with a *content.AutoDeletePolicyError for an invalid
// value.
func (a *AutoDelete) PreviewAutoDeletePolicy(ctx context.Context, accountID string, p content.AutoDeletePolicy) (AutoDeletePreview, error) {
	p.AccountID = accountID
	p, err := p.Normalize()
	if err != nil {
		return AutoDeletePreview{}, err
	}
	before := p.Cutoff(kernel.Normalize(a.d.Clock.Now()))
	var statuses []content.Status
	// A read in one short transaction: the repository lists the posts as the job would.
	err = a.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		statuses, err = a.d.AutoDeletes.ListStatusesToAutoDelete(ctx, tx, p, before, AutoDeletePreviewCap+1)
		return err
	})
	if err != nil {
		return AutoDeletePreview{}, err
	}
	out := AutoDeletePreview{Count: min(len(statuses), AutoDeletePreviewCap), More: len(statuses) > AutoDeletePreviewCap}
	out.Examples = statuses[:min(len(statuses), AutoDeletePreviewExamples)]
	return out, nil
}

// RegisterJobs registers the JobAutoDeleteStatuses handler and its hourly schedule.
func (a *AutoDelete) RegisterJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobAutoDeleteStatuses, a.handle)
	sched.Add(queue.Schedule{Name: JobAutoDeleteStatuses, Every: AutoDeleteInterval, Job: queue.NewJob{Type: JobAutoDeleteStatuses}})
}

func (a *AutoDelete) handle(ctx context.Context, j queue.Job) queue.Result {
	var pl autoDeletePayload
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("content: auto delete payload %s: %w", j.Payload, err))
		}
	}
	next, _, err := a.Run(ctx, pl.After)
	if err != nil {
		return queue.Retry(0, err)
	}
	if next == "" || a.d.Jobs == nil {
		return queue.Done()
	}
	b, err := json.Marshal(autoDeletePayload{After: next})
	if err != nil {
		return queue.Fail(err)
	}
	if err := a.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobAutoDeleteStatuses, Payload: b, IdempotencyKey: "auto_delete_statuses:" + next}); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Run carries out the enabled policies of the accounts after the cursor after ("" for the
// first), a bounded number of accounts and of posts per account. It returns how many posts it
// deleted and the cursor to continue from, "" when it reached the last policy. It is safe to
// repeat.
func (a *AutoDelete) Run(ctx context.Context, after string) (next string, deleted int, err error) {
	policies, err := a.d.AutoDeletes.ListEnabledAutoDeletePolicies(ctx, after, autoDeleteAccountsPerJob)
	if err != nil {
		return after, 0, err
	}
	for _, p := range policies {
		n, err := a.RunPolicy(ctx, p)
		deleted += n
		if err != nil {
			return after, deleted, fmt.Errorf("auto delete the posts of %s: %w", p.AccountID, err)
		}
		after = p.AccountID
	}
	if len(policies) < autoDeleteAccountsPerJob {
		return "", deleted, nil
	}
	return after, deleted, nil
}

// RunPolicy deletes up to PerAccount posts that p deletes now, in short transactions, and
// returns how many it deleted. It deletes none when p is disabled or its account is not an
// active local account.
func (a *AutoDelete) RunPolicy(ctx context.Context, p content.AutoDeletePolicy) (int, error) {
	if !p.Enabled {
		return 0, nil
	}
	actor, err := a.d.Accounts.FindByID(ctx, p.AccountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !actor.Account.IsLocal() || !actor.Lifecycle.Active() {
		return 0, nil
	}
	now := kernel.Normalize(a.d.Clock.Now())
	before := p.Cutoff(now)
	deleted := 0
	for deleted < a.d.PerAccount {
		limit := min(a.d.Batch, a.d.PerAccount-deleted)
		var n int
		// The transaction may run again (a retry); it lists the posts again each time.
		err := a.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			n, err = a.deleteBatch(ctx, tx, p, before, now, limit)
			return err
		})
		if err != nil {
			return deleted, err
		}
		deleted += n
		if n < limit {
			break
		}
	}
	return deleted, nil
}

// deleteBatch deletes up to limit posts inside tx and returns how many it deleted.
func (a *AutoDelete) deleteBatch(ctx context.Context, tx uow.Tx, p content.AutoDeletePolicy, before, now time.Time, limit int) (int, error) {
	statuses, err := a.d.AutoDeletes.ListStatusesToAutoDelete(ctx, tx, p, before, limit)
	if err != nil {
		return 0, err
	}
	for _, st := range statuses {
		d := content.Deletion{
			StatusID: st.ID, URI: st.URI, AuthorAccountID: st.AuthorAccountID, Origin: content.DeletedByAutoDelete, DeletedAt: now,
		}
		if err := a.d.Repository.InsertDeletion(ctx, tx, d); err != nil {
			return 0, fmt.Errorf("auto delete %s: %w", st.URI, err)
		}
		if err := tx.Emit(ctx, StatusDeleted{Deletion: d, Status: st}); err != nil {
			return 0, err
		}
	}
	return len(statuses), nil
}
