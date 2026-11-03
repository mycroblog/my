// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform"
)

// TestDeletedLocalActorIsGone deletes a local account on the real database and checks ADR-0144 §1:
// the actor, its WebFinger, and the ActivityPub request of its profile page answer 410 from the
// deleted event on and after the erasure, and another account is not affected.
func TestDeletedLocalActorIsGone(t *testing.T) {
	t.Parallel()
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{Clock: clock}, "alice", "bob")
	ctx := t.Context()
	get := func(target string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Accept", "application/activity+json")
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, req)
		return rec.Result()
	}
	webfinger := "/.well-known/webfinger?resource=acct:alice@example.test"
	want := func(when string, status int) {
		t.Helper()
		for _, target := range []string{"/ap/actors/" + c.ids["alice"], webfinger, "/@alice"} {
			wanted := status
			if status == http.StatusOK && target == "/@alice" {
				wanted = http.StatusFound
			}
			if res := get(target); res.StatusCode != wanted {
				t.Errorf("%s: GET %s = %d, want %d", when, target, res.StatusCode, wanted)
			}
		}
	}
	want("before the deletion", http.StatusOK)

	if err := c.app.Identity.DeleteOwnAccount(ctx, c.userIDs["alice"], identity.Confirmation{Password: "correct horse battery staple"}); err != nil {
		t.Fatalf("DeleteOwnAccount: %v", err)
	}
	want("after the deletion", http.StatusGone)
	res := get("/ap/actors/" + c.ids["alice"])
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["type"] != "Tombstone" || doc["id"] != "https://example.test/ap/actors/"+c.ids["alice"] {
		t.Errorf("the document = %v, want a Tombstone of the actor", doc)
	}

	handler, _, ok := c.app.Handlers.Lookup(appaccount.JobEraseDeletedAccounts)
	if !ok {
		t.Fatalf("no handler of %s", appaccount.JobEraseDeletedAccounts)
	}
	clock.ahead.Store(int64(appaccount.ErasureDelay + time.Minute))
	if res := handler(ctx, queue.Job{Type: appaccount.JobEraseDeletedAccounts}); res.Err() != nil {
		t.Fatalf("erasure: %v", res.Err())
	}
	if _, err := c.app.Runner.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	clock.ahead.Store(0)
	if a, err := c.app.Accounts.FindByID(ctx, c.ids["alice"]); err != nil || a.Profile != nil {
		t.Fatalf("the account after the erasure = %+v, %v; want it without a profile", a, err)
	}
	want("after the erasure", http.StatusGone)

	if res := get("/ap/actors/" + c.ids["bob"]); res.StatusCode != http.StatusOK {
		t.Errorf("GET of bob = %d, want 200", res.StatusCode)
	}
}
