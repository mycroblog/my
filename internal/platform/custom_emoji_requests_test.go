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
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/identity"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	domidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestCustomEmojiRequests runs the requests of a holder of create_custom_emojis through the
// client API and the composition root on SQLite (ADR-0070): a request is in no picker and no
// list of the emoji until an administrator approves it; the approval and the rejection are in
// the audit log and notified to the requester, and the request to the holders of
// manage_custom_emojis; the creator changes their own emoji and no other while they hold the
// permission; and a user without it is refused.
func TestCustomEmojiRequests(t *testing.T) {
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
	carol, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "carol", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	dave, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "dave", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Identity.CreateRole(ctx, identity.CreateRoleInput{
		Key: "emoji_makers", Position: 10, Permissions: []domidentity.Permission{domidentity.PermCreateCustomEmojis},
	}); err != nil {
		t.Fatal(err)
	}
	grant := identity.RoleChangeInput{UserID: carol.User.ID, RoleKey: "emoji_makers"}
	if _, err := app.Identity.GrantRole(ctx, grant); err != nil {
		t.Fatal(err)
	}
	adminToken := issueAccessToken(t, app, admin.User.ID, "read write admin")
	carolToken := issueAccessToken(t, app, carol.User.ID, "read write")
	daveToken := issueAccessToken(t, app, dave.User.ID, "read write")

	api := func(token, method, path string, body any, out any) int {
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
	image := func(accountID string) string {
		t.Helper()
		m, err := app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: accountID, Content: bytes.NewReader(testkit.FixturePNG(32, 32, true)), Purpose: domcontent.PurposeEmoji})
		if err != nil {
			t.Fatal(err)
		}
		if res := app.Media.Process(ctx, m.File.ID, domcontent.PurposeEmoji); !res.IsDone() {
			t.Fatalf("Process = %+v", res)
		}
		return m.File.ID
	}
	notifications := func(token string, typ clientapi.NotificationType) []clientapi.Notification {
		t.Helper()
		var page clientapi.NotificationPage
		if code := api(token, http.MethodGet, "/notifications", nil, &page); code != http.StatusOK {
			t.Fatalf("GET /notifications = %d", code)
		}
		var out []clientapi.Notification
		for _, n := range page.Items {
			if n.Type == typ {
				out = append(out, n)
			}
		}
		return out
	}
	auditActions := func() []clientapi.AdminAuditAction {
		t.Helper()
		var page clientapi.AdminAuditEntryPage
		if code := api(adminToken, http.MethodGet, "/admin/audit_log", nil, &page); code != http.StatusOK {
			t.Fatalf("GET /admin/audit_log = %d", code)
		}
		var out []clientapi.AdminAuditAction
		for _, e := range page.Items {
			out = append(out, e.Action)
		}
		return out
	}

	// A user without the permission is refused.
	var problem clientapi.Problem
	if code := api(daveToken, http.MethodPost, "/custom_emoji_requests", map[string]any{"kind": "create", "shortcode": "blobdave", "media_id": image(dave.AccountID)}, &problem); code != http.StatusForbidden ||
		problem.Type != clientapi.ProblemTypeMissingPermission {
		t.Errorf("a request without the permission = %d %+v", code, problem)
	}

	var req clientapi.CustomEmojiRequest
	if code := api(carolToken, http.MethodPost, "/custom_emoji_requests", map[string]any{"kind": "create", "shortcode": "blobcat", "media_id": image(carol.AccountID)}, &req); code != http.StatusOK ||
		req.State != clientapi.CustomEmojiRequestStatePending || req.Shortcode != "blobcat" || req.Url == nil {
		t.Fatalf("POST /custom_emoji_requests = %d %+v", code, req)
	}
	// The pending request counts toward the limit, apart from the emoji that exist.
	var usage clientapi.OwnCustomEmojiPage
	if code := api(carolToken, http.MethodGet, "/custom_emojis/mine", nil, &usage); code != http.StatusOK || len(usage.Items) != 0 || usage.Used != 1 || usage.Pending != 1 {
		t.Errorf("GET /custom_emojis/mine = %d %+v", code, usage)
	}
	// Nothing shows the requested emoji.
	var picker clientapi.PickerCustomEmojiList
	if code := api(carolToken, http.MethodGet, "/custom_emojis", nil, &picker); code != http.StatusOK || len(picker.Items) != 0 {
		t.Errorf("the picker shows a request: %d %+v", code, picker)
	}
	var emojis clientapi.AdminCustomEmojiPage
	if code := api(adminToken, http.MethodGet, "/admin/custom_emojis", nil, &emojis); code != http.StatusOK || len(emojis.Items) != 0 {
		t.Errorf("the emoji list shows a request: %d %+v", code, emojis)
	}
	// The administrators are told of it, and review it.
	if ns := notifications(adminToken, clientapi.NotificationTypeAdminCustomEmojiRequest); len(ns) != 1 || ns[0].CustomEmojiRequest == nil || ns[0].CustomEmojiRequest.Id != req.Id {
		t.Errorf("the administrator's notifications = %+v", ns)
	}
	var pending clientapi.AdminCustomEmojiRequestPage
	if code := api(adminToken, http.MethodGet, "/admin/custom_emoji_requests", nil, &pending); code != http.StatusOK || len(pending.Items) != 1 ||
		pending.Items[0].Requester == nil || pending.Items[0].Requester.Username != "carol" {
		t.Fatalf("GET /admin/custom_emoji_requests = %d %+v", code, pending)
	}
	if code := api(carolToken, http.MethodGet, "/admin/custom_emoji_requests", nil, nil); code != http.StatusForbidden {
		t.Errorf("a creator lists the requests = %d, want 403", code)
	}
	var approved clientapi.AdminCustomEmojiRequest
	if code := api(adminToken, http.MethodPost, "/admin/custom_emoji_requests/"+req.Id+"/approve", nil, &approved); code != http.StatusOK ||
		approved.State != clientapi.CustomEmojiRequestStateApproved || approved.EmojiId == nil {
		t.Fatalf("approve = %d %+v", code, approved)
	}
	var conflict clientapi.CustomEmojiRequestProblem
	if code := api(adminToken, http.MethodPost, "/admin/custom_emoji_requests/"+req.Id+"/approve", nil, &conflict); code != http.StatusConflict ||
		conflict.Reason == nil || *conflict.Reason != clientapi.CustomEmojiRequestConflictReasonNotPending {
		t.Errorf("approve again = %d %+v", code, conflict)
	}
	if code := api(carolToken, http.MethodGet, "/custom_emojis", nil, &picker); code != http.StatusOK || len(picker.Items) != 1 || picker.Items[0].Shortcode != "blobcat" {
		t.Errorf("the picker after the approval = %d %+v", code, picker)
	}
	if code := api(adminToken, http.MethodGet, "/admin/custom_emojis", nil, &emojis); code != http.StatusOK || len(emojis.Items) != 1 ||
		emojis.Items[0].CreatedBy == nil || emojis.Items[0].CreatedBy.Username != "carol" || emojis.Items[0].CreatedByDeleted {
		t.Errorf("the emoji list after the approval = %d %+v", code, emojis)
	}
	if ns := notifications(carolToken, clientapi.NotificationTypeCustomEmojiRequestDecided); len(ns) != 1 || ns[0].CustomEmojiRequest == nil ||
		ns[0].CustomEmojiRequest.State != clientapi.CustomEmojiRequestStateApproved {
		t.Errorf("the requester's notifications = %+v", ns)
	}

	// A rejection, with its reason.
	if code := api(carolToken, http.MethodPost, "/custom_emoji_requests", map[string]any{"kind": "create", "shortcode": "blobdog", "media_id": image(carol.AccountID)}, &req); code != http.StatusOK {
		t.Fatalf("POST /custom_emoji_requests = %d", code)
	}
	var rejected clientapi.AdminCustomEmojiRequest
	if code := api(adminToken, http.MethodPost, "/admin/custom_emoji_requests/"+req.Id+"/reject", map[string]any{"reason": "Too blurry"}, &rejected); code != http.StatusOK ||
		rejected.State != clientapi.CustomEmojiRequestStateRejected || deref(rejected.Reason) != "Too blurry" {
		t.Fatalf("reject = %d %+v", code, rejected)
	}
	decided := notifications(carolToken, clientapi.NotificationTypeCustomEmojiRequestDecided)
	if len(decided) != 2 || decided[0].CustomEmojiRequest == nil || deref(decided[0].CustomEmojiRequest.Reason) != "Too blurry" {
		t.Errorf("the requester's notifications after the rejection = %+v", decided)
	}
	var mine clientapi.CustomEmojiRequestPage
	if code := api(carolToken, http.MethodGet, "/custom_emoji_requests", nil, &mine); code != http.StatusOK || len(mine.Items) != 2 {
		t.Errorf("GET /custom_emoji_requests = %d %+v", code, mine)
	}
	got := auditActions()
	for _, want := range []clientapi.AdminAuditAction{
		clientapi.AdminAuditActionCustomEmojiRequestSubmitted, clientapi.AdminAuditActionCustomEmojiRequestApproved,
		clientapi.AdminAuditActionCustomEmojiCreated, clientapi.AdminAuditActionCustomEmojiRequestRejected,
	} {
		if !slices.Contains(got, want) {
			t.Errorf("the audit log %v has no %s", got, want)
		}
	}

	// The creator changes their own emoji, and no other.
	var own clientapi.OwnCustomEmojiPage
	if code := api(carolToken, http.MethodGet, "/custom_emojis/mine", nil, &own); code != http.StatusOK || len(own.Items) != 1 || own.Used != 1 || own.Pending != 0 || own.Limit != nil {
		t.Fatalf("GET /custom_emojis/mine = %d %+v", code, own)
	}
	var changed clientapi.OwnCustomEmoji
	if code := api(carolToken, http.MethodPatch, "/custom_emojis/mine/"+own.Items[0].Id, map[string]any{"category": "Cats", "aliases": []string{"neko"}}, &changed); code != http.StatusOK ||
		deref(changed.Category) != "Cats" || !slices.Equal(changed.Aliases, []string{"neko"}) {
		t.Errorf("PATCH own emoji = %d %+v", code, changed)
	}
	var adminEmoji clientapi.AdminCustomEmoji
	if code := api(adminToken, http.MethodPost, "/admin/custom_emojis", map[string]any{"shortcode": "blobfox", "media_id": image(admin.AccountID)}, &adminEmoji); code != http.StatusOK {
		t.Fatalf("POST /admin/custom_emojis = %d", code)
	}
	if code := api(carolToken, http.MethodPatch, "/custom_emojis/mine/"+adminEmoji.Id, map[string]any{"category": "Cats"}, nil); code != http.StatusNotFound {
		t.Errorf("PATCH another's emoji = %d, want 404", code)
	}
	if code := api(carolToken, http.MethodDelete, "/custom_emojis/mine/"+adminEmoji.Id, nil, nil); code != http.StatusNotFound {
		t.Errorf("DELETE another's emoji = %d, want 404", code)
	}
	var imageReq clientapi.CustomEmojiRequest
	if code := api(carolToken, http.MethodPost, "/custom_emoji_requests", map[string]any{"kind": "image", "emoji_id": own.Items[0].Id, "media_id": image(carol.AccountID)}, &imageReq); code != http.StatusOK ||
		imageReq.Kind != clientapi.CustomEmojiRequestKindImage {
		t.Errorf("an image request = %d %+v", code, imageReq)
	}
	// Once the role is gone, the emoji stays and its creator cannot change it.
	if _, err := app.Identity.RevokeRole(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if code := api(carolToken, http.MethodPatch, "/custom_emojis/mine/"+own.Items[0].Id, map[string]any{"category": "Dogs"}, nil); code != http.StatusForbidden {
		t.Errorf("PATCH own emoji without the permission = %d, want 403", code)
	}
	if code := api(adminToken, http.MethodPost, "/admin/custom_emoji_requests/"+imageReq.Id+"/approve", nil, &conflict); code != http.StatusConflict ||
		conflict.Reason == nil || *conflict.Reason != clientapi.CustomEmojiRequestConflictReasonRequesterCannotCreate {
		t.Errorf("approve a request of a user without the permission = %d %+v", code, conflict)
	}
	if code := api(carolToken, http.MethodGet, "/custom_emojis", nil, &picker); code != http.StatusOK || len(picker.Items) != 2 {
		t.Errorf("the picker after the revocation = %d %+v", code, picker)
	}
}
