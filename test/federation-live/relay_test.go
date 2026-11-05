// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// The relay scenario's users and relay (compose.yaml, scripts/federation-live.sh): ivan@my.test and
// judy@mastodon.test follow nobody and nobody follows them, so their posts reach the other server
// only through the relay, which Mastodon joined when the script created its users.
const (
	relayInbox   = "https://relay.test/inbox"
	relayActor   = "https://relay.test/actor"
	relayUser    = "ivan"
	mastodonJudy = "judy"
)

// adminRelay is a relay as the admin API of this server presents it.
type adminRelay struct {
	ID       string `json:"id"`
	Protocol string `json:"protocol"`
	URL      string `json:"url"`
	State    string `json:"state"`
	ActorURI string `json:"actor_uri"`
	Taken    int    `json:"taken_last_day"`
}

// TestRelay joins Activity-Relay at its inbox (ADR-0006): alice registers it through the admin API
// and the relay accepts the instance actor's Follow; a public post of ivan reaches Mastodon through
// the relay, which Mastodon takes only with ivan's linked data signature (RsaSignature2017); a
// public post of judy@mastodon.test reaches this server's federated timeline through the relay,
// fetched again from Mastodon; leaving sends the Undo, and the left relay can be deleted.
func TestRelay(t *testing.T) {
	e := stack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	transport := e.http.Transport
	ivan, err := signInLocalAs(ctx, transport, relayUser, os.Getenv("FEDERATION_LIVE_MY_PASSWORD"))
	if err != nil {
		t.Fatalf("signing in as %s: %v", relayUser, err)
	}
	judy := &mastodonAPI{host: mastodonHost, token: os.Getenv("FEDERATION_LIVE_MASTODON_TOKEN_JUDY"), http: e.http}

	var r adminRelay
	if err := e.local.call(ctx, http.MethodPost, "/admin/relays", map[string]any{"url": relayInbox, "protocol": "mastodon"}, &r); err != nil {
		t.Fatalf("registering the relay: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = e.local.call(ctx, http.MethodDelete, "/admin/relays/"+r.ID+"/follow", nil, nil)
		_ = e.local.call(ctx, http.MethodDelete, "/admin/relays/"+r.ID, nil, nil)
	})
	eventually(t, deliveryTimeout, "the relay accepts the Follow", func() (bool, string) {
		var list struct {
			Items []adminRelay `json:"items"`
		}
		if err := e.local.call(ctx, http.MethodGet, "/admin/relays", nil, &list); err != nil {
			return false, err.Error()
		}
		for _, it := range list.Items {
			if it.ID == r.ID {
				return it.State == "accepted" && it.ActorURI == relayActor, it.State + " " + it.ActorURI
			}
		}
		return false, "the relay is not listed"
	})

	// From this server to Mastodon: the relay forwards the Create as it is, and Mastodon verifies its
	// linked data signature.
	text := unique("relayed from my.test")
	if _, err := ivan.post(ctx, text, "public"); err != nil {
		t.Fatalf("posting as %s: %v", relayUser, err)
	}
	acct, err := judy.resolve(ctx, relayUser+"@"+localHost)
	if err != nil {
		t.Fatalf("resolving %s on %s: %v", relayUser, mastodonHost, err)
	}
	eventually(t, deliveryTimeout, "Mastodon stores ivan's post that the relay forwarded", func() (bool, string) {
		var statuses []status
		if err := judy.call(ctx, http.MethodGet, "/api/v1/accounts/"+url.PathEscape(acct.ID)+"/statuses", nil, &statuses); err != nil {
			return false, err.Error()
		}
		for _, s := range statuses {
			if strings.Contains(s.Content, text) {
				return true, ""
			}
		}
		return false, "not among ivan's statuses on Mastodon"
	})

	// From Mastodon to this server: the relay forwards judy's Create, signed by itself, and this
	// server fetches the post from Mastodon into its federated timeline.
	theirs := unique("relayed from mastodon.test")
	if _, err := judy.reply(ctx, theirs, "public", ""); err != nil {
		t.Fatalf("posting as %s: %v", mastodonJudy, err)
	}
	eventually(t, deliveryTimeout, "the federated timeline shows judy's post that the relay forwarded", func() (bool, string) {
		var page struct {
			Items []status `json:"items"`
		}
		if err := e.local.call(ctx, http.MethodGet, "/timelines/federated?limit=40", nil, &page); err != nil {
			return false, err.Error()
		}
		for _, s := range page.Items {
			if strings.Contains(s.Content, theirs) {
				return true, ""
			}
		}
		return false, "not on the federated timeline"
	})

	// Leaving sends the Undo; the left relay can be deleted.
	var left adminRelay
	if err := e.local.call(ctx, http.MethodDelete, "/admin/relays/"+r.ID+"/follow", nil, &left); err != nil || left.State != "left" {
		t.Fatalf("leaving the relay = %+v, %v", left, err)
	}
	if err := e.local.call(ctx, http.MethodDelete, "/admin/relays/"+r.ID, nil, nil); err != nil {
		t.Fatalf("deleting the relay: %v", err)
	}
}
