// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds the emoji reactions of local accounts (FR-116, ADR-0052 §1, §2) and the Like
// slot they share with favourites: a local account has, on each status, a favourite or one
// reaction, never both and never two reactions, so that the one Like it sends to the status's
// server always says what it has. Taking one fills the slot and takes back what was in it, in the
// same transaction; the federation module sends the Undo of what was taken back before the new
// Like.

// React makes accountID react to statusID with emoji and returns the reaction. emoji is a Unicode
// emoji, or the shortcode (with or without colons) of an enabled custom emoji of this instance
// whose image can be shown; anything else, a custom emoji of another server among them, fails
// with content.ErrInvalidReaction (ADR-0052 §2). Reacting takes back the account's favourite of
// the status and its reaction with another emoji; reacting again with the same emoji returns the
// existing reaction and changes nothing. The checks are those of Favourite: a status the account
// cannot see fails with ErrNotFound, and an account under the interact restriction with
// ErrInteractionRestricted.
func (s *Interactions) React(ctx context.Context, accountID, statusID, emoji string) (content.Reaction, error) {
	if err := s.checkRestrictions(ctx, accountID, false); err != nil {
		return content.Reaction{}, err
	}
	st, err := s.visibleStatus(ctx, accountID, statusID)
	if err != nil {
		return content.Reaction{}, err
	}
	e, tag, err := s.localReactionEmoji(ctx, emoji)
	if err != nil {
		return content.Reaction{}, err
	}
	taken, err := s.slotEmojis(ctx, accountID, st)
	if err != nil {
		return content.Reaction{}, err
	}
	id := s.d.IDs.NewID()
	r := content.Reaction{
		ID: id, AccountID: accountID, StatusID: st.ID, Emoji: e, URI: content.ActivityURI(s.d.Host, id), CreatedAt: s.d.Clock.Now(),
	}
	if st.IsLocal {
		tag = nil // nothing federates
	}
	inserted := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted = false
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		if err := s.emptySlot(ctx, tx, accountID, st, e.Key(), taken); err != nil {
			return err
		}
		ok, err := s.d.Interactions.InsertReaction(ctx, tx, r)
		if err != nil || !ok {
			return err
		}
		inserted = true
		return tx.Emit(ctx, Reacted{Reaction: r, Status: RefOf(st), Emoji: tag})
	})
	if err != nil {
		return content.Reaction{}, err
	}
	if inserted {
		return r, nil
	}
	return s.d.Interactions.FindReaction(ctx, accountID, st.ID, e.Key())
}

// Unreact takes back accountID's reaction to statusID. Without one it changes nothing.
func (s *Interactions) Unreact(ctx context.Context, accountID, statusID string) error {
	st, err := s.d.Statuses.FindStatus(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	taken, err := s.slotEmojis(ctx, accountID, st)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.takeBackReactions(ctx, tx, accountID, st, "", taken, now)
	})
}

// emptySlot takes back what fills accountID's Like slot on st, except a reaction with the emoji
// whose key is keep ("" for none): its favourite and its reactions, each with its event.
func (s *Interactions) emptySlot(ctx context.Context, tx uow.Tx, accountID string, st content.Status, keep string, taken map[string]*EmojiTag) error {
	now := s.d.Clock.Now()
	f, deleted, err := s.d.Interactions.DeleteFavourite(ctx, tx, accountID, st.ID)
	if err != nil {
		return err
	}
	if deleted {
		if err := tx.Emit(ctx, Unfavourited{Favourite: f, Status: RefOf(st), RemovedAt: now}); err != nil {
			return err
		}
	}
	return s.takeBackReactions(ctx, tx, accountID, st, keep, taken, now)
}

// takeBackReactions deletes accountID's reactions to st except the one with the emoji whose key
// is keep, each with its Unreacted.
func (s *Interactions) takeBackReactions(ctx context.Context, tx uow.Tx, accountID string, st content.Status, keep string, taken map[string]*EmojiTag, now time.Time) error {
	gone, err := s.d.Interactions.DeleteAccountStatusReactions(ctx, tx, accountID, st.ID, keep)
	if err != nil {
		return err
	}
	for _, r := range gone {
		ev := Unreacted{Reaction: r, Status: RefOf(st), RemovedAt: now}
		if id := r.Emoji.CustomEmojiID; id != "" && !st.IsLocal {
			ev.Emoji = taken[id]
			if ev.Emoji == nil {
				// Reacted after slotEmojis read the slot; the Undo goes without the emoji.
				ev.Emoji = &EmojiTag{ID: id, Local: true}
			}
		}
		if err := tx.Emit(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

// slotEmojis returns the custom emoji of accountID's reactions to a remote status st, keyed by
// ID, for the Undo of each: read before the transaction that takes them back, because their
// images are not read in it. A local status's reactions federate nowhere and need none.
func (s *Interactions) slotEmojis(ctx context.Context, accountID string, st content.Status) (map[string]*EmojiTag, error) {
	if st.IsLocal {
		return nil, nil
	}
	rs, err := s.d.Interactions.ListAccountStatusReactions(ctx, accountID, st.ID)
	if err != nil {
		return nil, err
	}
	out := map[string]*EmojiTag{}
	for _, r := range rs {
		id := r.Emoji.CustomEmojiID
		if id == "" {
			continue
		}
		tag, err := s.emojiTag(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = tag
	}
	return out, nil
}

// emojiTag returns the Emoji tag of the custom emoji id: without its image (URL "") when it is
// disabled or its image cannot be shown.
func (s *Interactions) emojiTag(ctx context.Context, id string) (*EmojiTag, error) {
	if s.d.EmojiTags != nil {
		tags, err := s.d.EmojiTags(ctx, []string{id})
		if err != nil {
			return nil, err
		}
		if len(tags) == 1 {
			return &tags[0], nil
		}
	}
	e, err := s.d.Statuses.FindCustomEmoji(ctx, id)
	if err != nil {
		return nil, err
	}
	return &EmojiTag{ID: e.ID, Shortcode: e.Shortcode, Local: e.Domain == "", URI: e.URI}, nil
}

// ReactionEmoji returns the emoji that a local account reacts with, as a voice chat room
// carries it (ADR-0100 §10): a Unicode emoji, normalized, or the :shortcode: of an enabled
// custom emoji of this instance whose image can be shown. Anything else fails with
// content.ErrInvalidReaction.
func (s *Interactions) ReactionEmoji(ctx context.Context, name string) (string, error) {
	e, tag, err := s.localReactionEmoji(ctx, name)
	if err != nil {
		return "", err
	}
	if e.Unicode != "" || tag == nil {
		return e.Unicode, nil
	}
	// The stored shortcode, whatever the spelling of name.
	return ":" + tag.Shortcode + ":", nil
}

// LocalEmoji returns the emoji that a local account gives a voice chat room as its icon
// (FR-923), under the rules of the emoji it reacts with (ADR-0052 §2): a Unicode emoji,
// normalized, or the ID of an enabled custom emoji of this instance whose image can be shown.
// Anything else fails with content.ErrInvalidReaction.
func (s *Interactions) LocalEmoji(ctx context.Context, name string) (unicode, customEmojiID string, err error) {
	e, _, err := s.localReactionEmoji(ctx, name)
	if err != nil {
		return "", "", err
	}
	return e.Unicode, e.CustomEmojiID, nil
}

// localReactionEmoji returns the emoji that a local account reacts with (ADR-0052 §2), with the
// Emoji tag of a custom emoji: a Unicode emoji, normalized, or an enabled custom emoji of this
// instance whose image can be shown.
func (s *Interactions) localReactionEmoji(ctx context.Context, name string) (content.ReactionEmoji, *EmojiTag, error) {
	if u, ok := content.NormalizeUnicodeEmoji(name); ok {
		return content.ReactionEmoji{Unicode: u}, nil, nil
	}
	code := content.TrimShortcode(name)
	if code == "" || strings.Contains(name, "@") || s.d.EmojiTags == nil {
		return content.ReactionEmoji{}, nil, fmt.Errorf("emoji %q: %w", name, content.ErrInvalidReaction)
	}
	e, err := s.d.Statuses.FindCustomEmojiByShortcode(ctx, code, "")
	if errors.Is(err, ErrNotFound) {
		return content.ReactionEmoji{}, nil, fmt.Errorf("emoji %q: %w", name, content.ErrInvalidReaction)
	}
	if err != nil {
		return content.ReactionEmoji{}, nil, err
	}
	tags, err := s.d.EmojiTags(ctx, []string{e.ID})
	if err != nil {
		return content.ReactionEmoji{}, nil, err
	}
	if len(tags) != 1 {
		return content.ReactionEmoji{}, nil, fmt.Errorf("emoji %q, disabled or without an image: %w", name, content.ErrInvalidReaction)
	}
	return content.ReactionEmoji{CustomEmojiID: e.ID}, &tags[0], nil
}
