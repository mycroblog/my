// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// The LitePub relay scenario's relay and user (compose.yaml, scripts/federation-live.sh): Akkoma's
// relay actor, which announces every public post of kate@akkoma.test to the servers that follow it.
// Nobody on this server follows kate and nobody on Akkoma follows ivan, so their posts reach the
// other server only through the relay.
const (
	litePubRelayActor = "https://" + akkomaHost + "/relay"
	akkomaUser        = "kate"
)

// TestRelayLitePub joins Akkoma's relay at its actor (ADR-0061): alice registers its URL with no
// protocol, which this server detects as litepub from the actor it fetches, and the relay actor
// accepts the instance actor's Follow. Akkoma's relay then follows the instance actor (pleroma_ctl
// relay follow), and this server accepts that Follow as the relay's. A public post of kate reaches
// this server's federated timeline as the relay's Announce, fetched again from Akkoma; a public post
// of ivan, delivered to the relay's inbox with its linked data signature, reaches Akkoma's federated
// timeline. Leaving sends the Undo, after which kate's posts no longer arrive, and the left relay can
// be deleted.
func TestRelayLitePub(t *testing.T) {
	e := stack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ivan, err := signInLocalAs(ctx, e.http.Transport, relayUser, os.Getenv("FEDERATION_LIVE_MY_PASSWORD"))
	if err != nil {
		t.Fatalf("signing in as %s: %v", relayUser, err)
	}
	kate := &mastodonAPI{host: akkomaHost, token: os.Getenv("FEDERATION_LIVE_AKKOMA_TOKEN"), http: e.http}

	var r adminRelay
	if err := e.local.call(ctx, http.MethodPost, "/admin/relays", map[string]any{"url": litePubRelayActor}, &r); err != nil {
		t.Fatalf("registering the relay: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = e.local.call(ctx, http.MethodPost, "/admin/relays/"+r.ID+"/unfollow", nil, nil)
		_ = e.local.call(ctx, http.MethodDelete, "/admin/relays/"+r.ID, nil, nil)
	})
	if r.Protocol != "litepub" {
		t.Fatalf("the relay registered with no protocol = %q, want litepub", r.Protocol)
	}
	eventually(t, deliveryTimeout, "the relay accepts the Follow", func() (bool, string) {
		got, err := e.adminRelay(ctx, r.ID)
		if err != nil {
			return false, err.Error()
		}
		return got.State == "accepted" && got.ActorURI == litePubRelayActor, got.State + " " + got.ActorURI
	})

	// The relay follows the instance actor back, and this server accepts the Follow of the actor of a
	// relay it joined. The instance actor, which signed the Follow, exists by now.
	instanceActor := e.instanceActor(t)
	t.Cleanup(func() { _, _ = akkomaCtl(context.Background(), "relay", "unfollow", instanceActor) })
	if out, err := akkomaCtl(ctx, "relay", "follow", instanceActor); err != nil {
		t.Fatalf("pleroma_ctl relay follow: %v: %s", err, out)
	}
	eventually(t, deliveryTimeout, "Akkoma's relay has the Accept of its Follow of the instance actor", func() (bool, string) {
		out, err := akkomaCtl(ctx, "relay", "list")
		if err != nil {
			return false, err.Error() + ": " + out
		}
		for line := range strings.SplitSeq(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), instanceActor) {
				return !strings.Contains(line, "no Accept received"), line
			}
		}
		return false, "not listed: " + out
	})

	// From Akkoma to this server: the relay actor announces kate's post, and this server takes the
	// post in from Akkoma, not as a boost.
	theirs := unique("relayed from akkoma.test")
	if _, err := kate.reply(ctx, theirs, "public", ""); err != nil {
		t.Fatalf("posting as %s: %v", akkomaUser, err)
	}
	eventually(t, deliveryTimeout, "the federated timeline shows kate's post that the relay announced", func() (bool, string) {
		return e.onFederatedTimeline(ctx, theirs)
	})

	// From this server to Akkoma: the relay's inbox takes ivan's Create, which carries his linked
	// data signature, and Akkoma stores the post.
	text := unique("relayed to akkoma.test")
	if _, err := ivan.post(ctx, text, "public"); err != nil {
		t.Fatalf("posting as %s: %v", relayUser, err)
	}
	eventually(t, deliveryTimeout, "Akkoma's federated timeline shows ivan's post", func() (bool, string) {
		var statuses []status
		if err := kate.call(ctx, http.MethodGet, "/api/v1/timelines/public?remote=true&limit=40", nil, &statuses); err != nil {
			return false, err.Error()
		}
		for _, s := range statuses {
			if strings.Contains(s.Content, text) {
				return true, ""
			}
		}
		return false, "not on Akkoma's federated timeline"
	})

	// Leaving sends the Undo: Akkoma's relay stops announcing to this server.
	var left adminRelay
	if err := e.local.call(ctx, http.MethodPost, "/admin/relays/"+r.ID+"/unfollow", nil, &left); err != nil || left.State != "left" {
		t.Fatalf("leaving the relay = %+v, %v", left, err)
	}
	time.Sleep(quietPeriod) // the Undo is delivered before kate posts again
	after := unique("announced after leaving akkoma.test")
	if _, err := kate.reply(ctx, after, "public", ""); err != nil {
		t.Fatalf("posting as %s: %v", akkomaUser, err)
	}
	time.Sleep(quietPeriod)
	if ok, _ := e.onFederatedTimeline(ctx, after); ok {
		t.Fatalf("kate's post after leaving the relay reached the federated timeline")
	}
	if err := e.local.call(ctx, http.MethodDelete, "/admin/relays/"+r.ID, nil, nil); err != nil {
		t.Fatalf("deleting the relay: %v", err)
	}
}

// adminRelay returns the relay with the id from the admin API's list.
func (e *env) adminRelay(ctx context.Context, id string) (adminRelay, error) {
	var list struct {
		Items []adminRelay `json:"items"`
	}
	if err := e.local.call(ctx, http.MethodGet, "/admin/relays", nil, &list); err != nil {
		return adminRelay{}, err
	}
	for _, it := range list.Items {
		if it.ID == id {
			return it, nil
		}
	}
	return adminRelay{}, fmt.Errorf("relay %s is not listed", id)
}

// onFederatedTimeline reports whether a status with the text is on this server's federated
// timeline.
func (e *env) onFederatedTimeline(ctx context.Context, text string) (bool, string) {
	var page struct {
		Items []status `json:"items"`
	}
	if err := e.local.call(ctx, http.MethodGet, "/timelines/public?limit=40", nil, &page); err != nil {
		return false, err.Error()
	}
	for _, s := range page.Items {
		if strings.Contains(s.Content, text) {
			return true, ""
		}
	}
	return false, "not on the federated timeline"
}

// instanceActor returns the URI of this server's instance actor, the only local Application.
func (e *env) instanceActor(t *testing.T) string {
	t.Helper()
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, e.db)
	if err != nil {
		t.Fatalf("connecting to the database of %s: %v", localHost, err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var uri string
	if err := conn.QueryRow(ctx, `SELECT uri FROM accounts WHERE domain = '' AND actor_type = 'application'`).Scan(&uri); err != nil {
		t.Fatalf("reading the instance actor: %v", err)
	}
	return uri
}

// akkomaCtl runs pleroma_ctl, Akkoma's administration CLI, with the arguments in Akkoma's container.
func akkomaCtl(ctx context.Context, args ...string) (string, error) {
	cmd := append([]string{"compose", "-p", "my-federation-live", "exec", "-T", "akkoma", "/opt/akkoma/bin/pleroma_ctl"}, args...)
	out, err := exec.CommandContext(ctx, "docker", cmd...).CombinedOutput() //nolint:gosec // G204: the scenario's own arguments
	return string(out), err
}
