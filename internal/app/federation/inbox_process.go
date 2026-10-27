// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Errors that an InboxHandler returns to tell the processor what became of an activity
// (federation.md §3.2, §3.3).
var (
	// ErrUnknownObject reports that the target of a Delete or an Undo is not known: the
	// processor records a tombstone for it, so that the object or activity arriving later is
	// dropped. For other activities it means the same as ErrDropActivity.
	ErrUnknownObject = errors.New("federation: the target of the activity is unknown")
	// ErrDropActivity reports an activity that is not processed and never will be, such as one
	// outside the scope of remote content (ADR-0034). The job completes.
	ErrDropActivity = errors.New("federation: the activity is dropped")
	// ErrUnprocessable reports an activity that can never be processed although it should be,
	// such as one whose document cannot be normalized. The job fails and stays as a dead job.
	ErrUnprocessable = errors.New("federation: the activity cannot be processed")
)

// InboxActivity is a received activity as its InboxHandler gets it (federation.md §3.2): the
// normalized activity, fetched again from its origin when its signer is not its actor, and the
// stored account of its actor.
type InboxActivity struct {
	Received federation.ReceivedActivity
	// Activity is the normalized activity. When Received.Trust is TrustRefetch, its embedded
	// documents are trusted only when they were fetched from their origin.
	Activity *federation.Activity
	// Document is the JSON the activity was read from: as received, or as fetched again.
	Document []byte
	// Actor is the actor of the activity: a remote account.
	Actor appaccount.Actor
	// Relay is the accepted relay that brought the activity in, nil for one that no relay
	// brought (ADR-0061).
	Relay *RelayMembership
	// Mode is the federation mode when the activity is processed (ADR-0076), which the handlers
	// of the activities that span several flows ask about theirs; "" is federation.DefaultMode.
	Mode federation.Mode
}

// mode returns the federation mode the activity is processed in.
func (a *InboxActivity) mode() federation.Mode {
	if a.Mode == "" {
		return federation.DefaultMode
	}
	return a.Mode
}

// InboxHandler processes one kind of received activity (federation.md §3.4). It writes in one
// short transaction through the public APIs of the modules, and must give the same rows however
// often it runs and in whatever order the activities about one object arrive (NFR-REL-1). It
// returns nil when the activity was processed or had nothing to do, ErrUnknownObject or
// ErrDropActivity (or an error wrapping them, or a fetch error of a document that does not
// exist), ErrUnprocessable for an activity that can never be processed, and any other error for a
// failure worth retrying.
type InboxHandler func(ctx context.Context, a *InboxActivity) error

// InboxActors resolves the actor of a received activity; *RemoteActors implements it.
type InboxActors interface {
	Resolve(ctx context.Context, uri string) (appaccount.Actor, error)
}

// RelayGate decides whether an activity that a relay brought in is taken in (ADR-0061); *Relays
// implements it.
type RelayGate interface {
	// Admit returns the relay of an admitted activity, and fails with an error wrapping
	// ErrDropActivity for one that is dropped.
	Admit(ctx context.Context, relayID string, act *federation.Activity) (*RelayMembership, error)
}

// DocumentFetcher fetches a document from its origin (federation.md §6.1); *Fetcher and
// *RemoteActors implement it.
type DocumentFetcher interface {
	Fetch(ctx context.Context, uri string) (*Fetched, error)
}

// Fetch implements DocumentFetcher with the Fetcher of r.
func (r *RemoteActors) Fetch(ctx context.Context, uri string) (*Fetched, error) {
	return r.d.Fetcher.Fetch(ctx, uri)
}

// FetchActor implements ActorFetcher with the Fetcher of r.
func (r *RemoteActors) FetchActor(ctx context.Context, uri string) (*Fetched, error) {
	return r.d.Fetcher.FetchActor(ctx, uri)
}

// ActorFetcher fetches an actor from its origin, which the federation mode allows unless it is
// off (identify, ADR-0076); *Fetcher and *RemoteActors implement it.
type ActorFetcher interface {
	FetchActor(ctx context.Context, uri string) (*Fetched, error)
}

// fetchActorWith fetches the actor at uri with f: as an actor when f is an ActorFetcher, as any
// other document otherwise.
func fetchActorWith(ctx context.Context, f DocumentFetcher, uri string) (*Fetched, error) {
	if af, ok := f.(ActorFetcher); ok {
		return af.FetchActor(ctx, uri)
	}
	return f.Fetch(ctx, uri)
}

// InboxProcessorDeps are the ports of InboxProcessor.
type InboxProcessorDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository InboxRepository
	Actors     InboxActors
	Fetcher    DocumentFetcher
	// Domains and Accounts are the moderation restrictions; nil means NoDomainRestrictions and
	// NoAccountRestrictions.
	Domains  DomainPolicy
	Accounts AccountPolicy
	// Relays admits the activities that relays bring in; nil processes them as any other.
	Relays RelayGate
	// Modes reads the federation mode (ADR-0076); nil federates both ways.
	Modes ModeSettings
	// Host is this server's serving domain, which tells the local objects of the activities.
	Host   string
	Clock  kernel.Clock
	Logger *slog.Logger
}

// InboxOutcome says what processing a received activity came to.
type InboxOutcome string

// Outcomes of InboxProcessor.Process.
const (
	// InboxHandled is an activity that its handler processed.
	InboxHandled InboxOutcome = "handled"
	// InboxUnhandled is an activity of a type that no handler takes (ADR-0033): it completes.
	InboxUnhandled InboxOutcome = "unhandled"
	// InboxTombstoned is a Delete or Undo of an unknown target, recorded as a tombstone.
	InboxTombstoned InboxOutcome = "tombstoned"
	// InboxDropped is an activity that is not processed: a restricted actor, a tombstoned
	// object, an activity that could not be confirmed at its origin, one whose flow the
	// federation mode stops (ADR-0076), or one its handler dropped.
	InboxDropped InboxOutcome = "dropped"
)

type inboxRoute struct {
	typ federation.ActivityType
	// ownActor routes the Update and Delete whose object is their actor.
	ownActor bool
}

// InboxProcessor is the asynchronous part of receiving an activity (federation.md §3.2,
// ADR-0033), the handler of JobInbox: it normalizes the stored document, fetches it again when
// its signer is not its actor, resolves the actor, applies the moderation restrictions and the
// tombstones, and dispatches the activity to the handler of its type. Activities of types
// without a handler complete; a Delete or Undo whose target is unknown leaves a tombstone.
//
// Register every handler with Handle, HandleOwnActor, and HandleUndo before the runners start.
type InboxProcessor struct {
	d        InboxProcessorDeps
	handlers map[inboxRoute]InboxHandler
	// undo are the handlers of Undo by the type of the undone activity, in registration order.
	undo      map[federation.ActivityType]InboxHandler
	undoOrder []federation.ActivityType
}

// NewInboxProcessor returns the InboxProcessor on d, without handlers.
func NewInboxProcessor(d InboxProcessorDeps) *InboxProcessor {
	if d.Domains == nil {
		d.Domains = NoDomainRestrictions{}
	}
	if d.Accounts == nil {
		d.Accounts = NoAccountRestrictions{}
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &InboxProcessor{d: d, handlers: map[inboxRoute]InboxHandler{}, undo: map[federation.ActivityType]InboxHandler{}}
}

// Handle sets the handler of the activities of type t (federation.md §3.4). The Update and
// Delete of an actor by itself go to the handler set with HandleOwnActor instead. It panics when
// t is unknown or already has a handler.
func (p *InboxProcessor) Handle(t federation.ActivityType, h InboxHandler) {
	p.register(inboxRoute{typ: t}, h)
}

// HandleOwnActor sets the handler of the Update or Delete (t) whose object is its own actor. It
// panics for another type or when t already has such a handler.
func (p *InboxProcessor) HandleOwnActor(t federation.ActivityType, h InboxHandler) {
	if t != federation.ActivityUpdate && t != federation.ActivityDelete {
		panic(fmt.Sprintf("federation: HandleOwnActor of %q", t))
	}
	p.register(inboxRoute{typ: t, ownActor: true}, h)
}

// HandleUndo sets the handler of the Undo of activities of type t (Follow, Like, Announce,
// Block, Accept; federation.md §3.4). The first call registers the Undo route, so Undo has no
// handler of its own through Handle. An Undo that embeds the activity it takes back goes to the
// handler of that activity's type, and to none when its type has no handler. An Undo whose
// object is an IRI alone goes to every handler in registration order until one knows the
// target, that is, returns something other than ErrUnknownObject. A handler must check that the
// target is the actor's own: the embedded activity's type only routes the Undo. It panics when t
// is unknown, is Undo, or already has a handler.
func (p *InboxProcessor) HandleUndo(t federation.ActivityType, h InboxHandler) {
	if t == federation.ActivityUnknown || t == federation.ActivityUndo || h == nil {
		panic("federation: an Undo handler needs a known activity type other than Undo and a function")
	}
	if _, dup := p.undo[t]; dup {
		panic(fmt.Sprintf("federation: Undo handler for %s registered twice", t))
	}
	if len(p.undo) == 0 {
		p.Handle(federation.ActivityUndo, p.dispatchUndo)
	}
	p.undo[t] = h
	p.undoOrder = append(p.undoOrder, t)
}

// dispatchUndo is the handler of Undo when HandleUndo set its handlers.
func (p *InboxProcessor) dispatchUndo(ctx context.Context, in *InboxActivity) error {
	if doc := in.Activity.Object().Untrusted(); doc != nil && doc.Activity != nil {
		h, ok := p.undo[doc.Activity.Type]
		if !ok {
			return fmt.Errorf("%w: no handler takes back a %s", ErrUnknownObject, doc.Activity.RawType)
		}
		return h(ctx, in)
	}
	for _, t := range p.undoOrder {
		if err := p.undo[t](ctx, in); !errors.Is(err, ErrUnknownObject) {
			return err
		}
	}
	return ErrUnknownObject
}

func (p *InboxProcessor) register(r inboxRoute, h InboxHandler) {
	if r.typ == federation.ActivityUnknown || h == nil {
		panic("federation: an inbox handler needs a known activity type and a function")
	}
	if _, dup := p.handlers[r]; dup {
		panic(fmt.Sprintf("federation: inbox handler for %s (own actor %t) registered twice", r.typ, r.ownActor))
	}
	p.handlers[r] = h
}

// Handles reports whether activities of type t have a handler, and, for Undo, which of the
// undone types have one; it lets the composition root check its wiring.
func (p *InboxProcessor) Handles(t federation.ActivityType, undone ...federation.ActivityType) bool {
	if _, ok := p.handlers[inboxRoute{typ: t}]; !ok {
		return false
	}
	for _, u := range undone {
		if _, ok := p.undo[u]; !ok {
			return false
		}
	}
	return true
}

// RegisterJobs registers the handler of JobInbox.
func (p *InboxProcessor) RegisterJobs(h *queue.JobHandlerRegistry) {
	h.Register(JobInbox, p.HandleJob)
}

// HandleJob is the queue.Handler of JobInbox: Done when the activity is processed or has
// nothing to do, Fail when it can never be processed, and Retry otherwise (federation.md §3.2
// step 5).
func (p *InboxProcessor) HandleJob(ctx context.Context, j queue.Job) queue.Result {
	var payload inboxPayload
	if err := json.Unmarshal(j.Payload, &payload); err != nil || payload.ActivityURI == "" || !payload.Trust.Valid() {
		return queue.Fail(fmt.Errorf("%w: payload %s", ErrUnprocessable, j.Payload))
	}
	_, err := p.ProcessRelayed(ctx, federation.IRI(payload.ActivityURI), payload.Trust, payload.RelayID)
	switch {
	case err == nil:
		return queue.Done()
	case errors.Is(err, ErrUnprocessable):
		return queue.Fail(err)
	}
	return queue.Retry(0, err)
}

// Process processes the received activity activityURI (federation.md §3.2). It returns an
// error wrapping ErrUnprocessable for an activity that can never be processed; any other error
// is worth retrying.
func (p *InboxProcessor) Process(ctx context.Context, activityURI federation.IRI, trust federation.Trust) (InboxOutcome, error) {
	return p.ProcessRelayed(ctx, activityURI, trust, "")
}

// ProcessRelayed is Process of an activity that the accepted relay relayID brought in ("" for
// none): before anything is fetched for it, the relay's gate admits or drops it (ADR-0061).
func (p *InboxProcessor) ProcessRelayed(ctx context.Context, activityURI federation.IRI, trust federation.Trust, relayID string) (InboxOutcome, error) {
	received, err := p.d.Repository.FindReceivedActivity(ctx, activityURI)
	if errors.Is(err, ErrNotFound) {
		return "", fmt.Errorf("%w: %w", ErrUnprocessable, err)
	} else if err != nil {
		return "", err
	}
	received.Trust = trust
	document, err := p.d.Repository.FindReceivedDocument(ctx, activityURI)
	if errors.Is(err, ErrNotFound) {
		return "", fmt.Errorf("%w: %w", ErrUnprocessable, err)
	} else if err != nil {
		return "", err
	}

	// 1. The document, normalized.
	doc, err := Normalize(document, p.d.Clock.Now())
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrUnprocessable, activityURI, err)
	}
	act := doc.Activity
	if act == nil || act.Actor != received.ActorURI {
		return "", fmt.Errorf("%w: %s is not an activity of %s", ErrUnprocessable, activityURI, received.ActorURI)
	}
	in := &InboxActivity{Received: received, Activity: act, Document: document}

	// The federation mode at the time of processing, before anything is fetched (ADR-0076).
	if in.Mode, err = currentMode(ctx, p.d.Modes); err != nil {
		return "", err
	}
	if !receives(in.Mode, act, p.d.Host) {
		return p.dropped(ctx, in, fmt.Errorf("%w: %w: %s in mode %s", ErrDropActivity, ErrFederationRestricted, act.RawType, in.Mode))
	}

	// What a relay brought in passes its gate before it costs a fetch.
	if relayID != "" && p.d.Relays != nil {
		if in.Relay, err = p.d.Relays.Admit(ctx, relayID, act); err != nil {
			return p.unconfirmed(ctx, in, err)
		}
	}

	// 2. The activity confirmed at its origin when someone else signed it.
	if trust == federation.TrustRefetch {
		if err := p.refetch(ctx, in); err != nil {
			return p.unconfirmed(ctx, in, err)
		}
	}

	// 3. The actor, and the restrictions on it and its domain.
	if in.Actor, err = p.d.Actors.Resolve(ctx, string(act.Actor)); err != nil {
		return p.dropped(ctx, in, err)
	}
	if in.Actor.Account.IsLocal() {
		return p.dropped(ctx, in, fmt.Errorf("%w: %s is a local actor", ErrDropActivity, act.Actor))
	}
	if err := p.restricted(ctx, in); err != nil {
		return p.dropped(ctx, in, err)
	}
	if err := p.tombstoned(ctx, in); err != nil {
		return p.dropped(ctx, in, err)
	}

	// 4. The handler of its type.
	return p.dispatch(ctx, in)
}

// refetch replaces what in carries on trust with what its origin serves (federation.md §3.2
// step 2, ActivityPub appendix B.2). Every embedded document becomes untrusted. The object of a
// Create or Update, and every other activity (by its id), is fetched again and used when it is
// the same; a Delete stands only when its object is gone. An Announce that cannot be fetched is
// kept with its object untrusted; any other activity that cannot be confirmed is dropped.
func (p *InboxProcessor) refetch(ctx context.Context, in *InboxActivity) error {
	act := distrusted(in.Activity)
	in.Activity = act
	object := act.Object().ID
	switch {
	case ownActorActivity(act) && act.Type == federation.ActivityUpdate:
		return nil // the handler fetches the untrusted actor again
	case act.Type == federation.ActivityDelete:
		// The object, or the actor deleting itself, must be gone at its origin.
		got, err := p.d.Fetcher.Fetch(ctx, string(object))
		switch {
		case errors.Is(err, ErrFetchGone), errors.Is(err, ErrFetchNotFound):
			return nil
		case err != nil:
			return err
		case got.Document.Object != nil && got.Document.Object.Type == federation.ObjectTombstone:
			return nil
		}
		return fmt.Errorf("%w: %s still exists at its origin", ErrDropActivity, object)
	case act.Type == federation.ActivityCreate || act.Type == federation.ActivityUpdate:
		got, err := p.d.Fetcher.Fetch(ctx, string(object))
		if err != nil {
			return err
		}
		if o := got.Document.Object; o == nil || o.ID != object || o.AttributedTo != act.Actor {
			return fmt.Errorf("%w: %s at its origin is not an object of %s", ErrDropActivity, object, act.Actor)
		}
		act.Objects[0] = federation.FetchedRef(got.Document)
		return nil
	}
	fetched, err := p.fetchActivity(ctx, act)
	switch {
	case err == nil:
		in.Activity, in.Document = fetched.Document.Activity, fetched.Raw
		return nil
	case act.Type == federation.ActivityAnnounce && dropping(err):
		return nil // the boost handler resolves the untrusted object by its id
	}
	return err
}

// fetchActivity fetches act by its id and returns it when its origin serves the same activity.
func (p *InboxProcessor) fetchActivity(ctx context.Context, act *federation.Activity) (*Fetched, error) {
	if act.ID == "" {
		return nil, fmt.Errorf("%w: an activity without an id cannot be fetched", ErrDropActivity)
	}
	got, err := p.d.Fetcher.Fetch(ctx, string(act.ID))
	if err != nil {
		return nil, err
	}
	if a := got.Document.Activity; a == nil || a.ID != act.ID || a.Type != act.Type || a.Actor != act.Actor {
		return nil, fmt.Errorf("%w: %s at its origin is not the activity received", ErrDropActivity, act.ID)
	}
	return got, nil
}

// restricted refuses an actor whose domain or account has the federation restriction, except
// for the Delete, Undo, and Reject that take something back (federation.md §11).
func (p *InboxProcessor) restricted(ctx context.Context, in *InboxActivity) error {
	switch in.Activity.Type {
	case federation.ActivityDelete, federation.ActivityUndo, federation.ActivityReject:
		return nil
	}
	domain := in.Activity.Actor.Host()
	suspended, err := p.d.Domains.FederationSuspended(ctx, domain)
	if err != nil {
		return err
	}
	if suspended {
		return fmt.Errorf("%w: the domain %s is suspended", ErrDropActivity, domain)
	}
	suspended, err = p.d.Accounts.AccountFederationSuspended(ctx, in.Actor.Account.ID)
	if err != nil {
		return err
	}
	if suspended {
		return fmt.Errorf("%w: the account %s is suspended", ErrDropActivity, in.Actor.Account.URI)
	}
	return nil
}

// tombstoned refuses an activity that an earlier Undo took back, and the Create or Update of an
// object that an earlier Delete removed (federation.md §3.3).
func (p *InboxProcessor) tombstoned(ctx context.Context, in *InboxActivity) error {
	act := in.Activity
	if act.Type == federation.ActivityDelete || act.Type == federation.ActivityUndo {
		return nil
	}
	uris := []federation.IRI{in.Received.ActivityURI}
	if (act.Type == federation.ActivityCreate || act.Type == federation.ActivityUpdate) && !ownActorActivity(act) && act.Object().ID != "" {
		uris = append(uris, act.Object().ID)
	}
	for _, uri := range uris {
		// A tombstone holds back only what the actor whose Delete or Undo left it owns: the
		// activity is the actor's, and so is the post of a Create or Update, which its handler
		// takes only when it is attributed to the actor.
		ok, err := p.d.Repository.RemoteObjectTombstoned(ctx, uri, act.Actor)
		if err != nil {
			return err
		}
		if ok {
			return fmt.Errorf("%w: %s has a tombstone", ErrDropActivity, uri)
		}
	}
	if act.Type == federation.ActivityFollow {
		return p.followTombstoned(ctx, in)
	}
	return nil
}

// followTombstoned refuses the first Follow of a pair that an earlier Undo of an unknown Follow
// named (federation.md §3.3). The Follow uses up the pair's tombstone: it records a tombstone of
// its own URI with the Undo's URI, so that it stays dropped when it is processed again, and a
// later Follow of the pair goes through.
func (p *InboxProcessor) followTombstoned(ctx context.Context, in *InboxActivity) error {
	act := in.Activity
	object := act.Object().ID
	if object == "" {
		return nil
	}
	var dropped federation.IRI
	err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		dropped = ""
		t, err := p.d.Repository.LiveRemoteFollowTombstone(ctx, tx, act.Actor, object)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		dropped = t.ActivityURI
		_, err = p.d.Repository.InsertRemoteObjectTombstone(ctx, tx, federation.RemoteObjectTombstone{
			URI: in.Received.ActivityURI, ActorURI: act.Actor, ActivityURI: t.ActivityURI,
			ReceivedAt: kernel.Normalize(p.d.Clock.Now()),
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("read follow tombstone of %s: %w", act.Actor, err)
	}
	if dropped != "" {
		return fmt.Errorf("%w: %s undid a Follow of %s before it arrived", ErrDropActivity, dropped, object)
	}
	return nil
}

// dispatch hands in to the handler of its type (federation.md §3.4). A Delete or Undo without a
// handler, or whose handler does not know its target, leaves a tombstone.
func (p *InboxProcessor) dispatch(ctx context.Context, in *InboxActivity) (InboxOutcome, error) {
	act := in.Activity
	route := inboxRoute{typ: act.Type, ownActor: ownActorActivity(act)}
	h, ok := p.handlers[route]
	takesBack := !route.ownActor && (act.Type == federation.ActivityDelete || act.Type == federation.ActivityUndo)
	var err error
	switch {
	case ok:
		err = h(ctx, in)
	case takesBack:
		err = ErrUnknownObject
	default:
		p.d.Logger.InfoContext(ctx, "inbox activity of an unhandled type",
			slog.String("type", act.RawType), slog.String("activity_uri", string(in.Received.ActivityURI)),
			slog.String("actor", string(act.Actor)))
		return InboxUnhandled, nil
	}
	switch {
	case err == nil:
		return InboxHandled, nil
	case takesBack && errors.Is(err, ErrUnknownObject):
		return p.tombstone(ctx, in)
	}
	return p.dropped(ctx, in, err)
}

// tombstone records the tombstone of the unknown target of a Delete or Undo, when it may have
// one: the pair of an embedded Follow of the actor's for an Undo of one, the target's URI
// otherwise.
func (p *InboxProcessor) tombstone(ctx context.Context, in *InboxActivity) (InboxOutcome, error) {
	now := kernel.Normalize(p.d.Clock.Now())
	if ft, ok := federation.FollowTombstoneFor(in.Activity, in.Received.ActivityURI, now); ok {
		err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			_, err := p.d.Repository.InsertRemoteFollowTombstone(ctx, tx, ft)
			return err
		})
		if err != nil {
			return "", fmt.Errorf("record follow tombstone of %s: %w", ft.ActivityURI, err)
		}
		return InboxTombstoned, nil
	}
	t, ok := federation.TombstoneFor(in.Activity, in.Received.ActivityURI, now)
	if !ok {
		return p.dropped(ctx, in, fmt.Errorf("%w: %s may not leave a tombstone for %s", ErrDropActivity, in.Activity.Actor, in.Activity.Object().ID))
	}
	err := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := p.d.Repository.InsertRemoteObjectTombstone(ctx, tx, t)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("record tombstone of %s: %w", t.URI, err)
	}
	return InboxTombstoned, nil
}

// unconfirmed is dropped for an activity that is dropped before its origin confirmed it. An
// activity of trust refetch that is dropped for good gives up its record, so that the activity
// that its origin serves is taken in when it arrives later, signed or relayed again.
func (p *InboxProcessor) unconfirmed(ctx context.Context, in *InboxActivity, err error) (InboxOutcome, error) {
	if in.Received.Trust == federation.TrustRefetch && dropping(err) {
		uri := in.Received.ActivityURI
		if terr := p.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			_, err := p.d.Repository.ReleaseUnconfirmedActivity(ctx, tx, uri)
			return err
		}); terr != nil {
			return "", fmt.Errorf("release unconfirmed activity %s: %w", uri, terr)
		}
	}
	return p.dropped(ctx, in, err)
}

// dropped turns err into the outcome of an activity that is not processed: InboxDropped for an
// activity that is dropped for good, the error itself when it is worth retrying or can never be
// processed.
func (p *InboxProcessor) dropped(ctx context.Context, in *InboxActivity, err error) (InboxOutcome, error) {
	if !dropping(err) {
		return "", err
	}
	p.d.Logger.InfoContext(ctx, "inbox activity dropped",
		slog.String("activity_uri", string(in.Received.ActivityURI)), slog.String("type", in.Received.Type), slog.Any("reason", err))
	return InboxDropped, nil
}

// dropping reports whether err drops an activity for good: dropped by a rule, an unknown target,
// or a document that does not exist or may not be fetched. Temporary fetch failures
// (ErrFetchFailed, ErrFetchUnavailable) are retried.
func dropping(err error) bool {
	return errors.Is(err, ErrDropActivity) || errors.Is(err, ErrUnknownObject) ||
		errors.Is(err, ErrFetchNotFound) || errors.Is(err, ErrFetchGone) || errors.Is(err, ErrFetchRefused)
}

// ownActorActivity reports whether act is an Update or Delete whose object is its own actor.
func ownActorActivity(act *federation.Activity) bool {
	return (act.Type == federation.ActivityUpdate || act.Type == federation.ActivityDelete) &&
		act.Actor != "" && act.Object().ID == act.Actor
}

// distrusted returns a copy of act whose embedded documents are untrusted.
func distrusted(act *federation.Activity) *federation.Activity {
	c := *act
	c.Objects = make([]federation.Ref, len(act.Objects))
	for i, r := range act.Objects {
		c.Objects[i] = r.Distrusted()
	}
	return &c
}
