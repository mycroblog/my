// SPDX-License-Identifier: AGPL-3.0-only

package timeline_test

import (
	"slices"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// TestCommitHooksSignalTheDeletions checks that a deletion by the author, a moderator, or a
// remote Delete, alone or among many, signals the deleted status with its author, domain, and
// visibility, and that the purges of many statuses at once signal nothing.
func TestCommitHooksSignalTheDeletions(t *testing.T) {
	pub := &recordingPublisher{}
	projectors := &uow.Projectors{}
	apptimeline.RegisterCommitHooks(projectors, pub)
	deleted := func(id string, local bool, v appcontent.Visibility) appcontent.StatusDeleted {
		return appcontent.StatusDeleted{Status: appcontent.Status{
			ID: id, URI: "https://remote.example/statuses/" + id, AuthorAccountID: "author", IsLocal: local, Visibility: v,
		}}
	}
	// The origins are those of status_deletions.origin.
	own, remote := deleted("own", true, appcontent.VisibilityPrivate), deleted("remote", false, appcontent.VisibilityPublic)
	own.Deletion.Origin, remote.Deletion.Origin = "author", "remote"
	erased, old := deleted("erased", true, appcontent.VisibilityPublic), deleted("old", true, appcontent.VisibilityPublic)
	erased.Deletion.Origin, old.Deletion.Origin = "account_deletion", "auto_delete"
	purged, moderated := deleted("purged", false, appcontent.VisibilityPublic), deleted("moderated", true, appcontent.VisibilityUnlisted)
	purged.Deletion.Origin, moderated.Deletion.Origin = "retention", "moderation"
	projectors.Committed(t.Context(), []uow.Event{
		own, remote, erased, old, purged,
		appcontent.StatusesDeleted{Deletions: []appcontent.StatusDeleted{moderated}},
	})
	want := []stream.Event{
		{Kind: stream.KindStatusDeleted, EntryID: "own", AuthorAccountID: "author", Visibility: "private"},
		{Kind: stream.KindStatusDeleted, EntryID: "remote", AuthorAccountID: "author", AuthorDomain: "remote.example", Visibility: "public"},
		{Kind: stream.KindStatusDeleted, EntryID: "moderated", AuthorAccountID: "author", Visibility: "unlisted"},
	}
	if !slices.Equal(pub.events, want) {
		t.Errorf("signals = %+v\nwant %+v", pub.events, want)
	}
}

// TestDeletedStatusOnTimeline checks which public timeline streams send a deletion: those whose
// timeline may have listed the status.
func TestDeletedStatusOnTimeline(t *testing.T) {
	for _, c := range []struct {
		domain           string
		v                appcontent.Visibility
		local, federated bool
	}{
		{"", appcontent.VisibilityPublic, true, true},
		{"", appcontent.VisibilityLocal, true, false},
		{"", appcontent.VisibilityUnlisted, false, false},
		{"remote.example", appcontent.VisibilityPublic, false, true},
		{"remote.example", appcontent.VisibilityPrivate, false, false},
	} {
		d := apptimeline.DeletedStatus{ID: "s", AuthorAccountID: "a", AuthorDomain: c.domain, Visibility: c.v}
		if got := d.OnTimeline(stream.TimelineLocal); got != c.local {
			t.Errorf("%q %s on local = %v, want %v", c.domain, c.v, got, c.local)
		}
		if got := d.OnTimeline(stream.TimelineFederated); got != c.federated {
			t.Errorf("%q %s on federated = %v, want %v", c.domain, c.v, got, c.federated)
		}
	}
}

type recordingPublisher struct{ events []stream.Event }

func (p *recordingPublisher) Publish(e stream.Event) { p.events = append(p.events, e) }

// TestCommitHooksSignalThePublicTimelines checks the signals of the commits: every post and
// boost is a home entry, and a post that the local or the federated timeline may list, local or
// stored from a remote server, is also a public timeline entry for those timelines.
func TestCommitHooksSignalThePublicTimelines(t *testing.T) {
	pub := &recordingPublisher{}
	projectors := &uow.Projectors{}
	apptimeline.RegisterCommitHooks(projectors, pub)
	created := func(id string, local bool, v appcontent.Visibility) uow.Event {
		return appcontent.StatusCreated{Status: appcontent.Status{ID: id, IsLocal: local, Visibility: v}}
	}
	projectors.Committed(t.Context(), []uow.Event{
		created("public", true, appcontent.VisibilityPublic),
		created("local", true, appcontent.VisibilityLocal),
		created("unlisted", true, appcontent.VisibilityUnlisted),
		created("private", true, appcontent.VisibilityPrivate),
		created("direct", true, appcontent.VisibilityDirect),
		created("remote", false, appcontent.VisibilityPublic),
		created("remote-unlisted", false, appcontent.VisibilityUnlisted),
		appcontent.Boosted{Boost: appcontent.Boost{ID: "boost", StatusID: "public", Visibility: appcontent.VisibilityPublic, IsLocal: true}},
	})
	home := func(id string) stream.Event { return stream.Event{Kind: stream.KindHomeEntry, EntryID: id} }
	want := []stream.Event{
		home("public"), {Kind: stream.KindPublicEntry, EntryID: "public", OnLocal: true, OnFederated: true},
		home("local"), {Kind: stream.KindPublicEntry, EntryID: "local", OnLocal: true},
		home("unlisted"), home("private"), home("direct"),
		home("remote"), {Kind: stream.KindPublicEntry, EntryID: "remote", OnFederated: true},
		home("remote-unlisted"),
		home("boost"),
	}
	if !slices.Equal(pub.events, want) {
		t.Errorf("signals = %+v\nwant %+v", pub.events, want)
	}
}
