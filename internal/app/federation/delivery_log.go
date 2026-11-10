// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The deliveries to a remote server and their attempts, as the admin API shows them to a holder
// of manage_domains (ADR-0058 §9): what this server sent to the server and what it answered.
// The activity type, the times, and every attempt are always shown. The local sender, the
// activity URI, and a personal inbox are masked when the activity is about a status that an
// anonymous visitor may not see (private, direct, local; content.CanView) or is a follow of a
// local account that approves its followers; Reveal shows them for one delivery, audited. A
// shared inbox names no recipient and is never masked. The body of the activity is never shown.

// DeliveryQuery selects the deliveries to the server Domain, newest first, before the delivery
// Before ("" for the newest), at most Limit of them: those of the activities of the local account
// ActorAccountID unless it is "", and of the activity ActivityURI unless it is "".
type DeliveryQuery struct {
	Domain         string
	ActorAccountID string
	ActivityURI    federation.IRI
	Before         string
	Limit          int
}

// DeliveryRecord is a delivery with its activity (Repository.ListDeliveriesTo).
type DeliveryRecord struct {
	Delivery federation.Delivery
	Activity federation.OutboundActivity
}

// DeliveryMask says why the admin API masks the sender, the activity URI, and a personal inbox of
// a delivery; "" is a delivery shown whole.
type DeliveryMask string

// The reasons of a mask: the visibility class of the status the activity is about, a follow of
// a local account that approves its followers, or an object that is gone, whose class is no
// longer known.
const (
	DeliveryMaskPrivate           DeliveryMask = "private"
	DeliveryMaskDirect            DeliveryMask = "direct"
	DeliveryMaskLocal             DeliveryMask = "local"
	DeliveryMaskApprovesFollowers DeliveryMask = "approves_followers"
	DeliveryMaskUnknown           DeliveryMask = "unknown"
)

// MaskedError replaces a masked inbox in the error of an attempt, which can quote the URL it
// failed to reach.
const MaskedError = "[masked]"

// DeliverySender is the local account that sent an activity; Username is "" for an account that
// is gone.
type DeliverySender struct {
	AccountID string
	Username  string
}

// DeliveryEntry is a delivery as the admin API lists it. Sender, ActivityURI, and a personal
// InboxURL are zero when Mask is set.
type DeliveryEntry struct {
	ID           string
	Domain       string
	ActivityType federation.ActivityType
	// ActivityAt is when the activity was recorded; CreatedAt when the inbox was added to it.
	ActivityAt  time.Time
	CreatedAt   time.Time
	SharedInbox bool
	Mask        DeliveryMask
	Sender      *DeliverySender
	ActivityURI federation.IRI
	InboxURL    federation.IRI
	// Attempts are the attempts of the delivery, oldest first, kept for DeliveryAttemptRetention.
	Attempts []federation.DeliveryAttempt
}

// RevealedDelivery is what Reveal shows of one delivery: the fields the list masks.
type RevealedDelivery struct {
	ID          string
	Mask        DeliveryMask
	Sender      DeliverySender
	ActivityURI federation.IRI
	InboxURL    federation.IRI
}

// SharedInboxes says which URLs remote actors declare as their shared inbox; *appaccount.Service
// implements it.
type SharedInboxes interface {
	SharedInboxesAmong(ctx context.Context, urls []string) (map[string]bool, error)
}

// ErrNotLocalAccount reports a filter by an account that is not a local one.
var ErrNotLocalAccount = errors.New("federation: not a local account")

// DeliveryLogDeps are the ports of DeliveryLog.
type DeliveryLogDeps struct {
	UnitOfWork    uow.UnitOfWork
	Repository    Repository
	Statuses      Statuses
	Accounts      Accounts
	SharedInboxes SharedInboxes
	Clock         kernel.Clock
	IDs           kernel.IDGenerator
}

// DeliveryLog lists the deliveries to a remote server and reveals the masked fields of one.
type DeliveryLog struct{ d DeliveryLogDeps }

// NewDeliveryLog returns the DeliveryLog on d.
func NewDeliveryLog(d DeliveryLogDeps) *DeliveryLog { return &DeliveryLog{d: d} }

// List returns the deliveries that q selects, each with its attempts and masked as the package
// documentation says, for byUserID ("" for the CLI). A query by a local account is audited
// (account_deliveries_viewed) in a transaction of its own before anything is returned; it fails
// with ErrNotLocalAccount for an account that is not local, and with an error wrapping
// appaccount.ErrNotFound for an unknown one.
func (l *DeliveryLog) List(ctx context.Context, q DeliveryQuery, byUserID string) ([]DeliveryEntry, error) {
	if q.ActorAccountID != "" {
		if err := l.auditAccountQuery(ctx, q, byUserID); err != nil {
			return nil, err
		}
	}
	recs, err := l.d.Repository.ListDeliveriesTo(ctx, q)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(recs))
	inboxes := make([]string, 0, len(recs))
	for i, r := range recs {
		ids[i] = r.Delivery.ID
		inboxes = append(inboxes, string(r.Delivery.InboxURL))
	}
	attempts, err := l.d.Repository.ListAttemptsOfDeliveries(ctx, ids)
	if err != nil {
		return nil, err
	}
	shared, err := l.sharedInboxes(ctx, inboxes)
	if err != nil {
		return nil, err
	}
	m := l.masker()
	out := make([]DeliveryEntry, 0, len(recs))
	for _, r := range recs {
		mask, err := m.mask(ctx, r.Activity)
		if err != nil {
			return nil, err
		}
		e := DeliveryEntry{
			ID: r.Delivery.ID, Domain: r.Delivery.Domain, ActivityType: r.Activity.Type, ActivityAt: r.Activity.CreatedAt,
			CreatedAt: r.Delivery.CreatedAt, SharedInbox: shared[string(r.Delivery.InboxURL)], Mask: mask,
			Attempts: attempts[r.Delivery.ID],
		}
		if e.Attempts == nil {
			e.Attempts = []federation.DeliveryAttempt{}
		}
		if mask == "" {
			sender, err := m.sender(ctx, r.Activity.ActorAccountID)
			if err != nil {
				return nil, err
			}
			e.Sender, e.ActivityURI = &sender, r.Activity.ActivityURI
		}
		if mask == "" || e.SharedInbox {
			e.InboxURL = r.Delivery.InboxURL
		} else {
			e.Attempts = maskAttemptErrors(e.Attempts, r.Delivery.InboxURL)
		}
		out = append(out, e)
	}
	return out, nil
}

// Reveal returns the sender, the activity URI, and the inbox of the delivery id to the server
// domain, for byUserID ("" for the CLI). The call is recorded in the audit log
// (delivery_revealed) in a transaction of its own before anything is returned, also for a
// delivery the list shows whole. It fails with an error wrapping ErrNotFound for an unknown
// delivery and for a delivery to another server.
func (l *DeliveryLog) Reveal(ctx context.Context, domain, id, byUserID string) (RevealedDelivery, error) {
	del, err := l.d.Repository.FindDelivery(ctx, id)
	if err != nil {
		return RevealedDelivery{}, err
	}
	if del.Domain != domain {
		return RevealedDelivery{}, fmt.Errorf("delivery %s to %s: %w", id, domain, ErrNotFound)
	}
	act, err := l.d.Repository.FindOutboundActivity(ctx, del.OutboundActivityID)
	if err != nil {
		return RevealedDelivery{}, err
	}
	m := l.masker()
	mask, err := m.mask(ctx, act)
	if err != nil {
		return RevealedDelivery{}, err
	}
	sender, err := m.sender(ctx, act.ActorAccountID)
	if err != nil {
		return RevealedDelivery{}, err
	}
	audit := appmoderation.AuditEntry{
		ID: l.d.IDs.NewID(), Action: appmoderation.AuditDeliveryRevealed, TargetKind: appmoderation.AuditTargetDomain,
		TargetID: domain, TargetLabel: domain, SourceEventID: del.ID, OccurredAt: kernel.Normalize(l.d.Clock.Now()),
	}
	audit.ActorKind, audit.ActorUserID = appmoderation.AuditActor(byUserID)
	if err := l.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: audit}) }); err != nil {
		return RevealedDelivery{}, fmt.Errorf("federation: audit the reveal of delivery %s: %w", id, err)
	}
	return RevealedDelivery{ID: del.ID, Mask: mask, Sender: sender, ActivityURI: act.ActivityURI, InboxURL: del.InboxURL}, nil
}

// auditAccountQuery records that byUserID listed the deliveries of the local account of q to its
// server.
func (l *DeliveryLog) auditAccountQuery(ctx context.Context, q DeliveryQuery, byUserID string) error {
	actor, err := l.d.Accounts.FindByID(ctx, q.ActorAccountID)
	if err != nil {
		return err
	}
	if actor.Local == nil {
		return ErrNotLocalAccount
	}
	audit := appmoderation.AuditEntry{
		ID: l.d.IDs.NewID(), Action: appmoderation.AuditAccountDeliveriesViewed, TargetKind: appmoderation.AuditTargetAccount,
		TargetID: q.ActorAccountID, TargetLabel: "@" + actor.Local.Username + " (" + q.Domain + ")",
		OccurredAt: kernel.Normalize(l.d.Clock.Now()),
	}
	audit.ActorKind, audit.ActorUserID = appmoderation.AuditActor(byUserID)
	if err := l.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: audit}) }); err != nil {
		return fmt.Errorf("federation: audit the deliveries of %s to %s: %w", q.ActorAccountID, q.Domain, err)
	}
	return nil
}

func (l *DeliveryLog) sharedInboxes(ctx context.Context, urls []string) (map[string]bool, error) {
	if len(urls) == 0 || l.d.SharedInboxes == nil {
		return map[string]bool{}, nil
	}
	return l.d.SharedInboxes.SharedInboxesAmong(ctx, urls)
}

func (l *DeliveryLog) masker() *deliveryMasker {
	return &deliveryMasker{d: l.d, actors: map[string]appaccount.Actor{}, masks: map[string]DeliveryMask{}}
}

// deliveryMasker decides the masks of the deliveries of one call, reading each object and
// account once.
type deliveryMasker struct {
	d      DeliveryLogDeps
	actors map[string]appaccount.Actor
	masks  map[string]DeliveryMask // by outbound activity
}

// mask returns the mask of the deliveries of act.
func (m *deliveryMasker) mask(ctx context.Context, act federation.OutboundActivity) (DeliveryMask, error) {
	if mask, ok := m.masks[act.ID]; ok {
		return mask, nil
	}
	mask, err := m.decide(ctx, act)
	if err != nil {
		return "", err
	}
	m.masks[act.ID] = mask
	return mask, nil
}

func (m *deliveryMasker) decide(ctx context.Context, act federation.OutboundActivity) (DeliveryMask, error) {
	switch act.ObjectKind {
	case federation.OutboundStatus, federation.OutboundFavourite, federation.OutboundPollVote:
		st, err := m.d.Statuses.FindStatus(ctx, act.ObjectID)
		if errors.Is(err, appcontent.ErrNotFound) {
			return DeliveryMaskUnknown, nil
		}
		if err != nil {
			return "", err
		}
		return visibilityMask(appcontent.StatusFacts{AuthorAccountID: st.AuthorAccountID, AuthorIsLocal: st.IsLocal, Visibility: st.Visibility}), nil
	case federation.OutboundBoost:
		b, err := m.d.Statuses.FindBoost(ctx, act.ObjectID)
		if errors.Is(err, appcontent.ErrNotFound) {
			return DeliveryMaskUnknown, nil
		}
		if err != nil {
			return "", err
		}
		return visibilityMask(appcontent.StatusFacts{AuthorAccountID: b.BoosterAccountID, AuthorIsLocal: b.IsLocal, Visibility: b.Visibility}), nil
	case federation.OutboundFollow, federation.OutboundRefusal:
		actor, found, err := m.actor(ctx, act.ActorAccountID)
		if err != nil {
			return "", err
		}
		if !found {
			return DeliveryMaskUnknown, nil
		}
		if actor.Profile != nil && actor.Profile.Locked {
			return DeliveryMaskApprovesFollowers, nil
		}
	}
	return "", nil
}

// visibilityMask masks what an anonymous visitor may not see of the status, by the single
// visibility policy function; the mask names the status's visibility class.
func visibilityMask(facts appcontent.StatusFacts) DeliveryMask {
	if appcontent.CanView(appcontent.ContextStatus, appcontent.Viewer{Kind: appcontent.ViewerAnonymous}, facts,
		appcontent.AuthorRestrictions{}, appcontent.Relation{}) {
		return ""
	}
	switch facts.Visibility {
	case appcontent.VisibilityPrivate:
		return DeliveryMaskPrivate
	case appcontent.VisibilityDirect:
		return DeliveryMaskDirect
	case appcontent.VisibilityLocal:
		return DeliveryMaskLocal
	}
	return DeliveryMaskUnknown
}

func (m *deliveryMasker) actor(ctx context.Context, id string) (appaccount.Actor, bool, error) {
	if a, ok := m.actors[id]; ok {
		return a, true, nil
	}
	a, err := m.d.Accounts.FindByID(ctx, id)
	if errors.Is(err, appaccount.ErrNotFound) {
		return appaccount.Actor{}, false, nil
	}
	if err != nil {
		return appaccount.Actor{}, false, err
	}
	m.actors[id] = a
	return a, true, nil
}

// sender returns the local account id; an account that is gone has no username.
func (m *deliveryMasker) sender(ctx context.Context, id string) (DeliverySender, error) {
	a, found, err := m.actor(ctx, id)
	if err != nil {
		return DeliverySender{}, err
	}
	s := DeliverySender{AccountID: id}
	if found && a.Local != nil {
		s.Username = a.Local.Username
	}
	return s, nil
}

// maskAttemptErrors returns attempts with inbox replaced by MaskedError in their errors.
func maskAttemptErrors(attempts []federation.DeliveryAttempt, inbox federation.IRI) []federation.DeliveryAttempt {
	out := make([]federation.DeliveryAttempt, len(attempts))
	for i, a := range attempts {
		if inbox != "" {
			a.Error = strings.ReplaceAll(a.Error, string(inbox), MaskedError)
		}
		out[i] = a
	}
	return out
}
