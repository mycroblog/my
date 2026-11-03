// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestClientAPIActionsOnStatusesAndBans checks, through the composition root and the client API,
// the actions that do more than restrict (FR-703, data-model §5.2): force_sensitive and
// mark_statuses_sensitive show the media as sensitive until they are lifted, delete_statuses
// deletes the posts it names for everyone, their author included, and a ban hides the account,
// schedules its deletion, and is never lifted.
func TestClientAPIActionsOnStatusesAndBans(t *testing.T) {
	t.Parallel()
	// The inline queue processes the upload without a worker.
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	mod := e.user("mod")
	member := func(name string) testUser {
		t.Helper()
		r, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{
			Username: name, Password: "correct horse battery staple", DisplayName: name,
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser(%s): %v", name, err)
		}
		return testUser{id: r.AccountID, token: issueTokens(t, app, r.User.ID, "").AccessToken}
	}
	alice, bob := member("alice"), member("bob")
	issue := func(kind string, statusIDs ...string) clientapi.AdminAccountAction {
		t.Helper()
		body := map[string]any{"kind": kind}
		if len(statusIDs) > 0 {
			body["status_ids"] = statusIDs
		}
		var a clientapi.AdminAccountAction
		e.want(e.do(http.MethodPost, "/admin/accounts/"+alice.id+"/actions", mod.token, body, &a), http.StatusCreated, "issue "+kind)
		return a
	}
	lift := func(a clientapi.AdminAccountAction) {
		t.Helper()
		e.want(e.do(http.MethodPost, "/admin/account_actions/"+a.Id+"/lift", mod.token, nil, nil), http.StatusOK, "lift "+string(a.Kind))
	}
	sensitive := func(who testUser, id string, want bool, when string) {
		t.Helper()
		var st clientapi.Status
		e.want(e.do(http.MethodGet, "/statuses/"+id, who.token, nil, &st), http.StatusOK, "get the status "+when)
		if st.Sensitive != want || len(st.MediaAttachments) != 1 {
			t.Errorf("%s: sensitive = %v with %d attachments, want %v", when, st.Sensitive, len(st.MediaAttachments), want)
		}
	}

	media := uploadImage(t, e, alice)
	var pictured, spam clientapi.Status
	e.want(e.do(http.MethodPost, "/statuses", alice.token, map[string]any{
		"text": "a picture", "visibility": "public", "media": []map[string]any{{"id": media}},
	}, &pictured), http.StatusCreated, "post with media")
	e.want(e.do(http.MethodPost, "/statuses", alice.token, map[string]any{"text": "spam", "visibility": "public"}, &spam), http.StatusCreated, "post spam")
	sensitive(bob, pictured.Id, false, "before any action")

	// force_sensitive shows every attachment of the account as sensitive until it is lifted.
	a := issue("force_sensitive")
	sensitive(bob, pictured.Id, true, "under force_sensitive")
	sensitive(alice, pictured.Id, true, "under force_sensitive, to the author")
	lift(a)
	sensitive(bob, pictured.Id, false, "after force_sensitive is lifted")

	// mark_statuses_sensitive shows the statuses it names as sensitive until it is lifted.
	a = issue("mark_statuses_sensitive", pictured.Id)
	if a.StatusIds == nil || len(*a.StatusIds) != 1 || (*a.StatusIds)[0] != pictured.Id {
		t.Errorf("mark_statuses_sensitive = %+v, want the status", a)
	}
	sensitive(bob, pictured.Id, true, "marked")
	lift(a)
	sensitive(bob, pictured.Id, false, "after the mark is lifted")

	// delete_statuses deletes the status for everyone, the author included, and for good.
	a = issue("delete_statuses", spam.Id)
	for _, who := range []testUser{alice, bob} {
		e.want(e.do(http.MethodGet, "/statuses/"+spam.Id, who.token, nil, nil), http.StatusNotFound, "the deleted status")
	}
	lift(a)
	e.want(e.do(http.MethodGet, "/statuses/"+spam.Id, alice.token, nil, nil), http.StatusNotFound, "the deleted status after the lift")
	e.want(e.do(http.MethodPost, "/admin/accounts/"+alice.id+"/actions", mod.token,
		map[string]any{"kind": "delete_statuses", "status_ids": []string{spam.Id}}, nil), http.StatusUnprocessableEntity, "delete a deleted status")
	e.want(e.do(http.MethodPost, "/admin/accounts/"+bob.id+"/actions", mod.token,
		map[string]any{"kind": "delete_statuses", "status_ids": []string{pictured.Id}}, nil), http.StatusUnprocessableEntity, "delete another account's status")

	// A ban hides the account, schedules its deletion, and is never lifted.
	ban := issue("ban")
	if typ := e.want(e.do(http.MethodGet, "/accounts/verify_credentials", alice.token, nil, nil), http.StatusUnauthorized, "alice while banned"); typ != clientapi.ProblemTypeInvalidToken {
		t.Errorf("alice's token while banned = %s, want invalid-token", typ)
	}
	e.want(e.do(http.MethodGet, "/accounts/"+alice.id, bob.token, nil, nil), http.StatusNotFound, "alice's profile while banned")
	e.want(e.do(http.MethodPost, "/admin/account_actions/"+ban.Id+"/lift", mod.token, nil, nil), http.StatusUnprocessableEntity, "lift the ban")
	actor, err := app.Accounts.FindByID(t.Context(), alice.id)
	if err != nil {
		t.Fatal(err)
	}
	if s := actor.Lifecycle; s.State != account.StateDeletionScheduled || s.DeletionDueAt == nil ||
		!s.DeletionDueAt.Equal(ban.IssuedAt.Add(moderation.BanDeletionDelay)) {
		t.Errorf("lifecycle after the ban = %+v, want the deletion due %v", s, ban.IssuedAt.Add(moderation.BanDeletionDelay))
	}
	if n, err := app.Accounts.ExecuteDueDeletions(t.Context()); err != nil || n != 0 {
		t.Errorf("ExecuteDueDeletions before the due time = %d, %v; want none", n, err)
	}
}

// uploadImage uploads a JPEG as who and waits until it is processed; it returns the media ID.
func uploadImage(t *testing.T, e *accountsEnv, who testUser) string {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="photo.jpg"`)
	hdr.Set("Content-Type", "image/jpeg")
	part, err := w.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(testkit.FixtureJPEG(64, 48, 6))
	_ = w.Close()
	send := func(method, path, contentType string, b []byte) (int, clientapi.MediaAttachment) {
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+who.token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res := e.spec.Serve(t, e.h, req)
		var m clientapi.MediaAttachment
		if res.StatusCode < 300 {
			if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
				t.Fatal(err)
			}
		}
		return res.StatusCode, m
	}
	code, m := send(http.MethodPost, "/media", w.FormDataContentType(), body.Bytes())
	if code != http.StatusAccepted {
		t.Fatalf("upload = %d", code)
	}
	for deadline := time.Now().Add(10 * time.Second); m.State == "processing" && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		_, m = send(http.MethodGet, "/media/"+m.Id, "", nil)
	}
	if m.State != "ready" {
		t.Fatalf("processed = %+v", m)
	}
	return m.Id
}
