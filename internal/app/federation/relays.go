// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
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

// The relays the instance joins (ADR-0061, FR-411, FR-724, federation.md §13): the operator
// registers a relay and the instance actor follows it; public posts go to the accepted relays as
// the last phase of their fan-out, and the posts that relays bring in are fetched again from
// their origin, within an hourly limit per relay.

// Errors of the relay use cases.
var (
	// ErrRelayExists reports a relay whose URL is already registered.
	ErrRelayExists = errors.New("federation: the relay is already registered")
	// ErrRelayDomainSuspended reports a relay on a server that a domain block suspends.
	ErrRelayDomainSuspended = errors.New("federation: the relay's server is suspended")
	// ErrRelayNotActor reports a LitePub relay whose URL does not serve an actor with an inbox.
	ErrRelayNotActor = errors.New("federation: the relay URL is not an actor")
	// ErrRelayNotRemovable reports the deletion of a relay that the instance is a member of.
	ErrRelayNotRemovable = errors.New("federation: the relay must be left or rejected before it is removed")
)

// DefaultRelayIngestHourlyLimit is how many activities a relay may bring in per hour
// (relay_ingest_hourly_limit), and DefaultRelayStatusRetention how long a post only a relay
// brought in is kept (relay_status_retention_days).
const (
	DefaultRelayIngestHourlyLimit = 1000
	DefaultRelayStatusRetention   = 7 * 24 * time.Hour
)

// KV namespaces of the relays: the activities each relay brought in, per hour (UTC), whose
// first relay_ingest_hourly_limit are taken in and the rest dropped.
const (
	RelayIngestNamespace = "federation.relay_ingest"
	relayCountTTL        = 25 * time.Hour
)

// RelayRecord is a relay with the current state of its membership. Status.State is "" for a
// relay without one, which a registration never leaves.
type RelayRecord struct {
	Relay  federation.Relay
	Status federation.RelayStatus
}

// RelayRepository is the persistence port of the relays (relays, relay_events, relay_states,
// data-model §4.8). The database adapters implement it and pass testkit.RelayContract. Methods
// that take a uow.Tx run inside that transaction; the others read committed data. Lookups that
// find nothing return an error wrapping ErrNotFound.
type RelayRepository interface {
	// InsertRelay registers a relay; it fails with ErrRelayExists when its URL is registered.
	InsertRelay(ctx context.Context, tx uow.Tx, r federation.Relay) error
	// LockRelay reads a relay and its status in tx, locking the relay's row where the database
	// needs it, so that concurrent changes of one relay do not lose each other.
	LockRelay(ctx context.Context, tx uow.Tx, id string) (RelayRecord, error)
	// InsertRelayEvent records an event unless the relay has one of its kind about its Follow,
	// and reports whether it did.
	InsertRelayEvent(ctx context.Context, tx uow.Tx, e federation.RelayEvent) (bool, error)
	// UpsertRelayState replaces the status of a relay; only the use cases of this file write it.
	UpsertRelayState(ctx context.Context, tx uow.Tx, s federation.RelayStatus) error
	// DeleteRelay deletes a relay with its events and status.
	DeleteRelay(ctx context.Context, tx uow.Tx, id string) error
	// RebuildRelayStates empties relay_states and derives it from the newest event of each
	// relay (federation.RelayEventKind.State).
	RebuildRelayStates(ctx context.Context, tx uow.Tx) error

	FindRelay(ctx context.Context, id string) (RelayRecord, error)
	// ListRelays returns every relay in registration order.
	ListRelays(ctx context.Context) ([]RelayRecord, error)
	// FindRelayByFollow returns the relay whose current Follow is followURI.
	FindRelayByFollow(ctx context.Context, followURI federation.IRI) (RelayRecord, error)
	// FindAcceptedRelayByActor returns an accepted relay whose actor is actorURI.
	FindAcceptedRelayByActor(ctx context.Context, actorURI federation.IRI) (RelayRecord, error)
	// ListAcceptedRelayInboxes returns the inboxes of the accepted relays, without duplicates.
	ListAcceptedRelayInboxes(ctx context.Context) ([]federation.IRI, error)
}

// RebuildRelayStates derives relay_states again from relay_events in one transaction
// (data-model §9).
func RebuildRelayStates(ctx context.Context, u uow.UnitOfWork, repo RelayRepository) error {
	if err := u.Tx(ctx, func(tx uow.Tx) error { return repo.RebuildRelayStates(ctx, tx) }); err != nil {
		return fmt.Errorf("rebuild the relay states: %w", err)
	}
	return nil
}

// RemoteInboxes reads the inboxes of remote actors; *appaccount.Service implements it.
type RemoteInboxes interface {
	RemoteInbox(ctx context.Context, accountID string) (appaccount.RemoteInbox, error)
}

// RelayPolicy says whether a domain block or an account action keeps a relayed post out: the
// federation or the public_discovery restriction of the domain (or a parent) or of the account
// ("" for an account that is not stored). *appmoderation.FederationPolicy implements it.
type RelayPolicy interface {
	DiscoveryRestricted(ctx context.Context, domain, accountID string) (bool, error)
}

// RelaysDeps are the ports and settings of Relays.
type RelaysDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository RelayRepository
	// Outbound records the Follow, Undo, Accept, and Reject of the instance actor, and Outbox
	// enqueues their fan-out in the same transaction.
	Outbound Repository
	Outbox   queue.Outbox
	// Instance returns the instance actor, which sends the Follow and Undo; it must create the
	// actor on first use.
	Instance InstanceActorReader
	// Fetcher fetches the actor of a LitePub relay when it is registered.
	Fetcher DocumentFetcher
	// Accounts finds the stored actors of relayed activities; Inboxes their inboxes.
	Accounts ActorsByURI
	Inboxes  RemoteInboxes
	// Domains are the domain blocks; nil means NoDomainRestrictions. Policy keeps out the posts
	// of restricted authors; nil keeps out none.
	Domains DomainPolicy
	Policy  RelayPolicy
	// KV counts the activities each relay brings in per hour.
	KV    kernel.KeyValueStore
	Clock kernel.Clock
	IDs   kernel.IDGenerator
	// Host is the serving domain of the activity URIs (ADR-0018).
	Host string
	// HourlyLimit is relay_ingest_hourly_limit; zero means DefaultRelayIngestHourlyLimit.
	HourlyLimit int
	Logger      *slog.Logger
	// Meter records my.relay.activities; nil records nothing.
	Meter kernel.Meter
}

// Relays runs the relays: their administration, the membership activities, and the gate of
// what they bring in.
type Relays struct {
	d          RelaysDeps
	rec        recorder
	activities kernel.Counter
}

// NewRelays returns the relays on d.
func NewRelays(d RelaysDeps) *Relays {
	if d.Domains == nil {
		d.Domains = NoDomainRestrictions{}
	}
	if d.HourlyLimit <= 0 {
		d.HourlyLimit = DefaultRelayIngestHourlyLimit
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Relays{
		d:          d,
		rec:        recorder{d: RecorderDeps{Repository: d.Outbound, Outbox: d.Outbox, IDs: d.IDs, Host: d.Host}},
		activities: kernel.MeterOrNop(d.Meter).Counter(RelayActivities),
	}
}

// RelayInput registers a relay. Protocol "" detects it: a URL that serves an actor is a LitePub
// relay, anything else a Mastodon one.
type RelayInput struct {
	Protocol federation.RelayProtocol
	URL      string
	Receive  bool
}

// RelayView is a relay as the administrators see it: its registration, its state, what it
// brought in during the last 24 hours (Taken, and Dropped over the hourly limit), and whether a
// domain block suspends its server, which stops what goes to it and comes from it.
type RelayView struct {
	RelayRecord
	Taken     int64
	Dropped   int64
	Suspended bool
}

// List returns every relay in registration order.
func (r *Relays) List(ctx context.Context) ([]RelayView, error) {
	recs, err := r.d.Repository.ListRelays(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RelayView, 0, len(recs))
	for _, rec := range recs {
		v, err := r.view(ctx, rec)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// ExistingIDs returns, in ID order, the relays among ids that are registered, for the audit log.
func (r *Relays) ExistingIDs(ctx context.Context, ids []string) ([]string, error) {
	all, err := r.d.Repository.ListRelays(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, rec := range all {
		if slices.Contains(ids, rec.Relay.ID) {
			out = append(out, rec.Relay.ID)
		}
	}
	return out, nil
}

// Find returns one relay; it fails with ErrNotFound for an unknown one.
func (r *Relays) Find(ctx context.Context, id string) (RelayView, error) {
	rec, err := r.d.Repository.FindRelay(ctx, id)
	if err != nil {
		return RelayView{}, err
	}
	return r.view(ctx, rec)
}

func (r *Relays) view(ctx context.Context, rec RelayRecord) (RelayView, error) {
	v := RelayView{RelayRecord: rec}
	var err error
	if v.Suspended, err = r.d.Domains.FederationSuspended(ctx, rec.Relay.InboxURL.Host()); err != nil {
		return RelayView{}, err
	}
	now := r.d.Clock.Now()
	for i := range 24 {
		n, err := r.count(ctx, rec.Relay.ID, now.Add(-time.Duration(i)*time.Hour))
		if err != nil {
			return RelayView{}, err
		}
		v.Taken += min(n, int64(r.d.HourlyLimit))
		v.Dropped += max(n-int64(r.d.HourlyLimit), 0)
	}
	return v, nil
}

// hourKey returns the KV key of what relay id brought in during the hour (UTC) of t.
func hourKey(id string, t time.Time) string { return id + ":" + t.UTC().Format("2006010215") }

func (r *Relays) count(ctx context.Context, id string, t time.Time) (int64, error) {
	b, err := r.d.KV.Get(ctx, RelayIngestNamespace, hourKey(id, t))
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("federation: the relay count %q: %w", b, err)
	}
	return n, nil
}

// Add registers a relay and joins it (ADR-0061): the instance actor sends its Follow, and the
// relay is pending until it answers. byUserID is the administrator, "" for the CLI. The URL must
// be https on a server no domain block suspends; a LitePub relay's actor is fetched for its
// inbox. It fails with federation.ErrInvalidRelay, ErrRelayExists, ErrRelayDomainSuspended, or
// ErrRelayNotActor.
func (r *Relays) Add(ctx context.Context, in RelayInput, byUserID string) (RelayView, error) {
	if in.Protocol != "" && !in.Protocol.Valid() {
		return RelayView{}, fmt.Errorf("%w: protocol %q", federation.ErrInvalidRelay, in.Protocol)
	}
	u, err := CanonicalIRI(in.URL)
	if err != nil || len(u) > federation.MaxRelayURL || u.Host() == "" || !strings.HasPrefix(string(u), "https://") {
		return RelayView{}, fmt.Errorf("%w: the URL must be an https URL of at most %d bytes", federation.ErrInvalidRelay, federation.MaxRelayURL)
	}
	suspended, err := r.d.Domains.FederationSuspended(ctx, u.Host())
	if err != nil {
		return RelayView{}, err
	}
	if suspended {
		return RelayView{}, fmt.Errorf("%w: %s", ErrRelayDomainSuspended, u.Host())
	}
	protocol, inbox, err := r.resolve(ctx, in.Protocol, u)
	if err != nil {
		return RelayView{}, err
	}
	instance, err := r.d.Instance.InstanceActor(ctx)
	if err != nil {
		return RelayView{}, err
	}
	now := kernel.Normalize(r.d.Clock.Now())
	rl := federation.Relay{
		ID: r.d.IDs.NewID(), Protocol: protocol, URL: u, InboxURL: inbox, Receive: in.Receive,
		CreatedByUserID: byUserID, CreatedAt: now,
	}
	if err := rl.Validate(); err != nil {
		return RelayView{}, err
	}
	addedAudit := r.d.IDs.NewID()
	follow := r.followEvent(rl.ID, byUserID, now)
	followAudit := r.d.IDs.NewID()
	err = r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := r.d.Repository.InsertRelay(ctx, tx, rl); err != nil {
			return err
		}
		if err := r.emitAudit(ctx, tx, addedAudit, appmoderation.AuditRelayAdded, rl, "", byUserID, now); err != nil {
			return err
		}
		return r.follow(ctx, tx, instance, RelayRecord{Relay: rl}, follow, followAudit)
	})
	if err != nil {
		return RelayView{}, err
	}
	return r.Find(ctx, rl.ID)
}

// resolve returns the protocol and the inbox of a relay at u: u itself for a Mastodon relay, and
// the shared inbox, or the inbox, of the actor at u for a LitePub one. An unknown protocol is
// LitePub when u serves an actor, Mastodon otherwise.
func (r *Relays) resolve(ctx context.Context, p federation.RelayProtocol, u federation.IRI) (federation.RelayProtocol, federation.IRI, error) {
	if p == federation.RelayMastodon {
		return p, u, nil
	}
	got, err := fetchActorWith(ctx, r.d.Fetcher, string(u))
	var actor *federation.Actor
	if err == nil && got.Document != nil {
		actor = got.Document.Actor
	}
	if actor == nil || actor.ID != u || federation.InboxFor(actor.Inbox, actor.SharedInbox) == "" {
		if p == "" {
			return federation.RelayMastodon, u, nil
		}
		if err == nil {
			err = errors.New("no actor with an inbox")
		}
		return "", "", fmt.Errorf("%w: %s: %w", ErrRelayNotActor, u, err)
	}
	return federation.RelayLitePub, federation.InboxFor(actor.Inbox, actor.SharedInbox), nil
}

// Follow joins a relay that rejected the instance or that the instance left, with a new Follow.
// It fails with ErrNotFound, federation.ErrRelayNotJoinable, or ErrRelayDomainSuspended.
func (r *Relays) Follow(ctx context.Context, id, byUserID string) (RelayView, error) {
	rec, err := r.d.Repository.FindRelay(ctx, id)
	if err != nil {
		return RelayView{}, err
	}
	suspended, err := r.d.Domains.FederationSuspended(ctx, rec.Relay.InboxURL.Host())
	if err != nil {
		return RelayView{}, err
	}
	if suspended {
		return RelayView{}, fmt.Errorf("%w: %s", ErrRelayDomainSuspended, rec.Relay.InboxURL.Host())
	}
	instance, err := r.d.Instance.InstanceActor(ctx)
	if err != nil {
		return RelayView{}, err
	}
	now := kernel.Normalize(r.d.Clock.Now())
	follow := r.followEvent(id, byUserID, now)
	audit := r.d.IDs.NewID()
	err = r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := r.d.Repository.LockRelay(ctx, tx, id)
		if err != nil {
			return err
		}
		return r.follow(ctx, tx, instance, rec, follow, audit)
	})
	if err != nil {
		return RelayView{}, err
	}
	return r.Find(ctx, id)
}

// followEvent returns the follow_sent event of a new Follow of relay id, whose id is the
// event's activity URI.
func (r *Relays) followEvent(relayID, byUserID string, at time.Time) federation.RelayEvent {
	id := r.d.IDs.NewID()
	return federation.RelayEvent{
		ID: id, RelayID: relayID, Kind: federation.RelayFollowSent,
		FollowActivityURI: federation.IRI(appcontent.ActivityURI(r.d.Host, id)), ActorUserID: byUserID, OccurredAt: at,
	}
}

// follow records the Follow e of the relay of rec in tx, with its state, its audit entry, and
// the activity that delivers it.
func (r *Relays) follow(ctx context.Context, tx uow.Tx, instance appaccount.Actor, rec RelayRecord, e federation.RelayEvent, auditID string) error {
	if err := r.apply(ctx, tx, rec.Status, e); err != nil {
		return err
	}
	doc := federation.RelayFollowDocument(e.FollowActivityURI, federation.IRI(instance.Account.URI), rec.Relay, e.OccurredAt)
	if err := r.send(ctx, tx, federation.OutboundActivity{
		ActivityURI: e.FollowActivityURI, Type: federation.ActivityFollow, ActorAccountID: instance.Account.ID,
		ObjectKind: federation.OutboundRelay, ObjectID: rec.Relay.ID, CreatedAt: e.OccurredAt,
	}, func(federation.OutboundActivity) federation.ActivityDocument { return doc }, rec.Relay.InboxURL); err != nil {
		return err
	}
	return r.emitAudit(ctx, tx, auditID, appmoderation.AuditRelayFollowed, rec.Relay, e.ID, e.ActorUserID, e.OccurredAt)
}

// Unfollow leaves a pending or accepted relay: the instance actor sends the Undo of its Follow,
// and nothing goes to the relay or comes from it any more. It fails with ErrNotFound or
// federation.ErrRelayNotJoined.
func (r *Relays) Unfollow(ctx context.Context, id, byUserID string) (RelayView, error) {
	instance, err := r.d.Instance.InstanceActor(ctx)
	if err != nil {
		return RelayView{}, err
	}
	now := kernel.Normalize(r.d.Clock.Now())
	eventID, audit := r.d.IDs.NewID(), r.d.IDs.NewID()
	err = r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := r.d.Repository.LockRelay(ctx, tx, id)
		if err != nil {
			return err
		}
		e := federation.RelayEvent{
			ID: eventID, RelayID: id, Kind: federation.RelayLeftSent, FollowActivityURI: rec.Status.FollowActivityURI,
			ActorUserID: byUserID, OccurredAt: now,
		}
		if e.FollowActivityURI == "" {
			return federation.ErrRelayNotJoined
		}
		if err := r.apply(ctx, tx, rec.Status, e); err != nil {
			return err
		}
		undo := federation.IRI(appcontent.ActivityURI(r.d.Host, eventID))
		doc := federation.RelayUndoDocument(undo, e.FollowActivityURI, federation.IRI(instance.Account.URI), rec.Relay, time.Time{}, now)
		if err := r.send(ctx, tx, federation.OutboundActivity{
			ActivityURI: undo, Type: federation.ActivityUndo, ActorAccountID: instance.Account.ID,
			ObjectKind: federation.OutboundRelay, ObjectID: id, CreatedAt: now,
		}, func(federation.OutboundActivity) federation.ActivityDocument { return doc }, rec.Relay.InboxURL); err != nil {
			return err
		}
		return r.emitAudit(ctx, tx, audit, appmoderation.AuditRelayUnfollowed, rec.Relay, eventID, byUserID, now)
	})
	if err != nil {
		return RelayView{}, err
	}
	return r.Find(ctx, id)
}

// Remove deletes the registration of a relay that the instance left or that rejected it, with
// its events and state. The Update and Delete of the posts that went to it still go there: they
// follow the deliveries of their Create. It fails with ErrNotFound or ErrRelayNotRemovable.
func (r *Relays) Remove(ctx context.Context, id, byUserID string) error {
	now := kernel.Normalize(r.d.Clock.Now())
	audit := r.d.IDs.NewID()
	return r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := r.d.Repository.LockRelay(ctx, tx, id)
		if err != nil {
			return err
		}
		if !rec.Status.State.Removable() {
			return ErrRelayNotRemovable
		}
		if err := r.d.Repository.DeleteRelay(ctx, tx, id); err != nil {
			return err
		}
		return r.emitAudit(ctx, tx, audit, appmoderation.AuditRelayRemoved, rec.Relay, "", byUserID, now)
	})
}

// apply records e and the status it leads from to, in tx.
func (r *Relays) apply(ctx context.Context, tx uow.Tx, from federation.RelayStatus, e federation.RelayEvent) error {
	next, err := from.Next(e)
	if err != nil {
		return err
	}
	inserted, err := r.d.Repository.InsertRelayEvent(ctx, tx, e)
	if err != nil {
		return err
	}
	if !inserted {
		return fmt.Errorf("%w: the relay has a %s event of %s", federation.ErrRelayNotAnswerable, e.Kind, e.FollowActivityURI)
	}
	return r.d.Repository.UpsertRelayState(ctx, tx, next)
}

// send records the activity a of the instance actor in tx with its document, built by doc from
// the recorded activity, and the fan-out that delivers it to inbox alone.
func (r *Relays) send(ctx context.Context, tx uow.Tx, a federation.OutboundActivity, doc func(federation.OutboundActivity) federation.ActivityDocument, inbox federation.IRI) error {
	act, err := r.rec.insert(ctx, tx, a)
	if err != nil {
		return err
	}
	b, err := doc(act).Marshal()
	if err != nil {
		return err
	}
	if _, err := r.d.Outbound.InsertOutboundDocument(ctx, tx, act.ID, b, act.CreatedAt); err != nil {
		return err
	}
	return r.rec.enqueue(ctx, tx, act, fanOutPayload{Inbox: string(inbox)})
}

// emitAudit emits the audit log entry of an operation on relay rl; byUserID "" is the CLI.
func (r *Relays) emitAudit(ctx context.Context, tx uow.Tx, id string, action appmoderation.AuditAction, rl federation.Relay, source, byUserID string, at time.Time) error {
	e := appmoderation.AuditEntry{
		ID: id, Action: action, TargetKind: appmoderation.AuditTargetRelay, TargetID: rl.ID,
		TargetLabel: string(rl.URL), SourceEventID: source, OccurredAt: at,
	}
	e.ActorKind, e.ActorUserID = appmoderation.AuditActor(byUserID)
	return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: e})
}

// The inbox side of the relays.

// RelayMembership is the relay that a received activity came through: the relay and its actor.
type RelayMembership struct {
	RelayID  string
	ActorURI federation.IRI
}

// AcceptedRelayOf returns the ID of the accepted relay whose actor is signer, "" when it is none
// (federation.md §3.1 step 12).
func (r *Relays) AcceptedRelayOf(ctx context.Context, signer federation.IRI) (string, error) {
	if signer == "" {
		return "", nil
	}
	rec, err := r.d.Repository.FindAcceptedRelayByActor(ctx, signer)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return rec.Relay.ID, nil
}

// Admit decides whether an activity that relay relayID sent is taken in, before anything is
// fetched for it (ADR-0061, federation.md §13):
//
//  1. The relay must be accepted and receive what it relays.
//  2. The relay may bring in HourlyLimit activities per hour (UTC); the rest are dropped.
//  3. A Create or Update whose actor, or an Announce whose object's server, has the federation
//     or public_discovery restriction is dropped: neither the federated timeline nor search
//     would show its post.
//
// A dropped activity fails with an error wrapping ErrDropActivity.
func (r *Relays) Admit(ctx context.Context, relayID string, act *federation.Activity) (*RelayMembership, error) {
	rec, err := r.d.Repository.FindRelay(ctx, relayID)
	switch {
	case errors.Is(err, ErrNotFound):
		r.record(ctx, RelayOutcomeNotReceiving)
		return nil, fmt.Errorf("%w: relay %s is gone", ErrDropActivity, relayID)
	case err != nil:
		return nil, err
	case !rec.Relay.Receive || rec.Status.State != federation.RelayAccepted:
		r.record(ctx, RelayOutcomeNotReceiving)
		return nil, fmt.Errorf("%w: relay %s does not bring in posts", ErrDropActivity, relayID)
	}
	n, err := r.d.KV.IncrBy(ctx, RelayIngestNamespace, hourKey(relayID, r.d.Clock.Now()), 1, relayCountTTL)
	if err != nil {
		return nil, err
	}
	if n > int64(r.d.HourlyLimit) {
		r.record(ctx, RelayOutcomeOverLimit)
		return nil, fmt.Errorf("%w: relay %s is over its hourly limit", ErrDropActivity, relayID)
	}
	restricted, err := r.restricted(ctx, act)
	if err != nil {
		return nil, err
	}
	if restricted {
		r.record(ctx, RelayOutcomeRestricted)
		return nil, fmt.Errorf("%w: the author of a relayed %s is restricted", ErrDropActivity, act.RawType)
	}
	r.record(ctx, RelayOutcomeAdmitted)
	return &RelayMembership{RelayID: relayID, ActorURI: rec.Status.RelayActorURI}, nil
}

// restricted reports whether the post that act brings in has a restricted author or server. A
// Delete and an Undo take something away and are never restricted.
func (r *Relays) restricted(ctx context.Context, act *federation.Activity) (bool, error) {
	if r.d.Policy == nil {
		return false, nil
	}
	var domain, accountID string
	switch act.Type {
	case federation.ActivityDelete, federation.ActivityUndo:
		return false, nil
	case federation.ActivityAnnounce:
		domain = act.Object().ID.Host()
	default:
		domain = act.Actor.Host()
		a, err := r.d.Accounts.FindByURI(ctx, string(act.Actor))
		switch {
		case err == nil:
			accountID = a.Account.ID
		case !errors.Is(err, appaccount.ErrNotFound):
			return false, err
		}
	}
	return r.d.Policy.DiscoveryRestricted(ctx, domain, accountID)
}

func (r *Relays) record(ctx context.Context, outcome string) {
	r.activities.Add(ctx, 1, kernel.NewLabels(LabelOutcome, outcome))
}

// ReceiveAnswer handles an Accept (accepted) or a Reject of the Follow of a relay, and reports
// whether it was one: its object is the relay's current Follow. The answer counts only from the
// relay's actor: the registered actor of a LitePub relay, and an actor whose inbox or shared
// inbox is the registered inbox of a Mastodon one. An Accept makes the relay accepted, with its
// actor; a Reject makes it rejected, also after it accepted. The same answer again changes
// nothing.
func (r *Relays) ReceiveAnswer(ctx context.Context, in *InboxActivity, accepted bool) (bool, error) {
	follow := in.Activity.Object().ID
	if follow == "" {
		return false, nil
	}
	rec, err := r.d.Repository.FindRelayByFollow(ctx, follow)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	ok, err := r.isRelayActor(ctx, rec.Relay, in)
	if err != nil {
		return true, err
	}
	if !ok {
		return true, fmt.Errorf("%w: %s is not the actor of relay %s", ErrDropActivity, in.Activity.Actor, rec.Relay.URL)
	}
	kind, action := federation.RelayAcceptReceived, appmoderation.AuditRelayAccepted
	if !accepted {
		kind, action = federation.RelayRejectReceived, appmoderation.AuditRelayRejected
	}
	now := kernel.Normalize(r.d.Clock.Now())
	e := federation.RelayEvent{ID: r.d.IDs.NewID(), RelayID: rec.Relay.ID, Kind: kind, FollowActivityURI: follow, OccurredAt: now}
	if accepted {
		e.RelayActorURI = in.Activity.Actor
	}
	audit := r.d.IDs.NewID()
	err = r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		cur, err := r.d.Repository.LockRelay(ctx, tx, rec.Relay.ID)
		if err != nil {
			return err
		}
		if err := r.apply(ctx, tx, cur.Status, e); err != nil {
			return err
		}
		entry := appmoderation.AuditEntry{
			ID: audit, ActorKind: appmoderation.AuditActorRemote, Action: action, TargetKind: appmoderation.AuditTargetRelay,
			TargetID: rec.Relay.ID, TargetLabel: string(rec.Relay.URL), SourceEventID: e.ID, OccurredAt: now,
		}
		return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: entry})
	})
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, federation.ErrRelayNotAnswerable):
		// The relay is gone, or the answer is about a Follow it no longer waits for: nothing to do.
		return true, nil
	case err != nil:
		return true, err
	}
	return true, nil
}

// isRelayActor reports whether the actor of in speaks for relay rl.
func (r *Relays) isRelayActor(ctx context.Context, rl federation.Relay, in *InboxActivity) (bool, error) {
	if rl.Protocol == federation.RelayLitePub {
		return in.Activity.Actor == rl.URL, nil
	}
	inbox, err := r.d.Inboxes.RemoteInbox(ctx, in.Actor.Account.ID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return federation.IRI(inbox.InboxURL) == rl.InboxURL || federation.IRI(inbox.SharedInboxURL) == rl.InboxURL, nil
}

// errRelayAnswerPending reports the Follow of a LitePub relay's actor that arrived before the
// relay's Accept of the instance's Follow: the relay sends both at once, so the Follow is
// processed again (Retry) until the Accept is.
var errRelayAnswerPending = errors.New("federation: the relay's Follow came before its Accept")

// ReceiveFollow answers a Follow of the instance actor: the actor of an accepted relay gets an
// Accept (a LitePub relay follows back the instances it accepts), anyone else a Reject. Neither
// is recorded as a follow. The Follow of the actor of a pending LitePub relay waits for the
// relay's Accept.
func (r *Relays) ReceiveFollow(ctx context.Context, in *InboxActivity, instance appaccount.Actor) error {
	t, objectID := federation.ActivityReject, in.Actor.Account.ID
	rec, err := r.d.Repository.FindAcceptedRelayByActor(ctx, in.Activity.Actor)
	switch {
	case err == nil:
		t, objectID = federation.ActivityAccept, rec.Relay.ID
	case !errors.Is(err, ErrNotFound):
		return err
	default:
		all, err := r.d.Repository.ListRelays(ctx)
		if err != nil {
			return err
		}
		for _, rec := range all {
			if rec.Relay.Protocol == federation.RelayLitePub && rec.Relay.URL == in.Activity.Actor && rec.Status.State == federation.RelayPending {
				return fmt.Errorf("%w: relay %s", errRelayAnswerPending, rec.Relay.ID)
			}
		}
	}
	inbox, err := r.d.Inboxes.RemoteInbox(ctx, in.Actor.Account.ID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return fmt.Errorf("%w: %s has no inbox", ErrDropActivity, in.Activity.Actor)
	}
	if err != nil {
		return err
	}
	target := federation.IRI(inbox.InboxURL)
	if target == "" {
		target = federation.IRI(inbox.SharedInboxURL)
	}
	follow := in.Activity.ID
	if follow == "" {
		follow = in.Received.ActivityURI
	}
	now := kernel.Normalize(r.d.Clock.Now())
	return r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		first, err := r.rec.firstAnswer(ctx, tx, in.Received.ActivityURI, now)
		if err != nil || !first {
			return err
		}
		return r.send(ctx, tx, federation.OutboundActivity{
			Type: t, ActorAccountID: instance.Account.ID, ObjectKind: federation.OutboundRelay, ObjectID: objectID, CreatedAt: now,
		}, func(act federation.OutboundActivity) federation.ActivityDocument {
			return federation.FollowAnswerDocument(act.ActivityURI, t, federation.IRI(instance.Account.URI), in.Activity.Actor, follow, now)
		}, target)
	})
}

// The fan-out of the relay activities and of public posts to the relays.

// RelayInboxes reads the inboxes of the accepted relays; *Relays implements it.
type RelayInboxes interface {
	AcceptedRelayInboxes(ctx context.Context) ([]federation.IRI, error)
}

// AcceptedRelayInboxes implements RelayInboxes.
func (r *Relays) AcceptedRelayInboxes(ctx context.Context) ([]federation.IRI, error) {
	return r.d.Repository.ListAcceptedRelayInboxes(ctx)
}

// relaySubject checks an activity of the relay membership, whose document is stored with it and
// whose only recipient is the inbox of its payload.
func relaySubject(s subject, p fanOutPayload) (subject, error) {
	switch s.act.Type {
	case federation.ActivityFollow, federation.ActivityUndo, federation.ActivityAccept, federation.ActivityReject:
	default:
		return subject{}, fmt.Errorf("%w: %s of a relay", federation.ErrInvalidOutbound, s.act.Type)
	}
	if p.Inbox == "" {
		return subject{}, fmt.Errorf("%w: %s of relay %s without an inbox", federation.ErrInvalidOutbound, s.act.Type, s.act.ObjectID)
	}
	s.inbox = federation.IRI(p.Inbox)
	return s, nil
}

// relayTargets returns the inboxes of the accepted relays, the last recipients of a public
// status's Create.
func (f *FanOut) relayTargets(ctx context.Context) ([]target, error) {
	if f.d.Relays == nil {
		return nil, nil
	}
	inboxes, err := f.d.Relays.AcceptedRelayInboxes(ctx)
	if err != nil {
		return nil, err
	}
	var out []target
	for _, inbox := range inboxes {
		if t, ok := targetOf(inbox); ok {
			out = append(out, t)
		}
	}
	return out, nil
}
