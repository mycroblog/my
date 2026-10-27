// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds the emoji reactions of remote accounts (FR-116, ADR-0052), which the federation
// module records from the EmojiReact and Like activities it receives and takes back on their
// Undo (docs/architecture/federation.md §3.4). Like the remote favourites, they are of local
// statuses only, emit the same events as a local command would, and send nothing.

// The emoji reactions as part of this module's public API, for the federation module, which
// decides from them whether a Like is a reaction (ADR-0052 §1).
type (
	// Reaction is content.Reaction.
	Reaction = content.Reaction
	// ReactionEmoji is content.ReactionEmoji.
	ReactionEmoji = content.ReactionEmoji
)

// ErrInvalidReaction is content.ErrInvalidReaction.
var ErrInvalidReaction = content.ErrInvalidReaction

// NormalizeUnicodeEmoji is content.NormalizeUnicodeEmoji.
func NormalizeUnicodeEmoji(s string) (string, bool) { return content.NormalizeUnicodeEmoji(s) }

// ParseReactionShortcode is content.ParseReactionShortcode.
func ParseReactionShortcode(s string) (string, bool) { return content.ParseReactionShortcode(s) }

// TrimShortcode is content.TrimShortcode.
func TrimShortcode(name string) string { return content.TrimShortcode(name) }

// RemoteReactionEmoji is the emoji of a received reaction: a Unicode emoji, or a custom emoji
// with its Emoji tag. Exactly one is set.
type RemoteReactionEmoji struct {
	// Unicode is the emoji as received; the commands normalize it (content.NormalizeUnicodeEmoji).
	Unicode string
	// Custom is the custom emoji, its Shortcode without colons.
	Custom *RemoteEmojiInput
	// Domain is the domain of the custom emoji: the host of its Emoji's id, or the reacting
	// account's domain when the Emoji has no id (ADR-0052 §1); "" for an emoji of this server.
	Domain string
	// Borrowed reports a custom emoji of another server than the reacting account's. Only that
	// server's documents record its emoji: a borrowed emoji is taken only when it is stored
	// already, and the reaction changes nothing of it.
	Borrowed bool
}

// RemoteReactionInput is the reaction that an EmojiReact, or a Like with an emoji, of a remote
// account records.
type RemoteReactionInput struct {
	// URI is the IRI of the activity.
	URI       string
	AccountID string
	StatusID  string
	Emoji     RemoteReactionEmoji
	// CreatedAt is the published time of the activity; zero, or a time in the future, is now.
	CreatedAt time.Time
}

// RecordRemoteReaction records the emoji reaction of a remote account (an EmojiReact, or a Like
// with an emoji, federation.md §3.4) and returns it. The rules are those of RecordRemoteFavourite:
// only a local status takes remote reactions, only one the account can see (content.CanView in
// ContextObjectFetch, which refuses a block either way), and anything else fails with
// ErrNotFound; an account under the interact restriction (FR-708) fails with
// ErrInteractionRestricted. An emoji that is neither one Unicode emoji nor a custom emoji with its
// image fails with content.ErrInvalidReaction. A custom emoji is stored with its image, as the
// custom emoji of remote posts are.
//
// Recording a reaction that the account already has with the emoji, or an activity whose URI is
// recorded already, changes nothing and returns the existing reaction. A reaction with another
// emoji is another reaction: the account may react with several (ADR-0052 §1).
func (s *Interactions) RecordRemoteReaction(ctx context.Context, in RemoteReactionInput) (content.Reaction, error) {
	unicode, err := remoteUnicode(in.Emoji)
	if err != nil {
		return content.Reaction{}, err
	}
	if err := s.checkRestrictions(ctx, in.AccountID, false); err != nil {
		return content.Reaction{}, err
	}
	st, err := s.remoteVisibleStatus(ctx, in.AccountID, in.StatusID)
	if err != nil {
		return content.Reaction{}, err
	}
	if !st.IsLocal {
		return content.Reaction{}, fmt.Errorf("remote status %s: %w", st.ID, ErrNotFound)
	}
	at := s.remoteTime(in.CreatedAt)
	r := content.Reaction{
		ID: s.d.IDs.NewIDAt(at), AccountID: in.AccountID, StatusID: st.ID, Emoji: content.ReactionEmoji{Unicode: unicode},
		URI: in.URI, CreatedAt: at,
	}
	inserted := false
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted = false
		if err := s.ensureLive(ctx, tx, st.ID); err != nil {
			return err
		}
		if c := in.Emoji.Custom; c != nil && in.Emoji.Borrowed {
			emoji, err := s.d.Statuses.FindCustomEmojiByShortcode(ctx, c.Shortcode, in.Emoji.Domain)
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("custom emoji %q of %s is not stored: %w", c.Shortcode, in.Emoji.Domain, content.ErrInvalidReaction)
			}
			if err != nil {
				return err
			}
			r.Emoji = content.ReactionEmoji{CustomEmojiID: emoji.ID}
		} else if c != nil {
			if s.d.RecordRemoteEmojis == nil {
				return errors.New("content: remote custom emoji reactions need a custom emoji recorder")
			}
			ids, err := s.d.RecordRemoteEmojis(ctx, tx, in.AccountID, in.Emoji.Domain, []RemoteEmojiInput{*c})
			if err != nil {
				return err
			}
			if len(ids) != 1 {
				return fmt.Errorf("custom emoji %q: %w", c.Shortcode, content.ErrInvalidReaction)
			}
			r.Emoji = content.ReactionEmoji{CustomEmojiID: ids[0]}
		}
		ok, err := s.d.Interactions.InsertReaction(ctx, tx, r)
		if err != nil || !ok {
			return err
		}
		inserted = true
		return tx.Emit(ctx, Reacted{Reaction: r, Status: RefOf(st)})
	})
	switch {
	case errors.Is(err, ErrReactionURIUsed):
		return s.d.Interactions.FindReactionByURI(ctx, in.URI)
	case err != nil:
		return content.Reaction{}, err
	case inserted:
		return r, nil
	}
	return s.d.Interactions.FindReaction(ctx, in.AccountID, st.ID, r.Emoji.Key())
}

// RemoteReactionUndo is what an Undo of a reaction names: the URI of the activity taken back,
// and, when the Undo embeds it, the status and the emoji of the reaction.
type RemoteReactionUndo struct {
	URI       string
	AccountID string
	// StatusID and Emoji are the reaction that the Undo embeds; StatusID is "" without one.
	StatusID string
	Emoji    RemoteReactionEmoji
}

// RemoveRemoteReaction removes the reaction of a remote account that an Undo takes back: the
// reaction whose activity is in.URI, or, when no reaction has that URI, the account's reaction to
// in.StatusID with in.Emoji ("" for none). A reaction that does not exist fails with
// ErrNotFound, and one of another account with ErrNotAuthor.
func (s *Interactions) RemoveRemoteReaction(ctx context.Context, in RemoteReactionUndo) error {
	r, err := s.d.Interactions.FindReactionByURI(ctx, in.URI)
	switch {
	case err == nil:
		if r.AccountID != in.AccountID {
			return fmt.Errorf("reaction %s: %w", in.URI, ErrNotAuthor)
		}
	case !errors.Is(err, ErrNotFound):
		return err
	case in.StatusID == "":
		return err
	default:
		key, err := s.remoteEmojiKey(ctx, in.Emoji)
		if err != nil {
			return err
		}
		if r, err = s.d.Interactions.FindReaction(ctx, in.AccountID, in.StatusID, key); err != nil {
			return err
		}
	}
	st, err := s.d.Statuses.FindStatus(ctx, r.StatusID)
	if err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		gone, deleted, err := s.d.Interactions.DeleteReaction(ctx, tx, r.ID)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("reaction %s: %w", r.ID, ErrNotFound)
		}
		return tx.Emit(ctx, Unreacted{Reaction: gone, Status: RefOf(st), RemovedAt: s.d.Clock.Now()})
	})
}

// remoteUnicode returns the normalized Unicode emoji of e, "" for a custom emoji, and fails with
// content.ErrInvalidReaction when e is neither.
func remoteUnicode(e RemoteReactionEmoji) (string, error) {
	switch c := e.Custom; {
	case c != nil && e.Unicode == "":
		if c.Shortcode == "" || c.ImageURL == "" || (e.Domain == "" && !e.Borrowed) {
			return "", fmt.Errorf("custom emoji %q: %w", c.Shortcode, content.ErrInvalidReaction)
		}
		return "", nil
	case c == nil:
		if u, ok := content.NormalizeUnicodeEmoji(e.Unicode); ok {
			return u, nil
		}
	}
	return "", fmt.Errorf("emoji %q: %w", e.Unicode, content.ErrInvalidReaction)
}

// remoteEmojiKey returns the key of the emoji e (content.ReactionEmoji.Key): the normalized
// Unicode emoji, or the ID of the stored custom emoji. A custom emoji that is not stored fails
// with ErrNotFound.
func (s *Interactions) remoteEmojiKey(ctx context.Context, e RemoteReactionEmoji) (string, error) {
	unicode, err := remoteUnicode(e)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	if unicode != "" {
		return unicode, nil
	}
	emoji, err := s.d.Statuses.FindCustomEmojiByShortcode(ctx, e.Custom.Shortcode, e.Domain)
	if err != nil {
		return "", err
	}
	return emoji.ID, nil
}
