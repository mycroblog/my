// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds what the Reader presents of the emoji reactions to a status (ADR-0051 §10): the
// counts per emoji on the status, and the list of the accounts that reacted.

// ReactionView is the count of the reactions to a status with one emoji, as a viewer sees it.
type ReactionView struct {
	EmojiView
	Count int64
	// Me is whether the viewer reacted with the emoji; always false for an anonymous viewer.
	Me bool
}

// EmojiView is the emoji of a reaction with what a client shows of it.
type EmojiView struct {
	Emoji content.ReactionEmoji
	// Custom is the custom emoji, nil for a Unicode emoji.
	Custom *ReactionCustomEmoji
}

// Name returns the name that clients show and filter the reactions with: the Unicode emoji,
// the shortcode of a custom emoji of this instance, or "shortcode@domain" for one of another
// server.
func (v EmojiView) Name() string {
	switch c := v.Custom; {
	case c == nil:
		return v.Emoji.Unicode
	case c.Domain == "":
		return c.Shortcode
	default:
		return c.Shortcode + "@" + c.Domain
	}
}

// ReactionCustomEmoji is the custom emoji of a reaction: its shortcode and server, with its image
// as clients show it (MediaService.CustomEmojis), nil when it cannot be shown.
type ReactionCustomEmoji struct {
	ID        string
	Shortcode string
	// Domain is the server of the emoji, "" for this instance.
	Domain string
	Image  *CustomEmojiView
}

// ReactedView is one reaction to a status, for the list of the accounts that reacted.
type ReactedView struct {
	Reaction content.Reaction
	Emoji    EmojiView
}

// ReactedPage is a page of the reactions to a status.
type ReactedPage struct {
	Reactions []ReactedView
	// Next is the cursor of the next page, "" on the last page.
	Next string
}

// Reactions returns a page of the reactions to a status that viewer may see, newest first: up to
// limit reactions older than the cursor before ("" for the newest), only those with the emoji
// whose name (EmojiView.Name) is emoji unless it is "". An emoji that no reaction can have
// gives an empty page, and a reaction whose emoji was deleted is left out. A status that the viewer may not see fails with ErrNotFound.
func (r *Reader) Reactions(ctx context.Context, viewer Viewer, statusID, emoji, before string, limit int) (ReactedPage, error) {
	if _, _, err := r.aud.visibleStatus(ctx, viewer, statusID); err != nil {
		return ReactedPage{}, err
	}
	var key string
	if emoji != "" {
		var ok bool
		var err error
		if key, ok, err = r.emojiKey(ctx, emoji); err != nil || !ok {
			return ReactedPage{}, err
		}
	}
	emojis := emojiCache{r: r, seen: map[string]*ReactionCustomEmoji{}}
	out := ReactedPage{Reactions: make([]ReactedView, 0, max(limit, 0))}
	// More than one read only when reactions were left out, to fill the page.
	for {
		want := limit - len(out.Reactions)
		rs, err := r.d.Interactions.ListStatusReactions(ctx, statusID, key, before, want)
		if err != nil {
			return ReactedPage{}, err
		}
		for _, re := range rs {
			v, err := emojis.view(ctx, re.Emoji)
			if errors.Is(err, ErrNotFound) {
				continue // its emoji was deleted, see reactions
			}
			if err != nil {
				return ReactedPage{}, err
			}
			out.Reactions = append(out.Reactions, ReactedView{Reaction: re, Emoji: v})
		}
		if len(rs) == 0 || len(rs) < want {
			return out, nil // the last page
		}
		before = rs[len(rs)-1].ID
		if len(out.Reactions) == limit {
			out.Next = before
			return out, nil
		}
	}
}

// emojiKey returns the key (content.ReactionEmoji.Key) of the emoji whose name is name, and
// false when no stored emoji has it.
func (r *Reader) emojiKey(ctx context.Context, name string) (string, bool, error) {
	if u, ok := content.NormalizeUnicodeEmoji(name); ok {
		return u, true, nil
	}
	shortcode, domain, _ := strings.Cut(name, "@")
	e, err := r.d.Statuses.FindCustomEmojiByShortcode(ctx, shortcode, domain)
	switch {
	case errors.Is(err, ErrNotFound):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return e.ID, true, nil
}

// reactions returns the counts of the reactions to st, the most used content.MaxStatusReactions
// emoji, with whether accountID ("" for an anonymous viewer) reacted with each. An emoji that was
// deleted is left out.
func (r *Reader) reactions(ctx context.Context, st content.Status, accountID string) ([]ReactionView, error) {
	counts, err := r.d.Interactions.ListReactionCounts(ctx, st.ID, content.MaxStatusReactions)
	if err != nil || len(counts) == 0 {
		return nil, err
	}
	mine := map[string]bool{}
	if accountID != "" {
		rs, err := r.d.Interactions.ListAccountStatusReactions(ctx, accountID, st.ID)
		if err != nil {
			return nil, err
		}
		for _, re := range rs {
			mine[re.Emoji.Key()] = true
		}
	}
	emojis := emojiCache{r: r, seen: r.reactionEmoji}
	if emojis.seen == nil {
		emojis.seen = map[string]*ReactionCustomEmoji{}
	}
	out := make([]ReactionView, 0, len(counts))
	for _, c := range counts {
		v, err := emojis.view(ctx, c.Emoji)
		if errors.Is(err, ErrNotFound) {
			// A deleted local emoji leaves its reactions behind with nothing to show them with,
			// which must not make the status they are on unreadable.
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, ReactionView{EmojiView: v, Count: c.Count, Me: mine[c.Emoji.Key()]})
	}
	return out, nil
}

// emojiCache presents the emoji of reactions, reading each custom emoji once.
type emojiCache struct {
	r    *Reader
	seen map[string]*ReactionCustomEmoji
}

func (c emojiCache) view(ctx context.Context, e content.ReactionEmoji) (EmojiView, error) {
	if e.CustomEmojiID == "" {
		return EmojiView{Emoji: e}, nil
	}
	if v, ok := c.seen[e.CustomEmojiID]; ok {
		return EmojiView{Emoji: e, Custom: v}, nil
	}
	emoji, err := c.r.d.Statuses.FindCustomEmoji(ctx, e.CustomEmojiID)
	if err != nil {
		return EmojiView{}, err
	}
	v := &ReactionCustomEmoji{ID: emoji.ID, Shortcode: emoji.Shortcode, Domain: emoji.Domain}
	if c.r.d.Media != nil {
		images, err := c.r.d.Media.CustomEmojis(ctx, []string{emoji.ID})
		if err != nil {
			return EmojiView{}, err
		}
		if img, ok := images[emoji.ID]; ok {
			v.Image = &img
		}
	}
	c.seen[e.CustomEmojiID] = v
	return EmojiView{Emoji: e, Custom: v}, nil
}

// load reads the custom emoji of ids that the cache has not seen, together: their rows and their
// images (MediaService.CustomEmojis). An emoji that does not exist is not recorded, so that view
// fails with ErrNotFound for it as it does alone.
func (c emojiCache) load(ctx context.Context, ids []string) error {
	var unknown []string
	for _, id := range ids {
		if _, ok := c.seen[id]; !ok && id != "" && !slices.Contains(unknown, id) {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	emojis, err := c.r.d.Statuses.FindCustomEmojis(ctx, unknown)
	if err != nil {
		return err
	}
	images := map[string]CustomEmojiView{}
	if c.r.d.Media != nil && len(emojis) > 0 {
		found := make([]string, 0, len(emojis))
		for id := range emojis {
			found = append(found, id)
		}
		if images, err = c.r.d.Media.CustomEmojis(ctx, found); err != nil {
			return err
		}
	}
	for id, emoji := range emojis {
		v := &ReactionCustomEmoji{ID: emoji.ID, Shortcode: emoji.Shortcode, Domain: emoji.Domain}
		if img, ok := images[id]; ok {
			v.Image = &img
		}
		c.seen[id] = v
	}
	return nil
}

// ReactionEmojis returns the emoji of many reactions with what a client shows of them, as
// ReactionEmoji returns one, by their keys (content.ReactionEmoji.Key); one whose custom emoji is
// gone is left out. It reads the custom emoji together.
func (r *Reader) ReactionEmojis(ctx context.Context, emojis []content.ReactionEmoji) (map[string]EmojiView, error) {
	cache := emojiCache{r: r, seen: map[string]*ReactionCustomEmoji{}}
	ids := make([]string, 0, len(emojis))
	for _, e := range emojis {
		ids = append(ids, e.CustomEmojiID)
	}
	if err := cache.load(ctx, ids); err != nil {
		return nil, err
	}
	out := make(map[string]EmojiView, len(emojis))
	for _, e := range emojis {
		v, err := cache.view(ctx, e)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[e.Key()] = v
	}
	return out, nil
}

// ReactionEmoji returns the emoji of a reaction with what a client shows of it, for the
// notifications of reactions.
func (r *Reader) ReactionEmoji(ctx context.Context, e content.ReactionEmoji) (EmojiView, error) {
	return emojiCache{r: r, seen: map[string]*ReactionCustomEmoji{}}.view(ctx, e)
}
