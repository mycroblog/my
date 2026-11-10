// SPDX-License-Identifier: AGPL-3.0-only

package timelinetest

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// The followed hashtags (ADR-0013 §24 to §28): the commands, and the posts of the followed tags
// on home next to the follows.

func (e *timelineEnv) followTag(t *testing.T, accountID, name string) string {
	t.Helper()
	tag, err := e.svc.FollowTag(t.Context(), accountID, name)
	if err != nil {
		t.Fatalf("FollowTag(%s): %v", name, err)
	}
	return tag.ID
}

func timelineFollowedTagCommands(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob := e.account(true), e.account(true)
	// A tag that no status used yet can be followed; following it again changes nothing.
	cats := e.followTag(t, alice, "#Cats")
	if again := e.followTag(t, alice, "cats"); again != cats {
		t.Errorf("FollowTag(again) = %s, want %s", again, cats)
	}
	if stored := e.tag(t, "CATS"); stored != cats {
		t.Errorf("the followed tag is %s, the stored one %s", cats, stored)
	}
	e.Clock.Advance(time.Second)
	dogs := e.followTag(t, alice, "dogs")
	for _, name := range []string{"", "#", "not a tag", "123", "a-b"} {
		if _, err := e.svc.FollowTag(ctx, alice, name); !errors.Is(err, appcontent.ErrInvalidHashtag) {
			t.Errorf("FollowTag(%q) = %v, want ErrInvalidHashtag", name, err)
		}
	}
	got, err := e.svc.FollowedTags(ctx, alice)
	if err != nil || len(got) != 2 || got[0].Tag.ID != dogs || got[1].Tag.ID != cats || got[1].Tag.Name != "Cats" {
		t.Errorf("FollowedTags = %+v, %v; want dogs, then Cats", got, err)
	}
	if tag, following, err := e.svc.TagFollowing(ctx, alice, "CATS"); err != nil || !following || tag.ID != cats {
		t.Errorf("TagFollowing(alice, CATS) = %+v, %v, %v; want cats, true", tag, following, err)
	}
	if _, following, err := e.svc.TagFollowing(ctx, bob, "cats"); err != nil || following {
		t.Errorf("TagFollowing(bob, cats) = %v, %v; want false", following, err)
	}
	if _, following, err := e.svc.TagFollowing(ctx, "", "cats"); err != nil || following {
		t.Errorf("TagFollowing(visitor, cats) = %v, %v; want false", following, err)
	}
	if tag, following, err := e.svc.TagFollowing(ctx, alice, "birds"); err != nil || following || tag.ID != "" || tag.Name != "birds" {
		t.Errorf("TagFollowing(alice, birds) = %+v, %v, %v; want only the name", tag, following, err)
	}

	// Unfollowing a tag that is not followed or not stored changes nothing.
	for _, name := range []string{"cats", "cats", "birds"} {
		if _, err := e.svc.UnfollowTag(ctx, alice, name); err != nil {
			t.Errorf("UnfollowTag(%s) = %v", name, err)
		}
	}
	if got, err := e.svc.FollowedTags(ctx, alice); err != nil || len(got) != 1 || got[0].Tag.ID != dogs {
		t.Errorf("FollowedTags after unfollowing = %+v, %v; want dogs", got, err)
	}

	// At most timeline.MaxFollowedTags, and following a followed tag at the limit still works.
	for i := range timeline.MaxFollowedTags {
		e.followTag(t, bob, fmt.Sprintf("tag%d", i))
	}
	if _, err := e.svc.FollowTag(ctx, bob, "onemore"); !errors.Is(err, timeline.ErrTooManyFollowedTags) {
		t.Errorf("FollowTag(beyond the limit) = %v, want ErrTooManyFollowedTags", err)
	}
	e.followTag(t, bob, "tag0")
	if got, err := e.svc.FollowedTags(ctx, bob); err != nil || len(got) != timeline.MaxFollowedTags {
		t.Errorf("FollowedTags(bob) = %d, %v; want %d", len(got), err, timeline.MaxFollowedTags)
	}
}

func timelineHomeFollowedTags(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	viewer, followee, stranger, muted, blocked, silenced := e.account(true), e.account(true), e.account(true), e.account(true), e.account(true), e.account(true)
	remote := e.account(false)
	e.follow(t, viewer, followee)
	cats := e.followTag(t, viewer, "cats")
	dogs := e.followTag(t, viewer, "dogs")
	birds := e.tag(t, "birds")

	pub := e.post(t, stranger, content.VisibilityPublic, postOpts{tags: []string{cats}})
	loc := e.post(t, stranger, content.VisibilityLocal, postOpts{tags: []string{cats, dogs}})
	reply := e.post(t, stranger, content.VisibilityPublic, postOpts{tags: []string{dogs}, inReplyTo: pub.URI})
	e.post(t, stranger, content.VisibilityUnlisted, postOpts{tags: []string{cats}})
	e.post(t, stranger, content.VisibilityPrivate, postOpts{tags: []string{cats}})
	e.post(t, stranger, content.VisibilityPublic, postOpts{tags: []string{birds}})
	relayed := e.post(t, remote, content.VisibilityPublic, postOpts{tags: []string{cats}})
	// A boost of a tagged post by someone the viewer does not follow is not a post of the tag.
	e.boost(t, stranger, relayed)
	// A post both of a followed author and of a followed tag, and the viewer's own, appear once.
	both := e.post(t, followee, content.VisibilityPublic, postOpts{tags: []string{cats}})
	own := e.post(t, viewer, content.VisibilityPublic, postOpts{tags: []string{cats}})
	mutedPost := e.post(t, muted, content.VisibilityPublic, postOpts{tags: []string{cats}})
	blockedPost := e.post(t, blocked, content.VisibilityPublic, postOpts{tags: []string{cats}})
	silencedPost := e.post(t, silenced, content.VisibilityPublic, postOpts{tags: []string{cats}})
	ja := e.post(t, stranger, content.VisibilityPublic, postOpts{tags: []string{cats}, language: "ja"})

	e.home(t, viewer, pub.ID, loc.ID, reply.ID, relayed.ID, both.ID, own.ID, mutedPost.ID, blockedPost.ID, silencedPost.ID, ja.ID)
	p, err := e.svc.Home(ctx, viewerOf(viewer), apptimeline.Query{Limit: apptimeline.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range p.Entries {
		var want []string
		switch en.ID {
		case both.ID, own.ID:
		case loc.ID:
			want = []string{"cats", "dogs"}
		case reply.ID:
			want = []string{"dogs"}
		default:
			want = []string{"cats"}
		}
		if !slices.Equal(en.FollowedTags, want) {
			t.Errorf("FollowedTags of %s = %v, want %v", en.ID, en.FollowedTags, want)
		}
	}

	// The timeline languages, mutes, blocks, and the restrictions of the hashtag timeline.
	p, err = e.svc.Home(ctx, viewerOf(viewer), apptimeline.Query{Limit: apptimeline.MaxLimit, Languages: []string{"en"}})
	expectPage(t, "Home(en)", p, err, pub.ID, loc.ID, reply.ID, relayed.ID, both.ID, own.ID, mutedPost.ID, blockedPost.ID, silencedPost.ID)
	if _, err := e.graph.Mute(ctx, viewer, muted, appgraph.MuteInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Block(ctx, blocked, viewer); err != nil {
		t.Fatal(err)
	}
	e.restrict(t, silenced, timeline.RestrictPublicDiscovery)
	e.home(t, viewer, pub.ID, loc.ID, reply.ID, relayed.ID, both.ID, own.ID, ja.ID)
	if _, err := e.graph.MuteDomain(ctx, viewer, timelineRemoteDomain); err != nil {
		t.Fatal(err)
	}
	e.home(t, viewer, pub.ID, loc.ID, reply.ID, both.ID, own.ID, ja.ID)

	// The tags of the current revision: an edit that drops the tag drops the post.
	e.reviseWith(t, pub, postOpts{language: "en", tags: []string{birds}})
	e.home(t, viewer, loc.ID, reply.ID, both.ID, own.ID, ja.ID)
	// Unfollowing the tag leaves the follows.
	if _, err := e.svc.UnfollowTag(ctx, viewer, "cats"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UnfollowTag(ctx, viewer, "dogs"); err != nil {
		t.Fatal(err)
	}
	e.home(t, viewer, both.ID, own.ID)
}

// timelineHomeFollowedTagStreams checks what the home streams ask: HomeAudience includes the
// followers of the tags of a public or local post, and HomeEntry lists it with its tags.
func timelineHomeFollowedTagStreams(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	viewer, other, author := e.account(true), e.account(true), e.account(true)
	cats := e.followTag(t, viewer, "cats")
	pub := e.post(t, author, content.VisibilityPublic, postOpts{tags: []string{cats}})
	unl := e.post(t, author, content.VisibilityUnlisted, postOpts{tags: []string{cats}})

	got, err := e.svc.HomeAudience(ctx, pub.ID, author, []string{viewer, other, author})
	if want := map[string]bool{viewer: true, author: true}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("HomeAudience(public) = %v, %v; want %v", got, err, want)
	}
	got, err = e.svc.HomeAudience(ctx, unl.ID, author, []string{viewer, other})
	if err != nil || len(got) != 0 {
		t.Errorf("HomeAudience(unlisted) = %v, %v; want none", got, err)
	}
	en, ok, err := e.svc.HomeEntry(ctx, viewerOf(viewer), pub.ID, apptimeline.Query{})
	if err != nil || !ok || !slices.Equal(en.FollowedTags, []string{"cats"}) {
		t.Errorf("HomeEntry(public) = %+v, %v, %v; want it with cats", en, ok, err)
	}
	if _, ok, err := e.svc.HomeEntry(ctx, viewerOf(viewer), unl.ID, apptimeline.Query{}); err != nil || ok {
		t.Errorf("HomeEntry(unlisted) = %v, %v; want false", ok, err)
	}
	if _, ok, err := e.svc.HomeEntry(ctx, viewerOf(other), pub.ID, apptimeline.Query{}); err != nil || ok {
		t.Errorf("HomeEntry(other) = %v, %v; want false", ok, err)
	}
}
