// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/content"
)

// tagUsage returns the tag_usage projection by tag ID.
func (e *timelineEnv) tagUsage(t *testing.T) map[string]apptimeline.TagUsage {
	t.Helper()
	snap, err := e.h.Timeline.Snapshot(t.Context())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	out := map[string]apptimeline.TagUsage{}
	for _, u := range snap.TagUsage {
		out[u.TagID] = u
	}
	return out
}

// expectUsage checks the usage of each tag: want[tag] is the count and the last use, or a zero
// count for a tag without a row.
func (e *timelineEnv) expectUsage(t *testing.T, what string, names map[string]string, want map[string]apptimeline.TagUsage) {
	t.Helper()
	got := e.tagUsage(t)
	for id, w := range want {
		g, ok := got[id]
		switch {
		case w.StatusesCount == 0 && ok:
			t.Errorf("%s: usage of %s = %+v, want none", what, names[id], g)
		case w.StatusesCount != 0 && (!ok || g.StatusesCount != w.StatusesCount || !g.LastUsedAt.Equal(w.LastUsedAt)):
			t.Errorf("%s: usage of %s = %+v (%v), want %d at %v", what, names[id], g, ok, w.StatusesCount, w.LastUsedAt)
		}
	}
}

func usage(n int64, at time.Time) apptimeline.TagUsage {
	return apptimeline.TagUsage{StatusesCount: n, LastUsedAt: at}
}

func timelineTagUsage(t *testing.T, e *timelineEnv) {
	alice, bob := e.account(true), e.account(true)
	remote := e.account(false)
	cats, dogs, birds, secret, here := e.tag(t, "cats"), e.tag(t, "dogs"), e.tag(t, "birds"), e.tag(t, "secret"), e.tag(t, "here")
	names := map[string]string{cats: "cats", dogs: "dogs", birds: "birds", secret: "secret", here: "here"}

	first := e.post(t, alice, content.VisibilityPublic, postOpts{tags: []string{cats, dogs}})
	second := e.post(t, remote, content.VisibilityUnlisted, postOpts{tags: []string{cats}})
	local := e.post(t, bob, content.VisibilityLocal, postOpts{tags: []string{here}})
	e.post(t, alice, content.VisibilityPrivate, postOpts{tags: []string{secret}})
	e.post(t, alice, content.VisibilityDirect, postOpts{tags: []string{secret, cats}, mentions: []string{bob}})
	// A boost adds no tag entry, so the count stays.
	e.boost(t, bob, first)
	e.expectUsage(t, "after posting", names, map[string]apptimeline.TagUsage{
		cats: usage(2, second.PublishedAt), dogs: usage(1, first.PublishedAt), here: usage(1, local.PublishedAt),
		secret: {}, birds: {},
	})

	// An edit that swaps dogs for birds moves the counts and makes the post's time the last use.
	e.reviseWith(t, first, postOpts{tags: []string{cats, birds}, language: "en"})
	revised := e.Clock.Now()
	e.expectUsage(t, "after an edit", names, map[string]apptimeline.TagUsage{
		cats: usage(2, revised), dogs: {}, birds: usage(1, revised),
	})

	// Deleting a post takes its tags out; the last use goes back to the posts that remain.
	e.MustRemove(t, first)
	e.expectUsage(t, "after deleting the edited post", names, map[string]apptimeline.TagUsage{
		cats: usage(1, second.PublishedAt), birds: {},
	})
	e.MustRemove(t, second)
	e.MustRemove(t, local)
	e.expectUsage(t, "after deleting every public post", names, map[string]apptimeline.TagUsage{
		cats: {}, here: {}, secret: {},
	})
	if got := e.tagUsage(t); len(got) != 0 {
		t.Errorf("tag usage after deleting every counted post = %+v, want none", got)
	}
}

func timelineTagSearch(t *testing.T, e *timelineEnv) {
	alice := e.account(true)
	cat, cats, catalog, category, catnip, dog := e.tag(t, "Cat"), e.tag(t, "cats"), e.tag(t, "catalog"), e.tag(t, "category"), e.tag(t, "catnip"), e.tag(t, "dog")
	for range 3 {
		e.post(t, alice, content.VisibilityPublic, postOpts{tags: []string{catalog}})
	}
	for range 3 {
		e.post(t, alice, content.VisibilityUnlisted, postOpts{tags: []string{cats}})
	}
	e.post(t, alice, content.VisibilityLocal, postOpts{tags: []string{cat, dog}})
	e.post(t, alice, content.VisibilityPrivate, postOpts{tags: []string{category}})
	e.post(t, alice, content.VisibilityDirect, postOpts{tags: []string{category}})
	e.MustRemove(t, e.post(t, alice, content.VisibilityPublic, postOpts{tags: []string{catnip}}))

	search := apptimeline.NewTagSearch(e.h.Statuses, e.h.Timeline)
	expect := func(q string, limit int, want ...apptimeline.TagSuggestion) {
		t.Helper()
		got, err := search.Search(t.Context(), q, limit)
		if err != nil {
			t.Fatalf("Search(%q, %d): %v", q, limit, err)
		}
		if want == nil {
			want = []apptimeline.TagSuggestion{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Search(%q, %d) = %+v, want %+v", q, limit, got, want)
		}
	}
	sCat := apptimeline.TagSuggestion{ID: cat, Name: "Cat", StatusesCount: 1}
	sCats := apptimeline.TagSuggestion{ID: cats, Name: "cats", StatusesCount: 3}
	sCatalog := apptimeline.TagSuggestion{ID: catalog, Name: "catalog", StatusesCount: 3}
	// The exact match first, then more posts, then the more recently used; a tag of private and
	// direct posts only (category) and one whose posts are deleted (catnip) never show.
	expect("cat", 0, sCat, sCats, sCatalog)
	expect("#CAT", 2, sCat, sCats)
	expect("ca", 0, sCats, sCatalog, sCat)
	expect("ｃａｔｓ", 0, sCats)
	expect("catalogue", 0)
	expect("zzz", 0)
	for _, q := range []string{"", "#", strings.Repeat("a", apptimeline.MaxTagSearchQueryLength+1)} {
		if _, err := search.Search(t.Context(), q, 0); !errors.Is(err, apptimeline.ErrInvalidTagSearchQuery) {
			t.Errorf("Search(%q) = %v, want ErrInvalidTagSearchQuery", q, err)
		}
	}
	if got, err := search.Search(t.Context(), "#"+strings.Repeat("a", apptimeline.MaxTagSearchQueryLength), 0); err != nil || len(got) != 0 {
		t.Errorf("Search(# and the longest query) = %v, %v; want no tags", got, err)
	}
}
