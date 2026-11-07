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
	// policies (FR-118), and gives the advance notices of the server maximum (ADR-0151 §5). Its
	// payload is the cursor of the run, an account ID (none for the first job of a run).
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

// EventAutoDeleteLimitNoticed is emitted in the transaction that records an advance notice of
// the server maximum to an account (ADR-0151 §5, §7); the notification module tells the account.
const EventAutoDeleteLimitNoticed = "content.auto_delete_limit_noticed"

// AutoDeleteLimitNoticed is the event of EventAutoDeleteLimitNoticed.
type AutoDeleteLimitNoticed struct {
	Notice content.AutoDeleteLimitNotice
}

// EventType implements uow.Event.
func (AutoDeleteLimitNoticed) EventType() string { return EventAutoDeleteLimitNoticed }

// AutoDeletePreview is what a policy would delete now: Count posts up to AutoDeletePreviewCap,
// More when there are more, and the oldest of them.
type AutoDeletePreview struct {
	Count    int
	More     bool
	Examples []content.Status
}

// AutoDeleteView is an account's automatic deletion as its settings show it: the policy it saved
// (the built-in default if it never saved one) and the server maximum when it applies.
type AutoDeleteView struct {
	Policy content.AutoDeletePolicy
	// Limit is the server maximum and what it makes of Policy, nil when there is no maximum or
	// the account has keep_posts_longer (ADR-0151 §8).
	Limit *AutoDeleteLimitView
}

// AutoDeleteLimitView is the server maximum as it applies to an account.
type AutoDeleteLimitView struct {
	content.AutoDeleteLimit
	// Effective is the policy that the automatic deletion uses under the maximum (§4).
	Effective content.AutoDeletePolicy
	// Bound reports whether Effective deletes posts that the saved policy keeps (§5), and
	// EnforceFrom when the job starts to use Effective: zero until the job notices it.
	Bound       bool
	EnforceFrom time.Time
}

// AutoDeleteRepository is the part of the content module's persistence that the automatic
// deletion of old posts uses. The database adapters' status repositories implement it and pass
// contenttest.AutoDeleteContract; contenttest.AutoDeleteFlows (internal/flowtest/content) drives the use
// cases on them.
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
	// that are not deleted, were published before before, and that none of p's exceptions keeps;
	// with p.MaxKeptPins positive, only the newest that many of the author's pins keep posts.
	ListStatusesToAutoDelete(ctx context.Context, tx uow.Tx, p content.AutoDeletePolicy, before time.Time, limit int) ([]content.Status, error)
	// CountAutoDeletePins returns, inside tx, how many statuses the account pins.
	CountAutoDeletePins(ctx context.Context, tx uow.Tx, accountID string) (int, error)
	// FindAutoDeleteLimit returns the notice in effect of an account that the server maximum
	// binds (status_auto_delete_limits), or ErrNotFound; FindAutoDeleteLimitTx reads it in tx.
	FindAutoDeleteLimit(ctx context.Context, accountID string) (content.AutoDeleteLimitNotice, error)
	FindAutoDeleteLimitTx(ctx context.Context, tx uow.Tx, accountID string) (content.AutoDeleteLimitNotice, error)
	// RecordAutoDeleteLimitNotice inserts the notice n and makes it the notice in effect of its
	// account.
	RecordAutoDeleteLimitNotice(ctx context.Context, tx uow.Tx, n content.AutoDeleteLimitNotice) error
	// DeleteAutoDeleteLimit removes the notice in effect of the account, and DeleteAutoDeleteLimits
	// that of every account; the notices stay.
	DeleteAutoDeleteLimit(ctx context.Context, tx uow.Tx, accountID string) error
	DeleteAutoDeleteLimits(ctx context.Context, tx uow.Tx) error
	// FindAutoDeleteLimitNotices returns the notices among ids that exist, in ID order.
	FindAutoDeleteLimitNotices(ctx context.Context, ids []string) ([]content.AutoDeleteLimitNotice, error)
}

// AutoDeleteAccounts finds the accounts whose policies run; *account.Service implements it.
type AutoDeleteAccounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	// LocalAccountIDs pages the local accounts in ID order, the instance actor included.
	LocalAccountIDs(ctx context.Context, after string, limit int) ([]string, error)
}

// AutoDeleteServer reads what the operator set for the automatic deletion of every account
// (ADR-0151); the composition root implements it on the instance settings. A nil tx reads
// outside transactions.
type AutoDeleteServer interface {
	// AutoDeleteDefault returns the server default that a new local account starts from (§2), as
	// a policy without an account.
	AutoDeleteDefault(ctx context.Context, tx uow.Tx) (content.AutoDeletePolicy, error)
	// AutoDeleteLimit returns the server maximum (§3).
	AutoDeleteLimit(ctx context.Context, tx uow.Tx) (content.AutoDeleteLimit, error)
}

// AutoDeleteExemptions tells which users the server maximum does not hold: the holders of the
// permission keep_posts_longer (ADR-0151 §6). The composition root implements it on the identity
// module.
type AutoDeleteExemptions interface {
	KeepsPostsLonger(ctx context.Context, userID string) (bool, error)
}

// AutoDeleteDeps are the ports of the automatic deletion.
type AutoDeleteDeps struct {
	UnitOfWork  uow.UnitOfWork
	Repository  Repository
	AutoDeletes AutoDeleteRepository
	Accounts    AutoDeleteAccounts
	// Server is the server default and maximum; nil gives new accounts no policy row, so that
	// they have the built-in default, and sets no maximum.
	Server AutoDeleteServer
	// Exemptions tells the holders of keep_posts_longer; nil exempts nobody.
	Exemptions AutoDeleteExemptions
	// Jobs enqueues the next job of a run, outside any transaction.
	Jobs  queue.JobQueue
	Clock kernel.Clock
	// IDs makes the IDs of the notices of the maximum.
	IDs kernel.IDGenerator
	// Batch and PerAccount override AutoDeleteBatch and AutoDeletePerAccount when positive.
	Batch, PerAccount int
}

// AutoDelete is the automatic deletion of local accounts' own old posts (FR-118): the policy
// that each account sets, and the hourly job that carries the policies out. The job deletes a
// post as its author would (a deletion with origin auto_delete, which takes it out of every
// projection and sends a Delete to where the post went), so it is safe to repeat: a deleted post
// is not listed again. It skips the accounts that are not active (deactivated, or scheduled for
// deletion or deleted), whose posts stay until they are active again.
//
// The operator may set a server maximum (ADR-0151 §3): then every active local account without
// keep_posts_longer goes by its effective policy (content.EffectiveAutoDeletePolicy), the saved
// one cut to the maximum, but only after an advance notice and a grace period when the maximum
// newly binds it (§5).
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

// savedPolicy returns the account's saved policy, the built-in default if it has never saved one.
func (a *AutoDelete) savedPolicy(ctx context.Context, accountID string) (content.AutoDeletePolicy, error) {
	p, err := a.d.AutoDeletes.FindAutoDeletePolicy(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return content.DefaultAutoDeletePolicy(accountID), nil
	}
	return p, err
}

// limit returns the server maximum, none without a Server.
func (a *AutoDelete) limit(ctx context.Context, tx uow.Tx) (content.AutoDeleteLimit, error) {
	if a.d.Server == nil {
		return content.AutoDeleteLimit{}, nil
	}
	l, err := a.d.Server.AutoDeleteLimit(ctx, tx)
	if err != nil {
		return content.AutoDeleteLimit{}, fmt.Errorf("content: the auto delete maximum: %w", err)
	}
	return l, nil
}

// exempt reports whether the maximum leaves the local account actor alone: it has
// keep_posts_longer, or it is not an account of a user.
func (a *AutoDelete) exempt(ctx context.Context, actor appaccount.Actor) (bool, error) {
	if actor.Local == nil || actor.Local.UserID == "" {
		return true, nil
	}
	if a.d.Exemptions == nil {
		return false, nil
	}
	return a.d.Exemptions.KeepsPostsLonger(ctx, actor.Local.UserID)
}

// applicableLimit returns the server maximum when it applies to the account, ok false when there
// is none or the account is exempt.
func (a *AutoDelete) applicableLimit(ctx context.Context, accountID string) (limit content.AutoDeleteLimit, ok bool, err error) {
	if limit, err = a.limit(ctx, nil); err != nil || !limit.Enabled() {
		return limit, false, err
	}
	actor, err := a.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return limit, false, err
	}
	exempt, err := a.exempt(ctx, actor)
	if err != nil {
		return limit, false, err
	}
	return limit, limit.Applies(exempt), nil
}

// AutoDeletePolicy returns the account's automatic deletion: its saved policy, the default one if
// it has never saved one, and the server maximum when it applies.
func (a *AutoDelete) AutoDeletePolicy(ctx context.Context, accountID string) (AutoDeleteView, error) {
	p, err := a.savedPolicy(ctx, accountID)
	if err != nil {
		return AutoDeleteView{}, err
	}
	return a.view(ctx, p)
}

// view returns the view of the saved policy p.
func (a *AutoDelete) view(ctx context.Context, p content.AutoDeletePolicy) (AutoDeleteView, error) {
	limit, ok, err := a.applicableLimit(ctx, p.AccountID)
	if err != nil || !ok {
		return AutoDeleteView{Policy: p}, err
	}
	lv := &AutoDeleteLimitView{AutoDeleteLimit: limit, Effective: content.EffectiveAutoDeletePolicy(p, limit, false)}
	err = a.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		pins, err := a.d.AutoDeletes.CountAutoDeletePins(ctx, tx, p.AccountID)
		if err != nil {
			return err
		}
		lv.Bound = content.AutoDeleteBound(p, lv.Effective, pins)
		return nil
	})
	if err != nil {
		return AutoDeleteView{}, err
	}
	if lv.Bound {
		switch n, err := a.d.AutoDeletes.FindAutoDeleteLimit(ctx, p.AccountID); {
		case err == nil:
			lv.EnforceFrom = n.EnforceFrom
		case !errors.Is(err, ErrNotFound):
			return AutoDeleteView{}, err
		}
	}
	return AutoDeleteView{Policy: p, Limit: lv}, nil
}

// UpdateAutoDeletePolicy replaces the account's policy with p and returns it as saved, with the
// server maximum. It fails with a *content.AutoDeletePolicyError for an invalid value, wrapping
// content.ErrAutoDeleteOverLimit for one that the maximum does not allow to an account without
// keep_posts_longer (ADR-0151 §3). The next run of the job removes the notice in effect of an
// account that the saved policy put within the maximum.
func (a *AutoDelete) UpdateAutoDeletePolicy(ctx context.Context, accountID string, p content.AutoDeletePolicy) (AutoDeleteView, error) {
	p.AccountID = accountID
	p.UpdatedAt = kernel.Normalize(a.d.Clock.Now())
	p, err := p.Normalize()
	if err != nil {
		return AutoDeleteView{}, err
	}
	limit, applies, err := a.applicableLimit(ctx, accountID)
	if err != nil {
		return AutoDeleteView{}, err
	}
	if applies {
		if err := limit.Check(p); err != nil {
			return AutoDeleteView{}, err
		}
	}
	err = a.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return a.d.AutoDeletes.SaveAutoDeletePolicy(ctx, tx, p)
	})
	if err != nil {
		return AutoDeleteView{}, err
	}
	return a.view(ctx, p)
}

// RegisterProjectors registers the projector that gives a new local account the server default
// as its own policy, in the transaction that creates the account (ADR-0151 §2). Every way of
// creating a local account (the sign-up form, an external login provider, an invite, the CLI)
// emits account.local_account_created; the instance actor, which has no user, gets no policy.
// The policy written is the default under the server maximum (§4), so that a new account starts
// within it; a new account has no role yet, so the maximum applies to it. The policy is the
// account's from then on: a later change of the default changes nothing.
func (a *AutoDelete) RegisterProjectors(r *uow.Projectors) {
	r.Register(appaccount.EventLocalAccountCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appaccount.LocalAccountCreated)
		if a.d.Server == nil || ev.Local.UserID == "" {
			return nil
		}
		p, err := a.d.Server.AutoDeleteDefault(ctx, tx)
		if err != nil {
			return fmt.Errorf("content: the auto delete default: %w", err)
		}
		limit, err := a.limit(ctx, tx)
		if err != nil {
			return err
		}
		p = content.EffectiveAutoDeletePolicy(p, limit, false)
		p.MaxKeptPins = 0 // a cap of the effective policy, not a field of a saved one
		p.AccountID, p.UpdatedAt = ev.Account.ID, kernel.Normalize(a.d.Clock.Now())
		if p, err = p.Normalize(); err != nil {
			return err
		}
		return a.d.AutoDeletes.SaveAutoDeletePolicy(ctx, tx, p)
	})
}

// autoDeletePayload is the payload of JobAutoDeleteStatuses: the cursor of the run.
type autoDeletePayload struct {
	After string `json:"after,omitempty"`
}

// PreviewAutoDeletePolicy returns what p would delete of the account's posts now if it were
// enabled, without saving it. Under the server maximum it counts what the effective policy of p
// deletes (ADR-0151 §8), so that the preview of the saved policy of a bound account shows what
// goes after its grace period. It fails with a *content.AutoDeletePolicyError for an invalid
// value.
func (a *AutoDelete) PreviewAutoDeletePolicy(ctx context.Context, accountID string, p content.AutoDeletePolicy) (AutoDeletePreview, error) {
	p.AccountID = accountID
	p, err := p.Normalize()
	if err != nil {
		return AutoDeletePreview{}, err
	}
	limit, applies, err := a.applicableLimit(ctx, accountID)
	if err != nil {
		return AutoDeletePreview{}, err
	}
	if applies {
		p = content.EffectiveAutoDeletePolicy(p, limit, false)
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

// LimitNotices returns the notices of the server maximum among ids that went to the account, by
// ID, for the notifications that tell of them.
func (a *AutoDelete) LimitNotices(ctx context.Context, accountID string, ids []string) (map[string]content.AutoDeleteLimitNotice, error) {
	ns, err := a.d.AutoDeletes.FindAutoDeleteLimitNotices(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]content.AutoDeleteLimitNotice, len(ns))
	for _, n := range ns {
		if n.AccountID == accountID {
			out[n.ID] = n
		}
	}
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

// Run carries out the policies of the accounts after the cursor after ("" for the first), a
// bounded number of accounts and of posts per account. Without a server maximum it reads the
// enabled policies only, and its first job of a run clears the notices in effect, which nothing
// binds any more; with one, it reads every local account (ADR-0151 §5). It returns how many posts
// it deleted and the cursor to continue from, "" when it reached the last account. It is safe to
// repeat.
func (a *AutoDelete) Run(ctx context.Context, after string) (next string, deleted int, err error) {
	limit, err := a.limit(ctx, nil)
	if err != nil {
		return after, 0, err
	}
	if limit.Enabled() {
		return a.runLimited(ctx, after, limit)
	}
	if after == "" {
		err := a.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return a.d.AutoDeletes.DeleteAutoDeleteLimits(ctx, tx) })
		if err != nil {
			return after, 0, err
		}
	}
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

// runLimited is Run under the server maximum limit: it reads the local accounts.
func (a *AutoDelete) runLimited(ctx context.Context, after string, limit content.AutoDeleteLimit) (next string, deleted int, err error) {
	ids, err := a.d.Accounts.LocalAccountIDs(ctx, after, autoDeleteAccountsPerJob)
	if err != nil {
		return after, 0, err
	}
	for _, id := range ids {
		n, err := a.runAccount(ctx, id, limit)
		deleted += n
		if err != nil {
			return after, deleted, fmt.Errorf("auto delete the posts of %s: %w", id, err)
		}
		after = id
	}
	if len(ids) < autoDeleteAccountsPerJob {
		return "", deleted, nil
	}
	return after, deleted, nil
}

// runAccount carries out the automatic deletion of one local account under the maximum limit
// (ADR-0151 §5): it records a notice when the maximum newly binds the account, or binds it lower
// than its notice in effect told, and removes the notice in effect of an account that nothing
// binds any more. A bound account goes by its saved policy until the notice's EnforceFrom and by
// its effective policy from then on; any other account goes by its effective policy, which is its
// saved policy unless the cap on the kept pins changes nothing of its own pins.
func (a *AutoDelete) runAccount(ctx context.Context, accountID string, limit content.AutoDeleteLimit) (int, error) {
	actor, err := a.d.Accounts.FindByID(ctx, accountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !actor.Account.IsLocal() || actor.Local == nil || actor.Local.UserID == "" || !actor.Lifecycle.Active() {
		return 0, nil
	}
	saved, err := a.savedPolicy(ctx, accountID)
	if err != nil {
		return 0, err
	}
	exempt, err := a.exempt(ctx, actor)
	if err != nil {
		return 0, err
	}
	effective := content.EffectiveAutoDeletePolicy(saved, limit, exempt)
	now := kernel.Normalize(a.d.Clock.Now())
	use := effective
	// The transaction may run again (a retry); it reads the notice in effect again each time.
	err = a.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		pins := 0
		if effective.MaxKeptPins > 0 {
			var err error
			if pins, err = a.d.AutoDeletes.CountAutoDeletePins(ctx, tx, accountID); err != nil {
				return err
			}
		}
		current, err := a.d.AutoDeletes.FindAutoDeleteLimitTx(ctx, tx, accountID)
		found := err == nil
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !content.AutoDeleteBound(saved, effective, pins) {
			use = effective
			if found {
				return a.d.AutoDeletes.DeleteAutoDeleteLimit(ctx, tx, accountID)
			}
			return nil
		}
		if !found || current.LoweredBy(limit) {
			current = content.NewAutoDeleteLimitNotice(a.d.IDs.NewID(), accountID, limit, now)
			if err := a.d.AutoDeletes.RecordAutoDeleteLimitNotice(ctx, tx, current); err != nil {
				return err
			}
			if err := tx.Emit(ctx, AutoDeleteLimitNoticed{Notice: current}); err != nil {
				return err
			}
		}
		use = saved
		if current.Enforced(now) {
			use = effective
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return a.deletePosts(ctx, use, now)
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
	return a.deletePosts(ctx, p, kernel.Normalize(a.d.Clock.Now()))
}

// deletePosts deletes up to PerAccount posts that p deletes at now, none when p is disabled.
func (a *AutoDelete) deletePosts(ctx context.Context, p content.AutoDeletePolicy, now time.Time) (int, error) {
	if !p.Enabled {
		return 0, nil
	}
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
