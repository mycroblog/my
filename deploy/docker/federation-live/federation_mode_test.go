// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// modeUser is the local user of TestFederationMode, created by scripts/federation-live.sh. Nobody
// on the peers knows him before the scenario, so the first thing a peer learns of him is the Follow
// he sends in the inbound mode, and the federation mode it changes leaves alice's follows alone.
const modeUser = "otto"

// TestFederationMode drives the federation direction (FR-413, ADR-0076) against every peer: the
// inbound mode, then outbound, then off, then both again. The mode is the whole server's, so the
// scenario sets it back to both when it ends; it runs first (the file sorts before the others), when
// no delivery of another scenario is still queued.
//
//   - inbound: otto follows the peer user and the peer accepts, although otto's actor is the minimal
//     document (no name, no icon, no collections); the peer's posts reach otto. The scenario logs
//     how the peer shows otto and whether it looked otto up through WebFinger (ADR-0076 leaves it to be verified).
//     The peer user's follow of otto is rejected and does not stay pending on either side.
//   - outbound: the peer user's follow of otto is accepted and otto's posts reach the peer; the
//     peer's posts, and its reply and favourite of otto's post, reach the inbox (2xx) and are
//     dropped: they show nowhere on this server.
//   - off: otto's actor, WebFinger, and NodeInfo answer 404 (not 410); the peer, made to fetch otto
//     again, still shows him, not deleted nor suspended, and still follows him.
//   - both: the follows of the narrower modes carry on: otto's next post reaches the peer.
func TestFederationMode(t *testing.T) {
	e := stack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	otto, err := signInLocalAs(ctx, e.http.Transport, modeUser, os.Getenv("FEDERATION_LIVE_MY_PASSWORD"))
	if err != nil {
		t.Fatalf("signing in as %s: %v", modeUser, err)
	}
	ottoHandle := modeUser + "@" + localHost
	self, err := otto.resolve(ctx, ottoHandle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.local.setFederationMode(context.Background(), "both"); err != nil {
			t.Errorf("setting the federation mode back to both: %v", err)
		}
	})

	// The peer's account of otto, by peer, once the peer has resolved him.
	onPeer := map[string]account{}
	// The peer user's account on this server, by peer.
	peerOnLocal := map[string]account{}

	t.Run("inbound", func(t *testing.T) {
		if err := e.local.setFederationMode(ctx, "inbound"); err != nil {
			t.Fatal(err)
		}
		code, body, err := e.getActivity(ctx, self.URI)
		if err != nil || code != http.StatusOK {
			t.Fatalf("GET %s in inbound = %d %v", self.URI, code, err)
		}
		t.Logf("the actor document of %s in inbound: %s", ottoHandle, body)
		for _, p := range e.peers {
			t.Run(p.name+"/follow out", func(t *testing.T) {
				since := time.Now()
				actor, err := otto.resolve(ctx, p.handle())
				if err != nil {
					t.Fatal(err)
				}
				peerOnLocal[p.name] = actor
				if _, err := otto.follow(ctx, actor.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, ottoHandle+" follows "+p.handle(), func() (bool, string) {
					r, err := otto.relationship(ctx, actor.ID)
					return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
				})
				// The peer accepted the Follow of the minimal actor: how it shows otto, and whether
				// it looked him up through WebFinger, before anything else made it resolve him.
				eventually(t, deliveryTimeout, p.handle()+" lists "+ottoHandle+" among the followers", func() (bool, string) {
					fs, err := p.followers(ctx)
					for _, f := range fs {
						if f.Acct == ottoHandle {
							t.Logf("%s shows the follower %s as %+v", p.host, ottoHandle, f)
							return true, ""
						}
					}
					return false, fmt.Sprintf("%+v %v", fs, err)
				})
				e.logPeerLookups(t, p, since)
			})
			t.Run(p.name+"/posts in", func(t *testing.T) {
				posted, err := p.post(ctx, unique("inbound from "+p.name), "public")
				if err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, posted.URI+" in the home timeline of "+ottoHandle, func() (bool, string) {
					items, err := otto.home(ctx)
					_, ok := findStatus(items, posted.URI)
					return ok, fmt.Sprintf("%d statuses %v", len(items), err)
				})
			})
			t.Run(p.name+"/follow in is rejected", func(t *testing.T) {
				a, err := p.resolve(ctx, ottoHandle)
				if err != nil {
					t.Fatal(err)
				}
				onPeer[p.name] = a
				if _, err := p.follow(ctx, a.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, "the Follow of "+p.handle()+" in received_activities", func() (bool, string) {
					got := e.receivedActivities(t, peerOnLocal[p.name].URI)
					return slices.ContainsFunc(got, func(a receivedActivity) bool { return a.Type == "Follow" }), fmt.Sprintf("%+v", got)
				})
				eventually(t, deliveryTimeout, p.handle()+" neither follows nor waits to follow "+ottoHandle, func() (bool, string) {
					r, err := p.relationship(ctx, a.ID)
					return err == nil && !r.Following && !r.Requested, fmt.Sprintf("%+v %v", r, err)
				})
				r, err := otto.relationship(ctx, peerOnLocal[p.name].ID)
				if err != nil || r.FollowedBy {
					t.Errorf("%s is followed by %s on %s: %+v %v", ottoHandle, p.handle(), localHost, r, err)
				}
				requests, err := otto.followRequests(ctx)
				if err != nil || len(requests) != 0 {
					t.Errorf("the follow requests of %s = %+v %v, want none", ottoHandle, requests, err)
				}
			})
		}
	})
	if t.Failed() {
		return
	}

	t.Run("outbound", func(t *testing.T) {
		if err := e.local.setFederationMode(ctx, "outbound"); err != nil {
			t.Fatal(err)
		}
		for _, p := range e.peers {
			t.Run(p.name, func(t *testing.T) {
				peer := peerOnLocal[p.name]
				a := onPeer[p.name]
				if _, err := p.follow(ctx, a.ID); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, p.handle()+" follows "+ottoHandle, func() (bool, string) {
					r, err := p.relationship(ctx, a.ID)
					return err == nil && r.Following, fmt.Sprintf("%+v %v", r, err)
				})
				eventually(t, deliveryTimeout, ottoHandle+" is followed by "+p.handle(), func() (bool, string) {
					r, err := otto.relationship(ctx, peer.ID)
					return err == nil && r.FollowedBy, fmt.Sprintf("%+v %v", r, err)
				})

				mine, err := otto.post(ctx, unique("outbound to "+p.name), "public")
				if err != nil {
					t.Fatal(err)
				}
				seen := peerHas(t, p, a.ID, mine.URI)

				// What the peer sends reaches the inbox and is dropped: its post (otto still follows
				// the peer user), its reply to otto's post, and its favourite of it.
				known := e.receivedURIs(t, peer.URI)
				theirs, err := p.post(ctx, unique("outbound from "+p.name), "public")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := p.reply(ctx, unique("@"+ottoHandle+" reply from "+p.name), "public", seen.ID); err != nil {
					t.Fatal(err)
				}
				if err := p.interact(ctx, seen.ID, "favourite", false); err != nil {
					t.Fatal(err)
				}
				eventually(t, deliveryTimeout, "the Creates and the Like of "+p.handle()+" in received_activities", func() (bool, string) {
					creates, likes := 0, 0
					for _, a := range e.receivedActivities(t, peer.URI) {
						if known[a.URI] {
							continue
						}
						switch a.Type {
						case "Create":
							creates++
						case "Like":
							likes++
						}
					}
					return creates >= 2 && likes >= 1, fmt.Sprintf("%d Creates, %d Likes", creates, likes)
				})
				never(t, "something of "+p.handle()+" is taken in on "+localHost, func() (bool, string) {
					items, err := otto.home(ctx)
					if _, ok := findStatus(items, theirs.URI); ok {
						return true, "the post is in the home timeline of " + ottoHandle
					}
					s, err2 := otto.status(ctx, mine.ID)
					if err == nil && err2 == nil && (s.FavouritesCount != 0 || len(s.Reactions) != 0) {
						return true, fmt.Sprintf("the post of %s has %d favourites and the reactions %+v", ottoHandle, s.FavouritesCount, s.Reactions)
					}
					th, err3 := otto.context(ctx, mine.ID)
					if err3 == nil && len(th.Descendants) != 0 {
						return true, fmt.Sprintf("the post of %s has the replies %+v", ottoHandle, th.Descendants)
					}
					ns, err4 := otto.notifications(ctx)
					for _, n := range ns {
						if n.Account.ID == peer.ID && !followNotifications[n.Type] {
							return true, fmt.Sprintf("%s got the notification %+v", ottoHandle, n)
						}
					}
					return false, fmt.Sprint(err, err2, err3, err4)
				})
			})
		}
	})
	if t.Failed() {
		return
	}

	t.Run("off", func(t *testing.T) {
		if err := e.local.setFederationMode(ctx, "off"); err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{
			self.URI,
			"https://" + localHost + "/.well-known/webfinger?resource=" + url.QueryEscape("acct:"+ottoHandle),
			"https://" + localHost + "/.well-known/nodeinfo",
			"https://" + localHost + "/nodeinfo/2.1",
		} {
			if code, body, err := e.getActivity(ctx, target); err != nil || code != http.StatusNotFound {
				t.Errorf("GET %s in off = %d %v, want 404: %s", target, code, err, body)
			}
		}
		for _, p := range e.peers {
			t.Run(p.name, func(t *testing.T) {
				a := onPeer[p.name]
				since := time.Now()
				// The peer fetches otto again and gets the 404; it must keep the account.
				if err := p.refetchActor(ctx, self.URI, a.ID); err != nil {
					t.Logf("%s refetching %s: %v", p.host, self.URI, err)
				}
				e.logPeerLookups(t, p, since)
				never(t, p.host+" takes "+ottoHandle+" as gone", func() (bool, string) {
					gone, state, err := p.gone(ctx, a.ID)
					if err != nil {
						return true, err.Error()
					}
					if gone {
						return true, state
					}
					r, err := p.relationship(ctx, a.ID)
					if err != nil || !r.Following {
						return true, fmt.Sprintf("%s no longer follows %s: %+v %v", p.handle(), ottoHandle, r, err)
					}
					return false, ""
				})
			})
		}
	})
	if t.Failed() {
		return
	}

	t.Run("both again", func(t *testing.T) {
		if err := e.local.setFederationMode(ctx, "both"); err != nil {
			t.Fatal(err)
		}
		mine, err := otto.post(ctx, unique("both again"), "public")
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range e.peers {
			t.Run(p.name, func(t *testing.T) { peerHas(t, p, onPeer[p.name].ID, mine.URI) })
		}
	})
}

// followNotifications are the notifications that the peer's follow and its acceptance give otto,
// which TestFederationMode allows in the outbound mode.
var followNotifications = map[string]bool{"follow": true, "follow_request": true, "follow_accepted": true}

// peerHas waits until the peer stores a status among those of the author, by the peer's ID of the
// author, and returns it. It reads the author's statuses rather than the home timeline: GoToSocial
// 0.22 stores otto's posts and lists them on his profile, but leaves them out of carol's home
// timeline, from its database too (README.md, Compatibility notes).
func peerHas(t *testing.T, p *peerClient, author, uri string) status {
	t.Helper()
	var found status
	eventually(t, deliveryTimeout, uri+" among the statuses of "+author+" on "+p.host, func() (bool, string) {
		items, err := p.statusesOf(t.Context(), author)
		s, ok := findStatus(items, uri)
		found = s
		return ok, fmt.Sprintf("%d statuses %v", len(items), err)
	})
	return found
}

// setFederationMode changes the instance setting federation_mode through the admin API.
func (l *localClient) setFederationMode(ctx context.Context, mode string) error {
	return l.call(ctx, http.MethodPatch, "/admin/settings", map[string]any{"values": map[string]string{"federation_mode": mode}}, nil)
}

// getActivity GETs a URL of this server as a remote server would, unsigned, and returns the status
// and the body.
func (e *env) getActivity(ctx context.Context, rawURL string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Accept", `application/activity+json, application/jrd+json, application/json`)
	resp, err := e.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), err
}

// receivedURIs returns the URIs of what this server recorded from the actor.
func (e *env) receivedURIs(t *testing.T, actorURI string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, a := range e.receivedActivities(t, actorURI) {
		out[a.URI] = true
	}
	return out
}

// proxyEntry is the part of an access log entry of the proxy (Caddyfile, JSON) that the scenarios
// read.
type proxyEntry struct {
	TS      float64 `json:"ts"`
	Status  int     `json:"status"`
	Request struct {
		Host    string              `json:"host"`
		Method  string              `json:"method"`
		URI     string              `json:"uri"`
		Headers map[string][]string `json:"headers"`
	} `json:"request"`
}

// logPeerLookups logs the GETs that the peer made to this server since a time, from the access log
// of the proxy (docker compose logs): its WebFinger lookups and its fetches of actors, with their
// status. The log is read through the Docker CLI; where it cannot be, the scenario says so and
// carries on.
func (e *env) logPeerLookups(t *testing.T, p *peerClient, since time.Time) {
	t.Helper()
	entries, err := peerRequests(t.Context(), p, http.MethodGet, since)
	if err != nil {
		t.Logf("cannot read the access log of the proxy: %v", err)
		return
	}
	webFinger := 0
	for _, en := range entries {
		ua := strings.Join(en.Request.Headers["User-Agent"], " ")
		path, _, _ := strings.Cut(en.Request.URI, "?")
		if path == "/.well-known/webfinger" {
			webFinger++
		}
		t.Logf("%s GET %s = %d (%s)", p.host, en.Request.URI, en.Status, ua)
	}
	t.Logf("%s made %d WebFinger lookups to %s", p.host, webFinger, localHost)
}

// time is when the proxy wrote the entry.
func (en proxyEntry) time() time.Time {
	return time.Unix(0, int64(en.TS*float64(time.Second)))
}

// peerRequests returns the requests with the method that the peer made to this server since a
// time, oldest first, from the access log of the proxy (docker compose logs). A request is the
// peer's when its User-Agent names the peer's host.
func peerRequests(ctx context.Context, p *peerClient, method string, since time.Time) ([]proxyEntry, error) {
	// The proxy writes the entry after the response; give the last one a moment.
	time.Sleep(2 * time.Second)
	out, err := exec.CommandContext(ctx, "docker", "compose", "-p", "my-federation-live",
		"logs", "--no-log-prefix", "--no-color", "proxy").Output()
	if err != nil {
		return nil, err
	}
	var entries []proxyEntry
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var en proxyEntry
		if json.Unmarshal(sc.Bytes(), &en) != nil || en.Request.Host != localHost || en.Request.Method != method {
			continue
		}
		if en.time().Before(since) {
			continue
		}
		if !strings.Contains(strings.ToLower(strings.Join(en.Request.Headers["User-Agent"], " ")), p.host) {
			continue
		}
		entries = append(entries, en)
	}
	return entries, sc.Err()
}

// --- What TestFederationMode asks of the peers ------------------------------------------------

// followers returns the accounts that follow the peer user.
func (p *mastodonAPI) followers(ctx context.Context) ([]account, error) {
	var me account
	if err := p.call(ctx, http.MethodGet, "/api/v1/accounts/verify_credentials", nil, &me); err != nil {
		return nil, err
	}
	var as []account
	err := p.call(ctx, http.MethodGet, "/api/v1/accounts/"+url.PathEscape(me.ID)+"/followers?limit=80", nil, &as)
	return as, err
}

// statusesOf returns the statuses of an account, by the peer's ID of it.
func (p *mastodonAPI) statusesOf(ctx context.Context, id string) ([]status, error) {
	var ss []status
	err := p.call(ctx, http.MethodGet, "/api/v1/accounts/"+url.PathEscape(id)+"/statuses?limit=40", nil, &ss)
	return ss, err
}

// refetchActor searches the actor's URI with resolve=true, which makes the peer fetch it.
func (p *mastodonAPI) refetchActor(ctx context.Context, uri, _ string) error {
	q := url.Values{"q": {uri}, "resolve": {"true"}, "type": {"accounts"}, "limit": {"1"}}
	return p.call(ctx, http.MethodGet, "/api/v2/search?"+q.Encode(), nil, nil)
}

// gone reads whether the peer shows the account as gone: suspended, or 404 / 410.
func (p *mastodonAPI) gone(ctx context.Context, id string) (bool, string, error) {
	var a struct {
		Acct      string `json:"acct"`
		Suspended bool   `json:"suspended"`
	}
	err := p.call(ctx, http.MethodGet, "/api/v1/accounts/"+url.PathEscape(id), nil, &a)
	var ae *apiError
	if errors.As(err, &ae) && (ae.status == http.StatusGone || ae.status == http.StatusNotFound) {
		return true, fmt.Sprintf("%d", ae.status), nil
	}
	if err != nil {
		return false, "", err
	}
	return a.Suspended, fmt.Sprintf("%+v", a), nil
}

// followers returns the users that follow the peer user (users/followers).
func (m *misskeyAPI) followers(ctx context.Context) ([]account, error) {
	var me mkUser
	if err := m.call(ctx, "i", nil, &me); err != nil {
		return nil, err
	}
	var rs []struct {
		Follower mkUser `json:"follower"`
	}
	if err := m.call(ctx, "users/followers", map[string]any{"userId": me.ID, "limit": 100}, &rs); err != nil {
		return nil, err
	}
	out := make([]account, 0, len(rs))
	for _, r := range rs {
		out = append(out, m.toAccount(r.Follower))
	}
	return out, nil
}

// statusesOf returns the notes of a user (users/notes).
func (m *misskeyAPI) statusesOf(ctx context.Context, id string) ([]status, error) {
	var ns []mkNote
	if err := m.call(ctx, "users/notes", map[string]any{"userId": id, "limit": 100}, &ns); err != nil {
		return nil, err
	}
	return m.statuses(ns), nil
}

// refetchActor makes Misskey fetch the remote user again (federation/update-remote-user).
func (m *misskeyAPI) refetchActor(ctx context.Context, _, id string) error {
	return m.call(ctx, "federation/update-remote-user", map[string]any{"userId": id}, nil)
}

// gone reads whether Misskey shows the user as gone: suspended, deleted, or missing.
func (m *misskeyAPI) gone(ctx context.Context, id string) (bool, string, error) {
	var u struct {
		Username    string `json:"username"`
		IsSuspended bool   `json:"isSuspended"`
		IsDeleted   bool   `json:"isDeleted"`
	}
	err := m.call(ctx, "users/show", map[string]any{"userId": id}, &u)
	if isNotFound(err) {
		return true, "NO_SUCH_USER", nil
	}
	if err != nil {
		return false, "", err
	}
	return u.IsSuspended || u.IsDeleted, fmt.Sprintf("%+v", u), nil
}
