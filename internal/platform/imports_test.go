// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	domidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestCSVExportAndImport checks the CSV files on the real database: what one user exports, in
// Mastodon's formats, another user imports, and the import's jobs follow, block, mute, and
// bookmark the same accounts and posts, local and remote, with the follow's settings; an
// address that leads to no account is counted as failed, and an overwrite removes the rest.
func TestCSVExportAndImport(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	ctx := t.Context()
	alice, bob, dave, erin := e.user(t, "alice"), e.user(t, "bob"), e.user(t, "dave"), e.user(t, "erin")
	const actor = "https://remote.example/users/carol"
	var carol account.Account
	err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		carol, err = e.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: actor, Domain: "remote.example", Type: "Person", PreferredUsername: "carol",
			InboxURL: actor + "/inbox", FollowersURL: actor + "/followers", Document: []byte(`{"id":"` + actor + `"}`),
			Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "carol", Domain: "remote.example"},
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	post := e.post(t, bob, "worth keeping", domcontent.VisibilityPublic, "en")

	// Alice follows bob without his boosts and carol, blocks dave, mutes erin, and bookmarks bob's post.
	e.follow(t, alice, bob)
	if _, err := e.app.Graph.UpdatePreferences(ctx, alice.accountID, bob.accountID, graph.PreferencesInput{ShowBoosts: false}); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Subscriptions.Subscribe(ctx, alice.accountID, bob.accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Graph.Follow(ctx, alice.accountID, carol.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Graph.Block(ctx, alice.accountID, dave.accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Graph.Mute(ctx, alice.accountID, erin.accountID, graph.MuteInput{HideNotifications: false}); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Interactions.Bookmark(ctx, alice.accountID, post); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"xn--r8jz45g.xn--zckzah", "spam.example"} {
		if _, err := e.app.Graph.MuteDomain(ctx, alice.accountID, d); err != nil {
			t.Fatal(err)
		}
	}

	domain := e.app.Config.Host
	if e.app.Config.AccountDomain != "" {
		domain = e.app.Config.AccountDomain
	}
	export := func(kind string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, clientapi.BasePath+"/csv_exports/"+kind, nil)
		req.Header.Set("Authorization", "Bearer "+alice.token)
		res := e.spec.Serve(t, e.h, req)
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /csv_exports/%s = %d %s", kind, res.StatusCode, body)
		}
		return string(body)
	}
	// The follow of carol is pending until her server accepts it, so only bob is listed.
	files := map[domidentity.ImportKind]string{}
	for kind, want := range map[domidentity.ImportKind]string{
		domidentity.ImportFollowing:   "Account address,Show boosts,Notify on new posts,Languages\nbob@" + domain + ",false,true,\n",
		domidentity.ImportBlocks:      "dave@" + domain + "\n",
		domidentity.ImportMutes:       "Account address,Hide notifications\nerin@" + domain + ",false\n",
		domidentity.ImportDomainMutes: "spam.example\nxn--r8jz45g.xn--zckzah\n",
	} {
		if got := export(string(kind)); got != want {
			t.Errorf("%s = %q, want %q", kind, got, want)
		}
		files[kind] = want
	}
	bookmarks := export("bookmarks")
	if want := "https://" + e.app.Config.Host + "/"; !bytes.HasPrefix([]byte(bookmarks), []byte(want)) {
		t.Errorf("bookmarks = %q, want the post's URI", bookmarks)
	}
	files[domidentity.ImportBookmarks] = bookmarks
	// A Mastodon file may name remote accounts and accounts that do not exist.
	files[domidentity.ImportFollowing] += "carol@remote.example,true,false,\nghost@" + domain + ",true,false,\n"
	// This server's own domain cannot be muted.
	files[domidentity.ImportDomainMutes] += e.app.Config.Host + "\n"

	// Bob imports them.
	for _, kind := range domidentity.ImportKinds {
		in := identity.ImportInput{UserID: bob.userID, AccountID: bob.accountID, Kind: kind, Mode: domidentity.ImportMerge, Text: files[kind]}
		started, err := e.app.Imports.StartImport(ctx, in)
		if err != nil {
			t.Fatalf("StartImport(%s) = %v", kind, err)
		}
		if err := e.app.Imports.ProcessImport(ctx, started.ID); err != nil {
			t.Fatalf("ProcessImport(%s) = %v", kind, err)
		}
	}
	list, err := e.app.Imports.ListImports(ctx, bob.userID)
	if err != nil || len(list) != len(domidentity.ImportKinds) {
		t.Fatalf("ListImports = %+v, %v", list, err)
	}
	for _, imp := range list {
		wantFailed := 0
		switch imp.Kind {
		case domidentity.ImportFollowing:
			wantFailed = 2 // himself, and the ghost
		case domidentity.ImportDomainMutes:
			wantFailed = 1 // this server's domain
		}
		if !imp.Finished() || imp.Failed != wantFailed {
			t.Errorf("%s import = %+v, want finished with %d failed", imp.Kind, imp, wantFailed)
		}
	}
	rel := func(target string) graph.Relationship {
		t.Helper()
		r, err := e.app.Graph.Relationship(ctx, bob.accountID, target)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := rel(carol.ID); !r.RequestedFollow() {
		t.Errorf("bob → carol = %+v, want a follow request sent to her server", r)
	}
	if r := rel(dave.accountID); !r.Blocking {
		t.Errorf("bob → dave = %+v, want blocking", r)
	}
	if r := rel(erin.accountID); !r.Muting || r.MutingNotifications {
		t.Errorf("bob → erin = %+v, want muting with notifications", r)
	}
	if mutes, err := e.app.Graph.DomainMutes(ctx, bob.accountID, graph.DomainMutePage{}); err != nil || len(mutes) != 2 ||
		mutes[0].Domain != "spam.example" || mutes[1].Domain != "xn--r8jz45g.xn--zckzah" {
		t.Errorf("bob's domain mutes = %+v, %v", mutes, err)
	}
	page, err := e.app.Statuses.Bookmarks(ctx, bob.accountID, "", 10)
	if err != nil || len(page.Statuses) != 1 || page.Statuses[0].Status.ID != post {
		t.Errorf("bob's bookmarks = %+v, %v; want the post", page, err)
	}

	// Alice overwrites her blocks with an empty list.
	in := identity.ImportInput{UserID: alice.userID, AccountID: alice.accountID, Kind: domidentity.ImportBlocks, Mode: domidentity.ImportOverwrite}
	started, err := e.app.Imports.StartImport(ctx, in)
	if err != nil || started.Total != 1 {
		t.Fatalf("StartImport(overwrite) = %+v, %v; want one removal", started, err)
	}
	if err := e.app.Imports.ProcessImport(ctx, started.ID); err != nil {
		t.Fatal(err)
	}
	if blocking, _ := e.app.Graph.IsBlocking(ctx, alice.accountID, dave.accountID); blocking {
		t.Error("alice still blocks dave after the overwrite")
	}

	// Alice overwrites her muted domains with a file that names one of them as a U-label.
	in = identity.ImportInput{
		UserID: alice.userID, AccountID: alice.accountID, Kind: domidentity.ImportDomainMutes, Mode: domidentity.ImportOverwrite,
		Text: "#domain\n例え.テスト\n",
	}
	preview, err := e.app.Imports.PreviewImport(ctx, in)
	if err != nil || preview.Present != 1 || len(preview.Removals) != 1 || preview.Removals[0].ID != "spam.example" {
		t.Fatalf("PreviewImport(domains) = %+v, %v; want one present and spam.example removed", preview, err)
	}
	started, err = e.app.Imports.StartImport(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.app.Imports.ProcessImport(ctx, started.ID); err != nil {
		t.Fatal(err)
	}
	if mutes, err := e.app.Graph.DomainMutes(ctx, alice.accountID, graph.DomainMutePage{}); err != nil || len(mutes) != 1 || mutes[0].Domain != "xn--r8jz45g.xn--zckzah" {
		t.Errorf("alice's domain mutes after the overwrite = %+v, %v", mutes, err)
	}
}
