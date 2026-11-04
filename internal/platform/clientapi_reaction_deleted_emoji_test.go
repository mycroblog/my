// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestReactionWithADeletedEmoji checks that deleting a local custom emoji leaves the posts that
// were reacted to with it readable: only the reactions that nothing can show any more go, from
// the counts and from the list. On a remote post, the reactor can still take such a reaction
// back.
func TestReactionWithADeletedEmoji(t *testing.T) {
	t.Parallel()
	c := newStatusesClient(t, "alice", "bob", "carol")
	ctx := t.Context()
	m, err := c.app.Media.Upload(ctx, appcontent.UploadInput{
		OwnerAccountID: c.ids["alice"], Content: bytes.NewReader(testkit.FixturePNG(32, 32, true)), Purpose: domcontent.PurposeEmoji,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res := c.app.Media.Process(ctx, m.File.ID, domcontent.PurposeEmoji); !res.IsDone() {
		t.Fatalf("Process = %+v", res)
	}
	var emoji clientapi.AdminCustomEmoji
	if res := c.do("alice", http.MethodPost, "/admin/custom_emojis", map[string]any{"shortcode": "blobcat", "media_id": m.File.ID}, &emoji); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /admin/custom_emojis = %d", res.StatusCode)
	}

	post := c.post("bob", map[string]any{"text": "reacted", "visibility": "public"})
	remote := remotePost(t, c.app)
	react := func(user, id, name string) {
		t.Helper()
		if res := c.do(user, http.MethodPut, "/statuses/"+id+"/reaction", map[string]any{"emoji": name}, nil); res.StatusCode != http.StatusOK {
			t.Fatalf("%s reacts with %s = %d", user, name, res.StatusCode)
		}
	}
	react("bob", post.Id, "🎉")
	react("carol", post.Id, "🎉")
	react("alice", post.Id, "blobcat")
	react("alice", remote, "blobcat")

	var report clientapi.Report
	if res := c.do("carol", http.MethodPost, "/reports", map[string]any{
		"account_id": c.ids["bob"], "status_ids": []string{post.Id}, "category": "spam",
	}, &report); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /reports = %d", res.StatusCode)
	}
	if res := c.do("alice", http.MethodDelete, "/admin/custom_emojis/"+emoji.Id, nil, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE emoji = %d", res.StatusCode)
	}

	wantOnlyParty := func(where string, rs []clientapi.StatusReaction) {
		t.Helper()
		if len(rs) != 1 || rs[0].Name != "🎉" {
			t.Errorf("%s: reactions = %+v, want only 🎉", where, rs)
		}
	}
	var st clientapi.Status
	if res := c.do("bob", http.MethodGet, "/statuses/"+post.Id, nil, &st); res.StatusCode != http.StatusOK {
		t.Fatalf("GET the post = %d, want it readable after its reaction's emoji is deleted", res.StatusCode)
	}
	wantOnlyParty("the post", st.Reactions)

	var inTimeline *clientapi.Status
	for _, item := range c.statusPage("bob", "/timelines/local").Items {
		if item.Id == post.Id {
			inTimeline = &item
		}
	}
	if inTimeline == nil {
		t.Fatal("the post left the local timeline")
	}
	wantOnlyParty("the timeline", inTimeline.Reactions)

	var detail clientapi.AdminReportDetail
	if res := c.do("carol", http.MethodGet, "/admin/reports/"+report.Id, nil, &detail); res.StatusCode != http.StatusOK || len(detail.Statuses) != 1 {
		t.Fatalf("GET report = %d, %d statuses", res.StatusCode, len(detail.Statuses))
	}
	wantOnlyParty("the report", detail.Statuses[0].Status.Reactions)

	// The list goes past the left out reaction, the newest, one page at a time.
	var listed []string
	path := "/statuses/" + post.Id + "/reactions?limit=1"
	for range 3 {
		var reactors clientapi.StatusReactionPage
		if res := c.do("bob", http.MethodGet, path, nil, &reactors); res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", path, res.StatusCode)
		}
		for _, item := range reactors.Items {
			listed = append(listed, item.Account.Username+" "+item.Emoji.Name)
		}
		if reactors.NextCursor == nil {
			break
		}
		path = "/statuses/" + post.Id + "/reactions?limit=1&cursor=" + url.QueryEscape(*reactors.NextCursor)
	}
	if want := []string{"carol 🎉", "bob 🎉"}; !slices.Equal(listed, want) {
		t.Errorf("the list of reactors = %q, want %q", listed, want)
	}

	if res := c.do("alice", http.MethodDelete, "/statuses/"+remote+"/reaction", nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("taking back the reaction on a remote post = %d", res.StatusCode)
	}
}

// remotePost ingests a public post of a remote actor and returns its ID.
func remotePost(t *testing.T, a *platform.App) string {
	t.Helper()
	ctx := t.Context()
	var dave string
	err := a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		d, err := a.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: "https://remote.example/users/dave", Domain: "remote.example", Type: "Person",
			PreferredUsername: "dave", InboxURL: "https://remote.example/users/dave/inbox",
			SharedInboxURL: "https://remote.example/inbox",
		})
		dave = d.ID
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	at := time.Now().UTC().Add(-time.Hour)
	var id string
	err = a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		st, err := a.Content.IngestRemoteStatus(ctx, tx, appcontent.RemoteStatusInput{
			URI: "https://remote.example/users/dave/statuses/1", AuthorAccountID: dave, AuthorDomain: "remote.example",
			Visibility: domcontent.VisibilityPublic, PublishedAt: at, RevisedAt: at,
			Bodies: []appcontent.RemoteBody{{Language: "und", HTML: "<p>hello</p>"}},
		})
		id = st.Status.ID
		return err
	})
	if err != nil {
		t.Fatalf("IngestRemoteStatus: %v", err)
	}
	return id
}
