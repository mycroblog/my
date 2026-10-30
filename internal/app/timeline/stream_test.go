// SPDX-License-Identifier: AGPL-3.0-only

package timeline_test

import (
	"slices"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/stream"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

type recordingPublisher struct{ events []stream.Event }

func (p *recordingPublisher) Publish(e stream.Event) { p.events = append(p.events, e) }

// TestCommitHooksSignalThePublicTimelines checks the signals of the commits: every post and
// boost is a home entry, and a post that the local or the federated timeline may list, local or
// stored from a remote server, is also a public timeline entry for those timelines.
func TestCommitHooksSignalThePublicTimelines(t *testing.T) {
	pub := &recordingPublisher{}
	projectors := &uow.Projectors{}
	apptimeline.RegisterCommitHooks(projectors, pub)
	created := func(id string, local bool, v content.Visibility) uow.Event {
		return appcontent.StatusCreated{Status: content.Status{ID: id, IsLocal: local, Visibility: v}}
	}
	projectors.Committed(t.Context(), []uow.Event{
		created("public", true, content.VisibilityPublic),
		created("local", true, content.VisibilityLocal),
		created("unlisted", true, content.VisibilityUnlisted),
		created("private", true, content.VisibilityPrivate),
		created("direct", true, content.VisibilityDirect),
		created("remote", false, content.VisibilityPublic),
		created("remote-unlisted", false, content.VisibilityUnlisted),
		appcontent.Boosted{Boost: content.Boost{ID: "boost", StatusID: "public", Visibility: content.VisibilityPublic, IsLocal: true}},
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
