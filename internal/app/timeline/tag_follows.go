// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"errors"
	"slices"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// The followed hashtags (ADR-0013 §24 to §28): an account follows up to timeline.MaxFollowedTags
// hashtags, and its home timeline lists, next to the entries of the accounts it follows, the
// public and local posts that carry one of them, read from feed_tag_entries fan-out-on-read
// (data-model §6.1).

// TagFollow is a hashtag that an account follows (feed_tag_follows).
type TagFollow struct {
	AccountID  string
	TagID      string
	FollowedAt time.Time
}

// TagFollowRepository is the part of Repository that keeps the followed hashtags and reads the
// hashtag branch of the home timeline.
type TagFollowRepository interface {
	// InsertTagFollow stores f and reports false, writing nothing, when the account follows the
	// tag.
	InsertTagFollow(ctx context.Context, tx uow.Tx, f TagFollow) (bool, error)
	// DeleteTagFollow ends the follow of a tag; one that does not exist changes nothing.
	DeleteTagFollow(ctx context.Context, tx uow.Tx, accountID, tagID string) error
	// CountTagFollows returns how many tags accountID follows.
	CountTagFollows(ctx context.Context, tx uow.Tx, accountID string) (int, error)
	// TagFollowsOf returns the tags that accountID follows, the latest followed first.
	TagFollowsOf(ctx context.Context, accountID string) ([]TagFollow, error)
	// FollowsTag reports whether accountID follows tagID.
	FollowsTag(ctx context.Context, accountID, tagID string) (bool, error)
	// AllTagFollows returns every followed tag, ordered by account and tag, for verifying them.
	AllTagFollows(ctx context.Context) ([]TagFollow, error)
	// ListHomeTags is the hashtag branch of the home timeline of q.ViewerAccountID (ADR-0013 §25,
	// §26), newest first, at most q.Limit entries: the posts of visibility public and local
	// with a tag that the viewer follows, by authors other than the viewer and the accounts the
	// viewer follows, in the read window q.Floor, without the authors and domains that the
	// hashtag timeline leaves out (kept from discovery, capped at private, or hidden), the
	// accounts the viewer blocks, is blocked by, or mutes, the domains the viewer mutes, and
	// the languages outside q.Languages. Each entry has the viewer's followed tags it carries
	// in FollowedTagIDs.
	ListHomeTags(ctx context.Context, q HomeQuery) ([]Entry, error)
	// FeedTagFollowersAmong returns the accounts among accountIDs that follow a tag of the post
	// entryID when its visibility is public or local, with one query.
	FeedTagFollowersAmong(ctx context.Context, entryID string, accountIDs []string) (map[string]bool, error)
}

// Tags is the part of the content module's public API that the followed hashtags use: the
// hashtags by name and ID. *appcontent.Service implements it.
type Tags interface {
	// EnsureTagTx returns the stored hashtag name inside tx, storing it first when no status has
	// used it, failing with appcontent.ErrInvalidHashtag for an invalid name.
	EnsureTagTx(ctx context.Context, tx uow.Tx, name string) (appcontent.Tag, error)
	// TagByName returns the stored hashtag name, failing with appcontent.ErrNotFound when it is
	// not stored and appcontent.ErrInvalidHashtag for an invalid name.
	TagByName(ctx context.Context, name string) (appcontent.Tag, error)
	// TagsByID returns the stored hashtags among ids, by ID.
	TagsByID(ctx context.Context, ids []string) (map[string]appcontent.Tag, error)
}

// FollowedTag is a hashtag that an account follows: the tag as content stores it and when the
// account followed it.
type FollowedTag struct {
	Tag        appcontent.Tag
	FollowedAt time.Time
}

// errNoTags reports a Service built without Deps.Tags.
var errNoTags = errors.New("timeline: the followed hashtags need the content tags")

// FollowTag makes accountID follow the hashtag name, which no status needs to have used yet,
// and returns the tag; following it again changes nothing. It fails with
// appcontent.ErrInvalidHashtag for an invalid name and with timeline.ErrTooManyFollowedTags when
// the account follows timeline.MaxFollowedTags tags.
func (s *Service) FollowTag(ctx context.Context, accountID, name string) (appcontent.Tag, error) {
	if s.d.Tags == nil {
		return appcontent.Tag{}, errNoTags
	}
	var tag appcontent.Tag
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if tag, err = s.d.Tags.EnsureTagTx(ctx, tx, name); err != nil {
			return err
		}
		inserted, err := s.d.Repository.InsertTagFollow(ctx, tx, TagFollow{
			AccountID: accountID, TagID: tag.ID, FollowedAt: kernel.Normalize(s.d.Clock.Now()),
		})
		if err != nil || !inserted {
			return err
		}
		// Counting after the insert keeps following a followed tag idempotent at the limit; the
		// error rolls the insert back.
		n, err := s.d.Repository.CountTagFollows(ctx, tx, accountID)
		if err == nil && n > timeline.MaxFollowedTags {
			err = timeline.ErrTooManyFollowedTags
		}
		return err
	})
	if err != nil {
		return appcontent.Tag{}, err
	}
	return tag, nil
}

// UnfollowTag makes accountID stop following the hashtag name and returns the tag; a tag that
// the account does not follow changes nothing, and one that is not stored comes back with only
// its name. It fails with appcontent.ErrInvalidHashtag for an invalid name.
func (s *Service) UnfollowTag(ctx context.Context, accountID, name string) (appcontent.Tag, error) {
	tag, err := s.storedTag(ctx, name)
	if err != nil || tag.ID == "" {
		return tag, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.DeleteTagFollow(ctx, tx, accountID, tag.ID)
	})
	return tag, err
}

// TagFollowing returns the hashtag name and whether accountID ("" for a visitor) follows it; a
// tag that is not stored comes back with only its name, unfollowed. It fails with
// appcontent.ErrInvalidHashtag for an invalid name.
func (s *Service) TagFollowing(ctx context.Context, accountID, name string) (appcontent.Tag, bool, error) {
	tag, err := s.storedTag(ctx, name)
	if err != nil || tag.ID == "" || accountID == "" {
		return tag, false, err
	}
	following, err := s.d.Repository.FollowsTag(ctx, accountID, tag.ID)
	return tag, following, err
}

// storedTag returns the stored hashtag name, or one with only its name (no ID) when it is not
// stored.
func (s *Service) storedTag(ctx context.Context, name string) (appcontent.Tag, error) {
	if s.d.Tags == nil {
		return appcontent.Tag{}, errNoTags
	}
	tag, err := s.d.Tags.TagByName(ctx, name)
	if errors.Is(err, appcontent.ErrNotFound) {
		h, _ := appcontent.ParseHashtag(name)
		return appcontent.Tag{Name: h.Name, NameNormalized: h.NameNormalized}, nil
	}
	return tag, err
}

// FollowedTags returns the hashtags that accountID follows, the latest followed first.
func (s *Service) FollowedTags(ctx context.Context, accountID string) ([]FollowedTag, error) {
	follows, err := s.d.Repository.TagFollowsOf(ctx, accountID)
	if err != nil || len(follows) == 0 {
		return []FollowedTag{}, err
	}
	if s.d.Tags == nil {
		return nil, errNoTags
	}
	ids := make([]string, len(follows))
	for i, f := range follows {
		ids[i] = f.TagID
	}
	tags, err := s.d.Tags.TagsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]FollowedTag, 0, len(follows))
	for _, f := range follows {
		if t, ok := tags[f.TagID]; ok {
			out = append(out, FollowedTag{Tag: t, FollowedAt: f.FollowedAt})
		}
	}
	return out, nil
}

// homeRows reads both branches of the home timeline with q (data-model §6.1): the entries of the
// viewer and of the accounts the viewer follows, and the posts of the hashtags the viewer
// follows, merged newest first, each ID once, at most q.Limit.
func (s *Service) homeRows(ctx context.Context, q HomeQuery) ([]Entry, error) {
	rows, err := s.d.Repository.ListHome(ctx, q)
	if err != nil {
		return nil, err
	}
	tagged, err := s.d.Repository.ListHomeTags(ctx, q)
	if err != nil || len(tagged) == 0 {
		return rows, err
	}
	return mergeEntries(rows, tagged, q.Limit), nil
}

// mergeEntries merges a and b, each newest first, into at most limit entries newest first, each
// ID once (the entry of a wins).
func mergeEntries(a, b []Entry, limit int) []Entry {
	out := make([]Entry, 0, min(limit, len(a)+len(b)))
	i, j := 0, 0
	for len(out) < limit && (i < len(a) || j < len(b)) {
		switch {
		case j >= len(b) || (i < len(a) && a[i].ID > b[j].ID):
			out = append(out, a[i])
			i++
		case i >= len(a) || b[j].ID > a[i].ID:
			out = append(out, b[j])
			j++
		default: // the same entry from both branches
			out = append(out, a[i])
			i++
			j++
		}
	}
	return out
}

// tagNames gives the entries of the hashtag branch the names of their followed tags, sorted
// (ADR-0013 §28), leaving out the tags that content no longer has.
func (s *Service) tagNames(ctx context.Context, entries []Entry) error {
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.FollowedTagIDs...)
	}
	if len(ids) == 0 {
		return nil
	}
	if s.d.Tags == nil {
		return errNoTags
	}
	tags, err := s.d.Tags.TagsByID(ctx, uniq(ids))
	if err != nil {
		return err
	}
	for i := range entries {
		var names []string
		for _, id := range entries[i].FollowedTagIDs {
			if t, ok := tags[id]; ok {
				names = append(names, t.Name)
			}
		}
		slices.Sort(names)
		entries[i].FollowedTags = names
	}
	return nil
}
