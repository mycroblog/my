// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"slices"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// timelinePublicEntries checks that LocalEntry and PublicEntry, which the streams of the public
// timelines ask (ADR-0127 §3), list an entry exactly when Local and Public list it for the same
// viewer and timeline languages: visibility (local posts on the local timeline only), boosts
// (on neither), blocks either way, mutes, domain mutes, discovery restrictions, and languages.
func timelinePublicEntries(t *testing.T, e *timelineEnv) {
	ctx := t.Context()
	alice, bob, carol, dave, muted, blocker, silenced := e.account(true), e.account(true), e.account(true),
		e.account(true), e.account(true), e.account(true), e.account(true)
	remote := e.account(false)
	var ids []string
	add := func(s content.Status) content.Status { ids = append(ids, s.ID); return s }
	pub := add(e.public(t, alice))
	add(e.post(t, alice, content.VisibilityLocal, postOpts{}))
	for _, v := range []content.Visibility{content.VisibilityUnlisted, content.VisibilityPrivate, content.VisibilityDirect} {
		add(e.post(t, alice, v, postOpts{mentions: []string{bob}}))
		add(e.post(t, remote, v, postOpts{}))
	}
	far := add(e.public(t, remote))
	add(e.post(t, alice, content.VisibilityPublic, postOpts{language: "ja"}))
	add(e.post(t, remote, content.VisibilityPublic, postOpts{language: "ja", inReplyTo: far.URI}))
	add(e.post(t, alice, content.VisibilityPublic, postOpts{language: timeline.UndeterminedLanguage}))
	add(e.public(t, muted))
	add(e.public(t, blocker))
	add(e.post(t, carol, content.VisibilityLocal, postOpts{}))
	add(e.public(t, silenced))
	ids = append(ids, e.boost(t, carol, pub).ID, e.boost(t, carol, far).ID)

	// dave mutes, is blocked, blocks, and mutes the remote domain; silenced is kept from discovery.
	if _, err := e.graph.Mute(ctx, dave, muted, appgraph.MuteInput{}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	if _, err := e.graph.Block(ctx, blocker, dave); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if _, err := e.graph.Block(ctx, dave, carol); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if _, err := e.graph.MuteDomain(ctx, dave, timelineRemoteDomain); err != nil {
		t.Fatalf("MuteDomain: %v", err)
	}
	e.restrict(t, silenced, timeline.RestrictPublicDiscovery)

	timelines := []struct {
		name  string
		list  func(appcontent.Viewer, apptimeline.Query) (apptimeline.Page, error)
		entry func(appcontent.Viewer, string, apptimeline.Query) (apptimeline.Entry, bool, error)
	}{
		{
			"Local",
			func(v appcontent.Viewer, q apptimeline.Query) (apptimeline.Page, error) {
				return e.svc.Local(ctx, v, q)
			},
			func(v appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Entry, bool, error) {
				return e.svc.LocalEntry(ctx, v, id, q)
			},
		},
		{
			"Public",
			func(v appcontent.Viewer, q apptimeline.Query) (apptimeline.Page, error) {
				return e.svc.Public(ctx, v, q)
			},
			func(v appcontent.Viewer, id string, q apptimeline.Query) (apptimeline.Entry, bool, error) {
				return e.svc.PublicEntry(ctx, v, id, q)
			},
		},
	}
	for _, tl := range timelines {
		listed := 0
		for _, viewer := range []string{bob, dave} {
			for _, langs := range [][]string{nil, {"ja"}} {
				q := apptimeline.Query{Limit: apptimeline.MaxLimit, Languages: langs}
				p, err := tl.list(viewerOf(viewer), q)
				if err != nil {
					t.Fatalf("%s: %v", tl.name, err)
				}
				want := entryIDs(p)
				listed += len(want)
				for _, id := range ids {
					got, ok, err := tl.entry(viewerOf(viewer), id, apptimeline.Query{Languages: langs})
					if err != nil {
						t.Fatalf("%sEntry(%s): %v", tl.name, id, err)
					}
					if ok != slices.Contains(want, id) || ok && got.ID != id {
						t.Errorf("%sEntry(%s) for %s in %v = %+v, %v; %s lists %v", tl.name, id, viewer, langs, got, ok, tl.name, want)
					}
				}
			}
		}
		if listed == 0 {
			t.Errorf("%s listed nothing to compare", tl.name)
		}
		if _, _, err := tl.entry(anonymous, pub.ID, apptimeline.Query{}); !errors.Is(err, apptimeline.ErrLoginRequired) {
			t.Errorf("%sEntry(anonymous) = %v, want ErrLoginRequired", tl.name, err)
		}
	}
}
