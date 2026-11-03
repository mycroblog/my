// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestClientAPIRemoteAccountActions checks the actions on a remote account through the
// composition root and the client API (FR-703, data-model §5.2, ADR-0036): a suspend drops what
// the account sends and hides its profile and posts, and its lift brings them back; the kinds
// that only a local account takes are a 422; issuing and lifting are audited (ADR-0037), and
// nothing is sent to the account's server.
func TestClientAPIRemoteAccountActions(t *testing.T) {
	t.Parallel()
	const bob = "https://remote.example/users/bob"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := newDatabase(t, true)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": path, "MY_QUEUE": "inline", "MY_WEB_ORIGIN": webOrigin})
	var app *platform.App
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		app = a
		err := a.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			_, err := a.Accounts.UpsertRemoteActor(t.Context(), tx, appaccount.RemoteActorInput{
				URI: bob, Domain: "remote.example", Type: "Person", PreferredUsername: "bob",
				InboxURL: bob + "/inbox", FollowersURL: bob + "/followers",
				Keys:     []appaccount.RemoteKeyInput{{KeyID: bob + "#main-key", PublicKey: &key.PublicKey}},
				Document: []byte(`{"id":"` + bob + `"}`),
			})
			return err
		})
		if err != nil {
			t.Fatalf("UpsertRemoteActor: %v", err)
		}
	})
	ctx := t.Context()
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mod := e.user("mod")
	r, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
		Username: "alice", Password: "correct horse battery staple", DisplayName: "Alice",
	})
	if err != nil {
		t.Fatalf("RegisterLocalUser: %v", err)
	}
	alice := issueTokens(t, app, r.User.ID, "").AccessToken
	actor, err := app.Accounts.FindByURI(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	bobID := actor.Account.ID
	aliceIRI := "https://" + cfg.Host + "/ap/actors/" + r.AccountID

	// create posts a public Note of bob that mentions alice (so that it is in the scope of what
	// the instance stores, ADR-0034) to the shared inbox, signed with bob's key; the inline queue
	// processes it before the inbox answers.
	create := func(n int) string {
		t.Helper()
		note := bob + "/statuses/" + strconv.Itoa(n)
		body, err := json.Marshal(map[string]any{
			"@context": "https://www.w3.org/ns/activitystreams", "id": note + "/activity", "type": "Create", "actor": bob,
			"object": map[string]any{
				"id": note, "type": "Note", "attributedTo": bob, "published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
				"to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "cc": []string{bob + "/followers", aliceIRI},
				"content": `<p>hello <a href="` + aliceIRI + `" class="u-url mention">@alice</a></p>`,
				"tag":     []any{map[string]any{"type": "Mention", "href": aliceIRI, "name": "@alice@" + cfg.Host}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		req := &kernel.HTTPRequest{
			Method: http.MethodPost, URL: "https://" + cfg.Host + "/ap/inbox", Body: body,
			Header: http.Header{"Date": {time.Now().UTC().Format(http.TimeFormat)}, "Host": {cfg.Host}},
		}
		err = httpsig.New().Sign(req, appfederation.SigningKey{KeyID: bob + "#main-key", PrivateKey: key}, appfederation.SignOptions{
			Scheme: appfederation.SchemeCavage, Algorithm: appfederation.AlgRSAPKCS1SHA256,
			Components: []string{"(request-target)", "host", "date", "digest"},
		})
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		hr, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url+"/ap/inbox", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range req.Header {
			if k != "Host" {
				hr.Header[k] = v
			}
		}
		hr.Header.Set("Content-Type", "application/activity+json")
		resp, err := http.DefaultTransport.RoundTrip(hr)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("POST Create %d = %d, want 202", n, resp.StatusCode)
		}
		return note
	}
	stored := func(note string) (string, bool) {
		t.Helper()
		st, err := app.Content.FindStatusByURI(ctx, note)
		if errors.Is(err, appcontent.ErrNotFound) {
			return "", false
		}
		if err != nil {
			t.Fatalf("FindStatusByURI(%s): %v", note, err)
		}
		return st.ID, true
	}
	// shown checks what alice sees of bob: his profile, his post, and his posts.
	shown := func(what string, statusID string, want bool) {
		t.Helper()
		status := http.StatusNotFound
		if want {
			status = http.StatusOK
		}
		e.want(e.do(http.MethodGet, "/accounts/"+bobID, alice, nil, nil), status, what+": bob's profile")
		e.want(e.do(http.MethodGet, "/statuses/"+statusID, alice, nil, nil), status, what+": bob's post")
		var page clientapi.StatusPage
		res := e.do(http.MethodGet, "/accounts/"+bobID+"/statuses", alice, nil, &page)
		if want && (res.StatusCode != http.StatusOK || len(page.Items) == 0) {
			t.Errorf("%s: bob's posts = %d %+v, want them", what, res.StatusCode, page.Items)
		}
		if !want && res.StatusCode == http.StatusOK && len(page.Items) > 0 {
			t.Errorf("%s: bob's posts = %+v, want none", what, page.Items)
		}
	}
	outbound := func() int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbound_activities`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	first, ok := stored(create(1))
	if !ok {
		t.Fatal("bob's first post was not stored")
	}
	shown("before the suspension", first, true)
	sent := outbound()

	// The kinds that only a local account takes are a 422, and record nothing.
	for _, body := range []map[string]any{
		{"kind": "warn"}, {"kind": "restrict_posting"}, {"kind": "restrict_interaction"}, {"kind": "disable_login"}, {"kind": "ban"},
		{"kind": "delete_statuses", "status_ids": []string{first}}, {"kind": "mark_statuses_sensitive", "status_ids": []string{first}},
	} {
		e.want(e.do(http.MethodPost, "/admin/accounts/"+bobID+"/actions", mod.token, body, nil), http.StatusUnprocessableEntity, "remote "+body["kind"].(string))
	}

	// suspend: what bob sends is dropped, and his profile and posts are hidden.
	var a clientapi.AdminAccountAction
	e.want(e.do(http.MethodPost, "/admin/accounts/"+bobID+"/actions", mod.token, map[string]any{"kind": "suspend", "reason_category": "spam"}, &a), http.StatusOK, "suspend bob")
	if a.AccountId != bobID || a.Kind != clientapi.AdminAccountActionKindSuspend || a.State != clientapi.ModerationActionStateActive || a.NotifyTarget {
		t.Errorf("suspension = %+v, want an active suspend of bob without a notice", a)
	}
	if _, ok := stored(create(2)); ok {
		t.Error("a post of the suspended bob was stored")
	}
	shown("while suspended", first, false)

	// The lift brings the profile and the posts back, and bob's posts are received again.
	e.want(e.do(http.MethodPost, "/admin/account_actions/"+a.Id+"/lift", mod.token, map[string]any{"reason": "mistake"}, &a), http.StatusOK, "lift")
	if a.State != clientapi.ModerationActionStateLifted {
		t.Errorf("lifted = %+v", a)
	}
	shown("after the lift", first, true)
	if _, ok := stored(create(3)); !ok {
		t.Error("bob's post after the lift was not stored")
	}

	// limit and force_sensitive are taken too.
	for _, kind := range []string{"limit", "force_sensitive"} {
		e.want(e.do(http.MethodPost, "/admin/accounts/"+bobID+"/actions", mod.token, map[string]any{"kind": kind, "expires_in": 3600}, nil), http.StatusOK, "remote "+kind)
	}

	// The actions are local: nothing is sent to bob's server.
	if n := outbound(); n != sent {
		t.Errorf("outbound activities = %d after the actions on bob, want %d", n, sent)
	}

	// Each issue and the lift are audited, with the moderator; the refused kinds are not.
	entries, err := app.Moderation.AuditLog(ctx, appmoderation.AuditQuery{TargetKind: moderation.AuditTargetAccount, TargetID: bobID}, 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []moderation.AuditAction
	for _, en := range entries {
		got = append(got, en.Action)
		if en.ActorKind != moderation.AuditActorUser || en.ActorUserID == "" || (en.TargetLabel != "@bob@remote.example" && en.TargetLabel != bob) {
			t.Errorf("audit entry = %+v, want by the moderator on bob", en)
		}
	}
	want := []moderation.AuditAction{
		moderation.AuditAccountActionIssued, moderation.AuditAccountActionIssued, moderation.AuditAccountActionLifted, moderation.AuditAccountActionIssued,
	}
	if !slices.Equal(got, want) {
		t.Errorf("audit log of bob = %v, want %v", got, want)
	}
}
