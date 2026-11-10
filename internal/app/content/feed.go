// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds the use cases behind the Atom feeds of profiles and hashtags (FR-213): the
// newest local public posts, without replies and boosts, as an anonymous visitor sees them, and
// the articles-only feed of a profile (ADR-0160 §11). Each post goes through Previews.status, so
// through content.CanView with an anonymous viewer, and an author's posts appear only while the
// author has a feed (ProfilePreview.Feed).

// FeedLength is the number of posts of an Atom feed.
const FeedLength = 20

// feedPages bounds how many pages of candidates a hashtag feed reads to fill FeedLength posts,
// when the authors of many of them have no feed.
const feedPages = 3

// FeedPostIDs lists the candidates of a feed, newest first: the IDs of one page of posts, and the
// cursor of the next page, "" when this page is the last. The timeline module's
// ProfileFeed and HashtagFeed give them.
type FeedPostIDs func(ctx context.Context, key, before string, limit int) (ids []string, next string, err error)

// FeedDeps are the ports Feeds uses.
type FeedDeps struct {
	Previews   *Previews
	Repository Repository
	// ProfilePosts lists the public posts of a local author that are not replies or boosts, by
	// the author's account ID; TagPosts lists the public posts of local authors with a hashtag
	// that are not replies, by the tag's ID.
	ProfilePosts, TagPosts FeedPostIDs
}

// Feeds reads the posts of the Atom feeds of profiles and hashtags.
type Feeds struct {
	d FeedDeps
}

// NewFeeds returns Feeds on d.
func NewFeeds(d FeedDeps) *Feeds { return &Feeds{d: d} }

// Feed is the content of an Atom feed.
type Feed struct {
	// Author is the profile of a profile's feed, nil for a hashtag's.
	Author *ProfilePreview
	// Tag is the hashtag of a hashtag's feed, the zero Tag for a profile's.
	Tag Tag
	// Articles is true for the articles-only feed of a profile.
	Articles bool
	// Entries are the posts, newest first.
	Entries []FeedEntry
}

// FeedEntry is a post of a feed.
type FeedEntry struct {
	StatusPreview
	// Tags are the names of the post's hashtags, sorted.
	Tags []string
	// Media are the post's attachments; none behind a content warning, whose body is left to the
	// post's page.
	Media []FeedMedia
}

// FeedMedia is an attachment of a post in a feed: its largest processed rendition, moving for an
// animation, video, or audio, and still otherwise.
type FeedMedia struct {
	URL, ContentType string
	MediaType        content.MediaType
	// Alt is the alt text, "" for none.
	Alt string
	// Sensitive is true when the author or a moderator marked the attachments sensitive.
	Sensitive bool
}

// Profile returns the feed of a local username: its posts and articles. A username that names no account an anonymous
// visitor may see, or one that has no feed, fails with ErrNotFound.
func (f *Feeds) Profile(ctx context.Context, username string) (Feed, error) {
	prof, err := f.d.Previews.Profile(ctx, username)
	if err != nil {
		return Feed{}, err
	}
	if !prof.Feed {
		return Feed{}, fmt.Errorf("feed of %s: %w", username, ErrNotFound)
	}
	authors := previewAuthors{prof.Actor.Account.ID: {actor: prof.Actor, found: true, profile: prof}}
	entries, err := f.entries(ctx, f.d.ProfilePosts, prof.Actor.Account.ID, authors, 1)
	if err != nil {
		return Feed{}, err
	}
	return Feed{Author: &prof, Entries: entries}, nil
}

// Articles returns the articles-only feed of a local username, with the rules of Profile.
func (f *Feeds) Articles(ctx context.Context, username string) (Feed, error) {
	prof, err := f.d.Previews.Profile(ctx, username)
	if err != nil {
		return Feed{}, err
	}
	if !prof.Feed {
		return Feed{}, fmt.Errorf("article feed of %s: %w", username, ErrNotFound)
	}
	authors := previewAuthors{prof.Actor.Account.ID: {actor: prof.Actor, found: true, profile: prof}}
	entries, err := f.entries(ctx, f.articleIDs, prof.Actor.Account.ID, authors, feedPages)
	if err != nil {
		return Feed{}, err
	}
	return Feed{Author: &prof, Articles: true, Entries: entries}, nil
}

// articleIDs lists the articles of an author as FeedPostIDs lists the candidates of a feed.
func (f *Feeds) articleIDs(ctx context.Context, authorAccountID, before string, limit int) ([]string, string, error) {
	ids, err := f.d.Repository.ListLiveArticleIDs(ctx, authorAccountID, before, limit)
	if err != nil || len(ids) < limit {
		return ids, "", err
	}
	return ids, ids[len(ids)-1], nil
}

// Tag returns the feed of a hashtag, by its name with or without #. A name that is not a hashtag
// or that no status has used fails with ErrNotFound.
func (f *Feeds) Tag(ctx context.Context, name string) (Feed, error) {
	h, ok := ParseHashtag(name)
	if !ok {
		return Feed{}, fmt.Errorf("hashtag %q: %w", name, ErrNotFound)
	}
	tag, err := f.d.Repository.FindTagByName(ctx, h.NameNormalized)
	if err != nil {
		return Feed{}, err
	}
	entries, err := f.entries(ctx, f.d.TagPosts, tag.ID, previewAuthors{}, feedPages)
	if err != nil {
		return Feed{}, err
	}
	return Feed{Tag: tag, Entries: entries}, nil
}

// entries reads up to FeedLength posts that list gives for key, in at most pages pages, keeping
// those an anonymous visitor may see of authors with a feed.
func (f *Feeds) entries(ctx context.Context, list FeedPostIDs, key string, authors previewAuthors, pages int) ([]FeedEntry, error) {
	var out []FeedEntry
	before := ""
	for range pages {
		ids, next, err := list(ctx, key, before, FeedLength)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			s, err := f.d.Previews.status(ctx, id, authors)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if s.Visibility != content.VisibilityPublic || !s.Author.Feed || s.Post.Status.InReplyToURI != "" {
				continue
			}
			e, err := f.entry(ctx, s)
			if err != nil {
				return nil, err
			}
			if out = append(out, e); len(out) == FeedLength {
				return out, f.tagNames(ctx, out)
			}
		}
		if next == "" {
			break
		}
		before = next
	}
	return out, f.tagNames(ctx, out)
}

// entry adds the attachments to a post of a feed. An article has none: its entry is its title and
// summary.
func (f *Feeds) entry(ctx context.Context, s StatusPreview) (FeedEntry, error) {
	e := FeedEntry{StatusPreview: s}
	rev := s.Post.Revision
	if rev.Article != nil || rev.SpoilerText != "" || len(rev.Attachments) == 0 || f.d.Previews.d.Media == nil {
		return e, nil
	}
	sensitive := rev.Sensitive
	if !sensitive {
		r, err := f.d.Previews.d.Restrictions.Restrictions(ctx, s.Post.Status.AuthorAccountID)
		if err != nil {
			return FeedEntry{}, err
		}
		if sensitive = r.MediaSensitive; !sensitive {
			if sensitive, err = f.d.Repository.SensitivityOverridden(ctx, s.Post.Status.ID); err != nil {
				return FeedEntry{}, err
			}
		}
	}
	for _, at := range rev.Attachments {
		m, err := f.d.Previews.d.Media.Media(ctx, at.MediaID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return FeedEntry{}, err
		}
		if !m.Ready() {
			continue
		}
		v, ok := m.Result.Variant(content.SizeLarge, content.MotionAnimated)
		if !ok {
			if v, ok = m.Result.Variant(content.SizeLarge, content.MotionStill); !ok {
				continue
			}
		}
		e.Media = append(e.Media, FeedMedia{
			URL: f.d.Previews.d.Media.URL(v), ContentType: v.ContentType, MediaType: m.Result.MediaType,
			Alt: at.Description, Sensitive: sensitive,
		})
	}
	return e, nil
}

// tagNames fills the names of the hashtags of entries, with one query.
func (f *Feeds) tagNames(ctx context.Context, entries []FeedEntry) error {
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.Post.Revision.TagIDs...)
	}
	if len(ids) == 0 {
		return nil
	}
	slices.Sort(ids)
	tags, err := f.d.Repository.FindTags(ctx, slices.Compact(ids))
	if err != nil {
		return err
	}
	for i := range entries {
		for _, id := range entries[i].Post.Revision.TagIDs {
			if t, ok := tags[id]; ok {
				entries[i].Tags = append(entries[i].Tags, t.Name)
			}
		}
		slices.Sort(entries[i].Tags)
	}
	return nil
}
