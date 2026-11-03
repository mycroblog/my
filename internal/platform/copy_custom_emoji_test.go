// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestCopyRemoteCustomEmoji copies the custom emoji of Mastodon and Akkoma notes into local ones
// through the admin API on the composition root (ADR-0051 §4): the copy has its own image, local
// posts use it, and it is in the audit log (§5); an image that cannot be fetched, that is too
// big, or of a domain whose media the instance rejects is refused with a 422 and leaves nothing.
func TestCopyRemoteCustomEmoji(t *testing.T) {
	t.Parallel()
	network := &fixtureNet{docs: map[string][]byte{}, selves: map[string]string{}}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), HTTPClient: network})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()
	admin, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	token := issueAccessToken(t, app, admin.User.ID, "read write admin")
	api := func(method, path string, body any, out any) int {
		t.Helper()
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, clientapi.BasePath+path, r)
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res := spec.Serve(t, h, req)
		if out != nil {
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
		}
		return res.StatusCode
	}

	// The images: Mastodon's :blobcat: qualifies, Akkoma's :joker_smile: is too big, and
	// :joker_disapprove: is gone.
	network.docs["https://mastodon.example/system/custom_emojis/images/000/000/001/original/blobcat.png"] = testkit.FixturePNG(32, 32, true)
	network.docs["https://akkoma.example/emoji/custom/joker_smile.png"] = testkit.FixturePNG(2048, 8, true)
	for _, c := range [][2]string{
		{"mastodon/4.7/create-note-emoji", "mastodon/4.8/actor-emoji"},
		{"akkoma/circa-2021/note-emoji-summary", "akkoma/3.20/actor-emoji"},
	} {
		note, author := network.serveNote(t, federationFixture(t, c[0]))
		network.serveActor(t, federationFixture(t, c[1]), author)
		if resolved, err := app.Resolver.Resolve(ctx, note); err != nil || resolved.Status == nil {
			t.Fatalf("Resolve(%s) = %+v, %v", note, resolved, err)
		}
	}
	remote := func(shortcode, domain string) string {
		t.Helper()
		var page clientapi.AdminCustomEmojiPage
		if code := api(http.MethodGet, "/admin/custom_emojis?local=false&shortcode="+shortcode+"&domain="+domain, nil, &page); code != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("the remote :%s: = %d %+v", shortcode, code, page)
		}
		return page.Items[0].Id
	}
	blobcat := remote("blobcat", "mastodon.example")

	var copied clientapi.AdminCustomEmoji
	if code := api(http.MethodPost, "/admin/custom_emojis/"+blobcat+"/copy", nil, &copied); code != http.StatusCreated {
		t.Fatalf("POST copy = %d", code)
	}
	if !copied.Local || copied.Id == blobcat || copied.Shortcode != "blobcat" || copied.Domain != nil || copied.State != clientapi.CustomEmojiStateEnabled || !copied.VisibleInPicker ||
		copied.Url == nil || strings.Contains(*copied.Url, "/media/proxy/") {
		t.Errorf("copied emoji = %+v", copied)
	}
	post, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: admin.AccountID, TextInput: appcontent.TextInput{Text: "a copied :blobcat:"}, Visibility: domcontent.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	var st clientapi.Status
	if code := api(http.MethodGet, "/statuses/"+post.Status.ID, nil, &st); code != http.StatusOK || len(st.Emojis) != 1 || st.Emojis[0].Url != deref(copied.Url) {
		t.Errorf("the emoji of a local post = %d %+v, want the copy", code, st.Emojis)
	}
	stored, _, err := app.Media.StoredBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Refused copies keep nothing: no emoji, no upload, no audit log entry.
	if _, err := app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "mastodon.example", Kind: moderation.DomainRejectMedia}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id   string
		body map[string]any
		want clientapi.CustomEmojiProblemReason
	}{
		{remote("joker_smile", "akkoma.example"), nil, clientapi.CustomEmojiProblemReasonTooBig},
		{remote("joker_disapprove", "akkoma.example"), nil, clientapi.CustomEmojiProblemReasonFetchFailed},
		{blobcat, map[string]any{"shortcode": "blobcat_again"}, clientapi.CustomEmojiProblemReasonFetchFailed},
		{blobcat, nil, clientapi.CustomEmojiProblemReasonShortcodeTaken},
		{copied.Id, map[string]any{"shortcode": "blobcat_again"}, clientapi.CustomEmojiProblemReasonLocalEmoji},
	} {
		var problem clientapi.CustomEmojiProblem
		if code := api(http.MethodPost, "/admin/custom_emojis/"+tc.id+"/copy", tc.body, &problem); code != http.StatusUnprocessableEntity ||
			problem.Reason == nil || *problem.Reason != tc.want {
			t.Errorf("POST copy %s %v = %d %+v, want %s", tc.id, tc.body, code, problem, tc.want)
		}
	}
	var local clientapi.AdminCustomEmojiPage
	if code := api(http.MethodGet, "/admin/custom_emojis?local=true", nil, &local); code != http.StatusOK || len(local.Items) != 1 {
		t.Errorf("local emoji = %d %+v, want the copy alone", code, local.Items)
	}
	if after, _, err := app.Media.StoredBytes(ctx); err != nil || after != stored {
		t.Errorf("stored bytes = %d, %v; want %d as before the refused copies", after, err, stored)
	}
	if code := api(http.MethodPost, "/admin/custom_emojis/0192f0c1-0000-7000-8000-00000000ffff/copy", nil, nil); code != http.StatusNotFound {
		t.Errorf("POST copy of an unknown emoji = %d", code)
	}

	entries, err := app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetCustomEmoji}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Action != moderation.AuditCustomEmojiCopied || entries[0].TargetID != copied.Id ||
		entries[0].TargetLabel != ":blobcat:" || entries[0].ActorUserID != admin.User.ID {
		t.Errorf("audit log = %+v", entries)
	}
}
