// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Bounds of ingesting remote posts (federation.md §3.3, §6.3).
const (
	// UpdateAsCreateWindow is how recent the published time of an Update of an unknown post
	// must be for the Update to be taken as its Create (Mastodon does the same).
	UpdateAsCreateWindow = 24 * time.Hour
	// MaxResolvedMentions bounds the unknown remote actors that one post's mentions fetch, and
	// MentionResolveTimeout the time they take together; the others are left out.
	MaxResolvedMentions   = 5
	MentionResolveTimeout = 10 * time.Second
	// MaxMentions bounds the mentions of one post that are stored.
	MaxMentions = federation.MaxTags
)

// RemoteStatuses is the part of the content module's public API that stores remote posts
// (federation.md §1.1). *appcontent.Service implements it.
type RemoteStatuses interface {
	IngestRemoteStatus(ctx context.Context, tx uow.Tx, in appcontent.RemoteStatusInput) (appcontent.Post, error)
	ReviseRemoteStatus(ctx context.Context, tx uow.Tx, in appcontent.RemoteStatusInput) (bool, error)
	DeleteRemoteStatus(ctx context.Context, tx uow.Tx, uri, authorAccountID string) error
	FindStatus(ctx context.Context, id string) (appcontent.Status, error)
	FindStatusByURI(ctx context.Context, uri string) (appcontent.Status, error)
	Tombstoned(ctx context.Context, uri string) (bool, error)
	LocalStatusInConversation(ctx context.Context, conversationURI string) (bool, error)
}

// RelayedStatuses records the remote posts that only a relay brought in, which are kept for
// relay_status_retention (ADR-0061); *appcontent.Service implements it.
type RelayedStatuses interface {
	RecordRelayedStatus(ctx context.Context, tx uow.Tx, statusID, relayID string, at time.Time) error
}

// StatusAccounts is the part of the account module's public API that ingesting posts reads: the
// accounts that a post mentions and the followers collection of its author.
// *appaccount.Service implements it.
type StatusAccounts interface {
	FindByURI(ctx context.Context, uri string) (appaccount.Actor, error)
	RemoteFollowersURL(ctx context.Context, accountID string) (string, error)
}

// StatusHandlersDeps are the ports of StatusHandlers.
type StatusHandlersDeps struct {
	UnitOfWork uow.UnitOfWork
	Statuses   RemoteStatuses
	Accounts   StatusAccounts
	// Actors resolves the remote actors that a post in scope mentions and that are not stored
	// yet; *RemoteActors implements it.
	Actors InboxActors
	// Fetcher fetches the object of a Create or Update that carries only its id.
	Fetcher DocumentFetcher
	// Followers tells whether local accounts follow the author (ADR-0034).
	Followers Followers
	// Tombstones tells which URIs a Delete or Undo of an unknown object left (federation.md
	// §3.3), so that a fetch does not bring them in; nil knows none.
	Tombstones RemoteObjectTombstones
	// Outbox enqueues the fetch of the parent of an ingested reply in its transaction; Jobs
	// enqueues the fetches of a thread outside transactions; KV keeps the thread fetches of
	// the last ThreadFetchTTL (federation.md §6.3). Without Outbox, parents are not fetched;
	// without Jobs and KV, threads are not.
	Outbox queue.Outbox
	Jobs   queue.JobQueue
	KV     kernel.KeyValueStore
	// Profiles reads the remote accounts whose outboxes FetchOutbox fetches, Relationships the
	// blocks between their viewers and them, and Restrictions their federation restriction
	// (ADR-0044). Without Profiles (and Jobs and KV), outboxes are not fetched; without
	// Relationships or Restrictions, no block or restriction keeps them.
	Profiles      OutboxAccounts
	Relationships Relationships
	Restrictions  AccountPolicy
	// Pins mirrors the featured collections of remote accounts (JobFetchFeatured, ADR-0044);
	// without Pins (or Profiles), featured collections are not fetched.
	Pins RemotePins
	// Relayed records the posts that only a relay brought in, and RelayPolicy keeps out those of
	// restricted authors (ADR-0061); without Relayed, nothing a relay brings in is stored.
	Relayed     RelayedStatuses
	RelayPolicy RelayPolicy
	// Votes records the votes of remote accounts in local polls; nil takes every Note in reply
	// to a local status as a post.
	Votes RemotePollVotes
	// Quotes records the approvals of the quotes of remote posts and takes a Delete of a
	// QuoteAuthorization as a revocation (ADR-0069); with Outbox, JobVerifyQuote fetches the
	// quoted posts and verifies the approvals, reading their authors through Profiles. Without
	// Quotes, quotes stay pending.
	Quotes RemoteQuotes
	// Approvals verifies the QuoteAuthorizations of this server that remote quotes of local
	// statuses name; nil leaves those quotes pending.
	Approvals QuoteApprovals
	Clock     kernel.Clock
	// Host is the serving domain: actors on it are local.
	Host   string
	Logger *slog.Logger
}

// StatusHandlers are the inbox handlers of remote posts (federation.md §3.4): Create and Update
// of Note, Question, and the converted types, Delete of a post, and Add and Remove of the pinned
// posts. Only posts related to a local
// account are ingested (ADR-0034).
type StatusHandlers struct {
	d      StatusHandlersDeps
	origin string
}

// NewStatusHandlers returns the StatusHandlers on d.
func NewStatusHandlers(d StatusHandlersDeps) *StatusHandlers {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &StatusHandlers{d: d, origin: federation.IRI("https://" + d.Host).Origin()}
}

// Register sets the handlers of Create, Update, and Delete, and of the Add and Remove of the
// pinned posts (FeaturedChanged), on p.
func (h *StatusHandlers) Register(p *InboxProcessor) {
	p.Handle(federation.ActivityCreate, h.Create)
	p.Handle(federation.ActivityUpdate, h.Update)
	p.Handle(federation.ActivityDelete, h.Delete)
	p.Handle(federation.ActivityAdd, h.FeaturedChanged)
	p.Handle(federation.ActivityRemove, h.FeaturedChanged)
}

// Create ingests the post of a Create when it is in scope (ADR-0034). A post that is stored
// already is left as it is, and one that is deleted, or whose URI a deletion keeps, is dropped.
// A Note with a name in reply to a local status with a poll is a vote in it (vote).
func (h *StatusHandlers) Create(ctx context.Context, a *InboxActivity) error {
	obj, err := h.object(ctx, a)
	if err != nil {
		return err
	}
	if voted, err := h.vote(ctx, a, obj); voted || err != nil {
		return err
	}
	switch _, err := h.d.Statuses.FindStatusByURI(ctx, string(obj.ID)); {
	case errors.Is(err, appcontent.ErrNotFound):
		return h.ingest(ctx, a, obj)
	case err != nil:
		return err
	}
	tombstoned, err := h.d.Statuses.Tombstoned(ctx, string(obj.ID))
	if err != nil {
		return err
	}
	if tombstoned {
		return fmt.Errorf("%w: %s is deleted", ErrDropActivity, obj.ID)
	}
	return nil
}

// RemotePollVotes records the votes of remote accounts in local polls; *appcontent.Interactions
// implements it.
type RemotePollVotes interface {
	RecordRemotePollVote(ctx context.Context, in appcontent.RemotePollVoteInput) (bool, error)
}

// vote records a Note of the actor that votes in a local poll (FR-112, federation.md §3.4): a
// Note with a name in reply to a local status, as Mastodon, Misskey, Pleroma (Akkoma), and
// GoToSocial send votes. It reports whether obj was a vote. A reply to a local status without a
// poll is no vote: it is a post like any other. A vote in a poll that has ended, for a choice
// the poll does not have, a second one in a single-choice poll, or in a status the actor may not
// see is dropped.
func (h *StatusHandlers) vote(ctx context.Context, a *InboxActivity, obj *federation.Object) (bool, error) {
	if h.d.Votes == nil || obj.Type != federation.ObjectNote || obj.Name == "" || obj.InReplyTo == "" || !h.isLocal(obj.InReplyTo) {
		return false, nil
	}
	st, err := h.d.Statuses.FindStatusByURI(ctx, string(obj.InReplyTo))
	if errors.Is(err, appcontent.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = h.d.Votes.RecordRemotePollVote(ctx, appcontent.RemotePollVoteInput{
		URI: string(obj.ID), AccountID: a.Actor.Account.ID, StatusID: st.ID, Title: obj.Name, VotedAt: obj.Published,
	})
	switch {
	case errors.Is(err, appcontent.ErrNoPoll):
		return false, nil
	case errors.Is(err, appcontent.ErrPollClosed), errors.Is(err, appcontent.ErrInvalidPollChoices),
		errors.Is(err, appcontent.ErrAlreadyVoted):
		return true, fmt.Errorf("%w: vote %s in %s: %w", ErrDropActivity, obj.ID, st.URI, err)
	}
	return true, interactionError(err)
}

// Update records a new revision of a stored post (federation.md §3.3): an Update older than the
// current revision makes none. The Update of a post that is not stored is taken as its Create
// when the post is in scope and was published within UpdateAsCreateWindow.
func (h *StatusHandlers) Update(ctx context.Context, a *InboxActivity) error {
	obj, err := h.object(ctx, a)
	if err != nil {
		return err
	}
	switch _, err := h.d.Statuses.FindStatusByURI(ctx, string(obj.ID)); {
	case errors.Is(err, appcontent.ErrNotFound):
		if obj.Published.IsZero() || h.d.Clock.Now().Sub(obj.Published) > UpdateAsCreateWindow {
			return fmt.Errorf("%w: Update of an unknown post %s published long ago", ErrDropActivity, obj.ID)
		}
		return h.ingest(ctx, a, obj)
	case err != nil:
		return err
	}
	followers, err := h.followers(ctx, a.Actor)
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
	in := h.input(a, obj, followers, mentions)
	err = h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := h.d.Statuses.ReviseRemoteStatus(ctx, tx, in); err != nil {
			return err
		}
		return h.verifyQuoteTx(ctx, tx, obj.ID, obj.Quote, true)
	})
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return fmt.Errorf("%w: Update of a deleted post %s", ErrDropActivity, obj.ID)
	case errors.Is(err, appcontent.ErrNotAuthor):
		return fmt.Errorf("%w: %s is not the author of %s", ErrDropActivity, a.Actor.Account.URI, obj.ID)
	}
	return err
}

// Delete deletes a stored post of the actor (federation.md §3.4). A post that is not stored is
// ErrUnknownObject, so that the processor keeps a tombstone for a Create arriving later.
func (h *StatusHandlers) Delete(ctx context.Context, a *InboxActivity) error {
	id := a.Activity.Object().ID
	if id == "" || !federation.SameOrigin(id, a.Activity.Actor) {
		return fmt.Errorf("%w: Delete of %q by %s", ErrDropActivity, id, a.Activity.Actor)
	}
	err := h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return h.d.Statuses.DeleteRemoteStatus(ctx, tx, string(id), a.Actor.Account.ID)
	})
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		if revoked, err := h.revokeQuoteAuthorization(ctx, id, a.Actor); err != nil || revoked {
			return err
		}
		return fmt.Errorf("%w: %s", ErrUnknownObject, id)
	case errors.Is(err, appcontent.ErrNotAuthor):
		return fmt.Errorf("%w: %s is not the author of %s", ErrDropActivity, a.Actor.Account.URI, id)
	case errors.Is(err, appcontent.ErrAlreadyDeleted):
		return nil
	}
	return err
}

// object returns the post of a Create or Update: the embedded one when it is trusted, otherwise
// the one its origin serves, fetched only when a local account follows the actor. It must be a
// post of the actor, from the actor's origin.
func (h *StatusHandlers) object(ctx context.Context, a *InboxActivity) (*federation.Object, error) {
	act := a.Activity
	ref := act.Object()
	if ref.ID == "" || !federation.SameOrigin(ref.ID, act.Actor) {
		return nil, fmt.Errorf("%w: %s of %q is not an object of the actor's origin", ErrDropActivity, act.Type, ref.ID)
	}
	doc := ref.Trusted()
	if doc == nil {
		// Only a post in scope is fetched (federation.md §3.3); without the post, only the
		// follows of its author tell.
		followed, err := h.followed(ctx, a.Actor)
		if err != nil {
			return nil, err
		}
		if !followed {
			return nil, fmt.Errorf("%w: %s of %s by its id only, from an author no local account follows", ErrDropActivity, act.Type, ref.ID)
		}
		got, err := h.d.Fetcher.Fetch(ctx, string(ref.ID))
		if err != nil {
			return nil, err
		}
		doc = got.Document
	}
	obj := doc.Object
	switch {
	case obj == nil || obj.ID != ref.ID:
		return nil, fmt.Errorf("%w: %s is not an object", ErrDropActivity, ref.ID)
	case !isPost(obj.Type):
		return nil, fmt.Errorf("%w: %s is a %s, not a post", ErrDropActivity, obj.ID, obj.RawType)
	case obj.AttributedTo != "" && obj.AttributedTo != act.Actor:
		return nil, fmt.Errorf("%w: %s is attributed to %s, not to %s", ErrDropActivity, obj.ID, obj.AttributedTo, act.Actor)
	}
	return obj, nil
}

func isPost(t federation.ObjectType) bool {
	switch t {
	case federation.ObjectNote, federation.ObjectQuestion, federation.ObjectArticle, federation.ObjectPage,
		federation.ObjectEvent, federation.ObjectImage, federation.ObjectVideo, federation.ObjectAudio:
		return true
	}
	return false
}

// ingest stores a post that is not stored yet, when it is in scope and its URI has no tombstone.
func (h *StatusHandlers) ingest(ctx context.Context, a *InboxActivity, obj *federation.Object) error {
	tombstoned, err := h.d.Statuses.Tombstoned(ctx, string(obj.ID))
	if err != nil {
		return err
	}
	if tombstoned {
		return fmt.Errorf("%w: %s is deleted", ErrDropActivity, obj.ID)
	}
	followers, err := h.followers(ctx, a.Actor)
	if err != nil {
		return err
	}
	mentions, err := h.mentions(ctx, obj, followers)
	if err != nil {
		return err
	}
	ok, err := h.inScope(ctx, a.Actor, obj, mentions)
	if err != nil {
		return err
	}
	// A post that no local account relates to is taken in from an accepted relay when it is
	// public (ADR-0061), without the fetches of its parent and of the unknown actors it mentions.
	relayed := !ok && a.Relay != nil && h.d.Relayed != nil
	if !ok && !relayed {
		return fmt.Errorf("%w: %s is not related to a local account (ADR-0034)", ErrDropActivity, obj.ID)
	}
	if !relayed {
		if err := h.resolveMentions(ctx, mentions); err != nil {
			return err
		}
	}
	in := h.input(a, obj, followers, mentions)
	if relayed && in.Visibility != appcontent.VisibilityPublic {
		return fmt.Errorf("%w: the relayed %s is not public", ErrDropActivity, obj.ID)
	}
	err = h.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		p, err := h.d.Statuses.IngestRemoteStatus(ctx, tx, in)
		if err != nil {
			return err
		}
		if err := h.verifyQuoteTx(ctx, tx, obj.ID, obj.Quote, !relayed); err != nil {
			return err
		}
		if relayed {
			return h.recordRelayed(ctx, tx, p.Status, a.Relay)
		}
		return h.fetchParentTx(ctx, tx, p.Status)
	})
	if errors.Is(err, appcontent.ErrStatusExists) {
		return nil // a concurrent activity stored it
	}
	return err
}

// recordRelayed records in tx that st came in through relay m alone.
func (h *StatusHandlers) recordRelayed(ctx context.Context, tx uow.Tx, st appcontent.Status, m *RelayMembership) error {
	if h.d.Relayed == nil {
		return fmt.Errorf("%w: nothing records relayed posts", ErrDropActivity)
	}
	return h.d.Relayed.RecordRelayedStatus(ctx, tx, st.ID, m.RelayID, kernel.Normalize(h.d.Clock.Now()))
}

// RelayedAnnounce takes in the post that an accepted relay announces (ADR-0061): it is fetched
// from its origin, whatever the Announce embeds, and stored as a relayed post when it is public
// and not stored yet. It is never recorded as a boost: the relay is not a reader of it.
func (h *StatusHandlers) RelayedAnnounce(ctx context.Context, in *InboxActivity) error {
	ref := in.Activity.Object()
	switch {
	case ref.ID == "":
		return fmt.Errorf("%w: Announce without an object", ErrDropActivity)
	case h.isLocal(ref.ID):
		return fmt.Errorf("%w: a relay announced the local post %s", ErrDropActivity, ref.ID)
	case h.d.Relayed == nil || in.Relay == nil:
		return fmt.Errorf("%w: nothing records relayed posts", ErrDropActivity)
	}
	switch _, err := h.d.Statuses.FindStatusByURI(ctx, string(ref.ID)); {
	case err == nil:
		return nil
	case !errors.Is(err, appcontent.ErrNotFound):
		return err
	}
	if err := h.untombstoned(ctx, ref.ID, ""); err != nil {
		return fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	got, err := h.d.Fetcher.Fetch(ctx, string(ref.ID))
	if err != nil {
		return err
	}
	if got.Document.ID() != ref.ID {
		return fmt.Errorf("%w: %s is not an object", ErrDropActivity, ref.ID)
	}
	_, err = h.fetchedWith(ctx, got, storeOptions{relay: in.Relay})
	if errors.Is(err, ErrNotTaken) && !errors.Is(err, ErrFetchFailed) && !errors.Is(err, ErrFetchUnavailable) {
		return fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	return err
}

// inScope reports whether a post is related to a local account (ADR-0034): a local account
// follows its author, it mentions or addresses a local account, or it replies to a local post or
// into a thread that has one.
func (h *StatusHandlers) inScope(ctx context.Context, author appaccount.Actor, obj *federation.Object, m *postMentions) (bool, error) {
	if m.local {
		return true, nil
	}
	if followed, err := h.followed(ctx, author); err != nil || followed {
		return followed, err
	}
	if obj.InReplyTo == "" {
		return false, nil
	}
	parent, err := h.d.Statuses.FindStatusByURI(ctx, string(obj.InReplyTo))
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	case parent.IsLocal:
		return true, nil
	}
	conversation := parent.ConversationURI
	if conversation == "" {
		conversation = parent.URI
	}
	return h.d.Statuses.LocalStatusInConversation(ctx, conversation)
}

// followed reports whether a local account follows the remote author: its followers are local.
func (h *StatusHandlers) followed(ctx context.Context, author appaccount.Actor) (bool, error) {
	followers, _, err := h.d.Followers.ActiveFollowerIDs(ctx, author.Account.ID, "", 1)
	return len(followers) > 0, err
}

// followers returns the followers collection of a remote author, "" when it is unknown.
func (h *StatusHandlers) followers(ctx context.Context, author appaccount.Actor) (federation.IRI, error) {
	u, err := h.d.Accounts.RemoteFollowersURL(ctx, author.Account.ID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return "", nil
	}
	return federation.IRI(u), err
}

// postMentions are the actors a post mentions or addresses: the stored accounts, and the remote
// actors that are not stored yet.
type postMentions struct {
	accountIDs []string
	unknown    []federation.IRI
	local      bool
}

// mentions looks up the actors a post mentions (its Mention tags, and the local actors it
// addresses, federation.md §4) among the stored accounts.
func (h *StatusHandlers) mentions(ctx context.Context, obj *federation.Object, followers federation.IRI) (*postMentions, error) {
	m := &postMentions{}
	for _, iri := range obj.MentionedIRIs(followers, h.isLocal) {
		if len(m.accountIDs)+len(m.unknown) == MaxMentions {
			break
		}
		a, err := h.d.Accounts.FindByURI(ctx, string(iri))
		switch {
		case errors.Is(err, appaccount.ErrNotFound):
			if !h.isLocal(iri) {
				m.unknown = append(m.unknown, iri)
			}
			continue
		case err != nil:
			return nil, err
		}
		if a.Account.IsLocal() {
			if !a.Lifecycle.Active() {
				continue
			}
			m.local = true
		}
		m.accountIDs = append(m.accountIDs, a.Account.ID)
	}
	return m, nil
}

// resolveMentions fetches the remote actors of m that are not stored, at most
// MaxResolvedMentions of them within MentionResolveTimeout; those that cannot be resolved are
// left out of the mentions.
func (h *StatusHandlers) resolveMentions(ctx context.Context, m *postMentions) error {
	if len(m.unknown) == 0 || h.d.Actors == nil {
		return nil
	}
	budget, cancel := context.WithTimeout(ctx, MentionResolveTimeout)
	defer cancel()
	for _, iri := range m.unknown[:min(len(m.unknown), MaxResolvedMentions)] {
		a, err := h.d.Actors.Resolve(budget, string(iri))
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			h.d.Logger.InfoContext(ctx, "mentioned actor not resolved", slog.String("actor", string(iri)), slog.Any("reason", err))
			continue
		}
		m.accountIDs = append(m.accountIDs, a.Account.ID)
	}
	return nil
}

func (h *StatusHandlers) isLocal(iri federation.IRI) bool { return iri.Origin() == h.origin }

// input maps a normalized post that activity a carried to what the content module stores
// (federation.md §4). A post without addressing takes the addressing of the activity only when
// the post's author signed the activity: the addressing of an activity that someone else signed
// is not the author's, even when the post itself was fetched from its origin.
func (h *StatusHandlers) input(a *InboxActivity, obj *federation.Object, followers federation.IRI, m *postMentions) appcontent.RemoteStatusInput {
	var fallback federation.Audience
	if a.Received.Trust == federation.TrustSigned {
		fallback = a.Activity.Audience
	}
	return h.inputOf(a.Actor, fallback, string(a.Received.ActivityURI), obj, followers, m)
}

// inputOf maps a normalized post by author to what the content module stores (federation.md
// §4). A post without addressing takes fallback, the addressing of the activity that carried it;
// activityURI is that activity, "" for a post fetched by its id.
func (h *StatusHandlers) inputOf(author appaccount.Actor, fallback federation.Audience, activityURI string, obj *federation.Object, followers federation.IRI, m *postMentions) appcontent.RemoteStatusInput {
	audience := obj.Audience
	if len(audience.To) == 0 && len(audience.Cc) == 0 {
		audience = fallback
	}
	in := appcontent.RemoteStatusInput{
		URI:                 string(obj.ID),
		AuthorAccountID:     author.Account.ID,
		AuthorDomain:        author.Account.Domain,
		Visibility:          appcontent.Visibility(audience.Visibility(followers)),
		InReplyToURI:        string(obj.InReplyTo),
		PublishedAt:         obj.Published,
		RevisedAt:           obj.Updated,
		ActivityURI:         activityURI,
		SpoilerText:         obj.SpoilerText,
		Sensitive:           obj.Sensitive,
		MentionedAccountIDs: m.accountIDs,
		Hashtags:            obj.Hashtags,
	}
	if obj.URL != obj.ID {
		in.URL = string(obj.URL)
	}
	for _, b := range obj.Bodies {
		in.Bodies = append(in.Bodies, appcontent.RemoteBody{Language: b.Language, HTML: b.HTML})
	}
	for _, e := range obj.Emojis {
		in.Emojis = append(in.Emojis, appcontent.RemoteEmojiInput{URI: string(e.ID), Shortcode: e.Shortcode, ImageURL: string(e.IconURL)})
	}
	for _, at := range obj.Attachments {
		in.Attachments = append(in.Attachments, appcontent.RemoteAttachmentInput{
			Type: at.Type, URL: string(at.URL), ContentType: at.MediaType, Description: at.Name, Blurhash: at.Blurhash,
			Width: at.Width, Height: at.Height, Focus: at.FocalPoint,
		})
	}
	if p := obj.Poll; p != nil && len(p.Options) > 0 {
		in.Poll = pollInput(p)
	}
	in.Quote = quoteInput(obj.Quote)
	in.QuotePolicy = quotePolicyInput(obj.CanQuote, followers)
	return in
}

func pollInput(p *federation.Poll) *appcontent.RemotePollInput {
	in := &appcontent.RemotePollInput{Multiple: p.Multiple}
	closes := p.EndTime
	if closes.IsZero() {
		closes = p.Closed
	}
	if !closes.IsZero() {
		in.ClosesAt = &closes
	}
	var total int64
	for _, o := range p.Options {
		in.Options = append(in.Options, o.Name)
		in.Votes = append(in.Votes, int64(o.Votes))
		total += int64(o.Votes)
	}
	// Without votersCount, a single-choice poll has one voter per vote; a multiple-choice one has
	// an unknown number, unless nobody voted.
	switch voters := int64(p.VotersCount); {
	case voters > 0:
		in.VotersCount = &voters
	case !p.Multiple || total == 0:
		in.VotersCount = &total
	}
	return in
}
