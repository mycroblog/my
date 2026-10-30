// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestAnnouncements writes, publishes, revises, and withdraws an announcement through the admin
// API on SQLite (FR-726), and checks that the operations need manage_announcements and a title
// and are audited, that every local user sees the published announcement as sanitized HTML with
// its title and marks it read for themselves, that each gets one announcement notification that
// the withdrawal hides, that the list with the past announcements leaves out the withdrawn one,
// and that an open stream gets the publication and the withdrawal.
func TestAnnouncements(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
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
	adminToken := issueAccessToken(t, app, admin.User.ID, "read write admin")
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	bobToken := issueAccessToken(t, app, bob.User.ID, "read write admin")

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
	list := func(token string) []clientapi.Announcement {
		t.Helper()
		var l clientapi.AnnouncementList
		if code := api(token, http.MethodGet, "/announcements", nil, &l); code != http.StatusOK {
			t.Fatalf("GET /announcements = %d", code)
		}
		return l.Items
	}

	// Without manage_announcements, nothing.
	if code := api(bobToken, http.MethodGet, "/admin/announcements", nil, nil); code != http.StatusForbidden {
		t.Errorf("GET /admin/announcements without manage_announcements = %d, want 403", code)
	}
	if code := api(adminToken, http.MethodPost, "/admin/announcements", map[string]any{"title": "Maintenance", "text": "  "}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("POST a blank announcement = %d, want 422", code)
	}
	for _, title := range []string{"", "  ", "Two\nlines", strings.Repeat("a", 101)} {
		var p clientapi.Problem
		if code := api(adminToken, http.MethodPost, "/admin/announcements", map[string]any{"title": title, "text": "ok"}, &p); code != http.StatusUnprocessableEntity ||
			p.Detail == nil || !strings.Contains(*p.Detail, "title") {
			t.Errorf("POST an announcement titled %q = %d %+v, want 422 naming title", title, code, p)
		}
	}
	if code := api(adminToken, http.MethodPost, "/admin/announcements", map[string]any{"text": "no title"}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("POST an announcement without a title = %d, want 422", code)
	}
	now := time.Now().UTC()
	if code := api(adminToken, http.MethodPost, "/admin/announcements", map[string]any{
		"title": "ok", "text": "ok", "starts_at": now.Add(time.Hour), "ends_at": now,
	}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("POST a window that ends first = %d, want 422", code)
	}

	var draft clientapi.AdminAnnouncement
	if code := api(adminToken, http.MethodPost, "/admin/announcements", map[string]any{
		"title": "  Maintenance on Sunday ", "text": "# Maintenance\n\nOn **Sunday**. <script>alert(1)</script>",
	}, &draft); code != http.StatusOK || draft.State != clientapi.AdminAnnouncementStateDraft || draft.PublishedAt != nil ||
		draft.Title != "Maintenance on Sunday" {
		t.Fatalf("POST /admin/announcements = %d %+v", code, draft)
	}
	if strings.Contains(draft.Content, "<script") || !strings.Contains(draft.Content, "<strong>Sunday</strong>") {
		t.Errorf("content = %q, want sanitized HTML", draft.Content)
	}
	if got := list(bobToken); len(got) != 0 {
		t.Errorf("GET /announcements with a draft = %+v, want none", got)
	}

	// An open stream of bob's gets the publication.
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	streamCtx, disconnect := context.WithCancel(ctx)
	defer disconnect()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+clientapi.BasePath+"/streaming", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+bobToken)
	res, err := srv.Client().Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("GET /streaming = %v, %v", res, err)
	}
	defer func() { _ = res.Body.Close() }()
	events := readEvents(bufio.NewReader(res.Body))
	next := func(name string) []byte {
		t.Helper()
		select {
		case ev, ok := <-events:
			if !ok || ev.name != name {
				t.Fatalf("event %q (%s), %v; want %q", ev.name, ev.data, ok, name)
			}
			return ev.data
		case <-time.After(5 * time.Second):
			t.Fatalf("no %s event within 5 seconds", name)
		}
		return nil
	}
	deadline := time.Now().Add(5 * time.Second)
	for app.Streams.Len() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the stream did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}

	var published clientapi.AdminAnnouncement
	if code := api(adminToken, http.MethodPost, "/admin/announcements/"+draft.Id+"/publish", nil, &published); code != http.StatusOK ||
		published.State != clientapi.AdminAnnouncementStatePublished || published.PublishedAt == nil {
		t.Fatalf("POST publish = %d %+v", code, published)
	}
	data := next("announcement")
	spec.ValidateSchema(t, "Announcement", data)
	var streamed clientapi.Announcement
	if err := json.Unmarshal(data, &streamed); err != nil || streamed.Id != draft.Id || streamed.Read || streamed.Title != "Maintenance on Sunday" {
		t.Errorf("announcement event = %s, %v", data, err)
	}

	got := list(bobToken)
	if len(got) != 1 || got[0].Id != draft.Id || got[0].Read || got[0].Content != draft.Content || got[0].Ended {
		t.Fatalf("GET /announcements = %+v, want the published announcement, unread", got)
	}

	// Each local user gets one announcement notification once the job runs, with the
	// announcement as the user sees it.
	notifications := func(token string) []clientapi.Notification {
		t.Helper()
		var page clientapi.NotificationPage
		if code := api(token, http.MethodGet, "/notifications?types[]=announcement", nil, &page); code != http.StatusOK {
			t.Fatalf("GET /notifications = %d", code)
		}
		return page.Items
	}
	if _, err := app.Runner.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if ns := notifications(bobToken); len(ns) != 1 || ns[0].Type != clientapi.NotificationTypeAnnouncement || ns[0].Account != nil ||
		ns[0].Announcement == nil || ns[0].Announcement.Id != draft.Id || ns[0].Announcement.Title != "Maintenance on Sunday" {
		t.Errorf("announcement notifications of bob = %+v, want one", ns)
	}
	history := func(token, cursor string) clientapi.AnnouncementPage {
		t.Helper()
		var page clientapi.AnnouncementPage
		path := "/announcements/history?limit=1"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		if code := api(token, http.MethodGet, path, nil, &page); code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, code)
		}
		return page
	}
	if page := history(bobToken, ""); len(page.Items) != 1 || page.Items[0].Id != draft.Id || page.NextCursor != nil {
		t.Errorf("GET /announcements/history = %+v, want the published announcement", page)
	}
	var read clientapi.Announcement
	if code := api(bobToken, http.MethodPost, "/announcements/"+draft.Id+"/read", nil, &read); code != http.StatusOK || !read.Read {
		t.Errorf("POST read = %d %+v", code, read)
	}
	if got := list(bobToken); len(got) != 1 || !got[0].Read {
		t.Errorf("GET /announcements after reading = %+v, want it read", got)
	}
	if got := list(adminToken); len(got) != 1 || got[0].Read {
		t.Errorf("GET /announcements of another user = %+v, want it unread", got)
	}

	var updated clientapi.AdminAnnouncement
	if code := api(adminToken, http.MethodPut, "/admin/announcements/"+draft.Id, map[string]any{"text": "Maintenance on Monday."}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("PUT without a title = %d, want 422", code)
	}
	if code := api(adminToken, http.MethodPut, "/admin/announcements/"+draft.Id, map[string]any{"title": "Maintenance on Monday", "text": "Maintenance on Monday."}, &updated); code != http.StatusOK ||
		updated.State != clientapi.AdminAnnouncementStatePublished || updated.Text != "Maintenance on Monday." || updated.Title != "Maintenance on Monday" {
		t.Errorf("PUT = %d %+v", code, updated)
	}
	next("announcement")
	if ns := notifications(bobToken); len(ns) != 1 {
		t.Errorf("announcement notifications of bob after the revision = %+v, want still one", ns)
	}

	var withdrawn clientapi.AdminAnnouncement
	if code := api(adminToken, http.MethodPost, "/admin/announcements/"+draft.Id+"/withdraw", nil, &withdrawn); code != http.StatusOK ||
		withdrawn.State != clientapi.AdminAnnouncementStateWithdrawn {
		t.Errorf("POST withdraw = %d %+v", code, withdrawn)
	}
	if data := next("announcement_removed"); !strings.Contains(string(data), draft.Id) {
		t.Errorf("announcement_removed = %s", data)
	}
	if got := list(bobToken); len(got) != 0 {
		t.Errorf("GET /announcements after the withdrawal = %+v, want none", got)
	}
	if ns := notifications(bobToken); len(ns) != 0 {
		t.Errorf("announcement notifications of bob after the withdrawal = %+v, want none", ns)
	}
	if page := history(bobToken, ""); len(page.Items) != 0 {
		t.Errorf("GET /announcements/history after the withdrawal = %+v, want none", page)
	}
	if code := api(bobToken, http.MethodPost, "/announcements/"+draft.Id+"/read", nil, nil); code != http.StatusNotFound {
		t.Errorf("POST read of a withdrawn announcement = %d, want 404", code)
	}
	if code := api(adminToken, http.MethodPost, "/admin/announcements/"+draft.Id+"/publish", nil, nil); code != http.StatusConflict {
		t.Errorf("POST publish of a withdrawn announcement = %d, want 409", code)
	}

	var all clientapi.AdminAnnouncementList
	if code := api(adminToken, http.MethodGet, "/admin/announcements", nil, &all); code != http.StatusOK || len(all.Items) != 1 {
		t.Errorf("GET /admin/announcements = %d %+v", code, all)
	}
	var audit clientapi.AdminAuditEntryPage
	if code := api(adminToken, http.MethodGet, "/admin/audit_log?target_kind=announcement&target_id="+draft.Id, nil, &audit); code != http.StatusOK || len(audit.Items) != 4 ||
		audit.Items[0].Action != clientapi.AdminAuditActionAnnouncementWithdrawn ||
		audit.Items[1].Action != clientapi.AdminAuditActionAnnouncementUpdated ||
		audit.Items[2].Action != clientapi.AdminAuditActionAnnouncementPublished ||
		audit.Items[3].Action != clientapi.AdminAuditActionAnnouncementCreated || audit.Items[3].TargetLabel != "Maintenance on Sunday" {
		t.Errorf("GET the audit log = %d %+v", code, audit)
	}
}
