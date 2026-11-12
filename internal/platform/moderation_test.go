// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
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
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/platform"
)

// TestFederationModeration checks the moderation of federation through the composition root
// (federation.md §1): a received Block ends the follows and blocks, a Flag
// files a report unless its domain has reject_reports, and a suspended domain's POSTs get 403 and
// its actors are not fetched.
func TestFederationModeration(t *testing.T) {
	t.Parallel()
	const (
		bob   = "https://remote.example/users/bob"
		carol = "https://bad.example/users/carol"
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
				host := u.Hostname()
				_, err = a.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
					URI: actor, Domain: host, Type: "Person", PreferredUsername: "x",
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
	activity := func(id, typ, actor, object string) []byte {
		return []byte(`{"@context":"https://www.w3.org/ns/activitystreams","id":"` + id + `","type":"` + typ +
			`","actor":"` + actor + `","object":"` + object + `","content":"spam"}`)
	}
	bobAccount, err := app.Accounts.FindByURI(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}

	// A received Block blocks alice; the inline queue processes it before the response.
	if got := post(t, bob, activity(bob+"#block-1", "Block", bob, alice)); got != http.StatusAccepted {
		t.Fatalf("POST Block = %d, want 202", got)
	}
	if rel, err := app.Graph.Relationship(ctx, aliceID, bobAccount.Account.ID); err != nil || !rel.BlockedBy {
		t.Errorf("Relationship(alice, bob) = %+v, %v; want blocked by bob", rel, err)
	}

	// A Flag is a report, until the domain's reports are rejected.
	if got := post(t, bob, activity(bob+"#flag-1", "Flag", bob, alice)); got != http.StatusAccepted {
		t.Fatalf("POST Flag = %d, want 202", got)
	}
	if _, err := app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "remote.example", Kind: moderation.DomainRejectReports}); err != nil {
		t.Fatal(err)
	}
	if got := post(t, bob, activity(bob+"#flag-2", "Flag", bob, alice)); got != http.StatusAccepted {
		t.Fatalf("POST Flag = %d, want 202", got)
	}
	if reports, err := app.Moderation.ReportsOf(ctx, aliceID); err != nil || len(reports) != 1 || reports[0].URI != bob+"#flag-1" {
		t.Errorf("reports of alice = %+v, %v; want only the Flag before reject_reports", reports, err)
	}

	// A suspended domain is refused before its signature is checked, and not fetched from.
	if got := post(t, carol, activity(carol+"#follow-1", "Follow", carol, alice)); got != http.StatusAccepted {
		t.Fatalf("POST Follow before the suspension = %d, want 202", got)
	}
	if _, err := app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "bad.example", Kind: moderation.DomainSuspend}); err != nil {
		t.Fatal(err)
	}
	if got := post(t, carol, activity(carol+"#follow-2", "Follow", carol, alice)); got != http.StatusForbidden {
		t.Errorf("POST from a suspended domain = %d, want 403", got)
	}
	if _, err := app.RemoteActors.Resolve(ctx, "https://sub.bad.example/users/dave"); !errors.Is(err, appfederation.ErrFetchRefused) {
		t.Errorf("Resolve on a suspended domain = %v, want ErrFetchRefused", err)
	}
}
