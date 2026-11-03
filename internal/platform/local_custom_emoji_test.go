// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
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

// TestLocalCustomEmoji registers a local custom emoji through the admin API, uses it in a post
// and a profile, and checks, through the composition root on SQLite, that the client API presents
// it with the post and the account, that the Note and the actor carry its Emoji tag with the id
// that GET /ap/emojis/{id} serves (ADR-0051 §3, §4), that the picker lists it, that disabling it
// hides it everywhere, and that each operation is in the audit log (§5).
func TestLocalCustomEmoji(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
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
	ap := func(path string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://"+cfg.Host+path, nil)
		req.Header.Set("Accept", "application/activity+json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var doc map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &doc)
		return rec.Code, doc
	}
	image := func(data []byte) string {
		t.Helper()
		m, err := app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: admin.AccountID, Content: bytes.NewReader(data), Purpose: domcontent.PurposeEmoji})
		if err != nil {
			t.Fatal(err)
		}
		if res := app.Media.Process(ctx, m.File.ID, domcontent.PurposeEmoji); !res.IsDone() {
			t.Fatalf("Process = %+v", res)
		}
		return m.File.ID
	}

	// A JPEG is refused with the reason, and nothing is stored.
	var problem clientapi.CustomEmojiProblem
	if code := api(http.MethodPost, "/admin/custom_emojis", map[string]any{"shortcode": "blobcat", "media_id": image(testkit.FixtureJPEG(32, 32, 1))}, &problem); code != http.StatusUnprocessableEntity ||
		problem.Type != clientapi.ProblemTypeInvalidCustomEmoji || problem.Reason == nil || *problem.Reason != clientapi.CustomEmojiProblemReasonUnsupportedFormat {
		t.Fatalf("POST a JPEG = %d %+v", code, problem)
	}
	var emoji clientapi.AdminCustomEmoji
	if code := api(http.MethodPost, "/admin/custom_emojis", map[string]any{
		"shortcode": "blobcat", "media_id": image(testkit.FixturePNG(32, 32, true)), "category": "Cats", "aliases": []string{"neko"},
	}, &emoji); code != http.StatusCreated {
		t.Fatalf("POST /admin/custom_emojis = %d", code)
	}
	wantURI := "https://" + cfg.Host + "/ap/emojis/" + emoji.Id
	if !emoji.Local || emoji.Shortcode != "blobcat" || deref(emoji.Uri) != wantURI || deref(emoji.Url) == "" || deref(emoji.Category) != "Cats" {
		t.Errorf("created emoji = %+v", emoji)
	}

	var picker clientapi.PickerCustomEmojiList
	if code := api(http.MethodGet, "/custom_emojis", nil, &picker); code != http.StatusOK || len(picker.Items) != 1 || picker.Items[0].Shortcode != "blobcat" ||
		!slices.Equal(picker.Items[0].Aliases, []string{"neko"}) {
		t.Errorf("GET /custom_emojis = %d %+v", code, picker)
	}

	// The emoji of a post and of a profile.
	post, err := app.Content.Create(ctx, appcontent.CreateInput{
		AuthorAccountID: admin.AccountID, TextInput: appcontent.TextInput{Text: "hello :blobcat: and :nothing:"}, Visibility: domcontent.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	name := "Alice :blobcat:"
	if _, err := app.Accounts.UpdateProfile(ctx, admin.AccountID, appaccount.ProfileUpdate{DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	var st clientapi.Status
	if code := api(http.MethodGet, "/statuses/"+post.Status.ID, nil, &st); code != http.StatusOK {
		t.Fatalf("GET status = %d", code)
	}
	for what, emojis := range map[string][]clientapi.CustomEmoji{"status": st.Emojis, "account": st.Account.Emojis} {
		if len(emojis) != 1 || emojis[0].Shortcode != "blobcat" || emojis[0].Url != deref(emoji.Url) || emojis[0].StaticUrl == "" {
			t.Errorf("%s emojis = %+v", what, emojis)
		}
	}
	emojiTag := func(doc map[string]any) map[string]any {
		t.Helper()
		tags, _ := doc["tag"].([]any)
		for _, x := range tags {
			if tag, _ := x.(map[string]any); tag["type"] == "Emoji" {
				return tag
			}
		}
		return nil
	}
	checkTag := func(what string, tag map[string]any) {
		t.Helper()
		icon, _ := tag["icon"].(map[string]any)
		if tag == nil || tag["id"] != wantURI || tag["name"] != ":blobcat:" || tag["updated"] == nil ||
			icon["type"] != "Image" || icon["mediaType"] != "image/png" || !strings.HasSuffix(icon["url"].(string), ".png") {
			t.Errorf("%s Emoji tag = %v", what, tag)
		}
	}
	code, note := ap("/ap/objects/" + post.Status.ID)
	if code != http.StatusOK {
		t.Fatalf("GET the Note = %d", code)
	}
	checkTag("Note", emojiTag(note))
	code, actor := ap("/ap/actors/" + admin.AccountID)
	if code != http.StatusOK {
		t.Fatalf("GET the actor = %d", code)
	}
	checkTag("actor", emojiTag(actor))
	code, served := ap("/ap/emojis/" + emoji.Id)
	if code != http.StatusOK || served["@context"] == nil || served["type"] != "Emoji" {
		t.Fatalf("GET %s = %d %v", wantURI, code, served)
	}
	delete(served, "@context")
	checkTag("served", served)

	// Disabled, the emoji is text again everywhere.
	if code := api(http.MethodPatch, "/admin/custom_emojis/"+emoji.Id, map[string]any{"enabled": false}, &emoji); code != http.StatusOK || emoji.Enabled {
		t.Fatalf("PATCH enabled false = %d %+v", code, emoji)
	}
	if code := api(http.MethodGet, "/statuses/"+post.Status.ID, nil, &st); code != http.StatusOK || len(st.Emojis) != 0 || len(st.Account.Emojis) != 0 {
		t.Errorf("a disabled emoji is shown: %d %+v %+v", code, st.Emojis, st.Account.Emojis)
	}
	if code, _ := ap("/ap/emojis/" + emoji.Id); code != http.StatusNotFound {
		t.Errorf("GET a disabled emoji = %d, want 404", code)
	}
	if _, note := ap("/ap/objects/" + post.Status.ID); emojiTag(note) != nil {
		t.Errorf("the Note carries a disabled emoji: %v", note["tag"])
	}
	if code := api(http.MethodGet, "/custom_emojis", nil, &picker); code != http.StatusOK || len(picker.Items) != 0 {
		t.Errorf("the picker lists a disabled emoji: %+v", picker)
	}
	if code := api(http.MethodDelete, "/admin/custom_emojis/"+emoji.Id, nil, nil); code != http.StatusNoContent {
		t.Errorf("DELETE = %d", code)
	}
	if code := api(http.MethodGet, "/admin/custom_emojis/"+emoji.Id, nil, nil); code != http.StatusNotFound {
		t.Errorf("GET a deleted emoji = %d", code)
	}

	entries, err := app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetCustomEmoji}, 10)
	if err != nil {
		t.Fatal(err)
	}
	var actions []moderation.AuditAction
	for _, e := range entries {
		if e.TargetID != emoji.Id || e.TargetLabel != ":blobcat:" || e.ActorUserID != admin.User.ID {
			t.Errorf("entry = %+v", e)
		}
		actions = append(actions, e.Action)
	}
	slices.Reverse(actions)
	want := []moderation.AuditAction{moderation.AuditCustomEmojiCreated, moderation.AuditCustomEmojiDisabled, moderation.AuditCustomEmojiDeleted}
	if !slices.Equal(actions, want) {
		t.Errorf("audit log = %v, want %v", actions, want)
	}
}
