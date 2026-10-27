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
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// Boosts, favourites, and emoji reactions across servers (federation.md §3.4, §7.1): the
// outbound projectors that turn a local account's favourite or emoji reaction of a remote status
// into Like and Undo Like, their fan-out to the status's author, and the inbox handlers of the Announce, Like,
// EmojiReact, and their Undo that remote servers send. The Announce and Undo Announce of local boosts are recorded with the
// other content activities (RegisterOutboundProjectors).

// registerFavouriteProjectors records the Like and Undo Like of a favourite of a remote status
// (federation.md §7.1). Remote accounts favourite only local statuses, so the favourite of a
// remote status is a local account's. A favourite is a current value whose row goes when it is
// taken back, so the activity names the status (object_id) and its actor the account, the pair
// that identifies the favourite, and its document is built here from the event and stored with
// it: the Undo Like embeds the Like by the favourite's URI after the row is gone.
func registerFavouriteProjectors(r *uow.Projectors, rec recorder) {
	record := func(ctx context.Context, tx uow.Tx, t federation.ActivityType, f appcontent.Favourite, st appcontent.StatusRef, at time.Time) error {
		if st.IsLocal || st.URI == "" {
			return nil
		}
		a := federation.OutboundActivity{
			Type: t, ActorAccountID: f.AccountID, ObjectKind: federation.OutboundFavourite, ObjectID: st.ID, CreatedAt: at,
		}
		if t == federation.ActivityLike {
			a.ActivityURI = federation.IRI(f.URI)
		}
		act, err := rec.insert(ctx, tx, a)
		if err != nil {
			return err
		}
		doc, err := likeDocument(act, rec.d.Host, f, st)
		if err != nil {
			return err
		}
		if _, err := rec.d.Repository.InsertOutboundDocument(ctx, tx, act.ID, doc, act.CreatedAt); err != nil {
			return err
		}
		return rec.enqueue(ctx, tx, act, fanOutPayload{})
	}
	r.Register(appcontent.EventFavourited, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.Favourited)
		if !ok {
			return nil
		}
		return record(ctx, tx, federation.ActivityLike, ev.Favourite, ev.Status, ev.Favourite.CreatedAt)
	})
	r.Register(appcontent.EventUnfavourited, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.Unfavourited)
		if !ok {
			return nil
		}
		return record(ctx, tx, federation.ActivityUndo, ev.Favourite, ev.Status, ev.RemovedAt)
	})
}

// registerReactionProjectors records the Like and Undo Like of a local account's emoji reaction
// to a remote status (ADR-0052 §3), as registerFavouriteProjectors does for a favourite: the same
// activity of the same Like slot, to the status's author, whose Like carries the emoji
// (reactionLikeDocument). Remote accounts react only to local statuses, so the reaction to a
// remote status is a local account's.
func registerReactionProjectors(r *uow.Projectors, rec recorder) {
	record := func(ctx context.Context, tx uow.Tx, t federation.ActivityType, re appcontent.Reaction, emoji *appcontent.EmojiTag, st appcontent.StatusRef, at time.Time) error {
		if st.IsLocal || st.URI == "" {
			return nil
		}
		a := federation.OutboundActivity{
			Type: t, ActorAccountID: re.AccountID, ObjectKind: federation.OutboundFavourite, ObjectID: st.ID, CreatedAt: at,
		}
		if t == federation.ActivityLike {
			a.ActivityURI = federation.IRI(re.URI)
		}
		act, err := rec.insert(ctx, tx, a)
		if err != nil {
			return err
		}
		doc, err := reactionLikeDocument(act, rec.d.Host, re, emoji, st)
		if err != nil {
			return err
		}
		if _, err := rec.d.Repository.InsertOutboundDocument(ctx, tx, act.ID, doc, act.CreatedAt); err != nil {
			return err
		}
		return rec.enqueue(ctx, tx, act, fanOutPayload{})
	}
	r.Register(appcontent.EventReacted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.Reacted)
		if !ok {
			return nil
		}
		return record(ctx, tx, federation.ActivityLike, ev.Reaction, ev.Emoji, ev.Status, ev.Reaction.CreatedAt)
	})
	r.Register(appcontent.EventUnreacted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev, ok := e.(appcontent.Unreacted)
		if !ok {
			return nil
		}
		return record(ctx, tx, federation.ActivityUndo, ev.Reaction, ev.Emoji, ev.Status, ev.RemovedAt)
	})
}

// reactionLikeDocument returns the Like of reaction re to the remote status st, or for an Undo
// the Undo that embeds it (ADR-0052 §3): the Like of likeDocument with the emoji as its content
// and _misskey_reaction, the Unicode emoji or ":shortcode:" of the custom emoji, and the Emoji tag
// of a custom emoji whose image can be shown. A custom emoji without its shortcode (one the
// command could not read) leaves the emoji out; the Undo still names the Like.
func reactionLikeDocument(act federation.OutboundActivity, host string, re appcontent.Reaction, emoji *appcontent.EmojiTag, st appcontent.StatusRef) ([]byte, error) {
	actor := federation.IRI(appaccount.ActorURI(host, re.AccountID))
	text := re.Emoji.Unicode
	var tag *federation.TagDocument
	if re.Emoji.CustomEmojiID != "" && emoji != nil && emoji.Shortcode != "" {
		text = ":" + emoji.Shortcode + ":"
		if emoji.URL != "" {
			t := EmojiTagOf(*emoji, host)
			tag = &t
		}
	}
	like := federation.NewReactionLike(federation.NewActivityDocument(federation.IRI(re.URI), federation.ActivityLike, actor, re.CreatedAt,
		federation.Audience{}, federation.IRI(st.URI)), text, tag)
	if act.Type == federation.ActivityLike {
		return like.Marshal()
	}
	undo := federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, federation.Audience{}, like.Embedded())
	undo.Context = federation.ReactionLDContext()
	return undo.Marshal()
}

// likeDocument returns the Like of favourite f of the remote status st, or for an Undo the Undo
// that embeds it. Like Mastodon's, they carry no addressing: they go to the status's author
// alone, and the Like's id is the favourite's URI.
func likeDocument(act federation.OutboundActivity, host string, f appcontent.Favourite, st appcontent.StatusRef) ([]byte, error) {
	actor := federation.IRI(appaccount.ActorURI(host, f.AccountID))
	like := federation.NewActivityDocument(federation.IRI(f.URI), federation.ActivityLike, actor, f.CreatedAt,
		federation.Audience{}, federation.IRI(st.URI))
	if act.Type == federation.ActivityLike {
		return like.Marshal()
	}
	return federation.NewActivityDocument(act.ActivityURI, act.Type, actor, act.CreatedAt, federation.Audience{}, like.Embedded()).Marshal()
}

// favouriteSubject loads the remote status of a Like or Undo Like, or of the Create of a vote in
// its poll, whose author receives it.
func (f *FanOut) favouriteSubject(ctx context.Context, s subject) (subject, error) {
	act := s.act
	switch {
	case act.ObjectKind == federation.OutboundPollVote && act.Type != federation.ActivityCreate,
		act.ObjectKind == federation.OutboundFavourite && act.Type != federation.ActivityLike && act.Type != federation.ActivityUndo:
		return subject{}, fmt.Errorf("%w: %s of a %s", federation.ErrInvalidOutbound, act.Type, act.ObjectKind)
	}
	st, err := f.d.Statuses.FindStatus(ctx, act.ObjectID)
	switch {
	case errors.Is(err, appcontent.ErrNotFound):
		return subject{}, fmt.Errorf("%w: status %s", errGone, act.ObjectID)
	case err != nil:
		return subject{}, err
	case st.IsLocal:
		return subject{}, fmt.Errorf("%w: %s of the %s of a local status %s", federation.ErrInvalidOutbound, act.Type, act.ObjectKind, st.ID)
	}
	s.status = st
	s.mentioned = []string{st.AuthorAccountID}
	return s, nil
}

// statusAuthorTargets returns the inbox of the author of a favourited status: its shared inbox,
// or its own inbox without one.
func (f *FanOut) statusAuthorTargets(ctx context.Context, s subject) ([]target, error) {
	inbox, err := f.d.Accounts.RemoteInbox(ctx, s.status.AuthorAccountID)
	if errors.Is(err, appaccount.ErrNotFound) {
		return nil, nil // a remote author never fetched
	}
	if err != nil {
		return nil, err
	}
	if t, ok := targetOf(federation.InboxFor(federation.IRI(inbox.InboxURL), federation.IRI(inbox.SharedInboxURL))); ok {
		return []target{t}, nil
	}
	return nil, nil
}

// RemoteInteractions is the part of the content module's public API that records the boosts,
// favourites, and emoji reactions of remote accounts (federation.md §1.1);
// *appcontent.Interactions implements it.
type RemoteInteractions interface {
	RecordRemoteBoost(ctx context.Context, in appcontent.RemoteBoostInput) (appcontent.Boost, error)
	RetractRemoteBoost(ctx context.Context, uri, boosterAccountID string) error
	RecordRemoteFavourite(ctx context.Context, in appcontent.RemoteFavouriteInput) (appcontent.Favourite, error)
	RemoveRemoteFavourite(ctx context.Context, uri, accountID, statusID string) error
	RecordRemoteReaction(ctx context.Context, in appcontent.RemoteReactionInput) (appcontent.Reaction, error)
	RemoveRemoteReaction(ctx context.Context, in appcontent.RemoteReactionUndo) error
}

// InteractionHandlersDeps are the ports of the boost, favourite, and reaction inbox handlers.
type InteractionHandlersDeps struct {
	Interactions RemoteInteractions
	// Statuses finds the posts that activities are about, and resolves and stores the remote
	// post that an Announce boosts.
	Statuses *StatusHandlers
}

// RegisterInteractionHandlers sets the inbox handlers of boosts, favourites, and emoji reactions
// (federation.md §3.4, ADR-0052 §1):
//
//   - Announce of a post: content.RecordRemoteBoost. The post is found by its id; a remote post
//     that is not stored is taken from the Announce when it is embedded from the actor's origin,
//     fetched from its origin otherwise, and stored only when a local account follows the
//     booster (ADR-0034). A local post must be one the booster may see and boost.
//   - EmojiReact of a local post whose content is an emoji (reactionOf):
//     content.RecordRemoteReaction; one with anything else is dropped.
//   - Like of a local post: content.RecordRemoteReaction when its _misskey_reaction, or else its
//     content, is an emoji (a Misskey reaction, FEP-c0e0), content.RecordRemoteFavourite
//     otherwise.
//   - Undo of an Announce: content.RetractRemoteBoost, by the URI of the Announce. Undo of a Like
//     or EmojiReact: the reaction with the URI of the activity taken back
//     (content.RemoveRemoteReaction), else the favourite with it (content.RemoveRemoteFavourite),
//     else what the embedded activity was: the actor's reaction to the post with the same emoji,
//     or its favourite of the post. An unknown one leaves a tombstone (federation.md §3.3).
func RegisterInteractionHandlers(p *InboxProcessor, d InteractionHandlersDeps) {
	h := interactionHandlers{d: d}
	p.Handle(federation.ActivityAnnounce, h.announce)
	p.Handle(federation.ActivityLike, h.like)
	p.Handle(federation.ActivityEmojiReact, h.like)
	p.HandleUndo(federation.ActivityAnnounce, h.undoAnnounce)
	p.HandleUndo(federation.ActivityLike, h.undoLike)
	p.HandleUndo(federation.ActivityEmojiReact, h.undoLike)
}

type interactionHandlers struct{ d InteractionHandlersDeps }

// interactionError turns the refusals of the content module's remote interaction commands into
// what the processor makes of them: a post that is missing, deleted, or hidden from the actor,
// or that the actor may not boost, has nothing to do (federation.md §3.3).
func interactionError(err error) error {
	switch {
	case errors.Is(err, appcontent.ErrNotFound), errors.Is(err, appcontent.ErrNotBoostable),
		errors.Is(err, appcontent.ErrActorUnavailable), errors.Is(err, appcontent.ErrNotAuthor),
		errors.Is(err, appcontent.ErrInvalidReaction), errors.Is(err, appcontent.ErrInteractionRestricted):
		return fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	return err
}

// activityURI returns the IRI of a received activity: its id, or the digest URI of one without.
func activityURI(in *InboxActivity) string {
	if in.Activity.ID != "" {
		return string(in.Activity.ID)
	}
	return string(in.Received.ActivityURI)
}

func (h interactionHandlers) announce(ctx context.Context, in *InboxActivity) error {
	if in.Relay != nil && in.Activity.Actor == in.Relay.ActorURI {
		// The Announce of a LitePub relay carries a post to take in, not a boost (ADR-0061).
		return h.d.Statuses.RelayedAnnounce(ctx, in)
	}
	st, err := h.d.Statuses.boosted(ctx, in)
	if err != nil {
		return err
	}
	followers, err := h.d.Statuses.followers(ctx, in.Actor)
	if err != nil {
		return err
	}
	_, err = h.d.Interactions.RecordRemoteBoost(ctx, appcontent.RemoteBoostInput{
		URI: activityURI(in), BoosterAccountID: in.Actor.Account.ID, StatusID: st.ID,
		Visibility: appcontent.Visibility(in.Activity.Audience.Visibility(followers)), BoostedAt: in.Activity.Published,
	})
	return interactionError(err)
}

func (h interactionHandlers) like(ctx context.Context, in *InboxActivity) error {
	id := in.Activity.Object().ID
	if id == "" || !h.d.Statuses.isLocal(id) {
		return fmt.Errorf("%w: %s of %q, not of a local post", ErrDropActivity, in.Activity.RawType, id)
	}
	emoji, isReaction := reactionOf(in.Activity, in.Actor.Account.Domain, h.d.Statuses.isLocal)
	if in.Activity.Type == federation.ActivityEmojiReact && !isReaction {
		return fmt.Errorf("%w: EmojiReact of %q, not an emoji", ErrDropActivity, in.Activity.Content)
	}
	st, err := h.d.Statuses.d.Statuses.FindStatusByURI(ctx, string(id))
	if err != nil {
		return interactionError(err)
	}
	if isReaction {
		_, err = h.d.Interactions.RecordRemoteReaction(ctx, appcontent.RemoteReactionInput{
			URI: activityURI(in), AccountID: in.Actor.Account.ID, StatusID: st.ID, Emoji: emoji, CreatedAt: in.Activity.Published,
		})
		return interactionError(err)
	}
	_, err = h.d.Interactions.RecordRemoteFavourite(ctx, appcontent.RemoteFavouriteInput{
		URI: activityURI(in), AccountID: in.Actor.Account.ID, StatusID: st.ID, CreatedAt: in.Activity.Published,
	})
	return interactionError(err)
}

// reactionOf returns the emoji of a Like or an EmojiReact, and whether it is an emoji reaction
// (ADR-0052 §1): its _misskey_reaction, or else its content, is one Unicode emoji or the
// ":shortcode:" of a custom emoji with exactly one Emoji tag of that name (with or without
// colons). A custom emoji is of the server of its Emoji's id, or of actorDomain without one. An
// emoji of another server than actorDomain is borrowed (RemoteReactionEmoji.Borrowed): one of this
// server (isLocal) is the local emoji, with the domain "".
func reactionOf(a *federation.Activity, actorDomain string, isLocal func(federation.IRI) bool) (appcontent.RemoteReactionEmoji, bool) {
	text := a.Reaction
	if text == "" {
		text = a.Content
	}
	if text == "" {
		return appcontent.RemoteReactionEmoji{}, false
	}
	if _, ok := appcontent.NormalizeUnicodeEmoji(text); ok {
		return appcontent.RemoteReactionEmoji{Unicode: text}, true
	}
	code, ok := appcontent.ParseReactionShortcode(text)
	if !ok {
		return appcontent.RemoteReactionEmoji{}, false
	}
	var found *federation.Emoji
	for i, e := range a.Emojis {
		if appcontent.TrimShortcode(e.Shortcode) != code {
			continue
		}
		if found != nil {
			return appcontent.RemoteReactionEmoji{}, false
		}
		found = &a.Emojis[i]
	}
	if found == nil || found.IconURL == "" {
		return appcontent.RemoteReactionEmoji{}, false
	}
	domain := actorDomain
	if h := found.ID.Host(); h != "" {
		domain = h
	}
	borrowed := !strings.EqualFold(domain, actorDomain)
	if borrowed && isLocal(found.ID) {
		domain = ""
	}
	return appcontent.RemoteReactionEmoji{
		Custom: &appcontent.RemoteEmojiInput{URI: string(found.ID), Shortcode: code, ImageURL: string(found.IconURL)},
		Domain: domain, Borrowed: borrowed,
	}, true
}

// undone returns the id of the activity that an Undo takes back, and the activity when the Undo
// embeds it. The embedded activity is a hint only: the content module matches it against the
// actor's own boosts and favourites.
func undone(in *InboxActivity) (federation.IRI, *federation.Activity) {
	ref := in.Activity.Object()
	if doc := ref.Untrusted(); doc != nil && doc.Activity != nil {
		return ref.ID, doc.Activity
	}
	return ref.ID, nil
}

func (h interactionHandlers) undoAnnounce(ctx context.Context, in *InboxActivity) error {
	id, _ := undone(in)
	if id == "" {
		return ErrUnknownObject
	}
	err := h.d.Interactions.RetractRemoteBoost(ctx, string(id), in.Actor.Account.ID)
	if errors.Is(err, appcontent.ErrNotFound) {
		return fmt.Errorf("%w: boost %s", ErrUnknownObject, id)
	}
	return interactionError(err)
}

func (h interactionHandlers) undoLike(ctx context.Context, in *InboxActivity) error {
	id, like := undone(in)
	if id == "" && like == nil {
		return ErrUnknownObject
	}
	account := in.Actor.Account.ID
	// The reaction, then the favourite, recorded from the activity's URI.
	err := h.d.Interactions.RemoveRemoteReaction(ctx, appcontent.RemoteReactionUndo{URI: string(id), AccountID: account})
	if !errors.Is(err, appcontent.ErrNotFound) {
		return interactionError(err)
	}
	err = h.d.Interactions.RemoveRemoteFavourite(ctx, string(id), account, "")
	if !errors.Is(err, appcontent.ErrNotFound) {
		return interactionError(err)
	}
	// Without either, the embedded activity names the post, and the emoji of a reaction, that
	// the actor took back.
	var statusID string
	if like != nil && like.Actor == in.Activity.Actor {
		if target := like.Object().ID; target != "" && h.d.Statuses.isLocal(target) {
			st, err := h.d.Statuses.d.Statuses.FindStatusByURI(ctx, string(target))
			switch {
			case err == nil:
				statusID = st.ID
			case !errors.Is(err, appcontent.ErrNotFound):
				return err
			}
		}
	}
	if statusID != "" {
		if emoji, ok := reactionOf(like, in.Actor.Account.Domain, h.d.Statuses.isLocal); ok {
			err = h.d.Interactions.RemoveRemoteReaction(ctx, appcontent.RemoteReactionUndo{
				URI: string(id), AccountID: account, StatusID: statusID, Emoji: emoji,
			})
		} else if like.Type == federation.ActivityLike {
			err = h.d.Interactions.RemoveRemoteFavourite(ctx, string(id), account, statusID)
		}
	}
	if errors.Is(err, appcontent.ErrNotFound) {
		return fmt.Errorf("%w: favourite or reaction %s", ErrUnknownObject, id)
	}
	return interactionError(err)
}

// boosted returns the post that an Announce boosts (federation.md §3.4, ADR-0034). A local post
// and a stored remote post are found by the object's id. A remote post that is not stored is
// stored only when a local account follows the booster: from the Announce when it embeds the
// post from the booster's origin, fetched from the post's origin otherwise. Anything else drops
// the Announce.
func (h *StatusHandlers) boosted(ctx context.Context, in *InboxActivity) (appcontent.Status, error) {
	act := in.Activity
	ref := act.Object()
	if ref.ID == "" {
		return appcontent.Status{}, fmt.Errorf("%w: Announce without an object", ErrDropActivity)
	}
	st, err := h.d.Statuses.FindStatusByURI(ctx, string(ref.ID))
	switch {
	case err == nil:
		return st, nil
	case !errors.Is(err, appcontent.ErrNotFound):
		return appcontent.Status{}, err
	case h.isLocal(ref.ID):
		return appcontent.Status{}, fmt.Errorf("%w: Announce of an unknown local post %s", ErrDropActivity, ref.ID)
	}
	followed, err := h.followed(ctx, in.Actor)
	if err != nil {
		return appcontent.Status{}, err
	}
	if !followed {
		return appcontent.Status{}, fmt.Errorf("%w: Announce of %s by %s, whom no local account follows (ADR-0034)", ErrDropActivity, ref.ID, act.Actor)
	}
	if err := h.untombstoned(ctx, ref.ID, ""); err != nil {
		return appcontent.Status{}, fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	got := &Fetched{}
	if federation.SameOrigin(ref.ID, act.Actor) {
		got.Document = ref.Trusted()
	}
	if got.Document == nil {
		if got, err = h.d.Fetcher.Fetch(ctx, string(ref.ID)); err != nil {
			return appcontent.Status{}, err
		}
	}
	if got.Document.ID() != ref.ID {
		return appcontent.Status{}, fmt.Errorf("%w: %s is not an object", ErrDropActivity, ref.ID)
	}
	// The same rules and storing as any fetched post (federation.md §6.3), its parent fetched one
	// level as on receipt.
	st, err = h.fetched(ctx, got, true)
	if errors.Is(err, ErrNotTaken) && !errors.Is(err, ErrFetchFailed) && !errors.Is(err, ErrFetchUnavailable) {
		return appcontent.Status{}, fmt.Errorf("%w: %w", ErrDropActivity, err)
	}
	return st, err
}
