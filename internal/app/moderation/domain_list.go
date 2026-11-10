// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The subscriptions to domain block lists that others publish (ADR-0159 §4, §5). A subscription
// is fetched every day (JobFetchDomainLists queues JobFetchDomainList for each) and on a
// moderator's request: the fetch reads the list through the SSRF-guarded client outside any
// transaction, records the fetch and replaces the subscription's entries in one transaction,
// and then, in the automatic mode, issues and lifts the subscription's actions a few dozen at a
// time. Every step reads what is in the database again, so running a fetch twice issues
// nothing twice. A failed fetch changes nothing but its record and the count of failures. The
// actions a subscription issues are ordinary domain actions, audited and sent to the webhooks
// as any other; a subscription lifts only the actions it issued (subscription_domain_actions).

// DomainListRepository is the part of Repository that keeps the subscriptions to domain block
// lists. Reads that take a transaction read outside any when tx is nil.
type DomainListRepository interface {
	InsertDomainListSubscription(ctx context.Context, tx uow.Tx, s moderation.DomainListSubscription) error
	InsertDomainListRevision(ctx context.Context, tx uow.Tx, r moderation.DomainListRevision) error
	// InsertDomainListDeletion fails with ErrAlreadyRecorded when the subscription is already
	// deleted.
	InsertDomainListDeletion(ctx context.Context, tx uow.Tx, d moderation.DomainListDeletion) error
	// PutDomainListHead writes the current value of a subscription.
	PutDomainListHead(ctx context.Context, tx uow.Tx, h moderation.DomainListHead) error
	// LockDomainListSubscription serializes, inside tx, the transactions that write the entries
	// and the actions of one subscription.
	LockDomainListSubscription(ctx context.Context, tx uow.Tx, id string) error
	// DomainListSubscription returns a subscription, deleted ones included; it fails with
	// ErrNotFound for an unknown ID.
	DomainListSubscription(ctx context.Context, tx uow.Tx, id string) (moderation.DomainListSubscriptionRecord, error)
	// ListDomainListSubscriptions returns up to limit subscriptions that are not deleted, newest
	// first, whose ID is below before (from the newest when empty).
	ListDomainListSubscriptions(ctx context.Context, before string, limit int) ([]moderation.DomainListSubscriptionRecord, error)
	InsertDomainListFetch(ctx context.Context, tx uow.Tx, f moderation.DomainListFetch) error
	// ListDomainListFetches returns up to limit fetches of a subscription, newest first, whose ID
	// is below before (from the newest when empty).
	ListDomainListFetches(ctx context.Context, subscriptionID, before string, limit int) ([]moderation.DomainListFetch, error)
	// DomainListEntries returns the entries of a subscription by domain.
	DomainListEntries(ctx context.Context, tx uow.Tx, subscriptionID string) ([]moderation.DomainListEntry, error)
	// DomainListEntry returns the entry of a domain, and false when the list does not hold it.
	DomainListEntry(ctx context.Context, tx uow.Tx, subscriptionID, domain string) (moderation.DomainListEntry, bool, error)
	// PutDomainListEntries writes entries, replacing those of the same subscription and domain.
	PutDomainListEntries(ctx context.Context, tx uow.Tx, entries []moderation.DomainListEntry) error
	// DeleteDomainListEntries removes the entries of the domains of a subscription; with no
	// domains, every entry of the subscription.
	DeleteDomainListEntries(ctx context.Context, tx uow.Tx, subscriptionID string, domains []string) error
	InsertSubscriptionDomainAction(ctx context.Context, tx uow.Tx, a moderation.SubscriptionDomainAction) error
	// SubscriptionOfDomainAction returns the subscription that issued an action, and false
	// for an action that a person issued.
	SubscriptionOfDomainAction(ctx context.Context, tx uow.Tx, actionID string) (moderation.SubscriptionDomainAction, bool, error)
	// SubscriptionDomainActionsInEffect returns the actions in effect at now that a subscription
	// issued, by ID: those on domain, or on every domain when it is "".
	SubscriptionDomainActionsInEffect(ctx context.Context, tx uow.Tx, subscriptionID, domain string, now time.Time) ([]moderation.DomainAction, error)
	InsertDomainListRejection(ctx context.Context, tx uow.Tx, r moderation.DomainListRejection) error
	// DomainListRejections returns the rejections of a subscription by ID.
	DomainListRejections(ctx context.Context, tx uow.Tx, subscriptionID string) ([]moderation.DomainListRejection, error)
}

const (
	// JobFetchDomainLists queues JobFetchDomainList for every subscription; no payload. It runs
	// every DomainListInterval.
	JobFetchDomainLists = "moderation.fetch_domain_lists"
	// JobFetchDomainList fetches one subscribed list and applies it; its payload is
	// fetchDomainListPayload.
	JobFetchDomainList = "moderation.fetch_domain_list"
	// DomainListInterval is how often every subscribed list is fetched.
	DomainListInterval = 24 * time.Hour
	// domainListChunk is the number of actions that one transaction of a subscription issues or
	// lifts.
	domainListChunk = 50
	// domainListPage is the number of subscriptions that one read of the daily fetch lists.
	domainListPage = 200
)

// ErrDomainListNotFetchable reports a fetch without the HTTP client that fetches lists.
var ErrDomainListNotFetchable = errors.New("moderation: the server does not fetch domain lists")

// EventDomainListFailing follows the fetch that fails FailingDomainListFetches times in a row.
const EventDomainListFailing = "moderation.domain_list_failing"

// DomainListFailing tells that the fetches of a subscription keep failing.
type DomainListFailing struct {
	SubscriptionID string
	Title          string
	FetchID        string
	At             time.Time
}

// EventType implements uow.Event.
func (DomainListFailing) EventType() string { return EventDomainListFailing }

type fetchDomainListPayload struct {
	SubscriptionID string `json:"subscription_id"`
}

// DomainListSubscriptionInput is a new subscription.
type DomainListSubscriptionInput struct {
	URL      string
	Format   moderation.DomainListFormat
	Settings moderation.DomainListSettings
	// CreatedByUserID is the moderator; "" for the CLI.
	CreatedByUserID string
}

// CreateDomainListSubscription subscribes to the list at in.URL, an https URL, with the
// settings in.Settings, audits it, and queues its first fetch. It fails with an error wrapping
// moderation.ErrInvalidDomainList for an invalid URL, format, or setting.
func (s *Service) CreateDomainListSubscription(ctx context.Context, in DomainListSubscriptionInput) (moderation.DomainListSubscriptionRecord, error) {
	in.URL = strings.TrimSpace(in.URL)
	in.Settings.Title = strings.TrimSpace(in.Settings.Title)
	if err := moderation.CheckDomainListURL(in.URL); err != nil {
		return moderation.DomainListSubscriptionRecord{}, err
	}
	if err := in.Format.Check(); err != nil {
		return moderation.DomainListSubscriptionRecord{}, err
	}
	if err := in.Settings.Validate(); err != nil {
		return moderation.DomainListSubscriptionRecord{}, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	sub := moderation.DomainListSubscription{ID: s.d.IDs.NewID(), URL: in.URL, Format: in.Format, CreatedByUserID: in.CreatedByUserID, CreatedAt: now}
	rev := moderation.DomainListRevision{ID: s.d.IDs.NewID(), SubscriptionID: sub.ID, Settings: in.Settings, RevisedByUserID: in.CreatedByUserID, RevisedAt: now}
	auditID := s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.InsertDomainListSubscription(ctx, tx, sub); err != nil {
			return err
		}
		if err := repo.InsertDomainListRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := repo.PutDomainListHead(ctx, tx, moderation.DomainListHead{SubscriptionID: sub.ID}); err != nil {
			return err
		}
		if err := s.enqueueDomainListFetch(ctx, tx, sub.ID, rev.ID); err != nil {
			return err
		}
		return s.emitAudit(ctx, tx, domainListAudit(auditID, moderation.AuditDomainListSubscriptionCreated, sub.ID, in.Settings.Title, sub.ID, now), in.CreatedByUserID)
	})
	if err != nil {
		return moderation.DomainListSubscriptionRecord{}, err
	}
	return s.DomainListSubscription(ctx, sub.ID)
}

// UpdateDomainListSubscription records new settings of a subscription, audits them, and queues
// a fetch that applies them; the same settings change nothing. It fails with ErrNotFound for an
// unknown or deleted subscription and with an error wrapping moderation.ErrInvalidDomainList
// for invalid settings.
func (s *Service) UpdateDomainListSubscription(ctx context.Context, id string, settings moderation.DomainListSettings, byUserID string) (moderation.DomainListSubscriptionRecord, error) {
	settings.Title = strings.TrimSpace(settings.Title)
	if err := settings.Validate(); err != nil {
		return moderation.DomainListSubscriptionRecord{}, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	rev := moderation.DomainListRevision{ID: s.d.IDs.NewID(), SubscriptionID: id, Settings: settings, RevisedByUserID: byUserID, RevisedAt: now}
	auditID := s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.LockDomainListSubscription(ctx, tx, id); err != nil {
			return err
		}
		rec, err := s.liveDomainList(ctx, tx, id)
		if err != nil {
			return err
		}
		if rec.Revision.Settings == settings {
			return nil
		}
		if err := repo.InsertDomainListRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := s.enqueueDomainListFetch(ctx, tx, id, rev.ID); err != nil {
			return err
		}
		return s.emitAudit(ctx, tx, domainListAudit(auditID, moderation.AuditDomainListSubscriptionUpdated, id, settings.Title, rev.ID, now), byUserID)
	})
	if err != nil {
		return moderation.DomainListSubscriptionRecord{}, err
	}
	return s.DomainListSubscription(ctx, id)
}

// DeleteDomainListSubscription deletes a subscription and audits it. With liftActions, it first
// lifts every action the subscription issued, as byUserID; without, the actions stay, and are
// then like the actions a person issued. It fails with ErrNotFound for an unknown or deleted
// subscription.
func (s *Service) DeleteDomainListSubscription(ctx context.Context, id string, liftActions bool, byUserID string) error {
	rec, err := s.liveDomainList(ctx, nil, id)
	if err != nil {
		return err
	}
	if liftActions {
		now := kernel.Normalize(s.d.Clock.Now())
		own, err := s.d.Repository.SubscriptionDomainActionsInEffect(ctx, nil, id, "", now)
		if err != nil {
			return err
		}
		changes := make([]moderation.DomainListChange, len(own))
		for i, a := range own {
			changes[i] = moderation.DomainListChange{Change: moderation.DomainListLift, Domain: a.Domain, Kind: a.Kind, ActionID: a.ID}
		}
		if _, err := s.applyDomainListChanges(ctx, rec, changes, domainListActor{userID: byUserID}); err != nil {
			return err
		}
	}
	now := kernel.Normalize(s.d.Clock.Now())
	auditID := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.LockDomainListSubscription(ctx, tx, id); err != nil {
			return err
		}
		err := repo.InsertDomainListDeletion(ctx, tx, moderation.DomainListDeletion{SubscriptionID: id, LiftActions: liftActions, DeletedByUserID: byUserID, DeletedAt: now})
		if errors.Is(err, ErrAlreadyRecorded) {
			return fmt.Errorf("domain list subscription %s: %w", id, ErrNotFound)
		}
		if err != nil {
			return err
		}
		if err := repo.DeleteDomainListEntries(ctx, tx, id, nil); err != nil {
			return err
		}
		return s.emitAudit(ctx, tx, domainListAudit(auditID, moderation.AuditDomainListSubscriptionDeleted, id, rec.Revision.Settings.Title, id, now), byUserID)
	})
}

// DomainListSubscription returns a subscription that is not deleted; it fails with ErrNotFound
// otherwise.
func (s *Service) DomainListSubscription(ctx context.Context, id string) (moderation.DomainListSubscriptionRecord, error) {
	return s.liveDomainList(ctx, nil, id)
}

// DomainListSubscriptions returns up to limit subscriptions that are not deleted, newest first,
// those older than the subscription before ("" for the first page).
func (s *Service) DomainListSubscriptions(ctx context.Context, before string, limit int) ([]moderation.DomainListSubscriptionRecord, error) {
	return s.d.Repository.ListDomainListSubscriptions(ctx, before, limit)
}

// DomainListFetches returns up to limit fetches of a subscription, newest first, those older
// than the fetch before ("" for the first page). It fails with ErrNotFound for an unknown or
// deleted subscription.
func (s *Service) DomainListFetches(ctx context.Context, id, before string, limit int) ([]moderation.DomainListFetch, error) {
	if _, err := s.liveDomainList(ctx, nil, id); err != nil {
		return nil, err
	}
	return s.d.Repository.ListDomainListFetches(ctx, id, before, limit)
}

// DomainListDrafts returns the drafts of a subscription, the differences that wait for a
// moderator (moderation.DiffDomainList), the lifts first. It fails with ErrNotFound for an
// unknown or deleted subscription.
func (s *Service) DomainListDrafts(ctx context.Context, id string) ([]moderation.DomainListChange, error) {
	rec, err := s.liveDomainList(ctx, nil, id)
	if err != nil {
		return nil, err
	}
	changes, err := s.domainListDiff(ctx, rec)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(changes, func(c moderation.DomainListChange) bool { return c.Held == "" }), nil
}

// DomainListDraftResult is what accepting or rejecting drafts did: the drafts it handled, and the
// keys it was given that name no draft.
type DomainListDraftResult struct {
	Done    []moderation.DomainListChange
	Missing []string
}

// AcceptDomainListDrafts accepts the drafts of a subscription whose keys (DomainListChange.Key)
// are given, or every draft when keys is empty: an issue issues its action as byUserID for the
// subscription, a lift lifts the subscription's action. Each is audited as accepted, besides the
// audit of the action. A draft that an action in effect covers by then issues nothing. It fails
// with ErrNotFound for an unknown or deleted subscription.
func (s *Service) AcceptDomainListDrafts(ctx context.Context, id string, keys []string, byUserID string) (DomainListDraftResult, error) {
	rec, drafts, res, err := s.selectDomainListDrafts(ctx, id, keys)
	if err != nil {
		return res, err
	}
	done, err := s.applyDomainListChanges(ctx, rec, drafts, domainListActor{userID: byUserID, accept: true})
	res.Done = done
	if err != nil {
		return res, err
	}
	if slices.ContainsFunc(done, func(c moderation.DomainListChange) bool { return c.Change == moderation.DomainListLift }) {
		err = s.reissueFromOtherDomainLists(ctx, id)
	}
	return res, err
}

// RejectDomainListDrafts rejects the drafts of a subscription whose keys are given, or every
// draft when keys is empty, and audits each: a rejected draft does not come back until the list
// changes the entry it comes from (an issue) or the action ends (a lift). It fails with
// ErrNotFound for an unknown or deleted subscription.
func (s *Service) RejectDomainListDrafts(ctx context.Context, id string, keys []string, byUserID string) (DomainListDraftResult, error) {
	rec, drafts, res, err := s.selectDomainListDrafts(ctx, id, keys)
	if err != nil {
		return res, err
	}
	for chunk := range slices.Chunk(drafts, domainListChunk) {
		now := kernel.Normalize(s.d.Clock.Now())
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			if err := s.d.Repository.LockDomainListSubscription(ctx, tx, id); err != nil {
				return err
			}
			for _, c := range chunk {
				r := domainListRejection(s.d.IDs.NewID(), id, c, byUserID, now)
				if err := s.d.Repository.InsertDomainListRejection(ctx, tx, r); err != nil {
					return err
				}
				e := domainListAudit(s.d.IDs.NewID(), moderation.AuditDomainListDraftRejected, id, rec.Revision.Settings.Title, r.ID, now)
				if err := s.emitAudit(ctx, tx, e, byUserID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return res, err
		}
		res.Done = append(res.Done, chunk...)
	}
	return res, nil
}

// selectDomainListDrafts returns a live subscription and its drafts named by keys (all of them
// when keys is empty), with the keys that name none in the result.
func (s *Service) selectDomainListDrafts(ctx context.Context, id string, keys []string) (moderation.DomainListSubscriptionRecord, []moderation.DomainListChange, DomainListDraftResult, error) {
	var res DomainListDraftResult
	rec, err := s.liveDomainList(ctx, nil, id)
	if err != nil {
		return rec, nil, res, err
	}
	drafts, err := s.DomainListDrafts(ctx, id)
	if err != nil || len(keys) == 0 {
		return rec, drafts, res, err
	}
	byKey := make(map[string]moderation.DomainListChange, len(drafts))
	for _, d := range drafts {
		byKey[d.Key()] = d
	}
	var out []moderation.DomainListChange
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		if d, ok := byKey[k]; ok {
			out = append(out, d)
		} else {
			res.Missing = append(res.Missing, k)
		}
	}
	return rec, out, res, nil
}

// QueueDomainListFetch queues a fetch of a subscription now and audits the request. It fails
// with ErrNotFound for an unknown or deleted subscription.
func (s *Service) QueueDomainListFetch(ctx context.Context, id, byUserID string) error {
	now := kernel.Normalize(s.d.Clock.Now())
	auditID := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := s.liveDomainList(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.enqueueDomainListFetch(ctx, tx, id, auditID); err != nil {
			return err
		}
		return s.emitAudit(ctx, tx, domainListAudit(auditID, moderation.AuditDomainListFetchQueued, id, rec.Revision.Settings.Title, "", now), byUserID)
	})
}

// FetchDomainListNow audits a request to fetch a subscription now and fetches it at once
// (FetchDomainList), for the CLI.
func (s *Service) FetchDomainListNow(ctx context.Context, id, byUserID string) (moderation.DomainListFetch, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	auditID := s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := s.liveDomainList(ctx, tx, id)
		if err != nil {
			return err
		}
		return s.emitAudit(ctx, tx, domainListAudit(auditID, moderation.AuditDomainListFetchQueued, id, rec.Revision.Settings.Title, "", now), byUserID)
	})
	if err != nil {
		return moderation.DomainListFetch{}, err
	}
	return s.FetchDomainList(ctx, id)
}

// enqueueDomainListFetch queues JobFetchDomainList for a subscription in tx; key tells the
// requests apart (a revision, an audit entry, a day of the schedule).
func (s *Service) enqueueDomainListFetch(ctx context.Context, tx uow.Tx, id, key string) error {
	if s.d.Outbox == nil {
		return nil
	}
	payload, err := json.Marshal(fetchDomainListPayload{SubscriptionID: id})
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{
		Type: JobFetchDomainList, Payload: payload, IdempotencyKey: JobFetchDomainList + ":" + id + ":" + key, OrderingKey: JobFetchDomainList + ":" + id,
	})
}

// liveDomainList returns a subscription that is not deleted, inside tx when it is not nil.
func (s *Service) liveDomainList(ctx context.Context, tx uow.Tx, id string) (moderation.DomainListSubscriptionRecord, error) {
	rec, err := s.d.Repository.DomainListSubscription(ctx, tx, id)
	if err != nil {
		return rec, err
	}
	if rec.Deletion != nil {
		return rec, fmt.Errorf("domain list subscription %s is deleted: %w", id, ErrNotFound)
	}
	return rec, nil
}

func domainListAudit(id string, action moderation.AuditAction, subscriptionID, title, sourceEventID string, at time.Time) moderation.AuditEntry {
	return moderation.AuditEntry{
		ID: id, Action: action, TargetKind: moderation.AuditTargetDomainListSubscription, TargetID: subscriptionID,
		TargetLabel: title, SourceEventID: sourceEventID, OccurredAt: at,
	}
}

func domainListRejection(id, subscriptionID string, c moderation.DomainListChange, byUserID string, at time.Time) moderation.DomainListRejection {
	r := moderation.DomainListRejection{
		ID: id, SubscriptionID: subscriptionID, Change: c.Change, Domain: c.Domain, Kind: c.Kind,
		RejectedByUserID: byUserID, RejectedAt: at,
	}
	if c.Change == moderation.DomainListIssue {
		r.EntrySinceFetchID = c.Entry.SinceFetchID
	} else {
		r.ActionID = c.ActionID
	}
	return r
}

// domainListDiff returns every difference of a subscription (moderation.DiffDomainList).
func (s *Service) domainListDiff(ctx context.Context, rec moderation.DomainListSubscriptionRecord) ([]moderation.DomainListChange, error) {
	repo := s.d.Repository
	id := rec.Subscription.ID
	now := kernel.Normalize(s.d.Clock.Now())
	entries, err := repo.DomainListEntries(ctx, nil, id)
	if err != nil {
		return nil, err
	}
	own, err := repo.SubscriptionDomainActionsInEffect(ctx, nil, id, "", now)
	if err != nil {
		return nil, err
	}
	covered, err := s.domainCoverage(ctx)
	if err != nil {
		return nil, err
	}
	rejections, err := repo.DomainListRejections(ctx, nil, id)
	if err != nil {
		return nil, err
	}
	return moderation.DiffDomainList(moderation.DomainListPlan{
		Settings: rec.Revision.Settings, Entries: entries, Own: own, Covered: covered, Rejections: rejections,
	}), nil
}

// domainListActor is who applies the changes of a subscription: the server in the automatic
// mode, or a moderator (userID, "" for the CLI) who accepts drafts or deletes the subscription.
type domainListActor struct {
	system bool
	userID string
	// accept audits each change as an accepted draft.
	accept bool
}

// applyDomainListChanges carries out changes of a subscription, the lifts first, a few dozen
// in each transaction, and returns those it carried out. It reads again in each transaction
// what the subscription has in effect on the domain: an issue whose kind the subscription
// already has in effect, or that an action issued before in the same run covers, issues
// nothing, and a lift of an action that has ended changes nothing, so carrying out the same
// changes twice issues nothing twice.
func (s *Service) applyDomainListChanges(ctx context.Context, rec moderation.DomainListSubscriptionRecord, changes []moderation.DomainListChange, by domainListActor) ([]moderation.DomainListChange, error) {
	id := rec.Subscription.ID
	settings := rec.Revision.Settings
	issued := map[string][]moderation.DomainActionKind{}
	var done []moderation.DomainListChange
	ordered := slices.Concat(
		slices.DeleteFunc(slices.Clone(changes), func(c moderation.DomainListChange) bool { return c.Change != moderation.DomainListLift }),
		slices.DeleteFunc(slices.Clone(changes), func(c moderation.DomainListChange) bool { return c.Change != moderation.DomainListIssue }),
	)
	for chunk := range slices.Chunk(ordered, domainListChunk) {
		var applied []moderation.DomainListChange
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			applied = applied[:0]
			repo := s.d.Repository
			if err := repo.LockDomainListSubscription(ctx, tx, id); err != nil {
				return err
			}
			now := kernel.Normalize(s.d.Clock.Now())
			for _, c := range chunk {
				actionID, err := s.applyDomainListChange(ctx, tx, rec, settings, c, by, issued, now)
				if err != nil {
					return err
				}
				if actionID == "" {
					continue
				}
				applied = append(applied, c)
				if by.accept {
					e := domainListAudit(s.d.IDs.NewID(), moderation.AuditDomainListDraftAccepted, id, settings.Title, actionID, now)
					if err := s.emitAudit(ctx, tx, e, by.userID); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return done, err
		}
		done = append(done, applied...)
	}
	return done, nil
}

// applyDomainListChange carries out one change inside tx and returns the action it issued or
// lifted, or "" when it changed nothing. issued collects the kinds of the actions issued by
// domain.
func (s *Service) applyDomainListChange(ctx context.Context, tx uow.Tx, rec moderation.DomainListSubscriptionRecord, settings moderation.DomainListSettings,
	c moderation.DomainListChange, by domainListActor, issued map[string][]moderation.DomainActionKind, now time.Time,
) (string, error) {
	repo := s.d.Repository
	id := rec.Subscription.ID
	if c.Change == moderation.DomainListLift {
		a, err := repo.DomainAction(ctx, tx, c.ActionID)
		if err != nil {
			return "", err
		}
		lifted, err := s.liftDomainAction(ctx, tx, a, by.userID, by.system, false, now)
		if err != nil || !lifted {
			return "", err
		}
		return a.ID, nil
	}
	if err := repo.LockDomainRestrictions(ctx, tx, c.Domain); err != nil {
		return "", err
	}
	if moderation.CoveredBy(issued, c.Domain, c.Kind) {
		return "", nil
	}
	own, err := repo.SubscriptionDomainActionsInEffect(ctx, tx, id, c.Domain, now)
	if err != nil {
		return "", err
	}
	if slices.ContainsFunc(own, func(a moderation.DomainAction) bool { return a.Kind == c.Kind }) {
		return "", nil
	}
	a := moderation.DomainAction{
		ID: s.d.IDs.NewID(), Domain: c.Domain, Kind: c.Kind, Obfuscate: c.Entry.Obfuscate, IssuedByUserID: by.userID, IssuedAt: now,
	}
	if settings.PublicCommentFromList {
		a.PublicComment = c.Entry.PublicComment
	}
	if err := a.Validate(); err != nil {
		// A comment of the list that this server cannot keep issues the action without it.
		a.PublicComment = ""
		if err := a.Validate(); err != nil {
			return "", err
		}
	}
	if err := s.insertDomainActionAs(ctx, tx, a, s.d.IDs.NewID(), by.system); err != nil {
		return "", err
	}
	if err := repo.InsertSubscriptionDomainAction(ctx, tx, moderation.SubscriptionDomainAction{ActionID: a.ID, SubscriptionID: id, FetchID: rec.Head.LastFetchID}); err != nil {
		return "", err
	}
	issued[c.Domain] = append(issued[c.Domain], c.Kind)
	return a.ID, nil
}

// rejectLiftOfSubscriptionAction records, inside tx, the lift of a subscription's action by a
// person as a rejected issue of its entry, so that the subscription does not issue it again
// while the list lists the domain alike.
func (s *Service) rejectLiftOfSubscriptionAction(ctx context.Context, tx uow.Tx, a moderation.DomainAction, byUserID string, now time.Time) error {
	repo := s.d.Repository
	link, ok, err := repo.SubscriptionOfDomainAction(ctx, tx, a.ID)
	if err != nil || !ok {
		return err
	}
	rec, err := repo.DomainListSubscription(ctx, tx, link.SubscriptionID)
	if errors.Is(err, ErrNotFound) || (err == nil && rec.Deletion != nil) {
		return nil
	}
	if err != nil {
		return err
	}
	e, ok, err := repo.DomainListEntry(ctx, tx, link.SubscriptionID, a.Domain)
	if err != nil || !ok || !slices.Contains(rec.Revision.Settings.Kinds(e), a.Kind) {
		return err
	}
	c := moderation.DomainListChange{Change: moderation.DomainListIssue, Domain: a.Domain, Kind: a.Kind, Entry: e}
	return repo.InsertDomainListRejection(ctx, tx, domainListRejection(s.d.IDs.NewID(), link.SubscriptionID, c, byUserID, now))
}

// FetchDomainList fetches the list of a subscription, records the fetch, and replaces the
// subscription's entries with what it read, then, in the automatic mode, applies the
// differences that wait for no moderator (applyDomainList). A failure to fetch or read the list
// is recorded and changes nothing else; after FailingDomainListFetches of them in a row, it
// emits DomainListFailing. It returns the fetch, and fails with ErrNotFound for an unknown or
// deleted subscription.
func (s *Service) FetchDomainList(ctx context.Context, id string) (moderation.DomainListFetch, error) {
	rec, err := s.liveDomainList(ctx, nil, id)
	if err != nil {
		return moderation.DomainListFetch{}, err
	}
	got := s.downloadDomainList(ctx, rec)
	var fetch moderation.DomainListFetch
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		fetch, err = s.recordDomainListFetch(ctx, tx, id, got)
		return err
	})
	if err != nil {
		return fetch, err
	}
	if fetch.Outcome == moderation.DomainListFailed {
		return fetch, nil
	}
	return fetch, s.applyDomainList(ctx, id, false)
}

// domainListDownload is what a fetch read: the rows of the list, or that it did not change,
// or why it failed.
type domainListDownload struct {
	status       int
	notModified  bool
	errCode      moderation.DomainListFetchError
	rows         []moderation.DomainBlockRow
	etag         string
	lastModified string
	digest       string
}

// downloadDomainList fetches and parses the list of rec, outside any transaction.
func (s *Service) downloadDomainList(ctx context.Context, rec moderation.DomainListSubscriptionRecord) domainListDownload {
	if s.d.HTTP == nil {
		return domainListDownload{errCode: moderation.DomainListErrorNetwork}
	}
	h := http.Header{}
	switch rec.Subscription.Format {
	case moderation.DomainListMastodonJSON:
		h.Set("Accept", "application/json")
	default:
		h.Set("Accept", "text/csv, text/plain;q=0.9, */*;q=0.1")
	}
	if rec.Head.ETag != "" {
		h.Set("If-None-Match", rec.Head.ETag)
	}
	if rec.Head.LastModified != "" {
		h.Set("If-Modified-Since", rec.Head.LastModified)
	}
	resp, err := s.d.HTTP.Do(ctx, &kernel.HTTPRequest{URL: rec.Subscription.URL, Header: h, Purpose: kernel.HTTPPurposeDomainList})
	if err != nil {
		return domainListDownload{errCode: domainListFetchError(err)}
	}
	out := domainListDownload{status: resp.StatusCode, etag: resp.Header.Get("ETag"), lastModified: resp.Header.Get("Last-Modified")}
	if resp.StatusCode == http.StatusNotModified {
		_ = resp.Body.Close()
		out.notModified = true
		return out
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_ = resp.Body.Close()
		out.errCode = moderation.DomainListErrorHTTPStatus
		return out
	}
	body, err := resp.ReadAll()
	if err != nil {
		out.errCode = domainListFetchError(err)
		return out
	}
	if len(body) > moderation.MaxDomainListBytes {
		out.errCode = moderation.DomainListErrorTooLarge
		return out
	}
	sum := sha256.Sum256(body)
	out.digest = hex.EncodeToString(sum[:])
	if out.digest == rec.Head.Digest {
		out.notModified = true
		return out
	}
	rows, err := moderation.ParseDomainList(rec.Subscription.Format, body)
	switch {
	case errors.Is(err, moderation.ErrTooManyDomainBlocks):
		out.errCode = moderation.DomainListErrorTooManyEntries
	case err != nil:
		out.errCode = moderation.DomainListErrorMalformed
	}
	out.rows = rows
	return out
}

// domainListFetchError classifies an error of the HTTP client.
func domainListFetchError(err error) moderation.DomainListFetchError {
	switch {
	case errors.Is(err, kernel.ErrForbiddenDestination), errors.Is(err, kernel.ErrInvalidHTTPRequest):
		return moderation.DomainListErrorForbiddenDestination
	case errors.Is(err, kernel.ErrResponseTooLarge):
		return moderation.DomainListErrorTooLarge
	}
	return moderation.DomainListErrorNetwork
}

// recordDomainListFetch records what a fetch read inside tx: the fetch, the head, and, for a
// list read, the entries that changed.
func (s *Service) recordDomainListFetch(ctx context.Context, tx uow.Tx, id string, got domainListDownload) (moderation.DomainListFetch, error) {
	repo := s.d.Repository
	if err := repo.LockDomainListSubscription(ctx, tx, id); err != nil {
		return moderation.DomainListFetch{}, err
	}
	rec, err := s.liveDomainList(ctx, tx, id)
	if err != nil {
		return moderation.DomainListFetch{}, err
	}
	now := kernel.Normalize(s.d.Clock.Now())
	fetch := moderation.DomainListFetch{ID: s.d.IDs.NewID(), SubscriptionID: id, HTTPStatus: got.status, FetchedAt: now}
	current, err := repo.DomainListEntries(ctx, tx, id)
	if err != nil {
		return fetch, err
	}
	head := rec.Head
	head.LastFetchID = fetch.ID
	fetch.EntryCount = len(current)
	switch {
	case got.errCode != "":
		fetch.Outcome, fetch.ErrorCode = moderation.DomainListFailed, got.errCode
		head.ConsecutiveFailures++
	case got.notModified:
		fetch.Outcome = moderation.DomainListNotModified
		head.ConsecutiveFailures = 0
		head.ETag, head.LastModified = cmpOr(got.etag, head.ETag), cmpOr(got.lastModified, head.LastModified)
	default:
		fetch.Outcome = moderation.DomainListApplied
		head.ConsecutiveFailures = 0
		head.ETag, head.LastModified, head.Digest = got.etag, got.lastModified, got.digest
		entries := s.domainListEntries(id, fetch.ID, got.rows)
		fetch.EntryCount = len(entries)
		if err := s.replaceDomainListEntries(ctx, tx, id, current, entries); err != nil {
			return fetch, err
		}
	}
	if err := repo.InsertDomainListFetch(ctx, tx, fetch); err != nil {
		return fetch, err
	}
	if err := repo.PutDomainListHead(ctx, tx, head); err != nil {
		return fetch, err
	}
	if head.ConsecutiveFailures == moderation.FailingDomainListFetches {
		return fetch, tx.Emit(ctx, DomainListFailing{SubscriptionID: id, Title: rec.Revision.Settings.Title, FetchID: fetch.ID, At: now})
	}
	return fetch, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// domainListEntries turns the rows of a list into entries of fetch: domains in canonical form,
// the first row of each, without the invalid domains and this instance's.
func (s *Service) domainListEntries(id, fetchID string, rows []moderation.DomainBlockRow) []moderation.DomainListEntry {
	out := make([]moderation.DomainListEntry, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		domain, err := s.canonicalDomain(r.Domain)
		if err != nil || !moderation.ValidDomain(domain) || s.ownDomain(domain) || seen[domain] {
			continue
		}
		seen[domain] = true
		severity := string(r.Severity)
		severity = strings.ReplaceAll(strings.ToValidUTF8(severity[:min(len(severity), moderation.MaxListedSeverity)], ""), "\x00", "")
		comment := r.PublicComment
		if moderation.CheckDomainActionTexts(comment, "") != nil {
			comment = ""
		}
		out = append(out, moderation.DomainListEntry{
			SubscriptionID: id, Domain: domain, Severity: moderation.DomainBlockSeverity(severity), RejectMedia: r.RejectMedia,
			RejectReports: r.RejectReports, PublicComment: comment, Obfuscate: r.Obfuscate, SinceFetchID: fetchID,
		})
	}
	return out
}

// replaceDomainListEntries writes the entries of a fetch over current: a domain listed alike
// keeps the fetch it is listed since, a new or changed one gets the fetch's, and the domains no
// longer listed are removed.
func (s *Service) replaceDomainListEntries(ctx context.Context, tx uow.Tx, id string, current, entries []moderation.DomainListEntry) error {
	before := make(map[string]moderation.DomainListEntry, len(current))
	for _, e := range current {
		before[e.Domain] = e
	}
	var put []moderation.DomainListEntry
	listed := make(map[string]bool, len(entries))
	for _, e := range entries {
		listed[e.Domain] = true
		if old, ok := before[e.Domain]; ok && old.SameListing(e) {
			continue
		}
		put = append(put, e)
	}
	var gone []string
	for _, e := range current {
		if !listed[e.Domain] {
			gone = append(gone, e.Domain)
		}
	}
	if len(gone) > 0 {
		if err := s.d.Repository.DeleteDomainListEntries(ctx, tx, id, gone); err != nil {
			return err
		}
	}
	if len(put) == 0 {
		return nil
	}
	return s.d.Repository.PutDomainListEntries(ctx, tx, put)
}

// applyDomainList carries out, for a subscription in the automatic mode, the differences that
// wait for no moderator, as the server. When it lifts an action, the other subscriptions in the
// automatic mode issue again what they list and the lift uncovered (reissueFromOtherDomainLists);
// with issuesOnly, it carries out only the issues, for that.
func (s *Service) applyDomainList(ctx context.Context, id string, issuesOnly bool) error {
	rec, err := s.liveDomainList(ctx, nil, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil || rec.Revision.Settings.Mode != moderation.DomainListAutomatic {
		return err
	}
	changes, err := s.domainListDiff(ctx, rec)
	if err != nil {
		return err
	}
	changes = slices.DeleteFunc(changes, func(c moderation.DomainListChange) bool {
		return c.Held != "" || (issuesOnly && c.Change == moderation.DomainListLift)
	})
	done, err := s.applyDomainListChanges(ctx, rec, changes, domainListActor{system: true})
	if err != nil || issuesOnly {
		return err
	}
	if slices.ContainsFunc(done, func(c moderation.DomainListChange) bool { return c.Change == moderation.DomainListLift }) {
		return s.reissueFromOtherDomainLists(ctx, id)
	}
	return nil
}

// reissueFromOtherDomainLists lets every subscription in the automatic mode other than except
// issue what it lists and an action that except lifted covered: of several subscriptions that
// list a domain, the action of the one that issued first is in effect, and when it is lifted,
// the next one issues its own.
func (s *Service) reissueFromOtherDomainLists(ctx context.Context, except string) error {
	before := ""
	var ids []string
	for {
		recs, err := s.d.Repository.ListDomainListSubscriptions(ctx, before, domainListPage)
		if err != nil {
			return err
		}
		for _, r := range recs {
			if r.Subscription.ID != except && r.Revision.Settings.Mode == moderation.DomainListAutomatic {
				ids = append(ids, r.Subscription.ID)
			}
		}
		if len(recs) < domainListPage {
			break
		}
		before = recs[len(recs)-1].Subscription.ID
	}
	// The oldest subscription issues first.
	slices.Sort(ids)
	for _, id := range ids {
		if err := s.applyDomainList(ctx, id, true); err != nil {
			return err
		}
	}
	return nil
}

// registerDomainListJobs registers the fetch of the subscribed lists on reg and its daily
// schedule on sched.
func (s *Service) registerDomainListJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobFetchDomainLists, func(ctx context.Context, j queue.Job) queue.Result {
		if err := s.queueDomainListFetches(ctx, j.ID); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	reg.Register(JobFetchDomainList, func(ctx context.Context, j queue.Job) queue.Result {
		var p fetchDomainListPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.SubscriptionID == "" {
			return queue.Fail(fmt.Errorf("moderation: payload of %s: %q", JobFetchDomainList, j.Payload))
		}
		_, err := s.FetchDomainList(ctx, p.SubscriptionID)
		switch {
		case errors.Is(err, ErrNotFound):
			return queue.Done() // deleted since it was queued
		case err != nil:
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobFetchDomainLists, Every: DomainListInterval, Job: queue.NewJob{Type: JobFetchDomainLists}})
}

// queueDomainListFetches queues JobFetchDomainList for every subscription that is not deleted,
// keyed by run, the job of the schedule.
func (s *Service) queueDomainListFetches(ctx context.Context, run string) error {
	before := ""
	for {
		recs, err := s.d.Repository.ListDomainListSubscriptions(ctx, before, domainListPage)
		if err != nil {
			return err
		}
		if len(recs) > 0 {
			err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				for _, r := range recs {
					if err := s.enqueueDomainListFetch(ctx, tx, r.Subscription.ID, "run:"+run); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		if len(recs) < domainListPage {
			return nil
		}
		before = recs[len(recs)-1].Subscription.ID
	}
}

// DomainListErrorText returns a short English text of a fetch's failure, for the CLI.
func DomainListErrorText(f moderation.DomainListFetch) string {
	if f.ErrorCode == moderation.DomainListErrorHTTPStatus && f.HTTPStatus != 0 {
		return string(f.ErrorCode) + " " + strconv.Itoa(f.HTTPStatus)
	}
	return string(f.ErrorCode)
}
