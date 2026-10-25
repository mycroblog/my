// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// FanOutBatch is the number of recipients one JobFanOut reads (federation.md §7.2).
const FanOutBatch = 500

// FanOutDeps are the ports and settings of the fan-out.
type FanOutDeps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	// Outbox enqueues the deliveries and the next batch in the transaction that records them.
	Outbox   queue.Outbox
	Statuses Statuses
	Accounts Accounts
	Graph    Graph
	// Domains are the domain blocks; nil means NoDomainRestrictions.
	Domains DomainPolicy
	// Suspensions tells the local actors that moderation suspends, whose Update carries the
	// suspended document (federation.md §11.1); nil means NoAccountRestrictions.
	Suspensions AccountPolicy
	Clock       kernel.Clock
	IDs         kernel.IDGenerator
	// Parts resolves the media, mentions, and hashtags of the revisions that Notes present.
	Parts NoteParts
	// Quotes reads the quotes and quote policies that Notes present (ADR-0069); nil presents
	// none.
	Quotes NoteQuotes
	// Images reads the avatars and headers of the actors that Updates carry; nil sends none.
	Images ProfileImages
	// EmojiTags reads the custom emoji of the profiles of the actors that Updates carry; nil
	// sends none.
	EmojiTags EmojiTagReader
	// Blocks reads the local blocks that Block and Undo Block are about.
	Blocks BlockFinder
	// Follows reads the follows that Follow, Accept, Reject, and Undo Follow are about; nil
	// fails those activities.
	Follows FollowReader
	// Reports reads the forwarded reports that Flags are about; nil fails those activities.
	Reports ReportReader
	// Relays reads the inboxes of the accepted relays, the last recipients of the Create of a
	// public status (ADR-0061); nil sends to no relay.
	Relays RelayInboxes
	// Modes reads the federation mode when each batch runs (ADR-0076); nil federates both ways.
	Modes ModeSettings
	// Closure reads the service closure when each batch runs (ADR-0088 §2); nil never closes.
	Closure ClosureSettings
	// Moves reads the moves of local accounts: the movedTo of the Updates of their actors and the
	// Moves they send (ADR-0089); nil means that no account moved, and fails the Moves.
	Moves MoveReader
	// Host is the serving domain of the activity and object URIs (ADR-0018).
	Host string
	// AccountDomain is the domain of local handles in mentions; empty means Host.
	AccountDomain string
	// Batch is the number of recipients per job; zero means FanOutBatch.
	Batch int
	// Logger logs the activities whose object is gone; nil means slog.Default().
	Logger *slog.Logger
}

// FanOut computes where outbound activities go and records the deliveries (federation.md §7.2).
// Each JobFanOut handles one batch of one source of recipients:
//
//  1. It builds the activity's document once, on the first batch, and stores it
//     (outbound_activity_documents), so that every delivery and retry sends the same bytes.
//     The Delete of a status has its document stored already, by the projector that recorded it.
//  2. It reads up to Batch recipients: the author's active followers through the graph module,
//     the accounts the object mentions, the inboxes an earlier activity went to, or, for the
//     Delete of an actor, the shared inboxes known to the instance.
//  3. It keeps the accounts that content.CanView admits in ContextDelivery, one account at a
//     time, and turns them into their shared inbox, or their own inbox without one. Local
//     accounts have no inbox and drop out.
//  4. It drops the inboxes of servers that a domain block suspends or that are unavailable
//     (federation.Deliverable).
//  5. In one transaction, it inserts the deliveries with ON CONFLICT DO NOTHING, enqueues a
//     JobDeliver per new row, and enqueues the job of the next batch.
//
// Each batch first asks the federation mode at the time it runs (ADR-0076, federation.Sends): an
// activity that the mode does not send is skipped, records nothing more, and is never sent again,
// even when the mode is widened later. A retraction in a mode that does not publish goes only to
// the inboxes of the original delivery and to the followers at the time, never to every known
// shared inbox.
//
// The service closure comes before the mode (ADR-0088 §2, federation.ClosureSends): once the
// server is closed, only the Delete of a local actor goes out, to its followers and every known
// shared inbox whatever the mode, and everything else is skipped. The end of the fan-out of a
// Delete of an actor and of a Move is recorded with its last batch (federation.RecordsFanOutEnd),
// and when it is skipped or its object is gone, so that the closure can wait for it.
//
// Running a batch again inserts no delivery twice and enqueues no JobDeliver twice: the
// deliveries are unique per (activity, inbox), and only new rows get a job.
type FanOut struct {
	d     FanOutDeps
	notes notes
}

// NewFanOut returns the fan-out on d.
func NewFanOut(d FanOutDeps) *FanOut {
	if d.Domains == nil {
		d.Domains = NoDomainRestrictions{}
	}
	if d.Suspensions == nil {
		d.Suspensions = NoAccountRestrictions{}
	}
	if d.Batch <= 0 {
		d.Batch = FanOutBatch
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.AccountDomain == "" {
		d.AccountDomain = d.Host
	}
	return &FanOut{d: d, notes: notes{parts: d.Parts, graph: d.Graph, quotes: d.Quotes, host: d.Host, accountDomain: d.AccountDomain}}
}

// RegisterJobs registers the JobFanOut handler.
func (f *FanOut) RegisterJobs(h *queue.JobHandlerRegistry) {
	h.Register(JobFanOut, f.handle)
}

// errGone reports an activity whose object no longer exists, so that it has nothing to send.
var errGone = errors.New("federation: the object of the activity is gone")

func (f *FanOut) handle(ctx context.Context, j queue.Job) queue.Result {
	var p fanOutPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.OutboundActivityID == "" {
		return queue.Fail(fmt.Errorf("federation: fan-out payload %s: %w", j.Payload, err))
	}
	err := f.Run(ctx, p)
	switch {
	case err == nil:
		return queue.Done()
	case errors.Is(err, errGone), errors.Is(err, ErrFederationRestricted):
		f.d.Logger.InfoContext(ctx, "outbound activity not fanned out", slog.String("outbound_activity_id", p.OutboundActivityID), slog.Any("reason", err))
		if err := f.ended(ctx, p.OutboundActivityID); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	case errors.Is(err, ErrNotFound), errors.Is(err, federation.ErrInvalidOutbound):
		return queue.Fail(err)
	}
	return queue.Retry(0, err)
}

// target is a resolved recipient: the inbox and its server.
type target struct {
	inbox  federation.IRI
	domain string
}

// Run handles one batch of the fan-out of p.OutboundActivityID. It is safe to repeat.
func (f *FanOut) Run(ctx context.Context, p fanOutPayload) error {
	act, err := f.d.Repository.FindOutboundActivity(ctx, p.OutboundActivityID)
	if err != nil {
		return err
	}
	subj, err := f.subject(ctx, act, p)
	if err != nil {
		return err
	}
	sends, adds, why, err := f.sends(ctx, act, subj)
	if err != nil {
		return err
	}
	if !sends {
		return fmt.Errorf("%w: %s %s skipped %s", ErrFederationRestricted, act.Type, act.ID, why)
	}
	phases := subj.phases()
	idx := slices.Index(phases, phase(p.Phase))
	if p.Phase == "" {
		idx = 0
	}
	if idx < 0 {
		return fmt.Errorf("%w: phase %q of %s %s", federation.ErrInvalidOutbound, p.Phase, act.Type, act.ID)
	}

	var document []byte
	if idx == 0 && p.After == "" {
		if document, err = f.document(ctx, act, subj); err != nil {
			return err
		}
	}
	// A phase that the mode does not let add recipients stays in the list, so that the batches
	// of one activity agree on its phases whatever the mode was when each ran; it finds no one.
	var targets []target
	var next string
	if !phases[idx].adds() || adds {
		if targets, next, err = f.batch(ctx, act, subj, phases[idx], p.After); err != nil {
			return err
		}
	}
	targets, err = f.deliverable(ctx, act.Type, targets)
	if err != nil {
		return err
	}

	var nextJob *queue.NewJob
	switch {
	case next != "":
		j, err := fanOutPayload{OutboundActivityID: act.ID, RevisionID: p.RevisionID, FollowURI: p.FollowURI, SharedInboxes: p.SharedInboxes, Phase: string(phases[idx]), After: next}.job(act)
		if err != nil {
			return err
		}
		nextJob = &j
	case idx+1 < len(phases):
		j, err := fanOutPayload{OutboundActivityID: act.ID, RevisionID: p.RevisionID, FollowURI: p.FollowURI, SharedInboxes: p.SharedInboxes, Phase: string(phases[idx+1])}.job(act)
		if err != nil {
			return err
		}
		nextJob = &j
	}

	now := f.d.Clock.Now()
	deliveries := make([]federation.Delivery, len(targets))
	for i, t := range targets {
		deliveries[i] = federation.Delivery{
			ID: f.d.IDs.NewID(), OutboundActivityID: act.ID, InboxURL: t.inbox, Domain: t.domain, CreatedAt: now,
		}
	}
	return f.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if document != nil {
			if _, err := f.d.Repository.InsertOutboundDocument(ctx, tx, act.ID, document, now); err != nil {
				return err
			}
		}
		var jobs []queue.NewJob
		for _, d := range deliveries {
			inserted, err := f.d.Repository.InsertDelivery(ctx, tx, d)
			if err != nil {
				return err
			}
			if !inserted {
				continue
			}
			j, err := deliverJob(act, d)
			if err != nil {
				return err
			}
			jobs = append(jobs, j)
		}
		if nextJob != nil {
			jobs = append(jobs, *nextJob)
		} else if federation.RecordsFanOutEnd(act) {
			if err := f.d.Repository.InsertFanOutCompletion(ctx, tx, act.ID, now); err != nil {
				return err
			}
		}
		if len(jobs) == 0 {
			return nil
		}
		return f.d.Outbox.EnqueueTx(ctx, tx, jobs...)
	})
}

// sends reports whether act goes out when the batch runs, and whether it may reach recipients
// that what it takes back never went to (federation.AddsRecipients); why tells a skipped one.
// While the service closure delivers its Deletes, it alone decides (federation.ClosureSends);
// otherwise the federation mode does.
func (f *FanOut) sends(ctx context.Context, act federation.OutboundActivity, subj subject) (sends, adds bool, why string, err error) {
	closing, err := closureRetracts(ctx, f.d.Closure)
	if err != nil {
		return false, false, "", err
	}
	if closing {
		ok := federation.ClosureSends(act)
		return ok, ok, "while the service is closed", nil
	}
	mode, err := currentMode(ctx, f.d.Modes)
	if err != nil {
		return false, false, "", err
	}
	return federation.Sends(mode, act, subj.retracting()), federation.AddsRecipients(mode, act, subj.retracting()), "in mode " + string(mode), nil
}

// ended records that the fan-out of the activity id ended without its last batch: it was
// skipped, or its object is gone (federation.RecordsFanOutEnd).
func (f *FanOut) ended(ctx context.Context, id string) error {
	act, err := f.d.Repository.FindOutboundActivity(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil || !federation.RecordsFanOutEnd(act) {
		return err
	}
	return f.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return f.d.Repository.InsertFanOutCompletion(ctx, tx, act.ID, f.d.Clock.Now())
	})
}

// phase names a source of recipients of a fan-out.
type phase string

const (
	// phaseMentions are the accounts a status mentions, in one batch.
	phaseMentions phase = "mentions"
	// phaseFollowers are the author's active followers, in batches.
	phaseFollowers phase = "followers"
	// phasePrior are the inboxes the status's Create went to, in batches.
	phasePrior phase = "prior"
	// phaseBoostedAuthor is the author of a boosted status, in one batch.
	phaseBoostedAuthor phase = "boosted_author"
	// phaseBlocked is the remote account a block is about, in one batch.
	phaseBlocked phase = "blocked"
	// phaseStatusAuthor is the author of a favourited status or of a poll voted in, in one
	// batch.
	phaseStatusAuthor phase = "status_author"
	// phaseCounterpart is the other account of a follow, in one batch.
	phaseCounterpart phase = "counterpart"
	// phaseSharedInboxes are the shared inboxes known to the instance, in batches.
	phaseSharedInboxes phase = "shared_inboxes"
	// phaseReported is the remote account a forwarded report is about, in one batch.
	phaseReported phase = "reported"
	// phaseRelay is the one inbox of the payload, in one batch: of an activity of the relay
	// membership or of the approval of a quote.
	phaseRelay phase = "relay"
	// phaseRelays are the inboxes of the accepted relays, in one batch.
	phaseRelays phase = "relays"
	// phaseVoters are the accounts that voted in the poll of a status, in batches.
	phaseVoters phase = "voters"
)

// adds reports whether the phase reaches recipients that the object never went to before:
// every known shared inbox, and the relays.
func (ph phase) adds() bool { return ph == phaseSharedInboxes || ph == phaseRelays }

// subject is what the fan-out of one activity needs to know about its object.
type subject struct {
	act federation.OutboundActivity
	// facts and restrictions are what CanView judges a recipient by: a status's own, or, for a
	// boost, the boost as a status of the booster with the boost's visibility.
	facts        appcontent.StatusFacts
	restrictions appcontent.AuthorRestrictions
	// visibility is the effective visibility the document is addressed by.
	visibility appcontent.Visibility
	// actorID is the actor whose followers receive the activity.
	actorID string
	// mentioned are the accounts the object addresses by name: a revision's mentions, the
	// author of a boosted status.
	mentioned []string

	status   appcontent.Status
	revision appcontent.Revision
	boost    appcontent.Boost
	// block is the local block a Block or Undo Block is about.
	block appgraph.Block
	// follow is the follow of a Follow, Accept, Reject, or Undo Follow, and counterpart the
	// account on its other side from the actor: the only recipient.
	follow      appgraph.FollowRecord
	counterpart string
	// followURI is the Follow that an Accept answers when it is not the follow's own (a Follow
	// the follower sent again), "" otherwise.
	followURI federation.IRI
	// actor is the local actor an account activity is about, or the instance actor of a Flag,
	// and suspended whether moderation suspends the former when the fan-out starts.
	actor     appaccount.Actor
	suspended bool
	// sharedInboxes sends an actor's Update to every known shared inbox too
	// (fanOutPayload.SharedInboxes).
	sharedInboxes bool
	// movedTo is the actor that the local actor of an account activity moved to, the movedTo of
	// its Update, or the target of its Move; "" when it has not moved (ADR-0089).
	movedTo federation.IRI
	// report is the forwarded report a Flag is about.
	report appmoderation.Report
	// inbox is the only recipient of an activity of the relay membership.
	inbox federation.IRI
}

// retracting reports whether the Update of an actor takes something back rather than publishing
// the profile: its suspension, or its move (federation.OutboundFlows).
func (s subject) retracting() bool { return s.suspended || s.movedTo != "" }

// phases returns the sources of recipients of the activity, in order (data-model §7,
// federation.md §7.1).
//
//   - Create: the mentioned accounts, then the followers, then, for a public status, the accepted
//     relays (ADR-0061).
//   - Update and Delete: the inboxes of the Create, then, for a public or unlisted status, the
//     followers at that time, and, for the Update of a status with a poll, the accounts that
//     voted in it (FR-112).
//   - Announce and Undo Announce: the booster's followers, then the boosted status's author.
//   - Update of an actor: its followers. Delete of an actor, and the Update of a suspension or
//     of its end: its followers, then every shared inbox known to the instance (data-model §8,
//     federation.md §11.1).
//   - Move of an actor: its followers (ADR-0089).
//   - Follow, Accept, Reject, and Undo Follow: the other account of the follow.
//   - Like and Undo Like: the author of the favourited status.
//   - Create of a vote: the author of the poll's status.
//   - Flag: the reported account.
//   - Follow, Undo, Accept, and Reject of the relay membership, and the activities of the
//     approval of a quote: the inbox of the payload; the Delete of a revoked QuoteAuthorization
//     then goes to the quoted author's followers too (ADR-0069 §3).
func (s subject) phases() []phase {
	switch s.act.ObjectKind {
	case federation.OutboundRelay, federation.OutboundRefusal:
		return []phase{phaseRelay}
	case federation.OutboundQuote:
		if s.act.Type == federation.ActivityDelete {
			return []phase{phaseRelay, phaseFollowers}
		}
		return []phase{phaseRelay}
	case federation.OutboundReport:
		return []phase{phaseReported}
	case federation.OutboundBlock:
		return []phase{phaseBlocked}
	case federation.OutboundAccount:
		if s.act.Type == federation.ActivityDelete || s.sharedInboxes {
			return []phase{phaseFollowers, phaseSharedInboxes}
		}
		return []phase{phaseFollowers}
	case federation.OutboundFollow:
		return []phase{phaseCounterpart}
	case federation.OutboundMove:
		return []phase{phaseFollowers}
	case federation.OutboundFavourite, federation.OutboundPollVote:
		return []phase{phaseStatusAuthor}
	case federation.OutboundStatus:
		if s.act.Type == federation.ActivityCreate {
			if s.visibility == appcontent.VisibilityPublic {
				return []phase{phaseMentions, phaseFollowers, phaseRelays}
			}
			return []phase{phaseMentions, phaseFollowers}
		}
		phases := []phase{phasePrior}
		if s.visibility == appcontent.VisibilityPublic || s.visibility == appcontent.VisibilityUnlisted {
			phases = append(phases, phaseFollowers)
		}
		if s.act.Type == federation.ActivityUpdate && s.revision.Poll != nil {
			phases = append(phases, phaseVoters)
		}
		return phases
	case federation.OutboundBoost:
		return []phase{phaseFollowers, phaseBoostedAuthor}
	}
	return nil
}

// subject loads the object of act. An object that is gone fails with errGone.
func (f *FanOut) subject(ctx context.Context, act federation.OutboundActivity, p fanOutPayload) (subject, error) {
	s := subject{act: act, actorID: act.ActorAccountID}
	if act.ObjectKind == federation.OutboundBlock {
		return f.blockSubject(ctx, s)
	}
	if act.ObjectKind == federation.OutboundReport {
		return f.reportSubject(ctx, s)
	}
	if act.ObjectKind == federation.OutboundRelay || act.ObjectKind == federation.OutboundRefusal {
		return relaySubject(s, p)
	}
	if act.ObjectKind == federation.OutboundQuote {
		return f.quoteSubject(ctx, s, p)
	}
	if act.ObjectKind == federation.OutboundFollow {
		s.followURI = federation.IRI(p.FollowURI)
		return f.followSubject(ctx, s)
	}
	if act.ObjectKind == federation.OutboundMove {
		return f.moveSubject(ctx, s)
	}
	revisionID := p.RevisionID
	if act.ObjectKind == federation.OutboundAccount {
		s.sharedInboxes = p.SharedInboxes
		return f.accountSubject(ctx, s)
	}
	if act.ObjectKind == federation.OutboundFavourite || act.ObjectKind == federation.OutboundPollVote {
		return f.favouriteSubject(ctx, s)
	}
	var err error
	switch {
	case act.ObjectKind == federation.OutboundStatus && (act.Type == federation.ActivityCreate || act.Type == federation.ActivityUpdate):
		s.status, s.revision, err = f.d.Statuses.LiveRevision(ctx, act.ObjectID, revisionID)
		if errors.Is(err, appcontent.ErrNotFound) {
			return subject{}, fmt.Errorf("%w: status %s revision %s", errGone, act.ObjectID, revisionID)
		}
		s.mentioned = s.revision.MentionedAccountIDs
	case act.ObjectKind == federation.OutboundStatus && act.Type == federation.ActivityDelete:
		// The status is deleted; its row stays until the purge, and without it the Delete still
		// reaches the inboxes of the Create.
		s.status, err = f.d.Statuses.FindStatus(ctx, act.ObjectID)
		if errors.Is(err, appcontent.ErrNotFound) {
			s.status, err = appcontent.Status{ID: act.ObjectID, AuthorAccountID: act.ActorAccountID, IsLocal: true}, nil
		}
	case act.ObjectKind == federation.OutboundBoost && (act.Type == federation.ActivityAnnounce || act.Type == federation.ActivityUndo):
		if s.boost, err = f.d.Statuses.FindBoost(ctx, act.ObjectID); errors.Is(err, appcontent.ErrNotFound) {
			return subject{}, fmt.Errorf("%w: boost %s", errGone, act.ObjectID)
		}
		if err != nil {
			return subject{}, err
		}
		s.status, err = f.d.Statuses.FindStatus(ctx, s.boost.StatusID)
		if errors.Is(err, appcontent.ErrNotFound) {
			return subject{}, fmt.Errorf("%w: status %s of boost %s", errGone, s.boost.StatusID, act.ObjectID)
		}
		if s.status.AuthorAccountID != s.boost.BoosterAccountID {
			s.mentioned = []string{s.status.AuthorAccountID}
		}
	default:
		return subject{}, fmt.Errorf("%w: %s of a %s", federation.ErrInvalidOutbound, act.Type, act.ObjectKind)
	}
	if err != nil {
		return subject{}, err
	}
	if s.restrictions, err = f.d.Statuses.AuthorRestrictions(ctx, act.ActorAccountID); err != nil {
		return subject{}, err
	}
	if act.ObjectKind == federation.OutboundBoost {
		s.facts = appcontent.StatusFacts{AuthorAccountID: s.boost.BoosterAccountID, AuthorIsLocal: s.boost.IsLocal, Visibility: s.boost.Visibility}
	} else {
		s.facts = appcontent.StatusFacts{AuthorAccountID: s.status.AuthorAccountID, AuthorIsLocal: s.status.IsLocal, Visibility: s.status.Visibility}
	}
	s.visibility = appcontent.EffectiveVisibility(s.facts.Visibility, s.restrictions)
	return s, nil
}

// accountSubject loads the local actor of an Update or a Delete of an actor, and whether
// moderation suspends it. The Update of an actor that is no longer active, or no longer local,
// has nothing to send: a later Delete supersedes it. A suspended actor that is not deleted is
// the exception: its Update tells that it is suspended, also while a ban schedules its
// deletion (federation.md §11.1). The Delete of an actor goes out whatever its state, since
// the account row stays after the purge (data-model §8).
func (f *FanOut) accountSubject(ctx context.Context, s subject) (subject, error) {
	act := s.act
	if act.ObjectID != act.ActorAccountID || (act.Type != federation.ActivityUpdate && act.Type != federation.ActivityDelete) {
		return subject{}, fmt.Errorf("%w: %s of the account %s by %s", federation.ErrInvalidOutbound, act.Type, act.ObjectID, act.ActorAccountID)
	}
	a, err := f.d.Accounts.FindByID(ctx, act.ObjectID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return subject{}, fmt.Errorf("%w: account %s", errGone, act.ObjectID)
	case err != nil:
		return subject{}, err
	case !a.Account.IsLocal() || a.IsInstanceActor():
		return subject{}, fmt.Errorf("%w: %s of the account %s, which is not a local user's", federation.ErrInvalidOutbound, act.Type, act.ObjectID)
	}
	s.actor = a
	if act.Type == federation.ActivityDelete {
		return s, nil
	}
	if s.suspended, err = f.d.Suspensions.AccountFederationSuspended(ctx, act.ObjectID); err != nil {
		return subject{}, err
	}
	if !a.Lifecycle.Active() && (!s.suspended || a.Deleted()) {
		return subject{}, fmt.Errorf("%w: account %s is not active", errGone, act.ObjectID)
	}
	if s.movedTo, err = LocalActorMovedTo(ctx, f.d.Moves, a); err != nil {
		return subject{}, err
	}
	return s, nil
}

// moveSubject loads the move of a Move, its local actor, and the actor it moves to. The Move of
// an actor that is no longer active has nothing to send; a cancelled move is still sent, since
// its Update came first and the cancellation's Update follows it.
func (f *FanOut) moveSubject(ctx context.Context, s subject) (subject, error) {
	act := s.act
	if act.Type != federation.ActivityMove || f.d.Moves == nil {
		return subject{}, fmt.Errorf("%w: %s of the move %s", federation.ErrInvalidOutbound, act.Type, act.ObjectID)
	}
	m, err := f.d.Moves.FindMove(ctx, act.ObjectID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return subject{}, fmt.Errorf("%w: move %s", errGone, act.ObjectID)
	case err != nil:
		return subject{}, err
	case m.AccountID != act.ActorAccountID:
		return subject{}, fmt.Errorf("%w: the move %s is not of %s", federation.ErrInvalidOutbound, m.ID, act.ActorAccountID)
	}
	a, err := f.d.Accounts.FindByID(ctx, m.AccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return subject{}, fmt.Errorf("%w: account %s", errGone, m.AccountID)
	case err != nil:
		return subject{}, err
	case !a.Account.IsLocal() || a.IsInstanceActor():
		return subject{}, fmt.Errorf("%w: the Move of %s, which is not a local user's", federation.ErrInvalidOutbound, m.AccountID)
	case !a.Lifecycle.Active():
		return subject{}, fmt.Errorf("%w: account %s is not active", errGone, m.AccountID)
	}
	t, err := f.d.Moves.FindByID(ctx, m.TargetAccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return subject{}, fmt.Errorf("%w: account %s", errGone, m.TargetAccountID)
	case err != nil:
		return subject{}, err
	}
	s.actor, s.movedTo = a, federation.IRI(t.Account.URI)
	return s, nil
}

// batch reads one batch of the phase after the cursor and resolves it to inboxes. next is the
// cursor of the following batch of the same phase, "" when the phase is done.
func (f *FanOut) batch(ctx context.Context, act federation.OutboundActivity, s subject, ph phase, after string) (targets []target, next string, err error) {
	switch ph {
	case phasePrior:
		create, err := f.createOf(ctx, act)
		if err != nil || create == "" {
			return nil, "", err
		}
		ds, err := f.d.Repository.ListDeliveries(ctx, create, after, f.d.Batch)
		if err != nil {
			return nil, "", err
		}
		for _, d := range ds {
			targets = append(targets, target{inbox: d.InboxURL, domain: d.Domain})
		}
		if len(ds) == f.d.Batch {
			next = ds[len(ds)-1].ID
		}
		return targets, next, nil
	case phaseFollowers:
		ids, next, err := f.d.Graph.ActiveFollowerIDs(ctx, s.actorID, after, f.d.Batch)
		if err != nil {
			return nil, "", err
		}
		targets, err = f.resolve(ctx, act, s, ids, true)
		return targets, next, err
	case phaseVoters:
		ids, err := f.d.Statuses.PollVoterIDs(ctx, s.revision.Poll.ID, after, f.d.Batch)
		if err != nil {
			return nil, "", err
		}
		if len(ids) == f.d.Batch {
			next = ids[len(ids)-1]
		}
		targets, err = f.resolveIn(ctx, act, s, ids, false, appcontent.ContextObjectFetch)
		return targets, next, err
	case phaseBlocked:
		targets, err = f.blockedInbox(ctx, s)
		return targets, "", err
	case phaseReported:
		targets, err = f.reportedInbox(ctx, s)
		return targets, "", err
	case phaseRelay:
		if t, ok := targetOf(s.inbox); ok {
			targets = []target{t}
		}
		return targets, "", nil
	case phaseRelays:
		targets, err = f.relayTargets(ctx)
		return targets, "", err
	case phaseStatusAuthor:
		targets, err = f.statusAuthorTargets(ctx, s)
		return targets, "", err
	case phaseCounterpart:
		targets, err = f.counterpartTargets(ctx, s)
		return targets, "", err
	case phaseMentions, phaseBoostedAuthor:
		targets, err = f.resolve(ctx, act, s, s.mentioned, false)
		return targets, "", err
	case phaseSharedInboxes:
		inboxes, err := f.d.Accounts.KnownSharedInboxes(ctx, after, f.d.Batch)
		if err != nil {
			return nil, "", err
		}
		for _, inbox := range inboxes {
			if t, ok := targetOf(federation.IRI(inbox)); ok {
				targets = append(targets, t)
			}
		}
		if len(inboxes) == f.d.Batch {
			next = inboxes[len(inboxes)-1]
		}
		return targets, next, nil
	}
	return nil, "", fmt.Errorf("%w: phase %q", federation.ErrInvalidOutbound, ph)
}

// createOf returns the ID of the Create of the status act is about, "" when it has none.
func (f *FanOut) createOf(ctx context.Context, act federation.OutboundActivity) (string, error) {
	all, err := f.d.Repository.ListOutboundActivitiesOf(ctx, act.ObjectKind, act.ObjectID)
	if err != nil {
		return "", err
	}
	for _, a := range all {
		if a.Type == federation.ActivityCreate {
			return a.ID, nil
		}
	}
	return "", nil
}

// resolve keeps the accounts that the visibility policy admits and returns their inboxes, in
// order, without duplicates. followers says that every account is an active follower of the
// actor.
//
// A Delete reveals nothing but that the object is gone, so its recipients are not judged: they
// are the followers of a status that every follower could see. Neither are the followers of an
// actor's Update: a profile is public.
func (f *FanOut) resolve(ctx context.Context, act federation.OutboundActivity, s subject, accountIDs []string, followers bool) ([]target, error) {
	return f.resolveIn(ctx, act, s, accountIDs, followers, appcontent.ContextDelivery)
}

// resolveIn is resolve with the recipients judged in the context view: ContextDelivery, or, for
// the accounts that voted in a poll, ContextObjectFetch, since they saw the status to vote in it
// without being among the accounts that a delivery pushes it to.
func (f *FanOut) resolveIn(ctx context.Context, act federation.OutboundActivity, s subject, accountIDs []string, followers bool, view appcontent.ViewContext) ([]target, error) {
	judged := act.Type != federation.ActivityDelete && act.ObjectKind != federation.OutboundAccount && act.ObjectKind != federation.OutboundMove
	var out []target
	for _, id := range accountIDs {
		if id == s.actorID {
			continue
		}
		inbox, err := f.d.Accounts.RemoteInbox(ctx, id)
		if errors.Is(err, appaccount.ErrNotFound) {
			continue // a local account, or a remote one never fetched
		}
		if err != nil {
			return nil, err
		}
		if judged {
			ok, err := f.admits(ctx, s, id, followers, view)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		t, ok := targetOf(federation.InboxFor(federation.IRI(inbox.InboxURL), federation.IRI(inbox.SharedInboxURL)))
		if ok && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out, nil
}

// admits reports whether content.CanView lets the remote account recipient see the object in
// the context view (ContextDelivery but for the voters of a poll). An active follower is not blocked by either side: a block ends the follows
// between the two accounts in its transaction (data-model §2.1). Any other recipient's
// relationship with the actor is read from the graph module.
func (f *FanOut) admits(ctx context.Context, s subject, recipient string, follower bool, view appcontent.ViewContext) (bool, error) {
	rel := appcontent.Relation{Mentioned: slices.Contains(s.mentioned, recipient)}
	if follower {
		rel.Follow = appcontent.FollowActive
	} else {
		r, err := f.d.Graph.Relationship(ctx, recipient, s.actorID)
		if err != nil {
			return false, err
		}
		switch {
		case r.ActivelyFollowing():
			rel.Follow = appcontent.FollowActive
		case r.RequestedFollow():
			rel.Follow = appcontent.FollowPending
		}
		rel.Blocking, rel.BlockedBy = r.Blocking, r.BlockedBy
	}
	viewer := appcontent.Viewer{Kind: appcontent.ViewerRemote, AccountID: recipient}
	return appcontent.CanView(view, viewer, s.facts, s.restrictions, rel), nil
}

// targetOf returns the target of an inbox, false when the inbox is not an https IRI with a host.
func targetOf(inbox federation.IRI) (target, bool) {
	host := strings.ToLower(inbox.Host())
	if host == "" || !strings.HasPrefix(string(inbox), "https://") {
		return target{}, false
	}
	return target{inbox: inbox, domain: host}, true
}

// deliverable drops the targets on servers that do not get activities of type t.
func (f *FanOut) deliverable(ctx context.Context, t federation.ActivityType, targets []target) ([]target, error) {
	statuses := map[string]bool{}
	var out []target
	for _, tg := range targets {
		ok, seen := statuses[tg.domain]
		if !seen {
			var st federation.DomainStatus
			var err error
			if st.Suspended, err = f.d.Domains.FederationSuspended(ctx, tg.domain); err != nil {
				return nil, err
			}
			if st.State, err = f.d.Repository.InstanceState(ctx, tg.domain); err != nil {
				return nil, err
			}
			ok = federation.Deliverable(t, st)
			statuses[tg.domain] = ok
		}
		if ok {
			out = append(out, tg)
		}
	}
	return out, nil
}
