// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// Bonfire Social (bonfire.test), for the approvals of TestInteractionApprovalBonfire only.
const (
	bonfireHost = "bonfire.test"
	bonfireUser = "bonnie"
)

// bonfire is a user of bonfire.test, driven through Bonfire's Mastodon client API where it has the
// call, its GraphQL API for quotes, and its release's RPC for what neither has (fetching a remote
// status, answering a quote request).
type bonfire struct {
	*mastodonAPI
	user string
}

func newBonfire(e *env, user, token string) *bonfire {
	return &bonfire{&mastodonAPI{host: bonfireHost, token: os.Getenv(token), http: e.http}, user}
}

// peer is the user as a peerClient, for the helpers that need only the host and the handle.
func (b *bonfire) peer() *peerClient {
	return &peerClient{name: "bonfire", host: bonfireHost, user: b.user}
}

// objectURI is the ActivityPub ID of a status of Bonfire by its ID: the API's uri is a path of its
// web pages (/post/{id}).
func (b *bonfire) objectURI(id string) string { return "https://" + bonfireHost + "/pub/objects/" + id }

var bonfireResult = regexp.MustCompile(`RESULT=(\S*)`)

// rpc evaluates Elixir code in Bonfire's running release and returns what it prints after RESULT=.
func (b *bonfire) rpc(ctx context.Context, code string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "compose", "-p", "my-federation-live", "exec", "-T", "bonfire", //nolint:gosec // G204: the scenario's own code
		"./bin/bonfire", "rpc", code).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bonfire rpc: %w: %s", err, out)
	}
	m := bonfireResult.FindSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("bonfire rpc printed no result: %s", out)
	}
	return string(m[1]), nil
}

// fetch makes Bonfire fetch an object of another server by its URI and returns Bonfire's ID of it.
func (b *bonfire) fetch(ctx context.Context, uri string) (string, error) {
	return b.rpc(ctx, fmt.Sprintf(`{:ok, o} = Bonfire.Federate.ActivityPub.AdapterUtils.get_by_url_ap_id_or_username(%s)
IO.puts("RESULT=" <> o.id)`, strconv.Quote(uri)))
}

// lookup finds an account by its handle, fetching it through WebFinger if needed.
func (b *bonfire) lookup(ctx context.Context, handle string) (account, error) {
	var a account
	err := b.call(ctx, http.MethodGet, "/api/v1/accounts/lookup?"+url.Values{"acct": {handle}}.Encode(), nil, &a)
	return a, err
}

// quote posts a public status that quotes the status with Bonfire's ID (GraphQL createPost).
func (b *bonfire) quote(ctx context.Context, text, quoted string) (string, error) {
	var res struct {
		Data struct {
			CreatePost struct {
				ID string `json:"id"`
			} `json:"createPost"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	query := fmt.Sprintf(`mutation { createPost(postContent: {htmlBody: %s}, quoteId: %s, boundary: "public") { id } }`,
		strconv.Quote(text), strconv.Quote(quoted))
	if err := b.call(ctx, http.MethodPost, "/api/graphql", map[string]any{"query": query}, &res); err != nil {
		return "", err
	}
	if len(res.Errors) > 0 || res.Data.CreatePost.ID == "" {
		return "", fmt.Errorf("createPost: %+v", res)
	}
	return res.Data.CreatePost.ID, nil
}

// answerQuote approves or refuses the request of the quote with the URI of the user's status with
// the ID (Bonfire.Social.Quotes; Bonfire has no API for it). The quote is Bonfire's cached
// copy of the remote status, which Bonfire records when it takes the QuoteRequest.
func (b *bonfire) answerQuote(ctx context.Context, quoteURI, quotedID string, accept bool) error {
	answer := map[bool]string{true: "accept_quote(q, p, current_user: Bonfire.Me.Users.by_username!(\"" + b.user + "\"))", false: "reject_quote(q, p)"}[accept]
	_, err := b.rpc(ctx, fmt.Sprintf(`{:ok, o} = ActivityPub.Object.get_cached(ap_id: %s)
{:ok, q} = Bonfire.Posts.read(o.pointer_id, skip_boundary_check: true)
{:ok, p} = Bonfire.Posts.read(%s, skip_boundary_check: true)
{:ok, _} = Bonfire.Social.Quotes.%s
IO.puts("RESULT=ok")`, strconv.Quote(quoteURI), strconv.Quote(quotedID), answer))
	return err
}

// quoteRequested reports whether Bonfire recorded the request of the quote with the URI of the
// user's status with the ID.
func (b *bonfire) quoteRequested(ctx context.Context, quoteURI, quotedID string) (bool, error) {
	out, err := b.rpc(ctx, fmt.Sprintf(`r = with {:ok, %%{pointer_id: id}} when is_binary(id) <- ActivityPub.Object.get_cached(ap_id: %s),
  {:ok, q} <- Bonfire.Posts.read(id, skip_boundary_check: true),
  {:ok, p} <- Bonfire.Posts.read(%s, skip_boundary_check: true),
  {:ok, _} <- Bonfire.Social.Quotes.requested(q, p), do: "yes", else: (_ -> "no")
IO.puts("RESULT=" <> r)`, strconv.Quote(quoteURI), strconv.Quote(quotedID)))
	return out == "yes", err
}

// bonfireDropsQuoteRequests is why Bonfire may not record a QuoteRequest of this server.
const bonfireDropsQuoteRequests = "Bonfire 1.0.7 records a received QuoteRequest only once it has made a post of " +
	"the quoting status (Bonfire.Social.Quotes.ap_receive_activity: AdapterUtils.return_pointable of the instrument), " +
	"which a separate job (RemoteFetcherWorker) makes after it fetches it; when that job has not run yet, it drops the " +
	"request (\"no match in handle_activity_with\") and the quote stays pending"

// awaitQuoteRequest waits for Bonfire to record the request of the quote with the URI of the
// user's status with the ID, and reports whether it did.
func (b *bonfire) awaitQuoteRequest(t *testing.T, quoteURI, quotedID string) bool {
	t.Helper()
	deadline := time.Now().Add(deliveryTimeout)
	for {
		ok, err := b.quoteRequested(t.Context(), quoteURI, quotedID)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Second)
	}
}

// note fetches the Note of a status of Bonfire by its ID, as other servers do (Bonfire serves the
// Create that wraps it).
func (b *bonfire) note(ctx context.Context, id string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.objectURI(id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/activity+json")
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s = %d: %s", b.objectURI(id), resp.StatusCode, body)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if o, ok := doc["object"].(map[string]any); ok && doc["type"] == "Create" {
		return o, nil
	}
	return doc, nil
}

// TestInteractionApprovalBonfire: Bonfire Social 1.0 asks for and gives the approvals of quotes
// (QuoteRequest, QuoteAuthorization, FEP-044f), and has none for replies and boosts: it reads only
// the automaticApproval of canReply and canAnnounce and sends its replies and boosts without a
// request (ADR-0155 §6). bonnie and alice follow each other for the replies and boosts; for the
// quotes, alice alone follows bonnie.
func TestInteractionApprovalBonfire(t *testing.T) {
	e := stack(t)
	b := newBonfire(e, bonfireUser, "FEDERATION_LIVE_BONFIRE_TOKEN")
	p := b.peer()
	ctx := t.Context()
	// The first lookup of an account that Bonfire does not know fetches it and answers 404.
	var alice account
	eventually(t, deliveryTimeout, p.host+" finds "+localHandle, func() (bool, string) {
		a, err := b.lookup(ctx, localHandle)
		alice = a
		return err == nil, fmt.Sprint(err)
	})
	if _, err := b.follow(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	actor, err := e.local.resolve(ctx, p.handle())
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, deliveryTimeout, localHandle+" is followed by "+p.handle(), func() (bool, string) {
		r, err := e.local.relationship(ctx, actor.ID)
		return err == nil && r.FollowedBy, fmt.Sprintf("%+v %v", r, err)
	})
	followLocalToPeer(t, e, p)

	t.Run("reply and boost from peer, admitted as a follower", func(t *testing.T) {
		// Followers at once, everyone else by hand: bonnie is a follower, so her reply and boost need
		// no answer of alice, and this server sends the Accept with the approval (ADR-0155 §6).
		followers := map[string]any{"automatic": "followers", "manual": "everyone"}
		original, err := e.local.postWithPolicy(ctx, unique("reply to me, "+p.name),
			map[string]any{"reply": followers, "announce": followers})
		if err != nil {
			t.Fatal(err)
		}
		onPeer, err := b.fetch(ctx, original.URI)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := b.reply(ctx, unique("a reply from "+p.name), "public", onPeer)
		if err != nil {
			t.Fatal(err)
		}
		uri := b.objectURI(reply.ID)
		eventually(t, deliveryTimeout, uri+" in the thread of "+original.URI, func() (bool, string) {
			th, err := e.local.context(ctx, original.ID)
			_, ok := findStatus(th.Descendants, uri)
			return ok, fmt.Sprintf("%+v %v", th.Descendants, err)
		})
		if err := b.interact(ctx, onPeer, "boost", false); err != nil {
			t.Fatal(err)
		}
		eventually(t, deliveryTimeout, "the boost count of "+original.URI+" is 1", func() (bool, string) {
			s, err := e.local.status(ctx, original.ID)
			return err == nil && s.BoostsCount == 1, fmt.Sprintf("%d %v", s.BoostsCount, err)
		})
	})

	// Bonfire answers our Accept of bonnie's quote only when it learned alice's status by fetching
	// it, not from the Create that reaches bonnie as a follower (Compatibility notes in
	// README.md): bonnie stops following alice for the quotes.
	if err := b.unfollow(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, deliveryTimeout, localHandle+" is no longer followed by "+p.handle(), func() (bool, string) {
		r, err := e.local.relationship(ctx, actor.ID)
		return err == nil && !r.FollowedBy, fmt.Sprintf("%+v %v", r, err)
	})

	t.Run("quote from peer", func(t *testing.T) {
		original, err := e.local.postWithPolicy(ctx, unique("quote me with my approval, "+p.name), approvalPolicy)
		if err != nil {
			t.Fatal(err)
		}
		onPeer, err := b.fetch(ctx, original.URI)
		if err != nil {
			t.Fatal(err)
		}
		for _, accept := range []bool{true, false} {
			id, err := b.quote(ctx, unique(fmt.Sprintf("a quote from %s (accept %t)", p.name, accept)), onPeer)
			if err != nil {
				t.Fatal(err)
			}
			req := localRequestOf(t, e, "quote", p, original.ID)
			if err := e.local.answerInteractionRequest(ctx, req.ID, accept); err != nil {
				t.Fatal(err)
			}
			authorized := func() (bool, string) {
				n, err := b.note(ctx, id)
				return err == nil && n["quoteAuthorization"] != nil && n["quote"] == original.URI,
					fmt.Sprintf("quote %v, quoteAuthorization %v, %v", n["quote"], n["quoteAuthorization"], err)
			}
			if accept {
				eventually(t, deliveryTimeout, "the quote "+b.objectURI(id)+" is authorized on "+p.host, authorized)
				// Bonfire sends the Update that names the quote and the approval, which this server
				// takes without asking again.
				onLocal := localSees(t, e, b.objectURI(id))
				eventually(t, deliveryTimeout, "the quote "+b.objectURI(id)+" is accepted on "+localHost, func() (bool, string) {
					s, err := e.local.status(ctx, onLocal.ID)
					return err == nil && s.Quote != nil && s.Quote.State == "accepted", fmt.Sprintf("%+v %v", s.Quote, err)
				})
			} else {
				never(t, "the refused quote "+b.objectURI(id)+" is authorized on "+p.host, authorized)
			}
		}
	})

	t.Run("quote from local", func(t *testing.T) {
		for _, accept := range []bool{true, false} {
			// Bonfire lets everyone quote a public status with its author's approval by default.
			original, err := b.reply(ctx, unique(fmt.Sprintf("quote me, %s (accept %t)", localHost, accept)), "public", "")
			if err != nil {
				t.Fatal(err)
			}
			onLocal := localSees(t, e, b.objectURI(original.ID))
			q, err := e.local.quote(ctx, unique("a quote from "+localHost), onLocal.ID)
			if err != nil {
				t.Fatal(err)
			}
			if q.Quote == nil || q.Quote.State != "pending" {
				t.Errorf("the quote %s before the answer: %+v", q.URI, q.Quote)
			}
			if !b.awaitQuoteRequest(t, q.URI, original.ID) {
				t.Skipf("%s did not record the QuoteRequest of %s: %s", p.host, q.URI, bonfireDropsQuoteRequests)
			}
			if err := b.answerQuote(ctx, q.URI, original.ID, accept); err != nil {
				t.Fatal(err)
			}
			want := map[bool]string{true: "accepted", false: "rejected"}[accept]
			eventually(t, deliveryTimeout, "the quote state of "+q.URI+" is "+want, func() (bool, string) {
				s, err := e.local.status(ctx, q.ID)
				return err == nil && s.Quote != nil && s.Quote.State == want, fmt.Sprintf("%+v %v", s.Quote, err)
			})
		}
	})

	t.Run("no reply without approval", func(t *testing.T) {
		// Bonfire reads the author alone in automaticApproval as nobody and skips manualApproval, so it
		// refuses bonnie's reply itself and sends nothing.
		original, err := e.local.postWithPolicy(ctx, unique("reply to me with my approval, "+p.name), approvalPolicy)
		if err != nil {
			t.Fatal(err)
		}
		onPeer, err := b.fetch(ctx, original.URI)
		if err != nil {
			t.Fatal(err)
		}
		if reply, err := b.reply(ctx, unique("a reply from "+p.name), "public", onPeer); err == nil {
			t.Logf("Bonfire took the reply %s", b.objectURI(reply.ID))
		} else {
			t.Logf("Bonfire refused the reply: %v", err)
		}
		never(t, "a request of "+p.handle()+" to "+original.URI, func() (bool, string) {
			_, ok, state := findLocalRequest(ctx, e, "reply", p, original.ID)
			return ok, state
		})
	})
}
