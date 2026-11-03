// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Refetching remote posts from their origin on an operator's request (FR-407, federation.md
// §6.4). Nothing schedules it: only the administration API and the CLI enqueue it.
const (
	// JobRefetchStatus fetches one stored remote post again and records what its origin serves
	// now: a new revision, the counts of its poll, or its deletion.
	JobRefetchStatus = "federation.refetch_status"
	// JobRefetchStatuses walks the remote posts of an account, or of the accounts of a domain,
	// newest first, and enqueues a JobRefetchStatus for each, one batch per run.
	JobRefetchStatuses = "federation.refetch_walk"
	// RefetchMaxStatuses bounds the posts that one request for an account or a domain refetches.
	RefetchMaxStatuses = 1000
	// RefetchStatusesPerAccount bounds the posts of each account that a request for a domain
	// refetches: the newest of each account, so that a few busy accounts do not use up the
	// request.
	RefetchStatusesPerAccount = 20
	// RefetchBatch is the number of posts, and RefetchAccountsBatch the number of accounts of a
	// domain, that one JobRefetchStatuses reads; RefetchBatchInterval is how long the next batch
	// waits, so that a request does not fetch from one server all at once.
	RefetchBatch         = 100
	RefetchAccountsBatch = 5
	RefetchBatchInterval = time.Minute
)

// ErrNotRemote reports a refetch of a local post or account, or of this server's domain: only
// remote posts are refetched.
var ErrNotRemote = errors.New("federation: the target is not remote")

// refetchStatusPayload is the payload of JobRefetchStatus.
type refetchStatusPayload struct {
	StatusID string `json:"status_id"`
}

func refetchStatusJob(statusID string) (queue.NewJob, error) {
	b, err := json.Marshal(refetchStatusPayload{StatusID: statusID})
	if err != nil {
		return queue.NewJob{}, err
	}
	// A pending refetch of the post stands in for another one.
	return queue.NewJob{Type: JobRefetchStatus, Payload: b, IdempotencyKey: "refetch_status:" + statusID}, nil
}

// refetchStatusesPayload is the payload of JobRefetchStatuses: the request (its audit log
// entry), what it refetches (an account or a domain), the cursor, and the posts it has left.
type refetchStatusesPayload struct {
	Request   string `json:"request"`
	AccountID string `json:"account_id,omitempty"`
	Domain    string `json:"domain,omitempty"`
	// Before is the last post read of the account, and BeforeAccount the last account read of
	// the domain; "" starts from the newest.
	Before        string `json:"before,omitempty"`
	BeforeAccount string `json:"before_account,omitempty"`
	Remaining     int    `json:"remaining"`
}

func refetchStatusesJob(p refetchStatusesPayload, runAt time.Time) (queue.NewJob, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return queue.NewJob{}, err
	}
	return queue.NewJob{
		Type: JobRefetchStatuses, Payload: b, RunAt: runAt,
		IdempotencyKey: "refetch_statuses:" + p.Request + ":" + p.BeforeAccount + ":" + p.Before,
	}, nil
}

// Refetch fetches the stored remote post statusID again from its origin (federation.md §6.4)
// and records what the origin serves now, with the rules of a received Update: a newer revision
// with its mentions, the counts of its poll, nothing when the document is unchanged. A document
// that is gone (410 Gone, or a Tombstone with the post's id) deletes the post as its author's
// Delete does. A post that is local, deleted, or unknown is left as it is; so is one whose
// document cannot be fetched or used, or whose attributedTo is another account. It fails with
// the errors of Fetcher.Fetch other than ErrFetchGone; other errors come from the stores.
func (h *StatusHandlers) Refetch(ctx context.Context, statusID string) error {
	st, err := h.d.Statuses.FindStatus(ctx, statusID)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return nil
	case err != nil:
		return err
	case st.IsLocal:
		return nil
	}
	if deleted, err := h.d.Statuses.Tombstoned(ctx, st.URI); err != nil || deleted {
		return err
	}
	got, err := h.d.Fetcher.Fetch(ctx, st.URI)
	switch {
	case errors.Is(err, ErrFetchGone):
		return h.refetchedGone(ctx, st)
	case err != nil:
		return err
	}
	obj := got.Document.Object
	switch {
	case obj == nil || obj.ID != federation.IRI(st.URI):
		return fmt.Errorf("%w: %s serves %s", ErrNotTaken, st.URI, got.Document.ID())
	case obj.Type == federation.ObjectTombstone:
		return h.refetchedGone(ctx, st)
	case !isPost(obj.Type):
		return fmt.Errorf("%w: %s is a %s, not a post", ErrNotTaken, obj.ID, obj.RawType)
	case obj.AttributedTo == "":
		return fmt.Errorf("%w: %s has no attributedTo", ErrNotTaken, obj.ID)
	}
	author, err := h.d.Accounts.FindByURI(ctx, string(obj.AttributedTo))
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return fmt.Errorf("%w: %s is attributed to the unknown %s", ErrNotTaken, obj.ID, obj.AttributedTo)
	case err != nil:
		return err
	case author.Account.ID != st.AuthorAccountID:
		return fmt.Errorf("%w: %s is attributed to %s, not to its author", ErrNotTaken, obj.ID, obj.AttributedTo)
	}
	followers, err := h.followers(ctx, author)
	if err != nil {
		return err
	}
	mentions, err := h.mentions(ctx, obj, followers)
	if err != nil {
		return err
	}
	if err := h.resolveMentions(ctx, mentions); err != nil {
		return err
	}
	in := h.inputOf(author, federation.Audience{}, "", obj, followers, mentions)
	err = h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := h.d.Statuses.ReviseRemoteStatus(ctx, tx, in)
		return err
	})
	if errors.Is(err, appcontent.ErrNotFound) {
		return nil // deleted meanwhile
	}
	return err
}

// refetchedGone deletes st, whose origin says it is gone, as its author's Delete does.
func (h *StatusHandlers) refetchedGone(ctx context.Context, st appcontent.Status) error {
	err := h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return h.d.Statuses.DeleteRemoteStatus(ctx, tx, st.URI, st.AuthorAccountID)
	})
	if errors.Is(err, appcontent.ErrNotFound) || errors.Is(err, appcontent.ErrAlreadyDeleted) {
		return nil
	}
	return err
}

// handleRefetchStatus is the queue.Handler of JobRefetchStatus. A document that cannot be
// fetched or used ends the job with a log; a fetch that failed is remembered by the Fetcher
// (§6.1), and the operator can request the refetch again.
func (h *StatusHandlers) handleRefetchStatus(ctx context.Context, j queue.Job) queue.Result {
	var p refetchStatusPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.StatusID == "" {
		return queue.Fail(fmt.Errorf("federation: refetch_status payload %s: %w", j.Payload, err))
	}
	err := h.Refetch(ctx, p.StatusID)
	switch {
	case err == nil:
		return queue.Done()
	case isFetchError(err), errors.Is(err, ErrNotTaken):
		h.d.Logger.InfoContext(ctx, "remote status not refetched", slog.String("status_id", p.StatusID), slog.Any("reason", err))
		return queue.Done()
	}
	return queue.Retry(0, err)
}

// RefetchStatuses is the part of the content module's public API that finds the posts to
// refetch; *appcontent.Service implements it.
type RefetchStatuses interface {
	FindStatus(ctx context.Context, id string) (appcontent.Status, error)
	Tombstoned(ctx context.Context, uri string) (bool, error)
	LiveRemoteStatusIDs(ctx context.Context, authorAccountID, before string, limit int) ([]string, error)
}

// RefetchAccounts is the part of the account module's public API that finds the accounts whose
// posts are refetched; *appaccount.Service implements it.
type RefetchAccounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	Search(ctx context.Context, q appaccount.SearchQuery, limit int) ([]string, error)
}

// RefetchesDeps are the ports of Refetches.
type RefetchesDeps struct {
	UnitOfWork uow.UnitOfWork
	Statuses   RefetchStatuses
	Accounts   RefetchAccounts
	// Outbox enqueues the first job of a request with its audit log entry; Jobs enqueues the
	// jobs that a walk leads to.
	Outbox queue.Outbox
	Jobs   queue.JobQueue
	Clock  kernel.Clock
	IDs    kernel.IDGenerator
	// Host is the serving domain, which is never refetched.
	Host string
}

// Refetches queues the refetch of remote posts that an operator requests (FR-407,
// federation.md §6.4): one post, the posts of a remote account, or those of the accounts of a
// remote domain. Each request is recorded in the audit log in the transaction that enqueues it.
type Refetches struct {
	d RefetchesDeps
}

// NewRefetches returns the Refetches on d.
func NewRefetches(d RefetchesDeps) *Refetches { return &Refetches{d: d} }

// RegisterJobs registers the handlers of JobRefetchStatus (on h) and JobRefetchStatuses. Neither
// is scheduled.
func (r *Refetches) RegisterJobs(reg *queue.JobHandlerRegistry, h *StatusHandlers) {
	reg.Register(JobRefetchStatus, h.handleRefetchStatus)
	reg.Register(JobRefetchStatuses, r.handleRefetchStatuses)
}

// RefetchStatus queues the refetch of the remote post statusID; byUserID is the operator, "" for
// the CLI. A post that is unknown or deleted fails with ErrNotFound, a local one with
// ErrNotRemote.
func (r *Refetches) RefetchStatus(ctx context.Context, statusID, byUserID string) error {
	st, err := r.d.Statuses.FindStatus(ctx, statusID)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return fmt.Errorf("%w: status %s", ErrNotFound, statusID)
	case err != nil:
		return err
	case st.IsLocal:
		return fmt.Errorf("%w: status %s", ErrNotRemote, statusID)
	}
	if deleted, err := r.d.Statuses.Tombstoned(ctx, st.URI); err != nil {
		return err
	} else if deleted {
		return fmt.Errorf("%w: status %s is deleted", ErrNotFound, statusID)
	}
	job, err := refetchStatusJob(st.ID)
	if err != nil {
		return err
	}
	return r.request(ctx, appmoderation.AuditEntry{
		Action: appmoderation.AuditStatusRefetchQueued, TargetKind: appmoderation.AuditTargetStatus,
		TargetID: st.ID, TargetLabel: st.URI,
	}, byUserID, func(string) (queue.NewJob, error) { return job, nil })
}

// RefetchAccount queues the refetch of the newest RefetchMaxStatuses remote posts of the
// account accountID that are not deleted. An unknown account fails with ErrNotFound, a local
// one with ErrNotRemote.
func (r *Refetches) RefetchAccount(ctx context.Context, accountID, byUserID string) error {
	a, err := r.d.Accounts.FindByID(ctx, accountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return fmt.Errorf("%w: account %s", ErrNotFound, accountID)
	case err != nil:
		return err
	case a.Account.IsLocal():
		return fmt.Errorf("%w: account %s", ErrNotRemote, accountID)
	}
	label := a.Account.URI
	if a.Handle != nil {
		label = "@" + a.Handle.Username + "@" + a.Handle.Domain
	}
	return r.request(ctx, appmoderation.AuditEntry{
		Action: appmoderation.AuditAccountStatusesRefetchQueued, TargetKind: appmoderation.AuditTargetAccount,
		TargetID: a.Account.ID, TargetLabel: label,
	}, byUserID, func(request string) (queue.NewJob, error) {
		return refetchStatusesJob(refetchStatusesPayload{Request: request, AccountID: a.Account.ID, Remaining: RefetchMaxStatuses}, time.Time{})
	})
}

// RefetchDomain queues the refetch of the remote posts of the accounts of domain: the newest
// RefetchStatusesPerAccount posts of each account, newest accounts first, RefetchMaxStatuses in
// all. It returns the canonical domain. A domain that is not a host fails with ErrInvalidHost,
// this server's with ErrNotRemote.
func (r *Refetches) RefetchDomain(ctx context.Context, domain, byUserID string) (string, error) {
	host, err := CanonicalHost(domain)
	if err != nil {
		return "", err
	}
	host = appaccount.NormalizeDomain(host)
	if host == "" || strings.EqualFold(host, r.d.Host) {
		return "", fmt.Errorf("%w: domain %q", ErrNotRemote, domain)
	}
	return host, r.request(ctx, appmoderation.AuditEntry{
		Action: appmoderation.AuditDomainStatusesRefetchQueued, TargetKind: appmoderation.AuditTargetDomain,
		TargetID: host, TargetLabel: host,
	}, byUserID, func(request string) (queue.NewJob, error) {
		return refetchStatusesJob(refetchStatusesPayload{Request: request, Domain: host, Remaining: RefetchMaxStatuses}, time.Time{})
	})
}

// request records the audit log entry e of a request and enqueues its first job, which job
// builds from the ID of the entry, in one transaction.
func (r *Refetches) request(ctx context.Context, e appmoderation.AuditEntry, byUserID string, job func(request string) (queue.NewJob, error)) error {
	e.ID, e.OccurredAt = r.d.IDs.NewID(), kernel.Normalize(r.d.Clock.Now())
	e.ActorKind, e.ActorUserID = appmoderation.AuditActor(byUserID)
	j, err := job(e.ID)
	if err != nil {
		return err
	}
	return r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := tx.Emit(ctx, appmoderation.AuditRecorded{Entry: e}); err != nil {
			return err
		}
		return r.d.Outbox.EnqueueTx(ctx, tx, j)
	})
}

// handleRefetchStatuses is the queue.Handler of JobRefetchStatuses.
func (r *Refetches) handleRefetchStatuses(ctx context.Context, j queue.Job) queue.Result {
	var p refetchStatusesPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.Request == "" || (p.AccountID == "") == (p.Domain == "") {
		return queue.Fail(fmt.Errorf("federation: refetch_statuses payload %s: %w", j.Payload, err))
	}
	jobs, err := r.walk(ctx, p)
	if err == nil && len(jobs) > 0 {
		err = r.d.Jobs.Enqueue(ctx, jobs...)
	}
	if err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// walk reads one batch of a request and returns the refetches of its posts, followed by the
// next batch when posts are left, delayed by RefetchBatchInterval.
func (r *Refetches) walk(ctx context.Context, p refetchStatusesPayload) ([]queue.NewJob, error) {
	if p.Remaining <= 0 {
		return nil, nil
	}
	var (
		ids  []string
		next *refetchStatusesPayload
	)
	if p.AccountID != "" {
		batch, err := r.d.Statuses.LiveRemoteStatusIDs(ctx, p.AccountID, p.Before, min(RefetchBatch, p.Remaining))
		if err != nil {
			return nil, err
		}
		ids = batch
		if len(batch) == min(RefetchBatch, p.Remaining) && len(batch) < p.Remaining {
			n := p
			n.Before, n.Remaining = batch[len(batch)-1], p.Remaining-len(batch)
			next = &n
		}
	} else {
		accounts, err := r.d.Accounts.Search(ctx, appaccount.SearchQuery{
			Domain: p.Domain, Origin: appaccount.OriginRemote, Before: p.BeforeAccount,
		}, RefetchAccountsBatch)
		if err != nil {
			return nil, err
		}
		for _, id := range accounts {
			if len(ids) == p.Remaining {
				break
			}
			batch, err := r.d.Statuses.LiveRemoteStatusIDs(ctx, id, "", min(RefetchStatusesPerAccount, p.Remaining-len(ids)))
			if err != nil {
				return nil, err
			}
			ids = append(ids, batch...)
		}
		if len(accounts) == RefetchAccountsBatch && len(ids) < p.Remaining {
			n := p
			n.BeforeAccount, n.Remaining = accounts[len(accounts)-1], p.Remaining-len(ids)
			next = &n
		}
	}
	jobs := make([]queue.NewJob, 0, len(ids)+1)
	for _, id := range ids {
		job, err := refetchStatusJob(id)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if next != nil {
		job, err := refetchStatusesJob(*next, r.d.Clock.Now().Add(RefetchBatchInterval))
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}
