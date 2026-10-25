// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/notification"
)

// RemoteFollowHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the repositories of the modules that follows with remote accounts touch.
type RemoteFollowHarness struct {
	FollowHarness
	Notifications appnotification.Repository
	Federation    appfederation.Repository
	Inbox         appfederation.InboxRepository
}

// RemoteFollowFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type RemoteFollowFactory func(t *testing.T, opts uow.Options) RemoteFollowHarness

// RemoteFollowContract verifies follows between local and remote accounts (federation.md §3.4,
// §7.1) on a database adapter, through the real account, graph, notification, and federation
// use cases and projectors: the Follow and Accept documents of Mastodon, Misskey, and
// GoToSocial set up a follow in both directions, and the Follow, Accept, Reject, and Undo
// recorded and delivered back carry the follow's URI; a locked account holds a remote follow
// as a request with a notification until it answers; Reject and Undo end follows, by URI or by
// the pair they name; a blocked follower gets a Reject; a Follow sent again for an active follow
// gets its own Accept without a new follow; an Undo that arrives before its Follow, even one
// that names the follow by the pair (Misskey), drops that Follow; and the final follows do not depend on
// the order or repetition of the activities (NFR-REL-1). Remote actors are stored accounts,
// and nothing is fetched or sent over the network.
func RemoteFollowContract(t *testing.T, newHarness RemoteFollowFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *remoteFollowEnv)
	}{
		{"FollowsPeersBothWays", remoteFollowsPeers},
		{"HoldsFollowsOfLockedAccounts", remoteFollowsLocked},
		{"EndsFollowsByRejectAndUndo", remoteFollowsEnd},
		{"RejectsBlockedFollowers", remoteFollowsBlocked},
		{"AcceptsAFollowSentAgain", remoteFollowsResent},
		{"DropsAFollowAfterAnEarlyUndo", remoteFollowsEarlyUndo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRemoteFollowEnv(t, newHarness)) })
	}
	t.Run("OrderAndDuplicates", func(t *testing.T) { remoteFollowOrder(t, newHarness) })
}

type remoteFollowEnv struct {
	*accountEnv
	h         RemoteFollowHarness
	graph     *appgraph.Service
	notifs    *appnotification.Service
	processor *appfederation.InboxProcessor
	jobs      *fedJobs
	handlers  *queue.JobHandlerRegistry
	// remotes stores remote actors; it has the HTML sanitizer that their profiles go through.
	remotes *appaccount.Service
}

func newRemoteFollowEnv(t *testing.T, newHarness RemoteFollowFactory) *remoteFollowEnv {
	t.Helper()
	var (
		h        RemoteFollowHarness
		captured uow.Options
	)
	ae := newAccountEnv(t, func(t *testing.T, opts uow.Options) AccountHarness {
		captured = opts
		h = newHarness(t, opts)
		return h.AccountHarness
	})
	e := &remoteFollowEnv{accountEnv: ae, h: h, jobs: &fedJobs{seen: map[string]bool{}}, handlers: &queue.JobHandlerRegistry{}}
	projectors := captured.Projectors
	e.remotes = appaccount.NewService(appaccount.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Accounts, Clock: ae.clock, IDs: ae.ids,
		Sealer: kernel.NewSealer(NewKeyManager("remote-follow")), Host: contractHost, Sanitizer: remoteSanitizer{},
	})
	e.graph = appgraph.NewService(appgraph.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Graph, Accounts: ae.accounts, Clock: ae.clock, IDs: ae.ids, Host: contractHost,
	})
	appgraph.RegisterProjectors(projectors, h.Graph)
	appnotification.RegisterProjectors(projectors, appnotification.ProjectorDeps{
		Repository: h.Notifications, Accounts: ae.accounts, Relationships: e.graph, IDs: ae.ids,
	})
	e.notifs = appnotification.NewService(appnotification.Deps{UnitOfWork: h.UnitOfWork, Repository: h.Notifications, Clock: ae.clock})
	outbox := queue.AfterCommit(projectors, e.jobs, nil)
	appfederation.RegisterOutboundProjectors(projectors, appfederation.RecorderDeps{
		Repository: h.Federation, Outbox: outbox, IDs: ae.ids, Host: contractHost, Accounts: ae.accounts,
	})
	appfederation.NewFanOut(appfederation.FanOutDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Federation, Outbox: outbox,
		Accounts: ae.accounts, Graph: e.graph, Follows: e.graph, Moves: ae.accounts,
		Clock: ae.clock, IDs: ae.ids, Host: contractHost,
	}).RegisterJobs(e.handlers)
	e.processor = appfederation.NewInboxProcessor(appfederation.InboxProcessorDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Inbox, Actors: storedActors{ae.accounts},
		Fetcher: &inboxFetcher{clock: ae.clock, docs: map[string][]byte{}, errs: map[string]error{}}, Clock: ae.clock,
	})
	appfederation.RegisterFollowHandlers(e.processor, appfederation.FollowHandlerDeps{
		Graph: e.graph, Accounts: ae.accounts, Moves: ae.accounts,
		Refusals: appfederation.NewFollowRefusals(appfederation.FollowRefusalsDeps{
			UnitOfWork: h.UnitOfWork, Outbound: h.Federation, Outbox: outbox, Inboxes: ae.accounts,
			Clock: ae.clock, IDs: ae.ids, Host: contractHost,
		}),
	})
	return e
}

// storedActors resolves the actors of activities among the stored accounts, without fetching.
type storedActors struct{ accounts *appaccount.Service }

func (s storedActors) Resolve(ctx context.Context, uri string) (appaccount.Actor, error) {
	a, err := s.accounts.FindByURI(ctx, uri)
	if err != nil {
		return appaccount.Actor{}, fmt.Errorf("%w: %s: %w", appfederation.ErrFetchNotFound, uri, err)
	}
	return a, nil
}

// remote stores the remote actor uri, with its inbox at uri/inbox, and returns its account ID.
func (e *remoteFollowEnv) remote(t *testing.T, uri string) string {
	t.Helper()
	ctx := t.Context()
	iri := federation.IRI(uri)
	var a account.Account
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		a, err = e.remotes.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: uri, Domain: iri.Host(), Type: "Person", PreferredUsername: uri[strings.LastIndex(uri, "/")+1:],
			InboxURL: uri + "/inbox", Document: []byte(`{"id":"` + uri + `"}`),
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor(%s): %v", uri, err)
	}
	return a.ID
}

// actorURI returns the ActivityPub id of a local account.
func (e *remoteFollowEnv) actorURI(id string) string { return appaccount.ActorURI(contractHost, id) }

// record records doc as the synchronous part of the inbox does, signed by its actor, and
// returns its activity URI.
func (e *remoteFollowEnv) record(t *testing.T, doc []byte) federation.IRI {
	t.Helper()
	ctx := t.Context()
	env, err := federation.ReadEnvelope(doc, appfederation.CanonicalHost)
	if err != nil {
		t.Fatalf("ReadEnvelope(%s): %v", doc, err)
	}
	e.clock.Advance(time.Second)
	a := federation.ReceivedActivity{
		ActivityURI: env.ActivityURI(doc), ActorURI: env.Actor, Type: env.Type, Scheme: "cavage", Trust: federation.TrustSigned,
		ReceivedAt: e.clock.Now(),
	}
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := e.h.Inbox.InsertReceivedActivity(ctx, tx, a)
		if err != nil || !ok {
			return err
		}
		return e.h.Inbox.InsertReceivedDocument(ctx, tx, a.ActivityURI, doc)
	})
	if err != nil {
		t.Fatalf("record %s: %v", a.ActivityURI, err)
	}
	return a.ActivityURI
}

// job runs the federation.inbox job of a recorded activity, which must complete.
func (e *remoteFollowEnv) job(t *testing.T, uri federation.IRI) {
	t.Helper()
	nj, err := appfederation.InboxJob(federation.ReceivedActivity{ActivityURI: uri, Trust: federation.TrustSigned})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	if r := e.processor.HandleJob(t.Context(), queue.Job{ID: NamedID("job"), Type: nj.Type, Payload: nj.Payload, IdempotencyKey: nj.IdempotencyKey}); !r.IsDone() {
		t.Fatalf("job of %s = %+v (%v), want Done", uri, r, r.Err())
	}
}

// receive records and processes doc and returns the outcome.
func (e *remoteFollowEnv) receive(t *testing.T, doc []byte) appfederation.InboxOutcome {
	t.Helper()
	uri := e.record(t, doc)
	e.clock.Advance(time.Second)
	got, err := e.processor.Process(t.Context(), uri, federation.TrustSigned)
	if err != nil {
		t.Fatalf("Process(%s): %v", uri, err)
	}
	return got
}

// relationship returns how account sees target.
func (e *remoteFollowEnv) relationship(t *testing.T, account, target string) appgraph.Relationship {
	t.Helper()
	r, err := e.graph.Relationship(t.Context(), account, target)
	if err != nil {
		t.Fatalf("Relationship(%s, %s): %v", account, target, err)
	}
	return r
}

// liveFollow returns the pending or active follow from follower to followee.
func (e *remoteFollowEnv) liveFollow(t *testing.T, follower, followee string) graph.Follow {
	t.Helper()
	edge, err := e.h.Graph.FindEdge(t.Context(), follower, followee)
	if err != nil {
		t.Fatalf("FindEdge(%s, %s): %v", follower, followee, err)
	}
	f, err := e.graph.FindFollow(t.Context(), edge.FollowID)
	if err != nil {
		t.Fatalf("FindFollow(%s): %v", edge.FollowID, err)
	}
	return f
}

// followOf returns the follow of the Follow activity uri.
func (e *remoteFollowEnv) followOf(t *testing.T, uri string) graph.Follow {
	t.Helper()
	var f graph.Follow
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		f, err = e.h.Graph.FollowByURI(t.Context(), tx, uri)
		return err
	})
	if err != nil {
		t.Fatalf("FollowByURI(%s): %v", uri, err)
	}
	return f
}

// sent drains the fan-out and returns the activities recorded about a follow, oldest first,
// as type → the document and the inboxes it goes to.
func (e *remoteFollowEnv) sent(t *testing.T, followID string) []remoteSent {
	t.Helper()
	return e.sentOf(t, federation.OutboundFollow, followID)
}

// sentOf is sent for the activities about the object id of kind.
func (e *remoteFollowEnv) sentOf(t *testing.T, kind federation.OutboundObjectKind, id string) []remoteSent {
	t.Helper()
	ctx := t.Context()
	fanOut, _, ok := e.handlers.Lookup(appfederation.JobFanOut)
	if !ok {
		t.Fatal("no handler for " + appfederation.JobFanOut)
	}
	for {
		nj, ok := e.jobs.pop()
		if !ok {
			break
		}
		if nj.Type != appfederation.JobFanOut {
			continue
		}
		j, err := nj.Build(e.ids.NewID(), e.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if r := fanOut(ctx, j); !r.IsDone() {
			t.Fatalf("fan-out %s = %+v (%v), want Done", j.Payload, r, r.Err())
		}
	}
	acts, err := e.h.Federation.ListOutboundActivitiesOf(ctx, kind, id)
	if err != nil {
		t.Fatalf("ListOutboundActivitiesOf: %v", err)
	}
	out := []remoteSent{}
	for _, a := range acts {
		s := remoteSent{Type: a.Type}
		b, err := e.h.Federation.FindOutboundDocument(ctx, a.ID)
		if err != nil {
			t.Fatalf("FindOutboundDocument(%s): %v", a.ID, err)
		}
		if err := json.Unmarshal(b, &s.Document); err != nil {
			t.Fatal(err)
		}
		ds, err := e.h.Federation.ListDeliveries(ctx, a.ID, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			s.Inboxes = append(s.Inboxes, string(d.InboxURL))
		}
		out = append(out, s)
	}
	return out
}

type remoteSent struct {
	Type     federation.ActivityType
	Document map[string]any
	Inboxes  []string
}

// types returns the types of sent, in order.
func types(sent []remoteSent) []federation.ActivityType {
	out := []federation.ActivityType{}
	for _, s := range sent {
		out = append(out, s.Type)
	}
	return out
}

// embeddedFollowOf returns the Follow that an activity document embeds.
func embeddedFollowOf(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	f, ok := doc["object"].(map[string]any)
	if !ok || f["type"] != "Follow" {
		t.Fatalf("object of %v is not a Follow", doc)
	}
	return f
}

// remotePeer is the follow documents of one implementation, with the IRIs to replace in them:
// the peer's actor (and its host, which the fixtures may write with http), the local actor it
// talks to, and the id of the local Follow that its Accept answers.
type remotePeer struct {
	name, version string
	// hostFrom → hostTo rewrites the peer's origin to an https one.
	hostFrom, hostTo string
	actor            string // the peer's actor, after the rewrite
	local            string // the actor of the receiving side in the fixtures
	acceptedFollow   string // the id of the Follow the Accept answers
}

var remotePeers = []remotePeer{
	{
		name: "mastodon", version: "circa-2018", hostFrom: "http://mastodon.example.org", hostTo: "https://mastodon.example.org",
		actor: "https://mastodon.example.org/users/admin", local: "http://remote.example/users/bob",
		acceptedFollow: "http://remote.example/users/bob#follows/4",
	},
	{
		name: "misskey", version: "2025.4", hostFrom: "https://misskey.example", hostTo: "https://misskey.example",
		actor: "https://misskey.example/users/9tjlknm0fl", local: "https://remote.example/users/bob",
		acceptedFollow: "https://remote.example/users/bob#follows/4",
	},
	{
		name: "gotosocial", version: "0.22", hostFrom: "https://gotosocial.example", hostTo: "https://gotosocial.example",
		actor: "https://gotosocial.example/users/the_mighty_zork", local: "https://remote.example/users/bob",
		acceptedFollow: "https://remote.example/users/bob#follows/4",
	},
}

// fixture returns the peer's fixture name with the local actor replaced by localURI, the
// accepted Follow by followURI, and the peer's origin made https.
func (p remotePeer) fixture(t *testing.T, name, localURI, followURI string) []byte {
	t.Helper()
	fixtures, err := FederationFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		if f.Implementation != p.name || f.Version != p.version || f.Name != name {
			continue
		}
		s := string(f.JSON)
		if followURI != "" {
			s = strings.ReplaceAll(s, `"`+p.acceptedFollow+`"`, `"`+followURI+`"`)
		}
		s = strings.ReplaceAll(s, `"`+p.local+`"`, `"`+localURI+`"`)
		s = strings.ReplaceAll(s, p.hostFrom, p.hostTo)
		return []byte(s)
	}
	t.Fatalf("no fixture %s/%s/%s", p.name, p.version, name)
	return nil
}

// renamed returns doc with its top-level id followed by suffix, to receive a fixture again as
// another activity.
func renamed(t *testing.T, doc []byte, suffix string) []byte {
	t.Helper()
	id := activityID(t, doc)
	return []byte(strings.Replace(string(doc), `"id": "`+id+`"`, `"id": "`+id+suffix+`"`, 1))
}

// activityID returns the id of an activity document.
func activityID(t *testing.T, doc []byte) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(doc, &v); err != nil {
		t.Fatal(err)
	}
	return v.ID
}

func remoteFollowsPeers(t *testing.T, e *remoteFollowEnv) {
	ctx := t.Context()
	for _, p := range remotePeers {
		t.Run(p.name, func(t *testing.T) {
			alice := e.register(t, "alice_"+p.name).AccountID
			peer := e.remote(t, p.actor)
			inbox := []string{p.actor + "/inbox"}

			// The peer follows alice; she is not locked, so the follow is active and Accepted.
			follow := p.fixture(t, "follow", e.actorURI(alice), "")
			if got := e.receive(t, follow); got != appfederation.InboxHandled {
				t.Fatalf("Follow = %q, want handled", got)
			}
			if r := e.relationship(t, alice, peer); r.FollowedBy != graph.StateActive {
				t.Fatalf("FollowedBy = %q after the Follow, want active", r.FollowedBy)
			}
			incoming := e.followOf(t, activityID(t, follow))
			sent := e.sent(t, incoming.ID)
			if !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityAccept}) {
				t.Fatalf("sent %v about the peer's follow, want one Accept", types(sent))
			}
			accept := sent[0]
			if accept.Document["actor"] != e.actorURI(alice) || !reflect.DeepEqual(accept.Inboxes, inbox) {
				t.Errorf("Accept by %v to %v, want by alice to %v", accept.Document["actor"], accept.Inboxes, inbox)
			}
			if f := embeddedFollowOf(t, accept.Document); f["id"] != activityID(t, follow) || f["actor"] != p.actor || f["object"] != e.actorURI(alice) {
				t.Errorf("Accept embeds %v, want the peer's Follow %s", f, activityID(t, follow))
			}
			// The same Follow again changes nothing and sends nothing.
			if got := e.receive(t, append(follow, ' ')); got != appfederation.InboxHandled {
				t.Errorf("Follow again = %q, want handled", got)
			}
			if again := e.sent(t, incoming.ID); len(again) != 1 {
				t.Errorf("sent %v after the Follow again, want the one Accept", types(again))
			}

			// Alice follows the peer: pending until its Accept.
			if _, err := e.graph.Follow(ctx, alice, peer); err != nil {
				t.Fatalf("Follow(alice, peer): %v", err)
			}
			if r := e.relationship(t, alice, peer); r.Following != graph.StatePending {
				t.Fatalf("Following = %q before the Accept, want pending", r.Following)
			}
			outgoing := e.liveFollow(t, alice, peer)
			sent = e.sent(t, outgoing.ID)
			if !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityFollow}) {
				t.Fatalf("sent %v about alice's follow, want one Follow", types(sent))
			}
			if d := sent[0].Document; d["id"] != outgoing.URI || d["actor"] != e.actorURI(alice) || d["object"] != p.actor || !reflect.DeepEqual(sent[0].Inboxes, inbox) {
				t.Errorf("Follow = %v to %v, want %s of the peer by alice to %v", d, sent[0].Inboxes, outgoing.URI, inbox)
			}
			if got := e.receive(t, p.fixture(t, "accept-follow", e.actorURI(alice), outgoing.URI)); got != appfederation.InboxHandled {
				t.Fatalf("Accept = %q, want handled", got)
			}
			if r := e.relationship(t, alice, peer); r.Following != graph.StateActive || r.FollowedBy != graph.StateActive {
				t.Errorf("relationship = %+v after the Accept, want following each other", r)
			}
			if again := e.sent(t, outgoing.ID); len(again) != 1 {
				t.Errorf("sent %v after the Accept, want nothing more", types(again))
			}
			if c, _ := e.graph.Counters(ctx, alice); c.FollowersCount != 1 || c.FollowingCount != 1 {
				t.Errorf("counters of alice = %+v, want 1 and 1", c)
			}
		})
	}
}

func remoteFollowsLocked(t *testing.T, e *remoteFollowEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").AccountID
	if _, err := e.accounts.ReviseProfile(ctx, alice, appaccount.ProfileInput{DisplayName: "alice", Locked: true}); err != nil {
		t.Fatal(err)
	}
	mastodon, misskey := remotePeers[0], remotePeers[1]
	peer, other := e.remote(t, mastodon.actor), e.remote(t, misskey.actor)

	follow := mastodon.fixture(t, "follow", e.actorURI(alice), "")
	e.receive(t, follow)
	if r := e.relationship(t, alice, peer); r.FollowedBy != graph.StatePending {
		t.Fatalf("FollowedBy = %q, want a pending request", r.FollowedBy)
	}
	ns, err := e.notifs.List(ctx, alice, appnotification.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 || ns[0].Type != notification.TypeFollowRequest || ns[0].SourceAccountID != peer {
		t.Errorf("notifications = %+v, want a follow request from the peer", ns)
	}
	f := e.followOf(t, activityID(t, follow))
	if sent := e.sent(t, f.ID); len(sent) != 0 {
		t.Errorf("sent %v before alice answered, want nothing", types(sent))
	}
	if _, err := e.graph.AcceptFollowRequest(ctx, alice, peer); err != nil {
		t.Fatal(err)
	}
	if sent := e.sent(t, f.ID); !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityAccept}) ||
		embeddedFollowOf(t, sent[0].Document)["id"] != activityID(t, follow) {
		t.Errorf("sent %v after alice accepted, want an Accept of the peer's Follow", types(sent))
	}

	follow = misskey.fixture(t, "follow", e.actorURI(alice), "")
	e.receive(t, follow)
	if _, err := e.graph.RejectFollowRequest(ctx, alice, other); err != nil {
		t.Fatal(err)
	}
	f = e.followOf(t, activityID(t, follow))
	sent := e.sent(t, f.ID)
	if !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityReject}) ||
		embeddedFollowOf(t, sent[0].Document)["id"] != activityID(t, follow) || sent[0].Document["actor"] != e.actorURI(alice) {
		t.Errorf("sent %v after alice rejected, want a Reject of the peer's Follow by alice", types(sent))
	}
	if r := e.relationship(t, alice, other); r.FollowedBy != graph.StateNone {
		t.Errorf("FollowedBy = %q after the rejection, want none", r.FollowedBy)
	}
}

// reject returns the Accept fixture of p turned into a Reject.
func (p remotePeer) reject(t *testing.T, localURI, followURI string) []byte {
	t.Helper()
	doc := p.fixture(t, "accept-follow", localURI, followURI)
	return []byte(strings.ReplaceAll(strings.Replace(string(doc), `"Accept"`, `"Reject"`, 1), "accept", "reject"))
}

func remoteFollowsEnd(t *testing.T, e *remoteFollowEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").AccountID
	mastodon, misskey := remotePeers[0], remotePeers[1]
	mPeer, kPeer := e.remote(t, mastodon.actor), e.remote(t, misskey.actor)

	// An Undo by the Follow's URI, and Misskey's Undo, which names the follow by the pair.
	for _, c := range []struct {
		p    remotePeer
		peer string
	}{{mastodon, mPeer}, {misskey, kPeer}} {
		e.receive(t, c.p.fixture(t, "follow", e.actorURI(alice), ""))
		if got := e.receive(t, c.p.fixture(t, "undo-follow", e.actorURI(alice), "")); got != appfederation.InboxHandled {
			t.Errorf("%s: Undo = %q, want handled", c.p.name, got)
		}
		if r := e.relationship(t, alice, c.peer); r.FollowedBy != graph.StateNone {
			t.Errorf("%s: FollowedBy = %q after the Undo, want none", c.p.name, r.FollowedBy)
		}
	}
	if c, _ := e.graph.Counters(ctx, alice); c.FollowersCount != 0 {
		t.Errorf("followers of alice = %d after the Undos, want 0", c.FollowersCount)
	}
	// An Undo before its Follow leaves a tombstone that drops the Follow.
	gts := remotePeers[2]
	gPeer := e.remote(t, gts.actor)
	follow := gts.fixture(t, "follow", e.actorURI(alice), "")
	undo := fmt.Appendf(nil, `{"@context":"https://www.w3.org/ns/activitystreams","id":"%s/undo","type":"Undo","actor":"%s","object":%s}`,
		activityID(t, follow), gts.actor, follow)
	if got := e.receive(t, undo); got != appfederation.InboxTombstoned {
		t.Errorf("Undo before its Follow = %q, want tombstoned", got)
	}
	if got := e.receive(t, follow); got != appfederation.InboxDropped {
		t.Errorf("Follow after its Undo = %q, want dropped", got)
	}
	if r := e.relationship(t, alice, gPeer); r.FollowedBy != graph.StateNone {
		t.Errorf("FollowedBy = %q, want none", r.FollowedBy)
	}

	// A Reject of a pending follow, and of an active one; neither sends anything back.
	for _, accepted := range []bool{false, true} {
		if _, err := e.graph.Follow(ctx, alice, mPeer); err != nil {
			t.Fatal(err)
		}
		f := e.liveFollow(t, alice, mPeer)
		suffix := fmt.Sprintf("-%t", accepted)
		if accepted {
			e.receive(t, renamed(t, mastodon.fixture(t, "accept-follow", e.actorURI(alice), f.URI), suffix))
		}
		if got := e.receive(t, renamed(t, mastodon.reject(t, e.actorURI(alice), f.URI), suffix)); got != appfederation.InboxHandled {
			t.Errorf("Reject (accepted %t) = %q, want handled", accepted, got)
		}
		if r := e.relationship(t, alice, mPeer); r.Following != graph.StateNone {
			t.Errorf("Following = %q after the Reject (accepted %t), want none", r.Following, accepted)
		}
		if sent := e.sent(t, f.ID); !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityFollow}) {
			t.Errorf("sent %v about a rejected follow (accepted %t), want only its Follow", types(sent), accepted)
		}
	}
	// An Accept of no known follow does nothing.
	if got := e.receive(t, renamed(t, mastodon.fixture(t, "accept-follow", e.actorURI(alice), "https://"+contractHost+"/ap/activities/unknown"), "-unknown")); got != appfederation.InboxDropped {
		t.Errorf("Accept of an unknown follow = %q, want dropped", got)
	}

	// Alice unfollows: Undo of her Follow.
	if _, err := e.graph.Follow(ctx, alice, kPeer); err != nil {
		t.Fatal(err)
	}
	f := e.liveFollow(t, alice, kPeer)
	e.receive(t, misskey.fixture(t, "accept-follow", e.actorURI(alice), f.URI))
	if _, err := e.graph.Unfollow(ctx, alice, kPeer); err != nil {
		t.Fatal(err)
	}
	sent := e.sent(t, f.ID)
	if !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityFollow, federation.ActivityUndo}) {
		t.Fatalf("sent %v about alice's follow, want Follow and Undo", types(sent))
	}
	if d := sent[1].Document; d["actor"] != e.actorURI(alice) || embeddedFollowOf(t, d)["id"] != f.URI || !reflect.DeepEqual(sent[1].Inboxes, []string{misskey.actor + "/inbox"}) {
		t.Errorf("Undo = %v to %v, want alice's Undo of %s to the peer", d, sent[1].Inboxes, f.URI)
	}
	// An Undo of the Accept by the followee ends a follow too.
	if _, err := e.graph.Follow(ctx, alice, mPeer); err != nil {
		t.Fatal(err)
	}
	f = e.liveFollow(t, alice, mPeer)
	accept := renamed(t, mastodon.fixture(t, "accept-follow", e.actorURI(alice), f.URI), "-undone")
	e.receive(t, accept)
	undoAccept := fmt.Appendf(nil, `{"@context":"https://www.w3.org/ns/activitystreams","id":"%s/undo","type":"Undo","actor":"%s","object":%s}`,
		activityID(t, accept), mastodon.actor, accept)
	if got := e.receive(t, undoAccept); got != appfederation.InboxHandled {
		t.Errorf("Undo of the Accept = %q, want handled", got)
	}
	if r := e.relationship(t, alice, mPeer); r.Following != graph.StateNone {
		t.Errorf("Following = %q after the Undo of the Accept, want none", r.Following)
	}
}

func remoteFollowsBlocked(t *testing.T, e *remoteFollowEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").AccountID
	mastodon := remotePeers[0]
	peer := e.remote(t, mastodon.actor)
	// The remote side of blocks arrives with Block activities; record one directly.
	b := graph.Block{ID: e.ids.NewID(), URI: mastodon.actor + "#blocks/1", BlockerAccountID: peer, BlockedAccountID: alice, CreatedAt: e.clock.Now()}
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.h.Graph.InsertBlock(ctx, tx, b); err != nil {
			return err
		}
		return tx.Emit(ctx, appgraph.Blocked{Block: b})
	})
	if err != nil {
		t.Fatal(err)
	}
	follow := mastodon.fixture(t, "follow", e.actorURI(alice), "")
	if got := e.receive(t, follow); got != appfederation.InboxHandled {
		t.Fatalf("Follow = %q, want handled", got)
	}
	if r := e.relationship(t, alice, peer); r.FollowedBy != graph.StateNone {
		t.Errorf("FollowedBy = %q across a block, want none", r.FollowedBy)
	}
	f := e.followOf(t, activityID(t, follow))
	if sent := e.sent(t, f.ID); !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityReject}) {
		t.Errorf("sent %v for a blocked follower, want a Reject", types(sent))
	}
}

// histories returns the number of follows recorded.
func (e *remoteFollowEnv) histories(t *testing.T) int {
	t.Helper()
	var n int
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		hs, err := e.h.Graph.ListHistories(t.Context(), tx)
		n = len(hs)
		return err
	})
	if err != nil {
		t.Fatalf("ListHistories: %v", err)
	}
	return n
}

// remoteFollowsResent checks a Follow that the follower's server sends again, under a new id,
// for a follow that is already active (Mastodon does until it gets an Accept): its Accept is
// sent again, and the follow keeps its one row.
func remoteFollowsResent(t *testing.T, e *remoteFollowEnv) {
	alice := e.register(t, "alice").AccountID
	mastodon := remotePeers[0]
	peer := e.remote(t, mastodon.actor)
	follow := mastodon.fixture(t, "follow", e.actorURI(alice), "")
	e.receive(t, follow)
	f := e.followOf(t, activityID(t, follow))

	again := renamed(t, follow, "-again")
	for i := range 2 {
		if got := e.receive(t, again); got != appfederation.InboxHandled {
			t.Fatalf("Follow sent again (run %d) = %q, want handled", i+1, got)
		}
	}
	sent := e.sent(t, f.ID)
	if !reflect.DeepEqual(types(sent), []federation.ActivityType{federation.ActivityAccept, federation.ActivityAccept}) {
		t.Fatalf("sent %v about the follow, want an Accept per Follow", types(sent))
	}
	if got := embeddedFollowOf(t, sent[1].Document)["id"]; got != activityID(t, again) {
		t.Errorf("the second Accept embeds %v, want the Follow sent again %s", got, activityID(t, again))
	}
	if sent[1].Document["id"] == sent[0].Document["id"] || !reflect.DeepEqual(sent[1].Inboxes, []string{mastodon.actor + "/inbox"}) {
		t.Errorf("the second Accept is %v to %v, want a new Accept to the peer", sent[1].Document["id"], sent[1].Inboxes)
	}
	if n := e.histories(t); n != 1 {
		t.Errorf("%d follows recorded, want 1", n)
	}
	if got := e.liveFollow(t, peer, alice); got.ID != f.ID {
		t.Errorf("live follow %s, want %s", got.ID, f.ID)
	}
	if c, _ := e.graph.Counters(t.Context(), alice); c.FollowersCount != 1 {
		t.Errorf("followers of alice = %d, want 1", c.FollowersCount)
	}
	// An Undo of the Follow sent again, by its id alone, ends the follow.
	undo := fmt.Appendf(nil, `{"@context":"https://www.w3.org/ns/activitystreams","id":"%s/undo","type":"Undo","actor":"%s","object":"%s"}`,
		activityID(t, again), mastodon.actor, activityID(t, again))
	if got := e.receive(t, undo); got != appfederation.InboxHandled {
		t.Errorf("Undo of the Follow sent again = %q, want handled", got)
	}
	if r := e.relationship(t, alice, peer); r.FollowedBy != graph.StateNone {
		t.Errorf("FollowedBy = %q after the Undo, want none", r.FollowedBy)
	}

	// A Follow sent again for a pending follow request changes nothing until alice answers.
	bob := e.register(t, "bob").AccountID
	if _, err := e.accounts.ReviseProfile(t.Context(), bob, appaccount.ProfileInput{DisplayName: "bob", Locked: true}); err != nil {
		t.Fatal(err)
	}
	request := renamed(t, mastodon.fixture(t, "follow", e.actorURI(bob), ""), "-bob")
	e.receive(t, request)
	if got := e.receive(t, renamed(t, request, "-again")); got != appfederation.InboxHandled {
		t.Errorf("Follow request sent again = %q, want handled", got)
	}
	r := e.followOf(t, activityID(t, request))
	if sent := e.sent(t, r.ID); len(sent) != 0 {
		t.Errorf("sent %v about a pending request, want nothing", types(sent))
	}
	if n := e.histories(t); n != 2 {
		t.Errorf("%d follows recorded, want 2", n)
	}
}

// remoteFollowsEarlyUndo checks Misskey's Undo arriving before its Follow: the Undo names the
// follow by the pair, with an id other than the Follow's, and the Follow arriving later is
// dropped. The next Follow of the pair goes through.
func remoteFollowsEarlyUndo(t *testing.T, e *remoteFollowEnv) {
	alice := e.register(t, "alice").AccountID
	misskey := remotePeers[1]
	peer := e.remote(t, misskey.actor)
	follow := misskey.fixture(t, "follow", e.actorURI(alice), "")
	if got := e.receive(t, misskey.fixture(t, "undo-follow", e.actorURI(alice), "")); got != appfederation.InboxTombstoned {
		t.Errorf("Undo before its Follow = %q, want tombstoned", got)
	}
	if got := e.receive(t, follow); got != appfederation.InboxDropped {
		t.Errorf("Follow after its Undo = %q, want dropped", got)
	}
	if r := e.relationship(t, alice, peer); r.FollowedBy != graph.StateNone {
		t.Errorf("FollowedBy = %q, want none", r.FollowedBy)
	}
	if n := e.histories(t); n != 0 {
		t.Errorf("%d follows recorded, want none", n)
	}
	later := renamed(t, follow, "-later")
	if got := e.receive(t, later); got != appfederation.InboxHandled {
		t.Errorf("a later Follow = %q, want handled", got)
	}
	if r := e.relationship(t, alice, peer); r.FollowedBy != graph.StateActive {
		t.Errorf("FollowedBy = %q after a later Follow, want active", r.FollowedBy)
	}
}

// remoteOrderCase is a set of activities about follows whose final follows must not depend on
// the order or repetition of their jobs.
type remoteOrderCase struct {
	name string
	// setUp prepares the env and returns the activities and the pair to check: from follower to
	// followee.
	setUp func(t *testing.T, e *remoteFollowEnv) (docs [][]byte, follower, followee string)
	want  graph.State
}

var remoteOrderCases = []remoteOrderCase{
	{
		name: "FollowAndUndo",
		setUp: func(t *testing.T, e *remoteFollowEnv) ([][]byte, string, string) {
			p := remotePeers[0]
			alice, peer := e.register(t, "alice").AccountID, e.remote(t, p.actor)
			return [][]byte{p.fixture(t, "follow", e.actorURI(alice), ""), p.fixture(t, "undo-follow", e.actorURI(alice), "")}, peer, alice
		},
		want: graph.StateNone,
	},
	{
		// Misskey's Undo names the follow by the pair, not by the id of its Follow.
		name: "MisskeyFollowAndUndo",
		setUp: func(t *testing.T, e *remoteFollowEnv) ([][]byte, string, string) {
			p := remotePeers[1]
			alice, peer := e.register(t, "alice").AccountID, e.remote(t, p.actor)
			return [][]byte{p.fixture(t, "follow", e.actorURI(alice), ""), p.fixture(t, "undo-follow", e.actorURI(alice), "")}, peer, alice
		},
		want: graph.StateNone,
	},
	{
		name: "FollowsOfTwoPeers",
		setUp: func(t *testing.T, e *remoteFollowEnv) ([][]byte, string, string) {
			alice := e.register(t, "alice").AccountID
			e.remote(t, remotePeers[1].actor)
			peer := e.remote(t, remotePeers[2].actor)
			return [][]byte{
				remotePeers[1].fixture(t, "follow", e.actorURI(alice), ""),
				remotePeers[2].fixture(t, "follow", e.actorURI(alice), ""),
			}, peer, alice
		},
		want: graph.StateActive,
	},
	{
		name: "AcceptAndUndoAccept",
		setUp: func(t *testing.T, e *remoteFollowEnv) ([][]byte, string, string) {
			p := remotePeers[0]
			alice, peer := e.register(t, "alice").AccountID, e.remote(t, p.actor)
			if _, err := e.graph.Follow(t.Context(), alice, peer); err != nil {
				t.Fatal(err)
			}
			accept := p.fixture(t, "accept-follow", e.actorURI(alice), e.liveFollow(t, alice, peer).URI)
			undo := fmt.Appendf(nil, `{"@context":"https://www.w3.org/ns/activitystreams","id":"%s/undo","type":"Undo","actor":"%s","object":%s}`,
				activityID(t, accept), p.actor, accept)
			return [][]byte{accept, undo}, alice, peer
		},
		want: graph.StateNone,
	},
}

// remoteFollowOrder runs every remoteOrderCases case in each of inboxSequences, each on a new
// database, and requires that every job completes and that the follow ends in the case's state,
// with follow_edges and follow_counters equal to a rebuild.
func remoteFollowOrder(t *testing.T, newHarness RemoteFollowFactory) {
	for _, c := range remoteOrderCases {
		t.Run(c.name, func(t *testing.T) {
			var n int
			{
				e := newRemoteFollowEnv(t, newHarness)
				docs, _, _ := c.setUp(t, e)
				n = len(docs)
			}
			for _, seq := range inboxSequences(n) {
				e := newRemoteFollowEnv(t, newHarness)
				docs, follower, followee := c.setUp(t, e)
				uris := make([]federation.IRI, len(docs))
				for i, doc := range docs {
					uris[i] = e.record(t, doc)
				}
				for _, i := range seq {
					e.job(t, uris[i])
				}
				if r := e.relationship(t, follower, followee); r.Following != c.want {
					t.Errorf("order %v: follow = %q, want %q", seq, r.Following, c.want)
				}
				before := snapshotFollowProjections(t, e.h.Graph)
				if err := appgraph.RebuildProjections(t.Context(), e.h.UnitOfWork, e.h.Graph); err != nil {
					t.Fatal(err)
				}
				if after := snapshotFollowProjections(t, e.h.Graph); !reflect.DeepEqual(before, after) {
					t.Errorf("order %v: projections %+v, rebuilt %+v", seq, before, after)
				}
			}
		})
	}
}
