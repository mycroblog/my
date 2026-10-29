// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestAccountSuspensionFederates checks the propagation of a local account's suspension
// through the composition root (federation.md §11.1, ADR-0036): the suspension, its lift, and
// its expiry each record an Update of the actor with its fan-out in the action's transaction,
// and GET of the actor serves the emptied actor marked suspended while the suspension lasts.
func TestAccountSuspensionFederates(t *testing.T) {
	t.Parallel()
	path := newDatabase(t, true)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": path, "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mod := e.user("mod")
	r, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{
		Username: "alice", Password: "correct horse battery staple", DisplayName: "Alice",
	})
	if err != nil {
		t.Fatalf("RegisterLocalUser: %v", err)
	}
	alice := r.AccountID

	// updates counts the Updates of alice's actor and the fan-outs enqueued with them.
	updates := func() (activities, fanOuts int) {
		t.Helper()
		if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM outbound_activities WHERE object_type = 'account' AND object_id = ? AND type = 'Update'`, alice).Scan(&activities); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM jobs
			WHERE type = 'federation.fanout' AND CAST(payload AS TEXT) LIKE '%"shared_inboxes":true%'`).Scan(&fanOuts); err != nil {
			t.Fatal(err)
		}
		return activities, fanOuts
	}
	wantUpdates := func(what string, n int) {
		t.Helper()
		if a, f := updates(); a != n || f != n {
			t.Errorf("%s: %d Updates of the actor and %d fan-outs, want %d of each", what, a, f, n)
		}
	}
	actor := func() map[string]any {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/ap/actors/"+alice, nil)
		req.Header.Set("Accept", "application/activity+json")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		var doc map[string]any
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &doc) != nil {
			t.Fatalf("GET of the actor = %d %s", rec.Code, rec.Body)
		}
		return doc
	}
	issue := func(body map[string]any) clientapi.AdminAccountAction {
		t.Helper()
		var a clientapi.AdminAccountAction
		e.want(e.do(http.MethodPost, "/admin/accounts/"+alice+"/actions", mod.token, body, &a), http.StatusOK, "issue")
		return a
	}

	wantUpdates("before the suspension", 0)
	if doc := actor(); doc["name"] != "Alice" || doc["suspended"] != nil {
		t.Errorf("actor before the suspension = %v", doc)
	}

	a := issue(map[string]any{"kind": "suspend"})
	wantUpdates("after the suspension", 1)
	if doc := actor(); doc["name"] != "" || doc["suspended"] != true || doc["publicKey"] == nil {
		t.Errorf("actor while suspended = %v, want the emptied actor marked suspended with its key", doc)
	}
	// Another kind of action sends nothing.
	issue(map[string]any{"kind": "limit"})
	wantUpdates("after a limit", 1)

	e.want(e.do(http.MethodPost, "/admin/account_actions/"+a.Id+"/lift", mod.token, map[string]any{"reason": "appealed"}, nil), http.StatusOK, "lift")
	wantUpdates("after the lift", 2)
	if doc := actor(); doc["name"] != "Alice" || doc["suspended"] != nil {
		t.Errorf("actor after the lift = %v, want the profile", doc)
	}

	// An expired suspension is told by the expiry job. The test moves the end of the suspension
	// into the past instead of sleeping past it: the server compares wall-clock times, which a
	// clock adjustment (NTP, the WSL2 host sync) can move back across a short sleep.
	a = issue(map[string]any{"kind": "suspend", "expires_in": 3600})
	wantUpdates("after the second suspension", 3)
	if doc := actor(); doc["suspended"] != true {
		t.Errorf("actor while the expiring suspension lasts = %v, want it marked suspended", doc)
	}
	past := time.Now().Add(-time.Hour).UnixMilli()
	if _, err := db.ExecContext(t.Context(), `UPDATE account_restrictions SET effective_until = ? WHERE source_action_id = ?`, past, a.Id); err != nil {
		t.Fatal(err)
	}
	if doc := actor(); doc["name"] != "Alice" || doc["suspended"] != nil {
		t.Errorf("actor after the expiry = %v, want the profile", doc)
	}
	if _, err := app.Moderation.ExpireAccountRestrictions(t.Context()); err != nil {
		t.Fatalf("ExpireAccountRestrictions: %v", err)
	}
	wantUpdates("after the expiry", 4)
}
