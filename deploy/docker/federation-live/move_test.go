// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// The users of TestMove, created by scripts/federation-live.sh. An account moves at most once a run:
// Mastodon and this server allow one move every 30 days, and the servers that take a Move process
// one per source every 7 days (Mastodon, this server) or a week (GoToSocial).
const (
	// moveArrival is, on this server, the account that leave of every peer moves to, and, on every
	// peer, the account that the local leave<peer> moves to.
	moveArrival = "arrive"
	// moveLeaver is the peer's account that moves to arrive@my.test; on this server, the account
	// that moves to arrive of the peer is moveLeaver + the peer's name (leavemastodon,
	// leavegotosocial, leavemisskey).
	moveLeaver = "leave"
	// moveTimeout bounds the wait for a move to land: the Move, the fetch of the target, the
	// migration job, and the Follow it sends and its Accept.
	moveTimeout = 3 * time.Minute
)

// peerMover sets the aliases of the peer's arrive and moves the peer's leave, each through the
// peer's own way of doing it.
type peerMover interface {
	// alias makes arrive of the peer name the account (handle user@host, actor uri) among its
	// aliases (alsoKnownAs).
	alias(ctx context.Context, handle, uri string) error
	// move moves leave of the peer to the account (handle user@host, actor uri).
	move(ctx context.Context, handle, uri string) error
}

// TestMove moves accounts in both directions with every peer (FR-414, ADR-0089 相互運用の確かめ方):
//
//   - out: the peer user (bob, carol, grace) follows leave<peer>@my.test; arrive of the peer names
//     it among its aliases; it moves to arrive of the peer (POST /accounts/move); the peer user
//     follows arrive instead (the peer took the Move and moved its follower). On Misskey the
//     target is arrive@my.test instead (misskeyOwnMoveTarget).
//   - in: alice follows leave of the peer; arrive@my.test names it among its aliases
//     (POST /accounts/aliases); it moves to arrive@my.test on the peer; alice follows arrive@my.test
//     instead and has an account_moved notification of the move, with the outcome followed.
//
// Each direction uses accounts of its own, so a run moves every account once.
func TestMove(t *testing.T) {
	e := stack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	password := os.Getenv("FEDERATION_LIVE_MY_PASSWORD")
	arrival, err := signInLocalAs(ctx, e.http.Transport, moveArrival, password)
	if err != nil {
		t.Fatalf("signing in as %s: %v", moveArrival, err)
	}
	movers, err := e.peerMovers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range e.peers {
		target := moveTarget{handle: moveArrival + "@" + p.host, alias: movers[p.name].alias}
		if p.name == "misskey" {
			// The move goes to arrive@my.test, an account of another server to Misskey.
			t.Logf("%s: %s moves to %s", misskeyOwnMoveTarget, moveLeaver+p.name, moveArrival+"@"+localHost)
			target = moveTarget{handle: moveArrival + "@" + localHost, alias: func(ctx context.Context, handle, _ string) error {
				return arrival.call(ctx, http.MethodPost, "/accounts/aliases", map[string]any{"acct": handle}, nil)
			}}
		}
		t.Run(p.name+"/out", func(t *testing.T) { e.moveOut(ctx, t, p, target, password) })
		t.Run(p.name+"/in", func(t *testing.T) { e.moveIn(ctx, t, p, movers[p.name], arrival) })
	}
}

// moveTarget is the account that leave<peer>@my.test moves to, and how that account names the
// leaver among its aliases (handle user@host, actor uri).
type moveTarget struct {
	handle string
	alias  func(ctx context.Context, handle, uri string) error
}

// misskeyOwnMoveTarget is why leavemisskey@my.test moves to arrive@my.test, not to
// arrive@misskey.test: Misskey 2026.10 fails the Move of a remote account to one of its own
// (ApPersonService.processRemoteMove looks the local target up by its uri, which a local user does
// not have: EntityNotFoundError), whoever sends the Move, and the follower stays where it was.
const misskeyOwnMoveTarget = "Misskey fails a Move to one of its own accounts"

// moveOut moves leave<peer>@my.test to the target, and checks that the peer user who followed it
// follows the target instead.
func (e *env) moveOut(ctx context.Context, t *testing.T, p *peerClient, to moveTarget, password string) {
	name := moveLeaver + p.name
	handle := name + "@" + localHost
	leaver, err := signInLocalAs(ctx, e.http.Transport, name, password)
	if err != nil {
		t.Fatalf("signing in as %s: %v", name, err)
	}
	self, err := leaver.resolve(ctx, handle)
	if err != nil {
		t.Fatal(err)
	}
	onPeer, err := p.resolve(ctx, handle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.follow(ctx, onPeer.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, deliveryTimeout, p.handle()+" follows "+handle, func() (bool, string) {
		r, err := p.relationship(ctx, onPeer.ID)
		return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
	})

	arrive := to.handle
	if err := to.alias(ctx, handle, self.URI); err != nil {
		t.Fatalf("naming %s among the aliases of %s: %v", handle, arrive, err)
	}
	target, err := leaver.resolve(ctx, arrive)
	if err != nil {
		t.Fatal(err)
	}
	// This server fetches the target again and refuses the move until its actor names the
	// leaver; the peer may serve its actor from a cache for a moment.
	eventually(t, deliveryTimeout, handle+" moves to "+arrive, func() (bool, string) {
		err := leaver.call(ctx, http.MethodPost, "/accounts/move", map[string]any{"target_account_id": target.ID, "password": password}, nil)
		return err == nil, fmt.Sprint(err)
	})

	arriveOnPeer, err := p.resolve(ctx, arrive)
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, moveTimeout, p.handle()+" follows "+arrive+" after the move", func() (bool, string) {
		r, err := p.relationship(ctx, arriveOnPeer.ID)
		return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
	})
	// Whether the peer also ends the follow of the source is the peer's choice; log it.
	r, err := p.relationship(ctx, onPeer.ID)
	t.Logf("after the move, %s → %s on %s: %+v %v", p.handle(), handle, p.host, r, err)
}

// moveIn moves leave of the peer to arrive@my.test, and checks that alice, who followed it,
// follows arrive@my.test instead and is notified.
func (e *env) moveIn(ctx context.Context, t *testing.T, p *peerClient, m peerMover, arrival *localClient) {
	leaver := moveLeaver + "@" + p.host
	arrive := moveArrival + "@" + localHost
	source, err := e.local.resolve(ctx, leaver)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.local.follow(ctx, source.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, deliveryTimeout, localHandle+" follows "+leaver, func() (bool, string) {
		r, err := e.local.relationship(ctx, source.ID)
		return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
	})

	if err := arrival.call(ctx, http.MethodPost, "/accounts/aliases", map[string]any{"acct": leaver}, nil); err != nil {
		t.Fatalf("naming %s among the aliases of %s: %v", leaver, arrive, err)
	}
	target, err := e.local.resolve(ctx, arrive)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.move(ctx, arrive, target.URI); err != nil {
		t.Fatalf("moving %s to %s: %v", leaver, arrive, err)
	}

	eventually(t, moveTimeout, localHandle+" follows "+arrive+" instead of "+leaver, func() (bool, string) {
		to, err := e.local.relationship(ctx, target.ID)
		if err != nil {
			return false, err.Error()
		}
		from, err := e.local.relationship(ctx, source.ID)
		return err == nil && to.Following && !from.Following, fmt.Sprintf("→ %s %+v, → %s %+v %v", arrive, to, leaver, from, err)
	})
	eventually(t, deliveryTimeout, "the account_moved notification of "+leaver, func() (bool, string) {
		ns, err := e.local.notifications(ctx)
		if err != nil {
			return false, err.Error()
		}
		ok := slices.ContainsFunc(ns, func(n notification) bool {
			return n.Type == "account_moved" && n.Account.ID == source.ID && n.AccountMove != nil &&
				n.AccountMove.Target.ID == target.ID && n.AccountMove.Outcome == "followed"
		})
		return ok, fmt.Sprintf("%d notifications", len(ns))
	})
}

// peerMovers returns the peerMover of every peer: Mastodon's through a Rails runner (it has no
// client API for aliases and moves), GoToSocial's and Misskey's through their APIs as arrive and
// leave. The GoToSocial accounts are unlocked, as the scenarios' other users are (setup).
func (e *env) peerMovers(ctx context.Context) (map[string]peerMover, error) {
	gts := map[string]*mastodonAPI{}
	for _, user := range []string{moveArrival, moveLeaver} {
		token, err := signInGoToSocial(ctx, e.http.Transport, user, os.Getenv("FEDERATION_LIVE_GTS_PASSWORD"))
		if err != nil {
			return nil, fmt.Errorf("signing in to %s as %s: %w", gtsHost, user, err)
		}
		gts[user] = &mastodonAPI{host: gtsHost, token: token, http: e.http}
		if err := gts[user].setLocked(ctx, false); err != nil {
			return nil, fmt.Errorf("unlocking %s@%s: %w", user, gtsHost, err)
		}
	}
	misskey := func(env string) *misskeyAPI {
		return &misskeyAPI{host: misskeyHost, token: os.Getenv(env), http: e.http}
	}
	return map[string]peerMover{
		"mastodon":   mastodonMover{},
		"gotosocial": gtsMover{arrive: gts[moveArrival], leave: gts[moveLeaver], password: os.Getenv("FEDERATION_LIVE_GTS_PASSWORD")},
		"misskey":    misskeyMover{arrive: misskey("FEDERATION_LIVE_MISSKEY_TOKEN_ARRIVE"), leave: misskey("FEDERATION_LIVE_MISSKEY_TOKEN_LEAVE")},
	}, nil
}

// mastodonMover does what Mastodon's settings do (Settings::AliasesController#create,
// Settings::MigrationsController#create, v4.7) in a Rails runner: the alias and the migration
// resolve the account again (the migration without the cache) and check its alsoKnownAs, and
// MoveService sends the Update of the actor and the Move to the followers.
type mastodonMover struct{}

func (mastodonMover) alias(ctx context.Context, handle, _ string) error {
	return mastodonRunner(ctx, `
		account = Account.find_local!(ARGV[0])
		account.aliases.create!(acct: ARGV[1])
		ActivityPub::UpdateDistributionWorker.perform_async(account.id)
	`, moveArrival, handle)
}

func (mastodonMover) move(ctx context.Context, handle, _ string) error {
	return mastodonRunner(ctx, `
		migration = Account.find_local!(ARGV[0]).migrations.build(acct: ARGV[1])
		migration.save!
		MoveService.new.call(migration)
	`, moveLeaver, handle)
}

// mastodonRunner runs Ruby code with the arguments (ARGV) in Mastodon's web container.
func mastodonRunner(ctx context.Context, code string, args ...string) error {
	cmd := append([]string{"compose", "-p", "my-federation-live", "exec", "-T", "mastodon-web", "bin/rails", "runner", code}, args...)
	out, err := exec.CommandContext(ctx, "docker", cmd...).CombinedOutput() //nolint:gosec // G204: the scenario's own code and handles
	if err != nil {
		return fmt.Errorf("rails runner: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// gtsMover uses GoToSocial's client API: POST /api/v1/accounts/alias, whose aliases are actor
// URIs, and POST /api/v1/accounts/move with the password.
type gtsMover struct {
	arrive, leave *mastodonAPI
	password      string
}

func (g gtsMover) alias(ctx context.Context, _, uri string) error {
	return g.arrive.call(ctx, http.MethodPost, "/api/v1/accounts/alias", map[string]any{"also_known_as_uris": []string{uri}}, nil)
}

func (g gtsMover) move(ctx context.Context, _, uri string) error {
	return g.leave.call(ctx, http.MethodPost, "/api/v1/accounts/move", map[string]any{"password": g.password, "moved_to_uri": uri}, nil)
}

// misskeyMover uses the Misskey API: i/update with alsoKnownAs, whose aliases are handles, and
// i/move.
type misskeyMover struct {
	arrive, leave *misskeyAPI
}

// alias is not called while misskeyOwnMoveTarget holds: the local leaver moves to arrive@my.test.
func (m misskeyMover) alias(ctx context.Context, handle, _ string) error {
	return m.arrive.call(ctx, "i/update", map[string]any{"alsoKnownAs": []string{"@" + handle}}, nil)
}

func (m misskeyMover) move(ctx context.Context, handle, _ string) error {
	return m.leave.call(ctx, "i/move", map[string]any{"moveToAccount": "@" + handle}, nil)
}
