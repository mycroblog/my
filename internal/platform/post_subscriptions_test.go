// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"slices"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestPostSubscriptionsThroughAPI drives the subscriptions to new posts on SQLite through the
// client API (ADR-0123): a local account without a follow can be subscribed to, a remote one only
// while it is followed (422 otherwise, also while the follow awaits its server, whose acceptance
// then makes the subscription that the follow's notify asked for); the new posts notify the
// subscribers who may see them, so a private post reaches only the subscribers who follow the
// author; unfollowing a remote account ends its subscription, a local one keeps it; and the list
// of subscriptions shows them.
func TestPostSubscriptionsThroughAPI(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	e := &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
	member := func(name string) testUser {
		t.Helper()
		r, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: name, Password: "correct horse battery staple"})
		if err != nil {
			t.Fatal(err)
		}
		return testUser{id: r.AccountID, token: issueTokens(t, app, r.User.ID, "").AccessToken}
	}
	alice, bob, carol := member("alice"), member("bob"), member("carol")
	drain := func() {
		t.Helper()
		if _, err := app.Runner.Drain(ctx); err != nil {
			t.Fatalf("Drain: %v", err)
		}
	}
	statusesOf := func(u testUser) []string {
		t.Helper()
		var page clientapi.NotificationPage
		e.want(e.do(http.MethodGet, "/notifications?types[]=status", u.token, nil, &page), http.StatusOK, "notifications")
		var out []string
		for _, n := range page.Items {
			if n.Type == clientapi.NotificationTypeStatus && n.Status != nil && n.Account != nil && n.Account.Id == alice.id {
				out = append(out, n.Status.Id)
			}
		}
		return out
	}
	post := func(visibility string) string {
		t.Helper()
		var st clientapi.Status
		e.want(e.do(http.MethodPost, "/statuses", alice.token, map[string]any{"text": "news", "visibility": visibility}, &st), http.StatusOK, "post "+visibility)
		drain()
		return st.Id
	}

	// Bob and carol subscribe to alice without following her; bob also follows her.
	if r := e.relate("subscribe", bob, alice, nil); !r.Notify || r.Following {
		t.Errorf("subscribe = %+v, want notify without a follow", r)
	}
	e.relate("subscribe", carol, alice, nil)
	e.relate("follow", bob, alice, nil)
	if p := e.want(e.do(http.MethodPost, "/accounts/"+alice.id+"/subscribe", alice.token, nil, nil), http.StatusUnprocessableEntity, "subscribe to oneself"); p != clientapi.ProblemTypeBlank {
		t.Errorf("subscribe to oneself = %s", p)
	}

	public := post("public")
	private := post("private")
	if got := statusesOf(bob); !slices.Equal(got, []string{private, public}) {
		t.Errorf("status notifications of bob = %v, want the private and the public post", got)
	}
	if got := statusesOf(carol); !slices.Equal(got, []string{public}) {
		t.Errorf("status notifications of carol = %v, want only the public post (she does not follow alice)", got)
	}

	// Unfollowing a local account keeps the subscription; unsubscribing ends it.
	e.relate("unfollow", bob, alice, nil)
	if rs := e.relationships(bob, alice.id); len(rs) != 1 || !rs[0].Notify {
		t.Errorf("relationship after unfollowing = %+v, want notify kept", rs)
	}
	if r := e.relate("unsubscribe", carol, alice, nil); r.Notify {
		t.Errorf("unsubscribe = %+v, want notify off", r)
	}
	var subs clientapi.AccountPage
	e.want(e.do(http.MethodGet, "/subscriptions", bob.token, nil, &subs), http.StatusOK, "subscriptions of bob")
	if len(subs.Accounts) != 1 || subs.Accounts[0].Id != alice.id {
		t.Errorf("subscriptions of bob = %+v, want alice", subs.Accounts)
	}

	// A remote account: 422 without a follow and while the follow awaits its server; the notify of
	// the follow becomes the subscription once the server accepts it, and unfollowing ends it.
	var remote account.Account
	err = app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		actor := "https://remote.example/users/dave"
		var err error
		remote, err = app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: actor, Domain: "remote.example", Type: "Person", PreferredUsername: "dave",
			InboxURL: actor + "/inbox", FollowersURL: actor + "/followers", Document: []byte(`{"id":"` + actor + `"}`),
			Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "dave", Domain: "remote.example"},
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	dave := testUser{id: remote.ID}
	e.want(e.do(http.MethodPost, "/accounts/"+dave.id+"/subscribe", carol.token, nil, nil), http.StatusUnprocessableEntity, "subscribe to a remote account")
	if r := e.relate("follow", carol, dave, map[string]bool{"notify": true}); !r.Requested || r.Notify {
		t.Errorf("follow with notify = %+v, want requested without notify yet", r)
	}
	e.want(e.do(http.MethodPost, "/accounts/"+dave.id+"/subscribe", carol.token, nil, nil), http.StatusUnprocessableEntity, "subscribe while requested")
	if err := app.Graph.AcceptRemote(ctx, appgraph.RemoteResponse{FollowURI: "https://remote.example/unknown", FolloweeID: dave.id, FollowerID: carol.id}); err != nil {
		t.Fatalf("AcceptRemote: %v", err)
	}
	if rs := e.relationships(carol, dave.id); len(rs) != 1 || !rs[0].Following || !rs[0].Notify {
		t.Errorf("relationship after the acceptance = %+v, want following with notify", rs)
	}
	e.relate("unfollow", carol, dave, nil)
	if rs := e.relationships(carol, dave.id); len(rs) != 1 || rs[0].Notify {
		t.Errorf("relationship after unfollowing a remote account = %+v, want notify off", rs)
	}
}
