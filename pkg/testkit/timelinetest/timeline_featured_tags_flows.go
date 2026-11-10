// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// The featured hashtags of profiles (ADR-0006 §17): the command of a local account, the copy of a
// remote account's collection, and the counts over the public and unlisted posts.

// featuredNames returns the names of tags, in their order.
func featuredNames(tags []apptimeline.FeaturedTag) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Tag.Name
	}
	return out
}

func timelineFeaturedTagCommands(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob := e.account(true), e.account(true)
	// Tags that no status used yet can be featured, a name given twice counts once, and the
	// order is the account's.
	got, err := e.svc.SetFeaturedTags(ctx, alice, []string{"#Cats", "dogs", "CATS"})
	if err != nil || !slices.Equal(featuredNames(got), []string{"Cats", "dogs"}) {
		t.Fatalf("SetFeaturedTags = %+v, %v; want Cats, dogs", got, err)
	}
	if got[0].StatusesCount != 0 || !got[0].LastStatusAt.IsZero() || !got[0].FeaturedAt.Equal(e.Clock.Now()) {
		t.Errorf("an unused featured tag = %+v, want no posts, featured now", got[0])
	}
	cats := e.tag(t, "cats")
	if got[0].Tag.ID != cats {
		t.Errorf("the featured tag is %s, the stored one %s", got[0].Tag.ID, cats)
	}
	if len(e.featuredChanges) != 1 || e.featuredChanges[0].AccountID != alice {
		t.Errorf("FeaturedTagsChanged = %+v, want one of alice", e.featuredChanges)
	}

	// The same list again changes nothing; a new order is a change, and a tag that stays keeps
	// the time it was first featured.
	first := e.Clock.Now()
	e.Clock.Advance(time.Minute)
	if _, err := e.svc.SetFeaturedTags(ctx, alice, []string{"cats", "Dogs"}); err != nil {
		t.Fatal(err)
	}
	if len(e.featuredChanges) != 1 {
		t.Errorf("the same list emitted %d FeaturedTagsChanged, want none more", len(e.featuredChanges)-1)
	}
	got, err = e.svc.SetFeaturedTags(ctx, alice, []string{"birds", "dogs", "cats"})
	if err != nil || !slices.Equal(featuredNames(got), []string{"birds", "dogs", "Cats"}) {
		t.Fatalf("SetFeaturedTags(reordered) = %+v, %v", got, err)
	}
	if !got[2].FeaturedAt.Equal(first) || !got[0].FeaturedAt.Equal(e.Clock.Now()) {
		t.Errorf("FeaturedAt = %s, %s; want %s for cats and now for birds", got[2].FeaturedAt, got[0].FeaturedAt, first)
	}
	if len(e.featuredChanges) != 2 {
		t.Errorf("FeaturedTagsChanged = %d, want 2", len(e.featuredChanges))
	}

	// Invalid names and more than timeline.MaxFeaturedTags change nothing.
	for _, name := range []string{"", "#", "not a tag", "123"} {
		if _, err := e.svc.SetFeaturedTags(ctx, alice, []string{"cats", name}); !errors.Is(err, appcontent.ErrInvalidHashtag) {
			t.Errorf("SetFeaturedTags(%q) = %v, want ErrInvalidHashtag", name, err)
		}
	}
	var many []string
	for i := range timeline.MaxFeaturedTags + 1 {
		many = append(many, fmt.Sprintf("tag%d", i))
	}
	if _, err := e.svc.SetFeaturedTags(ctx, bob, many); !errors.Is(err, timeline.ErrTooManyFeaturedTags) {
		t.Errorf("SetFeaturedTags(%d tags) = %v, want ErrTooManyFeaturedTags", len(many), err)
	}
	if got, err := e.svc.SetFeaturedTags(ctx, bob, many[:timeline.MaxFeaturedTags]); err != nil || len(got) != timeline.MaxFeaturedTags {
		t.Errorf("SetFeaturedTags(%d tags) = %d, %v", timeline.MaxFeaturedTags, len(got), err)
	}
	if got, err := e.svc.FeaturedTags(ctx, alice); err != nil || !slices.Equal(featuredNames(got), []string{"birds", "dogs", "Cats"}) {
		t.Errorf("FeaturedTags(alice) after the failures = %v, %v", featuredNames(got), err)
	}

	// The counts: alice's public and unlisted posts of each tag, not the other visibilities nor
	// the posts of others.
	e.Clock.Advance(time.Second)
	e.post(t, alice, content.VisibilityPublic, postOpts{tags: []string{cats}})
	e.Clock.Advance(time.Second)
	e.post(t, alice, content.VisibilityUnlisted, postOpts{tags: []string{cats}})
	lastUse := e.Clock.Now()
	e.Clock.Advance(time.Second)
	for _, v := range []content.Visibility{content.VisibilityLocal, content.VisibilityPrivate, content.VisibilityDirect} {
		e.post(t, alice, v, postOpts{tags: []string{cats}})
	}
	e.post(t, bob, content.VisibilityPublic, postOpts{tags: []string{cats}})
	got, err = e.svc.FeaturedTags(ctx, alice)
	if err != nil || len(got) != 3 {
		t.Fatalf("FeaturedTags(alice) = %+v, %v", got, err)
	}
	if c := got[2]; c.StatusesCount != 2 || !c.LastStatusAt.Equal(lastUse) {
		t.Errorf("cats = %d posts, last %s; want 2, %s", c.StatusesCount, c.LastStatusAt, lastUse)
	}
	if got[0].StatusesCount != 0 {
		t.Errorf("birds = %d posts, want 0", got[0].StatusesCount)
	}

	// An empty list removes them.
	if got, err := e.svc.SetFeaturedTags(ctx, alice, nil); err != nil || len(got) != 0 {
		t.Errorf("SetFeaturedTags(none) = %+v, %v", got, err)
	}
	if len(e.featuredChanges) != 4 {
		t.Errorf("FeaturedTagsChanged = %d, want 4 (bob's list and the emptied one)", len(e.featuredChanges))
	}
}

func timelineRemoteFeaturedTags(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	carol := e.account(false)
	var names []string
	for i := range timeline.MaxFeaturedTags + 2 {
		names = append(names, fmt.Sprintf("#tag%d", i))
	}
	// Invalid names and repeats are left out, and only the first timeline.MaxFeaturedTags are
	// kept.
	names = append([]string{"not a tag", "#Tag0"}, names...)
	if err := e.svc.ReplaceRemoteFeaturedTags(ctx, carol, names); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.FeaturedTags(ctx, carol)
	if err != nil || len(got) != timeline.MaxFeaturedTags || got[0].Tag.Name != "Tag0" || got[1].Tag.Name != "tag1" {
		t.Errorf("FeaturedTags(carol) = %v, %v; want Tag0 to tag9", featuredNames(got), err)
	}
	if len(e.featuredChanges) != 0 {
		t.Errorf("a remote copy emitted %+v, want nothing", e.featuredChanges)
	}
	// The counts read the posts of the account that this server knows.
	e.post(t, carol, content.VisibilityPublic, postOpts{tags: []string{got[1].Tag.ID}})
	if got, err := e.svc.FeaturedTags(ctx, carol); err != nil || got[1].StatusesCount != 1 {
		t.Errorf("FeaturedTags(carol) = %+v, %v; want one post of tag1", got, err)
	}
	if err := e.svc.ReplaceRemoteFeaturedTags(ctx, carol, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := e.svc.FeaturedTags(ctx, carol); err != nil || len(got) != 0 {
		t.Errorf("FeaturedTags(carol) after an empty collection = %+v, %v", got, err)
	}
}
