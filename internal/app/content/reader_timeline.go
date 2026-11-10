// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds what the timelines of the client API read besides the timeline module's
// pages: the statuses and boosts of a page, hashtags by name, and the pinned statuses of a
// profile.

// Views returns the statuses of ids as viewer sees them, in the order of ids. Statuses that
// are missing, deleted, or hidden from the viewer are left out, so that a page read before a
// deletion or a new block shows none of them. It reads the statuses and their authors once for
// the page (Reader.page), and decides and presents each status as View does.
func (r *Reader) Views(ctx context.Context, viewer Viewer, ids []string) ([]StatusView, error) {
	page, err := r.page(ctx, viewer, ids)
	if err != nil {
		return nil, err
	}
	out := make([]StatusView, 0, len(ids))
	for _, id := range ids {
		v, err := page.View(ctx, viewer, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Boosts returns the boosts of ids, in their order, read together; missing ones are left out. It does not
// decide who may see them: the timeline module lists only the boosts that the viewer may see,
// and Views decides about the boosted statuses.
func (r *Reader) Boosts(ctx context.Context, ids []string) ([]content.Boost, error) {
	found, err := r.d.Interactions.FindBoosts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]content.Boost, 0, len(ids))
	for _, id := range ids {
		if b, ok := found[id]; ok {
			out = append(out, b)
		}
	}
	return out, nil
}

// Tag returns the hashtag name, matched under NormalizeTag, failing with ErrNotFound when no
// status ever used it.
func (r *Reader) Tag(ctx context.Context, name string) (content.Tag, error) {
	return r.d.Statuses.FindTagByName(ctx, NormalizeTag(name))
}

// Pinned returns the statuses that accountID pins to its profile and viewer may see, newest
// pin first.
func (r *Reader) Pinned(ctx context.Context, viewer Viewer, accountID string) ([]StatusView, error) {
	pins, err := r.d.Interactions.ListPins(ctx, accountID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(pins))
	for _, p := range pins {
		ids = append(ids, p.StatusID)
	}
	return r.Views(ctx, viewer, ids)
}
