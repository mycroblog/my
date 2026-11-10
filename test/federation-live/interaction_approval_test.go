// SPDX-License-Identifier: AGPL-3.0-only

//go:build federationlive

package federationlive

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The approvals of replies, boosts, and quotes (ADR-0155 §6 to §8, ADR-0069, federation.md §3.5).

// --- This server ------------------------------------------------------------------------------

// approvalPolicy is the interaction_policy of a status whose replies, boosts, and quotes wait for
// its author's approval, whoever makes them: no one is admitted at once, everyone by hand.
var approvalPolicy = map[string]any{
	"reply":    map[string]any{"automatic": "nobody", "manual": "everyone"},
	"announce": map[string]any{"automatic": "nobody", "manual": "everyone"},
	"quote":    map[string]any{"automatic": "nobody", "manual": "everyone"},
}

// localInteractionRequest is an interaction request to alice (this server's client API).
type localInteractionRequest struct {
	ID                string  `json:"id"`
	Kind              string  `json:"kind"`
	State             string  `json:"state"`
	Account           account `json:"account"`
	Status            status  `json:"status"`
	InteractingStatus *status `json:"interacting_status"`
}

// postWithPolicy posts a public status with an interaction policy.
func (l *localClient) postWithPolicy(ctx context.Context, text string, policy map[string]any) (status, error) {
	var s status
	err := l.api(ctx, http.MethodPost, "/statuses",
		map[string]any{"text": text, "visibility": "public", "interaction_policy": policy}, &s)
	return s, err
}

// quote posts a public status that quotes the status with the ID.
func (l *localClient) quote(ctx context.Context, text, quoted string) (status, error) {
	var s status
	err := l.api(ctx, http.MethodPost, "/statuses",
		map[string]any{"text": text, "visibility": "public", "quoted_status_id": quoted}, &s)
	return s, err
}

// interactionRequests returns the requests that wait for alice's approval.
func (l *localClient) interactionRequests(ctx context.Context) ([]localInteractionRequest, error) {
	var page struct {
		Items []localInteractionRequest `json:"items"`
	}
	err := l.api(ctx, http.MethodGet, "/interaction_requests?limit=40", nil, &page)
	return page.Items, err
}

// answerInteractionRequest approves or refuses a request.
func (l *localClient) answerInteractionRequest(ctx context.Context, id string, accept bool) error {
	if accept {
		return l.api(ctx, http.MethodPost, "/interaction_requests/"+url.PathEscape(id)+"/accept", nil, nil)
	}
	return l.api(ctx, http.MethodPost, "/interaction_requests/"+url.PathEscape(id)+"/reject", nil, nil)
}

// --- The peers ----------------------------------------------------------------------------------

// peerInteractionRequest is an interaction request to the peer user, in the terms of GoToSocial's
// client API: the kind is reply or reblog.
type peerInteractionRequest struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"`
	Account account `json:"account"`
	Status  status  `json:"status"`
	Reply   *status `json:"reply"`
}

// approver is a peer user who asks for approvals and gives them: GoToSocial's interaction
// policies and interaction requests.
type approver interface {
	// postNeedingApproval posts a public status whose replies and boosts wait for the user's
	// approval, whoever makes them.
	postNeedingApproval(ctx context.Context, text string) (status, error)
	// interactionRequests returns the requests that wait for the user's approval of
	// interactions with the status, by the peer's ID of it.
	interactionRequests(ctx context.Context, statusID string) ([]peerInteractionRequest, error)
	answerInteractionRequest(ctx context.Context, id string, accept bool) error
}

// postNeedingApproval posts with GoToSocial's interaction_policy: replies and boosts of the author
// at once, of everyone by hand; favourites of everyone.
func (p *mastodonAPI) postNeedingApproval(ctx context.Context, text string) (status, error) {
	byHand := map[string]any{"automatic_approval": []string{"author"}, "manual_approval": []string{"public"}}
	body := map[string]any{"status": text, "visibility": "public", "interaction_policy": map[string]any{
		"can_reply": byHand, "can_reblog": byHand,
		"can_favourite": map[string]any{"automatic_approval": []string{"public"}},
	}}
	var s status
	err := p.call(ctx, http.MethodPost, "/api/v1/statuses", body, &s)
	return s, err
}

// interactionRequests filters the requests itself: GoToSocial 0.21 and 0.22 answer the status_id
// parameter with a 500 (their query names a column that the table does not have).
func (p *mastodonAPI) interactionRequests(ctx context.Context, statusID string) ([]peerInteractionRequest, error) {
	var all []peerInteractionRequest
	if err := p.call(ctx, http.MethodGet, "/api/v1/interaction_requests?favourites=false&limit=80", nil, &all); err != nil {
		return nil, err
	}
	var rs []peerInteractionRequest
	for _, r := range all {
		if r.Status.ID == statusID {
			rs = append(rs, r)
		}
	}
	return rs, nil
}

func (p *mastodonAPI) answerInteractionRequest(ctx context.Context, id string, accept bool) error {
	verb := map[bool]string{true: "authorize", false: "reject"}[accept]
	return p.call(ctx, http.MethodPost, "/api/v1/interaction_requests/"+url.PathEscape(id)+"/"+verb, nil, nil)
}

// quote posts a public status that quotes the status with the peer's ID (Mastodon 4.5 and later).
func (p *mastodonAPI) quote(ctx context.Context, text, quoted string) (status, error) {
	var s status
	err := p.call(ctx, http.MethodPost, "/api/v1/statuses",
		map[string]any{"status": text, "visibility": "public", "quoted_status_id": quoted}, &s)
	return s, err
}

// --- Waiting for requests -----------------------------------------------------------------------

// localRequestOf waits for the request of the kind by the peer to the status of alice and returns
// it.
func localRequestOf(t *testing.T, e *env, kind string, p *peerClient, statusID string) localInteractionRequest {
	t.Helper()
	var found localInteractionRequest
	eventually(t, deliveryTimeout, "the "+kind+" request of "+p.handle()+" to "+statusID+" on "+localHost, func() (bool, string) {
		r, ok, state := findLocalRequest(t.Context(), e, kind, p, statusID)
		found = r
		return ok, state
	})
	return found
}

// findLocalRequest returns the pending request of the kind by the peer to the status of alice.
func findLocalRequest(ctx context.Context, e *env, kind string, p *peerClient, statusID string) (localInteractionRequest, bool, string) {
	rs, err := e.local.interactionRequests(ctx)
	for _, r := range rs {
		if r.Kind == kind && r.Status.ID == statusID && r.State == "pending" &&
			strings.HasPrefix(r.Account.URI, "https://"+p.host+"/") {
			return r, true, ""
		}
	}
	return localInteractionRequest{}, false, fmt.Sprintf("%+v %v", rs, err)
}

// peerRequestOf waits for the request of the kind (reply / reblog) of the interaction with the URI
// to the peer user's status and returns it. GoToSocial names the reply of a request, and not the
// boost.
func peerRequestOf(t *testing.T, a approver, kind, statusID, interaction string) peerInteractionRequest {
	t.Helper()
	var found peerInteractionRequest
	eventually(t, deliveryTimeout, "the "+kind+" request of "+localHandle+" to "+statusID, func() (bool, string) {
		r, ok, state := findPeerRequest(t.Context(), a, kind, statusID, interaction)
		found = r
		return ok, state
	})
	return found
}

func findPeerRequest(ctx context.Context, a approver, kind, statusID, interaction string) (peerInteractionRequest, bool, string) {
	rs, err := a.interactionRequests(ctx, statusID)
	for _, r := range rs {
		if r.Type != kind || r.Account.Acct != localHandle {
			continue
		}
		if kind == "reply" && (r.Reply == nil || r.Reply.URI != interaction) {
			continue
		}
		return r, true, ""
	}
	return peerInteractionRequest{}, false, fmt.Sprintf("%+v %v", rs, err)
}

// --- Scenarios ----------------------------------------------------------------------------------

// TestInteractionApproval: the approvals of replies and boosts with GoToSocial 0.22 and 0.21, in
// both directions (ADR-0155 §6, §7). Each peer user and alice post statuses whose replies and
// boosts wait for their approval; the other side asks (ReplyRequest, AnnounceRequest), and the
// answer (Accept with ReplyAuthorization / AnnounceAuthorization, or Reject) reaches the side that
// asked. A request that the side asking withdraws (Delete of the reply, Undo of the boost) leaves
// the requests of the side answering.
func TestInteractionApproval(t *testing.T) {
	e := stack(t)
	for _, p := range e.approvers {
		t.Run(p.name, func(t *testing.T) {
			a, ok := p.peerAPI.(approver)
			if !ok {
				t.Fatalf("%s gives no approvals", p.name)
			}
			followLocalToPeer(t, e, p)
			t.Run("reply from local", func(t *testing.T) { approveLocalReply(t, e, p, a) })
			t.Run("boost from local", func(t *testing.T) { approveLocalBoost(t, e, p, a) })
			t.Run("quote from local", func(t *testing.T) { refuseLocalQuote(t, e, p) })
			t.Run("withdrawn from local", func(t *testing.T) { withdrawLocal(t, e, p, a) })
			t.Run("reply from peer", func(t *testing.T) { approvePeerReply(t, e, p) })
			t.Run("boost from peer", func(t *testing.T) { approvePeerBoost(t, e, p) })
			t.Run("withdrawn from peer", func(t *testing.T) { withdrawPeer(t, e, p) })
		})
	}
}

// approveLocalReply: alice's reply to a status of the peer user that needs approval waits
// (approval pending, sent as a ReplyRequest); approved, it joins the thread on the peer; refused,
// it shows as rejected to alice and never joins it.
func approveLocalReply(t *testing.T, e *env, p *peerClient, a approver) {
	ctx := t.Context()
	original, err := a.postNeedingApproval(ctx, unique("reply to me with my approval, "+localHost))
	if err != nil {
		t.Fatal(err)
	}
	onLocal := localSees(t, e, original.URI)
	for _, accept := range []bool{true, false} {
		// GoToSocial drops a status of an account its user does not follow that does not mention her.
		reply, err := e.local.reply(ctx, unique(fmt.Sprintf("@%s a reply from %s (accept %t)", p.handle(), localHost, accept)), "public", onLocal.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reply.Approval != "pending" {
			t.Errorf("the reply %s: approval = %q, want pending", reply.URI, reply.Approval)
		}
		req := peerRequestOf(t, a, "reply", original.ID, reply.URI)
		if err := a.answerInteractionRequest(ctx, req.ID, accept); err != nil {
			t.Fatal(err)
		}
		want := map[bool]string{true: "", false: "rejected"}[accept]
		eventually(t, deliveryTimeout, fmt.Sprintf("the approval of %s on %s is %q", reply.URI, localHost, want), func() (bool, string) {
			s, err := e.local.status(ctx, reply.ID)
			return err == nil && s.Approval == want, fmt.Sprintf("%q %v", s.Approval, err)
		})
		inThread := func() (bool, string) {
			th, err := p.context(ctx, original.ID)
			_, ok := findStatus(th.Descendants, reply.URI)
			return ok, fmt.Sprintf("%d descendants %v", len(th.Descendants), err)
		}
		if accept {
			eventually(t, deliveryTimeout, reply.URI+" in the thread of "+original.URI+" on "+p.host, inThread)
		} else {
			never(t, "the refused "+reply.URI+" is in the thread of "+original.URI+" on "+p.host, inThread)
		}
	}
}

// approveLocalBoost: alice's boost of a status of the peer user that needs approval waits
// (boost_approval pending, sent as an AnnounceRequest); approved, it counts on the peer; refused,
// it shows as rejected to alice and never counts.
func approveLocalBoost(t *testing.T, e *env, p *peerClient, a approver) {
	ctx := t.Context()
	for _, accept := range []bool{true, false} {
		original, err := a.postNeedingApproval(ctx, unique(fmt.Sprintf("boost me with my approval, %s (accept %t)", localHost, accept)))
		if err != nil {
			t.Fatal(err)
		}
		onLocal := localSees(t, e, original.URI)
		if err := e.local.interact(ctx, onLocal.ID, "boost", false); err != nil {
			t.Fatal(err)
		}
		if s, err := e.local.status(ctx, onLocal.ID); err != nil || s.BoostApproval != "pending" || s.Boosted {
			t.Errorf("before the answer: boost_approval %q, boosted %t, %v", s.BoostApproval, s.Boosted, err)
		}
		req := peerRequestOf(t, a, "reblog", original.ID, "")
		err = a.answerInteractionRequest(ctx, req.ID, accept)
		if accept && p.remoteBoostNotApproved != "" {
			t.Log(p.remoteBoostNotApproved)
			if !isNotFound(err) {
				t.Errorf("approving the boost of %s on %s = %v, want a 404", localHandle, p.host, err)
			}
			never(t, "the boost of "+original.URI+" is approved", func() (bool, string) {
				s, err := e.local.status(ctx, onLocal.ID)
				return err == nil && s.BoostApproval != "pending", fmt.Sprintf("boost_approval %q, %v", s.BoostApproval, err)
			})
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		want := map[bool]string{true: "", false: "rejected"}[accept]
		eventually(t, deliveryTimeout, fmt.Sprintf("the boost approval of %s on %s is %q", original.URI, localHost, want), func() (bool, string) {
			s, err := e.local.status(ctx, onLocal.ID)
			return err == nil && s.BoostApproval == want && s.Boosted == accept,
				fmt.Sprintf("boost_approval %q, boosted %t, %v", s.BoostApproval, s.Boosted, err)
		})
		counted := func() (bool, string) {
			s, err := p.status(ctx, original.ID)
			return err == nil && s.ReblogsCount == 1, fmt.Sprintf("%d %v", s.ReblogsCount, err)
		}
		if accept {
			eventually(t, deliveryTimeout, "the boost count of "+original.URI+" on "+p.host+" is 1", counted)
		} else {
			never(t, "the refused boost of "+original.URI+" counts on "+p.host, counted)
		}
	}
}

// refuseLocalQuote: GoToSocial lets only the author quote a status (canQuote), so alice may not
// quote it and this server sends nothing (ADR-0155 §7).
func refuseLocalQuote(t *testing.T, e *env, p *peerClient) {
	ctx := t.Context()
	original, err := p.post(ctx, unique("do not quote me, "+localHost), "public")
	if err != nil {
		t.Fatal(err)
	}
	onLocal := localSees(t, e, original.URI)
	if s, err := e.local.status(ctx, onLocal.ID); err != nil || s.Permissions == nil || s.Permissions.Quote != "denied" {
		t.Errorf("%s: permissions = %+v, %v; want the quote denied", original.URI, s.Permissions, err)
	}
	if q, err := e.local.quote(ctx, unique("a quote from "+localHost), onLocal.ID); err == nil {
		t.Errorf("alice quoted %s: %+v", original.URI, q)
	}
}

// withdrawLocal: alice deletes her reply that waits for its approval (Delete) and undoes her boost
// that waits (Undo), and the peer drops their requests.
func withdrawLocal(t *testing.T, e *env, p *peerClient, a approver) {
	ctx := t.Context()
	original, err := a.postNeedingApproval(ctx, unique("withdraw from me, "+localHost))
	if err != nil {
		t.Fatal(err)
	}
	onLocal := localSees(t, e, original.URI)
	reply, err := e.local.reply(ctx, unique("@"+p.handle()+" a withdrawn reply from "+localHost), "public", onLocal.ID)
	if err != nil {
		t.Fatal(err)
	}
	peerRequestOf(t, a, "reply", original.ID, reply.URI)
	if err := e.local.interact(ctx, onLocal.ID, "boost", false); err != nil {
		t.Fatal(err)
	}
	peerRequestOf(t, a, "reblog", original.ID, "")

	if err := e.local.delete(ctx, reply.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.local.interact(ctx, onLocal.ID, "boost", true); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"reply", "reblog"} {
		interaction := map[string]string{"reply": reply.URI}[kind]
		if p.keepsWithdrawnRequests != "" {
			// The peer takes the Delete and the Undo (they are delivered) but keeps listing the
			// requests.
			eventually(t, deliveryTimeout, "the withdrawals of "+localHandle+" delivered to "+p.host, func() (bool, string) {
				return withdrawalsDelivered(t, e, p.host, reply.URI)
			})
			_, ok, state := findPeerRequest(ctx, a, kind, original.ID, interaction)
			t.Logf("%s; the %s request listed afterwards: %t %s", p.keepsWithdrawnRequests, kind, ok, state)
			continue
		}
		eventually(t, deliveryTimeout, "the "+kind+" request of "+localHandle+" leaves "+p.host, func() (bool, string) {
			_, ok, state := findPeerRequest(ctx, a, kind, original.ID, interaction)
			return !ok, state
		})
	}
	if s, err := e.local.status(ctx, onLocal.ID); err != nil || s.BoostApproval != "" || s.Boosted {
		t.Errorf("after the withdrawal: boost_approval %q, boosted %t, %v", s.BoostApproval, s.Boosted, err)
	}
}

// gtsKeepsWithdrawnRequests is how GoToSocial takes the withdrawal of a request.
const gtsKeepsWithdrawnRequests = "GoToSocial 0.21 and 0.22 take the Delete of a reply that waits (they stub it) and the " +
	"Undo of a boost that waits (they delete it), but keep its interaction request in GET /api/v1/interaction_requests"

// withdrawalsDelivered reports whether the Delete of the reply with the URI and an Undo, of the
// last five minutes, were delivered (2xx) to the peer host.
func withdrawalsDelivered(t *testing.T, e *env, host, replyURI string) (bool, string) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), e.db)
	if err != nil {
		t.Fatalf("connecting to the database of %s: %v", localHost, err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var deletes, undos int
	err = conn.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE a.type = 'Delete' AND doc.document::text LIKE '%' || $1 || '%'),
		       count(*) FILTER (WHERE a.type = 'Undo')
		FROM outbound_activities a
		JOIN (SELECT outbound_activity_id, convert_from(document, 'UTF8') AS document FROM outbound_activity_documents) doc
		  ON doc.outbound_activity_id = a.id
		JOIN deliveries d ON d.outbound_activity_id = a.id
		JOIN delivery_attempts x ON x.delivery_id = d.id AND x.outcome = 'succeeded'
		WHERE a.created_at > now() - interval '5 minutes' AND d.inbox_url LIKE 'https://' || $2 || '/%'`, replyURI, host).Scan(&deletes, &undos)
	return err == nil && deletes > 0 && undos > 0, fmt.Sprintf("Delete %d, Undo %d, %v", deletes, undos, err)
}

// approvePeerReply: the peer user's reply to alice's status that needs approval waits for her
// (an interaction request on this server, out of the thread); approved, it joins the thread;
// refused, it never does, and a peer that takes the Reject (takesRejections) deletes it.
func approvePeerReply(t *testing.T, e *env, p *peerClient) {
	ctx := t.Context()
	original, err := e.local.postWithPolicy(ctx, unique("reply to me with my approval, "+p.name), approvalPolicy)
	if err != nil {
		t.Fatal(err)
	}
	onPeer, err := p.resolveStatus(ctx, original.URI)
	if err != nil {
		t.Fatal(err)
	}
	for _, accept := range []bool{true, false} {
		reply, err := p.reply(ctx, unique(fmt.Sprintf("@%s a reply from %s (accept %t)", localHandle, p.name, accept)), "public", onPeer.ID)
		if err != nil {
			t.Fatal(err)
		}
		req := localRequestOf(t, e, "reply", p, original.ID)
		if req.InteractingStatus != nil && req.InteractingStatus.URI != reply.URI {
			t.Errorf("the request %s is of %s, want %s", req.ID, req.InteractingStatus.URI, reply.URI)
		}
		inThread := func() (bool, string) {
			th, err := e.local.context(ctx, original.ID)
			_, ok := findStatus(th.Descendants, reply.URI)
			return ok, fmt.Sprintf("%d descendants %v", len(th.Descendants), err)
		}
		if ok, _ := inThread(); ok {
			t.Errorf("%s is in the thread of %s before its approval", reply.URI, original.URI)
		}
		if err := e.local.answerInteractionRequest(ctx, req.ID, accept); err != nil {
			t.Fatal(err)
		}
		if accept {
			eventually(t, deliveryTimeout, reply.URI+" in the thread of "+original.URI, inThread)
			continue
		}
		never(t, "the refused "+reply.URI+" is in the thread of "+original.URI, inThread)
		if p.takesRejections {
			eventually(t, deliveryTimeout, "the refused "+reply.URI+" is gone on "+p.host, func() (bool, string) {
				_, err := p.status(ctx, reply.ID)
				return isNotFound(err), fmt.Sprint(err)
			})
		}
	}
}

// gtsRemoteBoostNotApproved is why a GoToSocial user cannot approve alice's boost.
const gtsRemoteBoostNotApproved = "GoToSocial 0.21 and 0.22 cannot approve the boost request of a remote account: " +
	"processing/interactionrequests/accept.go acceptAnnounce checks the request's Reply instead of its Announce and " +
	"answers 404 (\"no Announce found\"), and sends no Accept"

// approvePeerBoost: the peer user's boost of alice's status that needs approval waits for her and
// does not count; approved, it counts; refused, it never does, and a peer that takes the Reject
// (takesRejections) undoes the boost there.
func approvePeerBoost(t *testing.T, e *env, p *peerClient) {
	ctx := t.Context()
	for _, accept := range []bool{true, false} {
		original, err := e.local.postWithPolicy(ctx, unique(fmt.Sprintf("boost me with my approval, %s (accept %t)", p.name, accept)), approvalPolicy)
		if err != nil {
			t.Fatal(err)
		}
		onPeer, err := p.resolveStatus(ctx, original.URI)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.interact(ctx, onPeer.ID, "boost", false); err != nil {
			t.Fatal(err)
		}
		req := localRequestOf(t, e, "announce", p, original.ID)
		counted := func() (bool, string) {
			s, err := e.local.status(ctx, original.ID)
			return err == nil && s.BoostsCount == 1, fmt.Sprintf("%d %v", s.BoostsCount, err)
		}
		if ok, state := counted(); ok {
			t.Errorf("the boost of %s counts before its approval: %s", original.URI, state)
		}
		if err := e.local.answerInteractionRequest(ctx, req.ID, accept); err != nil {
			t.Fatal(err)
		}
		if accept {
			eventually(t, deliveryTimeout, "the boost count of "+original.URI+" is 1", counted)
			continue
		}
		never(t, "the refused boost of "+original.URI+" counts", counted)
		if p.takesRejections {
			eventually(t, deliveryTimeout, "the refused boost of "+original.URI+" is undone on "+p.host, func() (bool, string) {
				s, err := p.status(ctx, onPeer.ID)
				return err == nil && !s.Reblogged, fmt.Sprintf("reblogged %t, %v", s.Reblogged, err)
			})
		}
	}
}

// withdrawPeer: the peer user deletes the reply that waits for alice's approval and undoes the
// boost that waits, and their requests leave alice's.
func withdrawPeer(t *testing.T, e *env, p *peerClient) {
	ctx := t.Context()
	original, err := e.local.postWithPolicy(ctx, unique("withdraw from me, "+p.name), approvalPolicy)
	if err != nil {
		t.Fatal(err)
	}
	onPeer, err := p.resolveStatus(ctx, original.URI)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.reply(ctx, unique("@"+localHandle+" a withdrawn reply from "+p.name), "public", onPeer.ID)
	if err != nil {
		t.Fatal(err)
	}
	localRequestOf(t, e, "reply", p, original.ID)
	if err := p.interact(ctx, onPeer.ID, "boost", false); err != nil {
		t.Fatal(err)
	}
	localRequestOf(t, e, "announce", p, original.ID)

	if err := p.delete(ctx, reply.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.interact(ctx, onPeer.ID, "boost", true); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"reply", "announce"} {
		eventually(t, deliveryTimeout, "the "+kind+" request of "+p.handle()+" leaves "+localHost, func() (bool, string) {
			_, ok, state := findLocalRequest(ctx, e, kind, p, original.ID)
			return !ok, state
		})
	}
}

// TestInteractionApprovalWithoutRequests: Mastodon reads no canReply or canAnnounce and sends its
// replies and boosts without a request (ADR-0155 §6). A reply to or a boost of alice's status that
// needs approval waits for her all the same; approved, the reply joins the thread and the boost
// counts; refused, neither does. Deleting the waiting reply and undoing the waiting boost withdraw
// them. Quotes go through Mastodon's QuoteRequest (ADR-0069), in both directions.
func TestInteractionApprovalWithoutRequests(t *testing.T) {
	e := stack(t)
	var p *peerClient
	for _, peer := range e.peers {
		if peer.name == "mastodon" {
			p = peer
		}
	}
	m, ok := p.peerAPI.(*mastodonAPI)
	if p == nil || !ok {
		t.Fatal("the stack has no Mastodon")
	}
	t.Run("reply from peer", func(t *testing.T) { approvePeerReply(t, e, p) })
	t.Run("boost from peer", func(t *testing.T) { approvePeerBoost(t, e, p) })
	t.Run("withdrawn from peer", func(t *testing.T) { withdrawPeer(t, e, p) })

	t.Run("quote from peer", func(t *testing.T) {
		ctx := t.Context()
		original, err := e.local.postWithPolicy(ctx, unique("quote me with my approval, "+p.name), approvalPolicy)
		if err != nil {
			t.Fatal(err)
		}
		onPeer, err := p.resolveStatus(ctx, original.URI)
		if err != nil {
			t.Fatal(err)
		}
		for _, accept := range []bool{true, false} {
			q, err := m.quote(ctx, unique(fmt.Sprintf("a quote from %s (accept %t)", p.name, accept)), onPeer.ID)
			if err != nil {
				t.Fatal(err)
			}
			req := localRequestOf(t, e, "quote", p, original.ID)
			if err := e.local.answerInteractionRequest(ctx, req.ID, accept); err != nil {
				t.Fatal(err)
			}
			want := map[bool]string{true: "accepted", false: "rejected"}[accept]
			eventually(t, deliveryTimeout, "the quote state of "+q.URI+" on "+p.host+" is "+want, func() (bool, string) {
				s, err := p.status(ctx, q.ID)
				return err == nil && s.Quote != nil && s.Quote.State == want, fmt.Sprintf("%+v %v", s.Quote, err)
			})
		}
	})

	t.Run("quote from local", func(t *testing.T) {
		ctx := t.Context()
		followLocalToPeer(t, e, p)
		// Mastodon lets everyone quote a public status at once by default.
		original, err := p.post(ctx, unique("quote me, "+localHost), "public")
		if err != nil {
			t.Fatal(err)
		}
		onLocal := localSees(t, e, original.URI)
		q, err := e.local.quote(ctx, unique("a quote from "+localHost), onLocal.ID)
		if err != nil {
			t.Fatal(err)
		}
		eventually(t, deliveryTimeout, "the quote state of "+q.URI+" on "+localHost+" is accepted", func() (bool, string) {
			s, err := e.local.status(ctx, q.ID)
			return err == nil && s.Quote != nil && s.Quote.State == "accepted", fmt.Sprintf("%+v %v", s.Quote, err)
		})
		eventually(t, deliveryTimeout, "the quote "+q.URI+" on "+p.host, func() (bool, string) {
			s, err := p.resolveStatus(ctx, q.URI)
			return err == nil && s.Quote != nil && s.Quote.State == "accepted" && s.Quote.QuotedStatus != nil &&
				s.Quote.QuotedStatus.URI == original.URI, fmt.Sprintf("%+v %v", s.Quote, err)
		})
	})
}
