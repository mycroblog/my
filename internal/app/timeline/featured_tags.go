// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"slices"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// The featured hashtags of profiles (ADR-0006 §17): an account features up to
// timeline.MaxFeaturedTags hashtags in an order of its choosing. A local account sets them with
// SetFeaturedTags, which sends the Update of its actor through FeaturedTagsChanged; those of a
// remote account are the copy of its featuredTags collection, which ReplaceRemoteFeaturedTags
// writes. Every profile counts, for each tag, the account's public and unlisted posts that carry
// it, read from feed_tag_entries: the posts of the other visibilities are left out, since a count
// would reveal that they exist.

// FeaturedTagRow is a hashtag that an account features (featured_tags): Ordinal is its place in
// the account's order, from 1.
type FeaturedTagRow struct {
	AccountID  string
	TagID      string
	Ordinal    int
	FeaturedAt time.Time
}

// FeaturedTagUsage is how many public and unlisted posts of an account carry a hashtag now, and
// the latest time one of them was revised with it.
type FeaturedTagUsage struct {
	TagID         string
	StatusesCount int64
	LastStatusAt  time.Time
}

// FeaturedTagRepository is the part of Repository that keeps the featured hashtags.
type FeaturedTagRepository interface {
	// FeaturedTagsTx returns the hashtags that accountID features, in its order, inside tx.
	FeaturedTagsTx(ctx context.Context, tx uow.Tx, accountID string) ([]FeaturedTagRow, error)
	// ReplaceFeaturedTags makes the featured hashtags of accountID rows, which are in its order
	// with the ordinals 1 to len(rows); empty rows removes them all.
	ReplaceFeaturedTags(ctx context.Context, tx uow.Tx, accountID string, rows []FeaturedTagRow) error
	// FeaturedTagsOf returns the hashtags that accountID features, in its order.
	FeaturedTagsOf(ctx context.Context, accountID string) ([]FeaturedTagRow, error)
	// FeaturedTagUsageOf returns the usage of each hashtag that accountID features among its
	// public and unlisted posts, by tag ID; a tag that no such post carries has none.
	FeaturedTagUsageOf(ctx context.Context, accountID string) (map[string]FeaturedTagUsage, error)
	// AllFeaturedTags returns every featured hashtag, ordered by account and ordinal, for
	// verifying them.
	AllFeaturedTags(ctx context.Context) ([]FeaturedTagRow, error)
}

// FeaturedTag is a hashtag that an account features: the tag as content stores it, how many of
// the account's public and unlisted posts carry it, and when one of them last did (zero when
// none does).
type FeaturedTag struct {
	Tag           appcontent.Tag
	StatusesCount int64
	LastStatusAt  time.Time
	FeaturedAt    time.Time
}

// EventFeaturedTagsChanged is emitted when a local account changes its featured hashtags.
const EventFeaturedTagsChanged = "timeline.featured_tags_changed"

// FeaturedTagsChanged is emitted, in the transaction of the change, when a local account
// changes its featured hashtags; the federation module sends the Update of its actor, whose
// featuredTags collection lists them (ADR-0006 §17).
type FeaturedTagsChanged struct {
	AccountID string
	ChangedAt time.Time
}

// EventType implements uow.Event.
func (FeaturedTagsChanged) EventType() string { return EventFeaturedTagsChanged }

// SetFeaturedTags makes the featured hashtags of the local account accountID names, in their
// order, and returns them. A name given twice counts once, and no status needs to have used a
// name. The same list again changes nothing; any other change emits FeaturedTagsChanged. It fails
// with appcontent.ErrInvalidHashtag for an invalid name and with timeline.ErrTooManyFeaturedTags
// for more than timeline.MaxFeaturedTags names.
func (s *Service) SetFeaturedTags(ctx context.Context, accountID string, names []string) ([]FeaturedTag, error) {
	if s.d.Tags == nil {
		return nil, errNoTags
	}
	var hashtags []appcontent.Hashtag
	for _, name := range names {
		h, ok := appcontent.ParseHashtag(name)
		if !ok {
			return nil, appcontent.ErrInvalidHashtag
		}
		hashtags = appendHashtag(hashtags, h)
	}
	if len(hashtags) > timeline.MaxFeaturedTags {
		return nil, timeline.ErrTooManyFeaturedTags
	}
	if err := s.replaceFeaturedTags(ctx, accountID, hashtags, true); err != nil {
		return nil, err
	}
	return s.FeaturedTags(ctx, accountID)
}

// ReplaceRemoteFeaturedTags makes the featured hashtags of the remote account accountID the
// first timeline.MaxFeaturedTags valid names among names, in their order, each once: the copy of
// its featuredTags collection (ADR-0006 §17). The names that are not valid hashtags are left out,
// and empty names remove them all. It sends nothing.
func (s *Service) ReplaceRemoteFeaturedTags(ctx context.Context, accountID string, names []string) error {
	if s.d.Tags == nil {
		return errNoTags
	}
	var hashtags []appcontent.Hashtag
	for _, name := range names {
		if len(hashtags) == timeline.MaxFeaturedTags {
			break
		}
		if h, ok := appcontent.ParseHashtag(name); ok {
			hashtags = appendHashtag(hashtags, h)
		}
	}
	return s.replaceFeaturedTags(ctx, accountID, hashtags, false)
}

// appendHashtag appends h to hashtags unless one of them has its normalized name.
func appendHashtag(hashtags []appcontent.Hashtag, h appcontent.Hashtag) []appcontent.Hashtag {
	if slices.ContainsFunc(hashtags, func(o appcontent.Hashtag) bool { return o.NameNormalized == h.NameNormalized }) {
		return hashtags
	}
	return append(hashtags, h)
}

// replaceFeaturedTags makes the featured hashtags of accountID hashtags in one transaction,
// storing the tags that are not stored. A tag that stays featured keeps the time it was first
// featured. When the list changes and local is set, it emits FeaturedTagsChanged.
func (s *Service) replaceFeaturedTags(ctx context.Context, accountID string, hashtags []appcontent.Hashtag, local bool) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		current, err := s.d.Repository.FeaturedTagsTx(ctx, tx, accountID)
		if err != nil {
			return err
		}
		since := make(map[string]time.Time, len(current))
		for _, r := range current {
			since[r.TagID] = r.FeaturedAt
		}
		now := kernel.Normalize(s.d.Clock.Now())
		rows := make([]FeaturedTagRow, 0, len(hashtags))
		for i, h := range hashtags {
			tag, err := s.d.Tags.EnsureTagTx(ctx, tx, h.Name)
			if err != nil {
				return err
			}
			at, ok := since[tag.ID]
			if !ok {
				at = now
			}
			rows = append(rows, FeaturedTagRow{AccountID: accountID, TagID: tag.ID, Ordinal: i + 1, FeaturedAt: at})
		}
		if sameFeaturedTags(current, rows) {
			return nil
		}
		if err := s.d.Repository.ReplaceFeaturedTags(ctx, tx, accountID, rows); err != nil {
			return err
		}
		if !local {
			return nil
		}
		return tx.Emit(ctx, FeaturedTagsChanged{AccountID: accountID, ChangedAt: now})
	})
}

// sameFeaturedTags reports whether a and b feature the same tags in the same order.
func sameFeaturedTags(a, b []FeaturedTagRow) bool {
	return slices.EqualFunc(a, b, func(x, y FeaturedTagRow) bool { return x.TagID == y.TagID })
}

// FeaturedTags returns the hashtags that accountID features, in its order, with the usage of
// each among its public and unlisted posts.
func (s *Service) FeaturedTags(ctx context.Context, accountID string) ([]FeaturedTag, error) {
	rows, err := s.d.Repository.FeaturedTagsOf(ctx, accountID)
	if err != nil || len(rows) == 0 {
		return []FeaturedTag{}, err
	}
	if s.d.Tags == nil {
		return nil, errNoTags
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.TagID
	}
	tags, err := s.d.Tags.TagsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	usage, err := s.d.Repository.FeaturedTagUsageOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]FeaturedTag, 0, len(rows))
	for _, r := range rows {
		t, ok := tags[r.TagID]
		if !ok {
			continue
		}
		u := usage[r.TagID]
		out = append(out, FeaturedTag{Tag: t, StatusesCount: u.StatusesCount, LastStatusAt: u.LastStatusAt, FeaturedAt: r.FeaturedAt})
	}
	return out, nil
}
