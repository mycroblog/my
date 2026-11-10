// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpsig"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
)

// TestFederationAllowlist checks the allowlist of federation through the composition root
// (ADR-0159): under open every domain without an action federates; switching federation_scope to
// allowlist refuses the POSTs of the domains it does not allow before their signatures are checked,
// fetches nothing from them, ends the follows with their accounts, requires a signature for the
// ActivityPub GETs, and tells NodeInfo, while an allowed domain still federates; switching back to
// open lets the other domains in again.
func TestFederationAllowlist(t *testing.T) {
	t.Parallel()
	const (
		bob   = "https://friend.example/users/bob"
		carol = "https://stranger.example/users/carol"
	)
	keys := map[string]*rsa.PrivateKey{}
	for _, actor := range []string{bob, carol} {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		keys[actor] = k
	}
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_QUEUE": "inline"})
	var (
		app     *platform.App
		aliceID string
	)
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		app = a
		ctx := t.Context()
		reg, err := a.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: "alice", Password: "correct horse battery staple", DisplayName: "Alice",
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser: %v", err)
		}
		aliceID = reg.AccountID
		for _, actor := range []string{bob, carol} {
			err = a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				u, err := url.Parse(actor)
				if err != nil {
					return err
				}
				_, err = a.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
					URI: actor, Domain: u.Hostname(), Type: "Person", PreferredUsername: "x",
					InboxURL: actor + "/inbox", Keys: []appaccount.RemoteKeyInput{{KeyID: actor + "#main-key", PublicKey: &keys[actor].PublicKey}},
					Document: []byte(`{"id":"` + actor + `"}`),
				})
				return err
			})
			if err != nil {
				t.Fatalf("UpsertRemoteActor(%s): %v", actor, err)
			}
		}
	})
	ctx := t.Context()
	alice := "https://" + cfg.Host + "/ap/actors/" + aliceID
	post := func(t *testing.T, actor string, body []byte) int {
		t.Helper()
		req := &kernel.HTTPRequest{
			Method: http.MethodPost, URL: "https://" + cfg.Host + "/ap/inbox", Body: body,
			Header: http.Header{"Date": {time.Now().UTC().Format(http.TimeFormat)}, "Host": {cfg.Host}},
		}
		err := httpsig.New().Sign(req, appfederation.SigningKey{KeyID: actor + "#main-key", PrivateKey: keys[actor]}, appfederation.SignOptions{
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
		return resp.StatusCode
	}
	follow := func(id, actor string) []byte {
		return []byte(`{"@context":"https://www.w3.org/ns/activitystreams","id":"` + id + `","type":"Follow","actor":"` + actor + `","object":"` + alice + `"}`)
	}
	setScope := func(t *testing.T, scope instance.FederationScope) {
		t.Helper()
		if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingFederationScope, Value: string(scope)}); err != nil {
			t.Fatalf("SetSetting(federation_scope, %s): %v", scope, err)
		}
	}
	nodeScope := func(t *testing.T) string {
		t.Helper()
		resp, body := s.do(t, http.MethodGet, "/nodeinfo/2.1", nil)
		var doc struct {
			Metadata struct {
				FederationScope string `json:"federationScope"`
			} `json:"metadata"`
		}
		if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(body), &doc) != nil {
			t.Fatalf("nodeinfo = %d %s", resp.StatusCode, body)
		}
		return doc.Metadata.FederationScope
	}
	outbox := "/ap/actors/" + aliceID + "/outbox"
	ap := map[string]string{"Accept": "application/activity+json"}

	// Under open, every domain federates and the GETs need no signature.
	if got := post(t, carol, follow(carol+"#follow-1", carol)); got != http.StatusAccepted {
		t.Fatalf("POST Follow from stranger.example under open = %d, want 202", got)
	}
	carolAccount, err := app.Accounts.FindByURI(ctx, carol)
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := app.Graph.Relationship(ctx, aliceID, carolAccount.Account.ID); err != nil || rel.FollowedBy != graph.StateActive {
		t.Fatalf("Relationship(alice, carol) = %+v, %v; want followed by carol", rel, err)
	}
	if resp, body := s.do(t, http.MethodGet, outbox, ap); resp.StatusCode != http.StatusOK {
		t.Errorf("unsigned GET of the outbox under open = %d %s, want 200", resp.StatusCode, body)
	}
	if got := nodeScope(t); got != "open" {
		t.Errorf("NodeInfo federationScope = %q, want open", got)
	}

	// Under allowlist, only friend.example federates.
	if _, _, err := app.Moderation.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: "friend.example"}); err != nil {
		t.Fatalf("AllowDomain: %v", err)
	}
	setScope(t, instance.FederationScopeAllowlist)
	if got := post(t, carol, follow(carol+"#follow-2", carol)); got != http.StatusForbidden {
		t.Errorf("POST from stranger.example under allowlist = %d, want 403", got)
	}
	if got := post(t, bob, follow(bob+"#follow-1", bob)); got != http.StatusAccepted {
		t.Errorf("POST from friend.example under allowlist = %d, want 202", got)
	}
	if _, err := app.RemoteActors.Resolve(ctx, "https://other.stranger.example/users/dave"); !errors.Is(err, appfederation.ErrFetchRefused) {
		t.Errorf("Resolve on a domain without an allow = %v, want ErrFetchRefused", err)
	}
	if rel, err := app.Graph.Relationship(ctx, aliceID, carolAccount.Account.ID); err != nil || rel.FollowedBy != graph.StateNone {
		t.Errorf("Relationship(alice, carol) after the switch = %+v, %v; want the follow ended", rel, err)
	}
	if resp, body := s.do(t, http.MethodGet, outbox, ap); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unsigned GET of the outbox under allowlist = %d %s, want 401", resp.StatusCode, body)
	}
	if got := nodeScope(t); got != "allowlist" {
		t.Errorf("NodeInfo federationScope = %q, want allowlist", got)
	}
	impact, err := app.Moderation.FederationScopeImpact(ctx)
	if err != nil || impact.Follows != 0 || impact.Scope != appmoderation.ScopeAllowlist {
		t.Errorf("FederationScopeImpact after the switch = %+v, %v; want nothing left", impact, err)
	}

	// Back to open, the other domains federate again; the follow stays ended.
	setScope(t, instance.FederationScopeOpen)
	if got := post(t, carol, follow(carol+"#follow-3", carol)); got != http.StatusAccepted {
		t.Errorf("POST from stranger.example back under open = %d, want 202", got)
	}
}
