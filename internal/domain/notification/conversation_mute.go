// SPDX-License-Identifier: AGPL-3.0-only

package notification

import "time"

// ConversationMute is a local account's mute of a conversation, a thread of statuses
// (ADR-0158 §8): the notifications of ThreadMutedTypes about its statuses are not created for the
// account while it lasts, whoever wrote them, the account's own statuses included. It has no
// expiry, and the conversation's statuses still appear in timelines.
type ConversationMute struct {
	AccountID      string
	ConversationID string
	CreatedAt      time.Time
}

// ThreadMuted reports whether a mute of the conversation of a status keeps a notification of
// type t about that status from being created (ADR-0158 §8): mention (replies included), boost,
// favourite, reaction, quote, and status. The status is the one that mentions for mention and
// status, the one interacted with for boost, favourite, and reaction, and the quoted one for
// quote. The other types, the notices of the instance and of the moderators among them, are
// never muted by a thread.
func ThreadMuted(t Type) bool {
	switch t {
	case TypeMention, TypeBoost, TypeFavourite, TypeReaction, TypeQuote, TypeStatus:
		return true
	}
	return false
}
