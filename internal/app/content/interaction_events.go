// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"time"

	"github.com/mycroblog/my/internal/domain/content"
)

// Event types of the interactions with statuses, for uow.Projectors (data-model §4.4). Each
// interaction command emits at most one of them inside its transaction, and only when it
// actually wrote a row: repeating a command that changes nothing emits nothing, so subscribers
// never count twice. Each event carries everything its subscribers need; they never read the
// content tables.
//
//   - EventBoosted: a boost was created. This module projects boost_edges and boosts_count;
//     the timeline module projects a feed entry of kind boost, the notification module a boost
//     notification to the status's author, and the federation module delivers the Announce
//     when Boost.Federates.
//   - EventBoostRetracted: a live boost was retracted. This module removes the edge and
//     decrements boosts_count; the timeline module removes the feed entry, and the federation
//     module delivers Undo Announce when the boost federated.
//   - EventFavourited: a favourite was inserted. This module increments favourites_count; the
//     notification module notifies the status's author, and the federation module delivers the
//     Like of a remote status.
//   - EventUnfavourited: a favourite was deleted. This module decrements favourites_count; the
//     federation module delivers Undo Like.
//   - EventReacted: an emoji reaction was inserted (ADR-0051). This module increments the count
//     of its emoji in status_reaction_counts; the notification module notifies the status's
//     author, and the federation module delivers the Like with the emoji of a remote status.
//   - EventUnreacted: an emoji reaction was deleted. This module decrements the count of its
//     emoji; the federation module delivers Undo Like.
//
// Bookmarks and pins are private to their account and emit no events.
const (
	EventBoosted        = "content.boosted"
	EventBoostRetracted = "content.boost_retracted"
	EventFavourited     = "content.favourited"
	EventUnfavourited   = "content.unfavourited"
	EventReacted        = "content.reacted"
	EventUnreacted      = "content.unreacted"
)

// StatusRef is what subscribers of an interaction event need to know about the status.
type StatusRef struct {
	ID              string
	URI             string
	AuthorAccountID string
	IsLocal         bool
	Visibility      content.Visibility
}

// RefOf returns the reference to s.
func RefOf(s content.Status) StatusRef {
	return StatusRef{ID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, IsLocal: s.IsLocal, Visibility: s.Visibility}
}

// Boosted is emitted when a boost is inserted.
type Boosted struct {
	Boost  content.Boost
	Status StatusRef
}

// EventType implements uow.Event.
func (Boosted) EventType() string { return EventBoosted }

// BoostRetracted is emitted when the retraction of a live boost is inserted.
type BoostRetracted struct {
	Boost      content.Boost
	Retraction content.BoostRetraction
	Status     StatusRef
}

// EventType implements uow.Event.
func (BoostRetracted) EventType() string { return EventBoostRetracted }

// Favourited is emitted when a favourite is inserted.
type Favourited struct {
	Favourite content.Favourite
	Status    StatusRef
}

// EventType implements uow.Event.
func (Favourited) EventType() string { return EventFavourited }

// Unfavourited is emitted when a favourite is deleted.
type Unfavourited struct {
	Favourite content.Favourite
	Status    StatusRef
	// RemovedAt is when the favourite was deleted.
	RemovedAt time.Time
}

// EventType implements uow.Event.
func (Unfavourited) EventType() string { return EventUnfavourited }

// Reacted is emitted when a reaction is inserted.
type Reacted struct {
	Reaction content.Reaction
	Status   StatusRef
	// Emoji is the custom emoji of a local account's reaction to a remote status, for the Like
	// that carries it (ADR-0051 §8); nil otherwise. Its URL is "" when its image cannot be shown.
	Emoji *EmojiTag
}

// EventType implements uow.Event.
func (Reacted) EventType() string { return EventReacted }

// Unreacted is emitted when a reaction is deleted.
type Unreacted struct {
	Reaction content.Reaction
	Status   StatusRef
	// RemovedAt is when the reaction was deleted.
	RemovedAt time.Time
	// Emoji is as in Reacted.
	Emoji *EmojiTag
}

// EventType implements uow.Event.
func (Unreacted) EventType() string { return EventUnreacted }
