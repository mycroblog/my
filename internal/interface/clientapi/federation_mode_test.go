// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// followGraph follows any account at once, and reports the accounts in following as followed
// already.
type followGraph struct {
	fakeGraph
	following map[string]bool
	follows   *[]string
}

func (g followGraph) Relationship(_ context.Context, _, targetID string) (appgraph.Relationship, error) {
	r := appgraph.Relationship{TargetAccountID: targetID}
	if g.following[targetID] {
		r.Following = graph.StateActive
	}
	return r, nil
}

func (g followGraph) Follow(_ context.Context, _, followeeID string) (appgraph.Relationship, error) {
	*g.follows = append(*g.follows, followeeID)
	return appgraph.Relationship{TargetAccountID: followeeID, Following: graph.StateActive}, nil
}

func (g followGraph) FollowWith(ctx context.Context, followerID, followeeID string, _ appgraph.FollowOptions) (appgraph.Relationship, error) {
	return g.Follow(ctx, followerID, followeeID)
}

// TestFederationModeRestrictsRemoteOperations checks that, in each federation mode, following
// a remote account, resolving one, and quoting a remote status pass or are the 403 of
// federation-restricted as ADR-0076 says, and that a refused resolution fetches nothing.
func TestFederationModeRestrictsRemoteOperations(t *testing.T) {
	const remoteQuoted = "01890a5d-ac96-774b-bcce-b302099a8300"
	for _, tt := range []struct {
		mode                  string
		followsRemote, quotes bool
		wantInstanceMode      clientapi.InstanceFederationMode
	}{
		{"off", false, false, clientapi.InstanceFederationModeOff},
		{"inbound", true, false, clientapi.InstanceFederationModeInbound},
		{"outbound", false, true, clientapi.InstanceFederationModeOutbound},
		{"both", true, true, clientapi.InstanceFederationModeBoth},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			resolver := &fakeResolver{}
			var follows []string
			var created []string
			statuses := &fakeStatuses{
				create: func(in appcontent.CreateInput) (appcontent.Post, error) {
					created = append(created, in.QuotedStatusID)
					return appcontent.Post{}, errNotFound
				},
				view: func(v appcontent.Viewer, id string) (appcontent.StatusView, error) {
					view := sampleView(v)
					view.Status.ID = id
					view.Status.IsLocal = id != remoteQuoted
					return view, nil
				},
			}
			var accounts fakeAccounts
			e := newAuthEnv(t, func(o *clientapi.Options) {
				o.Instance = &fakeInstance{settings: []instance.CurrentSetting{
					{Key: instance.SettingFederationMode, RevisionID: testkit.NamedID("revision-1"), Value: tt.mode},
				}}
				o.Resolver = resolver
				accounts = o.Accounts.(fakeAccounts)
				o.Graph = followGraph{following: map[string]bool{}, follows: &follows}
				o.Statuses, o.StatusReader, o.Interactions = statuses, statuses, statuses
			})
			restricted := func(t *testing.T, what string, res *http.Response, allowed bool) {
				t.Helper()
				if allowed {
					if res.StatusCode == http.StatusForbidden {
						t.Errorf("%s = 403, want it to pass", what)
					}
					return
				}
				if res.StatusCode != http.StatusForbidden {
					t.Fatalf("%s = %d, want 403", what, res.StatusCode)
				}
				if p := decodeProblem(t, res); p.Type != clientapi.ProblemTypeFederationRestricted || p.Title == "" {
					t.Errorf("%s: problem = %+v", what, p)
				}
			}

			// GET /instance tells the mode.
			if got := decode[clientapi.Instance](t, e.get(t, "/instance", nil)); got.Federation.Mode != tt.wantInstanceMode {
				t.Errorf("instance federation mode = %q, want %q", got.Federation.Mode, tt.wantInstanceMode)
			}

			// Resolving an unknown remote account by handle or URL.
			for _, acct := range []string{"bob@remote.example", "https://remote.example/users/bob"} {
				q := url.Values{"acct": {acct}, "resolve": {"true"}}
				res := e.get(t, "/accounts/lookup?"+q.Encode(), map[string]string{"Authorization": "Bearer read-accounts"})
				restricted(t, "resolve "+acct, res, tt.followsRemote)
			}
			if got := resolver.calls(); tt.followsRemote != (len(got) == 2) {
				t.Errorf("resolver asked %v", got)
			}
			// A local account is found in every mode.
			q := url.Values{"acct": {"alice"}, "resolve": {"true"}}
			if res := e.get(t, "/accounts/lookup?"+q.Encode(), map[string]string{"Authorization": "Bearer read-accounts"}); res.StatusCode != http.StatusOK {
				t.Errorf("lookup of a local account = %d, want 200", res.StatusCode)
			}

			// Following the remote account, which the instance knows now.
			accounts[remoteAccountID] = remoteBob
			// A remote account that the instance knows is a local result, found in every mode.
			q = url.Values{"acct": {"bob@remote.example"}, "resolve": {"true"}}
			if res := e.get(t, "/accounts/lookup?"+q.Encode(), map[string]string{"Authorization": "Bearer read-accounts"}); res.StatusCode != http.StatusOK {
				t.Errorf("lookup of a known remote account = %d, want 200", res.StatusCode)
			}
			res := e.send(t, http.MethodPost, "/accounts/"+remoteAccountID+"/follow", "write", "")
			restricted(t, "follow", res, tt.followsRemote)
			if tt.followsRemote != (len(follows) == 1) {
				t.Errorf("follows = %v", follows)
			}

			// Quoting a remote status; a local status may be quoted in every mode.
			res = e.send(t, http.MethodPost, "/statuses", "write", `{"text":"look","visibility":"public","quoted_status_id":"`+remoteQuoted+`"}`)
			restricted(t, "quote of a remote status", res, tt.quotes)
			res = e.send(t, http.MethodPost, "/statuses", "write", `{"text":"look","visibility":"public","quoted_status_id":"`+sampleStatusID+`"}`)
			restricted(t, "quote of a local status", res, true)
			want := 1
			if tt.quotes {
				want = 2
			}
			if len(created) != want {
				t.Errorf("created quotes of %v, want %d", created, want)
			}
		})
	}
}

// TestFederationModeKeepsRemoteFollowPreferences checks that a follow of a remote account that
// exists already may still change its preferences while the mode does not follow remote
// accounts, since nothing goes out for that.
func TestFederationModeKeepsRemoteFollowPreferences(t *testing.T) {
	var follows []string
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Instance = &fakeInstance{settings: []instance.CurrentSetting{
			{Key: instance.SettingFederationMode, RevisionID: testkit.NamedID("revision-1"), Value: "off"},
		}}
		o.Accounts.(fakeAccounts)[remoteAccountID] = remoteBob
		o.Graph = followGraph{following: map[string]bool{remoteAccountID: true}, follows: &follows}
	})
	if res := e.send(t, http.MethodPost, "/accounts/"+remoteAccountID+"/follow", "write", ""); res.StatusCode != http.StatusOK {
		t.Errorf("follow of a followed remote account = %d, want 200", res.StatusCode)
	}
}
