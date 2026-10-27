// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InboxProcessingContract verifies the asynchronous part of the inbox (federation.md §3.2,
// §3.3) on the inbox repository of a database adapter: activities of unknown types complete, a
// Delete or Undo of an unknown target leaves a tombstone (only for objects of the actor's origin)
// that drops the Create or Follow arriving later (for an Undo of an embedded Follow, the next
// Follow of its pair), restricted actors are dropped except for what
// takes something back, activities of other signers are confirmed at their origin, activities
// that can never be processed fail, the own-actor Update and Delete have their own route, and an
// Undo goes to the handler of the type it undoes. The handlers of Block, Undo Block, and Flag run
// for real on the graph and moderation repositories.
//
// Its subtest OrderAndDuplicates is the property test of NFR-REL-1: every case's activities are
// processed in several orders and with duplicated jobs, each run on a new database, and the
// final rows must be the same. The tasks that add inbox handlers add their cases to
// inboxOrderCases. The actors, fetches, restrictions, and the handlers of notes and follows
// are in-memory stand-ins.
func InboxProcessingContract(t *testing.T, newHarness InboxFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *inboxProcessEnv)
	}{
		{"CompletesUnknownTypes", inboxCompletesUnknownTypes},
		{"DropsACreateAfterItsDelete", inboxDropsCreateAfterDelete},
		{"DropsAFollowAfterItsUndo", inboxDropsFollowAfterUndo},
		{"DropsTheNextFollowOfAnUndonePair", inboxDropsFollowOfUndonePair},
		{"TombstonesOnlyObjectsOfTheActor", inboxTombstonesOnlyOwnObjects},
		{"HoldsBackOnlyTheObjectsOfTheTombstonesActor", inboxTombstonesBindTheActor},
		{"DropsRestrictedActors", inboxDropsRestrictedActors},
		{"ConfirmsActivitiesOfOtherSigners", inboxConfirmsRefetched},
		{"ReleasesActivitiesItsOriginDoesNotConfirm", inboxReleasesUnconfirmed},
		{"FailsWhatCanNeverBeProcessed", inboxFailsUnprocessable},
		{"RoutesTheOwnActorsUpdateAndDelete", inboxRoutesOwnActor},
		{"RoutesUndoByTheUndoneType", inboxRoutesUndo},
		{"HandlesBlocksAndFlags", inboxHandlesModeration},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newInboxProcessEnv(t, newHarness)
			e.wireStandIns()
			c.run(t, e)
		})
	}
	t.Run("OrderAndDuplicates", func(t *testing.T) { inboxOrderAndDuplicates(t, newHarness) })
}

// inboxProcessEnv is one database with an InboxProcessor on it and the stand-ins it uses.
type inboxProcessEnv struct {
	h         InboxHarness
	clock     *Clock
	actors    *inboxActors
	fetcher   *inboxFetcher
	domains   inboxDomains
	suspended inboxAccountPolicy
	processor *appfederation.InboxProcessor
	notes     *inboxNotes
	follows   *inboxFollows
	// projectors are the projectors of the harness's UnitOfWork, empty unless a case's wire
	// registers some.
	projectors *uow.Projectors
	// accounts is the account module of the own-actor handlers, nil unless wired.
	accounts *appaccount.Service
}

func newInboxProcessEnv(t *testing.T, newHarness InboxFactory) *inboxProcessEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	e := &inboxProcessEnv{
		h: h, projectors: projectors, clock: NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)),
		actors:  &inboxActors{known: map[federation.IRI]bool{inboxAlice: true, inboxBob: true}},
		fetcher: &inboxFetcher{docs: map[string][]byte{}, errs: map[string]error{}},
		domains: inboxDomains{}, suspended: inboxAccountPolicy{},
		notes:   &inboxNotes{live: map[federation.IRI]bool{}, deleted: map[federation.IRI]bool{}},
		follows: &inboxFollows{active: map[federation.IRI]bool{}, ended: map[federation.IRI]bool{}},
	}
	e.fetcher.clock = e.clock
	e.processor = e.newProcessor()
	return e
}

// newProcessor returns an InboxProcessor on the env's harness and stand-ins, without handlers.
func (e *inboxProcessEnv) newProcessor() *appfederation.InboxProcessor {
	return appfederation.NewInboxProcessor(appfederation.InboxProcessorDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Inbox, Actors: e.actors, Fetcher: e.fetcher,
		Domains: e.domains, Accounts: e.suspended, Clock: e.clock,
	})
}

// wireStandIns registers the stand-in handlers of notes (Create, Delete) and follows (Follow,
// Undo), which behave like the content and graph handlers do toward order and repetition: a
// deleted note and an ended follow stay so.
func (e *inboxProcessEnv) wireStandIns() {
	e.processor.Handle(federation.ActivityCreate, e.notes.create)
	e.processor.Handle(federation.ActivityDelete, e.notes.delete)
	e.processor.Handle(federation.ActivityFollow, e.follows.follow)
	e.processor.HandleUndo(federation.ActivityFollow, e.follows.undo)
}

// receive records doc as the synchronous part of the inbox does, with trust, and returns its
// activity URI. Recording the same activity again does nothing.
func (e *inboxProcessEnv) receive(t *testing.T, doc []byte, trust federation.Trust) federation.IRI {
	t.Helper()
	ctx := t.Context()
	env, err := federation.ReadEnvelope(doc, appfederation.CanonicalHost)
	if err != nil {
		t.Fatalf("ReadEnvelope(%s): %v", doc, err)
	}
	a := federation.ReceivedActivity{
		ActivityURI: env.ActivityURI(doc), ActorURI: env.Actor, Type: env.Type, Scheme: "cavage", Trust: trust,
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

// job runs the federation.inbox job of a recorded activity.
func (e *inboxProcessEnv) job(t *testing.T, uri federation.IRI, trust federation.Trust) queue.Result {
	t.Helper()
	nj, err := appfederation.InboxJob(federation.ReceivedActivity{ActivityURI: uri, ActorURI: inboxAlice, Trust: trust})
	if err != nil {
		t.Fatal(err)
	}
	return e.processor.HandleJob(t.Context(), queue.Job{ID: NamedID("job"), Type: nj.Type, Payload: nj.Payload, IdempotencyKey: nj.IdempotencyKey})
}

// process records doc as signed by its actor and processes it, failing the test on an error.
func (e *inboxProcessEnv) process(t *testing.T, doc []byte) appfederation.InboxOutcome {
	t.Helper()
	return e.processAs(t, doc, federation.TrustSigned)
}

func (e *inboxProcessEnv) processAs(t *testing.T, doc []byte, trust federation.Trust) appfederation.InboxOutcome {
	t.Helper()
	uri := e.receive(t, doc, trust)
	got, err := e.processor.Process(t.Context(), uri, trust)
	if err != nil {
		t.Fatalf("Process(%s): %v", uri, err)
	}
	return got
}

// tombstoned reports whether uri has a tombstone that alice left.
func (e *inboxProcessEnv) tombstoned(t *testing.T, uri federation.IRI) bool {
	t.Helper()
	ok, err := e.h.Inbox.RemoteObjectTombstoned(t.Context(), uri, inboxAlice)
	if err != nil {
		t.Fatalf("RemoteObjectTombstoned(%s): %v", uri, err)
	}
	return ok
}

const inboxBob = "https://other.example/users/bob"

// inboxNote returns a Note of alice.
func inboxNote(id string) map[string]any {
	return map[string]any{"id": id, "type": "Note", "attributedTo": inboxAlice, "content": "hi", "to": []string{string(federation.PublicIRI)}}
}

// inboxDoc returns an activity of actor (alice when "") as JSON.
func inboxDoc(t *testing.T, typ, id, actor string, object any) []byte {
	t.Helper()
	if actor == "" {
		actor = inboxAlice
	}
	fields := map[string]any{"type": typ, "actor": actor, "object": object}
	if id != "" {
		fields["id"] = id
	}
	return inboxActivity(t, fields)
}

// inboxActors resolves the actors in known as remote accounts.
type inboxActors struct {
	mu    sync.Mutex
	known map[federation.IRI]bool
}

func (a *inboxActors) Resolve(_ context.Context, uri string) (appaccount.Actor, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	iri := federation.IRI(uri)
	if !a.known[iri] {
		return appaccount.Actor{}, fmt.Errorf("%w: %s", appfederation.ErrFetchNotFound, uri)
	}
	var actor appaccount.Actor
	actor.Account = account.Account{ID: NamedID("account:" + uri), URI: uri, Domain: iri.Host()}
	return actor, nil
}

// inboxFetcher serves the documents in docs and the errors in errs; any other URI is not found.
type inboxFetcher struct {
	mu      sync.Mutex
	clock   *Clock
	docs    map[string][]byte
	errs    map[string]error
	fetched []string
}

func (f *inboxFetcher) Fetch(_ context.Context, uri string) (*appfederation.Fetched, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetched = append(f.fetched, uri)
	if err, ok := f.errs[uri]; ok {
		return nil, err
	}
	raw, ok := f.docs[uri]
	if !ok {
		return nil, fmt.Errorf("%w: %s", appfederation.ErrFetchNotFound, uri)
	}
	doc, err := appfederation.Normalize(raw, f.clock.Now())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", appfederation.ErrFetchNotFound, err)
	}
	return &appfederation.Fetched{Document: doc, Raw: raw}, nil
}

func (f *inboxFetcher) serve(t *testing.T, uri string, doc map[string]any) {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.docs[uri] = b
}

type inboxAccountPolicy map[string]bool

func (p inboxAccountPolicy) AccountFederationSuspended(_ context.Context, accountID string) (bool, error) {
	return p[accountID], nil
}

// inboxNotes stands in for the content module's remote statuses: live notes, and deleted ones
// whose URI stays as a tombstone (status_deletions).
type inboxNotes struct {
	mu      sync.Mutex
	live    map[federation.IRI]bool
	deleted map[federation.IRI]bool
	// seen are the trusted contents of the notes created, by URI.
	seen map[federation.IRI]string
}

func (n *inboxNotes) create(_ context.Context, a *appfederation.InboxActivity) error {
	ref := a.Activity.Object()
	doc := ref.Trusted()
	if doc == nil || doc.Object == nil {
		return fmt.Errorf("%w: no trusted object", appfederation.ErrDropActivity)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.deleted[ref.ID] {
		return nil
	}
	n.live[ref.ID] = true
	if n.seen == nil {
		n.seen = map[federation.IRI]string{}
	}
	if len(doc.Object.Bodies) > 0 {
		n.seen[ref.ID] = doc.Object.Bodies[0].HTML
	}
	return nil
}

func (n *inboxNotes) delete(_ context.Context, a *appfederation.InboxActivity) error {
	id := a.Activity.Object().ID
	n.mu.Lock()
	defer n.mu.Unlock()
	switch {
	case n.live[id]:
		delete(n.live, id)
		n.deleted[id] = true
	case n.deleted[id]:
	default:
		return appfederation.ErrUnknownObject
	}
	return nil
}

// inboxFollows stands in for the graph module's remote follows, by the URI of the Follow.
type inboxFollows struct {
	mu     sync.Mutex
	active map[federation.IRI]bool
	ended  map[federation.IRI]bool
}

func (f *inboxFollows) follow(_ context.Context, a *appfederation.InboxActivity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ended[a.Received.ActivityURI] {
		f.active[a.Received.ActivityURI] = true
	}
	return nil
}

func (f *inboxFollows) undo(_ context.Context, a *appfederation.InboxActivity) error {
	id := a.Activity.Object().ID
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.active[id]:
		delete(f.active, id)
		f.ended[id] = true
	case f.ended[id]:
	default:
		return appfederation.ErrUnknownObject
	}
	return nil
}

func inboxCompletesUnknownTypes(t *testing.T, e *inboxProcessEnv) {
	docs := [][]byte{
		inboxDoc(t, "Move", inboxAlice+"#move", "", inboxAlice),
		inboxDoc(t, "Add", inboxAlice+"#add", "", inboxAlice+"/notes/1"),
		inboxDoc(t, "Bite", inboxAlice+"#bite", "", "https://"+contractHost+"/ap/actors/x"),
		inboxDoc(t, "Like", inboxAlice+"#like", "", "https://"+contractHost+"/ap/objects/x"),
	}
	for _, doc := range docs {
		uri := e.receive(t, doc, federation.TrustSigned)
		if r := e.job(t, uri, federation.TrustSigned); !r.IsDone() {
			t.Errorf("job of %s = %+v, want Done", uri, r)
		}
		if got, err := e.processor.Process(t.Context(), uri, federation.TrustSigned); err != nil || got != appfederation.InboxUnhandled {
			t.Errorf("Process(%s) = %q, %v; want %q", uri, got, err, appfederation.InboxUnhandled)
		}
	}
}

func inboxDropsCreateAfterDelete(t *testing.T, e *inboxProcessEnv) {
	note := inboxAlice + "/notes/1"
	if got := e.process(t, inboxDoc(t, "Delete", inboxAlice+"/notes/1#delete", "", map[string]any{"id": note, "type": "Tombstone"})); got != appfederation.InboxTombstoned {
		t.Errorf("Delete of an unknown note = %q, want %q", got, appfederation.InboxTombstoned)
	}
	if !e.tombstoned(t, federation.IRI(note)) {
		t.Errorf("%s has no tombstone", note)
	}
	if got := e.process(t, inboxDoc(t, "Create", note+"/activity", "", inboxNote(note))); got != appfederation.InboxDropped {
		t.Errorf("Create after its Delete = %q, want %q", got, appfederation.InboxDropped)
	}
	// The Update of the note is dropped too; no handler would have seen it.
	if got := e.process(t, inboxDoc(t, "Update", note+"#update", "", inboxNote(note))); got != appfederation.InboxDropped {
		t.Errorf("Update after its Delete = %q, want %q", got, appfederation.InboxDropped)
	}
	if e.notes.live[federation.IRI(note)] {
		t.Errorf("%s was ingested after its Delete", note)
	}
	// Another note is ingested.
	other := inboxAlice + "/notes/2"
	if got := e.process(t, inboxDoc(t, "Create", other+"/activity", "", inboxNote(other))); got != appfederation.InboxHandled || !e.notes.live[federation.IRI(other)] {
		t.Errorf("Create of another note = %q, want it handled", got)
	}
}

func inboxDropsFollowAfterUndo(t *testing.T, e *inboxProcessEnv) {
	follow := inboxAlice + "#follow-1"
	local := "https://" + contractHost + "/ap/actors/x"
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo-1", "", map[string]any{"id": follow, "type": "Follow", "actor": inboxAlice, "object": local})); got != appfederation.InboxTombstoned {
		t.Errorf("Undo of an unknown Follow = %q, want %q", got, appfederation.InboxTombstoned)
	}
	if got := e.process(t, inboxDoc(t, "Follow", follow, "", local)); got != appfederation.InboxDropped {
		t.Errorf("Follow after its Undo = %q, want %q", got, appfederation.InboxDropped)
	}
	if e.follows.active[federation.IRI(follow)] {
		t.Errorf("%s is active after its Undo", follow)
	}
}

// inboxDropsFollowOfUndonePair checks the tombstone of the pair that an Undo of an unknown
// Follow names: Misskey's Undo names its Follow by another id than the Follow it sent.
func inboxDropsFollowOfUndonePair(t *testing.T, e *inboxProcessEnv) {
	local := "https://" + contractHost + "/ap/actors/x"
	undone := map[string]any{"id": inboxAlice + "/follows/x", "type": "Follow", "actor": inboxAlice, "object": local}
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"/follows/x/undo", "", undone)); got != appfederation.InboxTombstoned {
		t.Errorf("Undo of an unknown Follow = %q, want %q", got, appfederation.InboxTombstoned)
	}
	// Bob's Follow of the same actor, and alice's of another, go through.
	if got := e.process(t, inboxDoc(t, "Follow", inboxBob+"#follow", inboxBob, local)); got != appfederation.InboxHandled {
		t.Errorf("bob's Follow = %q, want %q", got, appfederation.InboxHandled)
	}
	other := "https://" + contractHost + "/ap/actors/y"
	if got := e.process(t, inboxDoc(t, "Follow", inboxAlice+"#follow-y", "", other)); got != appfederation.InboxHandled {
		t.Errorf("alice's Follow of another actor = %q, want %q", got, appfederation.InboxHandled)
	}
	// The first Follow of the pair is dropped, also when it is processed again.
	first := federation.IRI(inboxAlice + "#follow-1")
	doc := inboxDoc(t, "Follow", string(first), "", local)
	for i := range 2 {
		if got := e.process(t, doc); got != appfederation.InboxDropped {
			t.Errorf("Follow after the Undo of its pair (run %d) = %q, want %q", i+1, got, appfederation.InboxDropped)
		}
	}
	if e.follows.active[first] || !e.tombstoned(t, first) {
		t.Errorf("%s: active %t, tombstoned %t; want dropped with a tombstone", first, e.follows.active[first], e.tombstoned(t, first))
	}
	// The Undo again does not bring the tombstone of the pair back.
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"/follows/x/undo", "", undone)); got != appfederation.InboxTombstoned {
		t.Errorf("Undo again = %q, want %q", got, appfederation.InboxTombstoned)
	}
	// The tombstone is used up: a later Follow of the pair goes through.
	later := federation.IRI(inboxAlice + "#follow-2")
	if got := e.process(t, inboxDoc(t, "Follow", string(later), "", local)); got != appfederation.InboxHandled || !e.follows.active[later] {
		t.Errorf("a later Follow of the pair = %q, want it handled", got)
	}
	// An Undo of a Follow of someone else leaves no tombstone of a pair.
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo-bob", "", map[string]any{"id": inboxBob + "#follow-z", "type": "Follow", "actor": inboxBob, "object": local})); got == appfederation.InboxHandled {
		t.Errorf("Undo of bob's Follow by alice = %q, want it not handled", got)
	}
	if got := e.process(t, inboxDoc(t, "Follow", inboxBob+"#follow-z2", inboxBob, other)); got != appfederation.InboxHandled {
		t.Errorf("bob's Follow after alice's Undo of one of his = %q, want %q", got, appfederation.InboxHandled)
	}
}

func inboxTombstonesOnlyOwnObjects(t *testing.T, e *inboxProcessEnv) {
	foreign := "https://other.example/notes/1"
	if got := e.process(t, inboxDoc(t, "Delete", inboxAlice+"#delete-foreign", "", foreign)); got != appfederation.InboxDropped {
		t.Errorf("Delete of another server's note = %q, want %q", got, appfederation.InboxDropped)
	}
	if e.tombstoned(t, federation.IRI(foreign)) {
		t.Errorf("alice left a tombstone for %s", foreign)
	}
	// Bob's note is still ingested.
	bobNote := inboxNote(foreign)
	bobNote["attributedTo"] = inboxBob
	if got := e.process(t, inboxDoc(t, "Create", foreign+"/activity", inboxBob, bobNote)); got != appfederation.InboxHandled {
		t.Errorf("Create of bob's note = %q, want %q", got, appfederation.InboxHandled)
	}
	// A Delete of an actor by itself leaves no tombstone and has no handler.
	if got := e.process(t, inboxDoc(t, "Delete", inboxAlice+"#delete-self", "", inboxAlice)); got != appfederation.InboxUnhandled {
		t.Errorf("Delete of alice by herself = %q, want %q", got, appfederation.InboxUnhandled)
	}
	if e.tombstoned(t, inboxAlice) {
		t.Error("alice's Delete of herself left a tombstone")
	}
}

// inboxTombstonesBindTheActor checks that the tombstone of an unknown object that one actor
// deleted does not hold back the object of another actor of the same origin, nor its activities.
func inboxTombstonesBindTheActor(t *testing.T, e *inboxProcessEnv) {
	carol := "https://remote.example/users/carol"
	e.actors.known[federation.IRI(carol)] = true
	note := inboxAlice + "/notes/later"
	if got := e.process(t, inboxDoc(t, "Delete", carol+"#delete-1", carol, note)); got != appfederation.InboxTombstoned {
		t.Fatalf("Delete of an unknown note of the same origin = %q, want %q", got, appfederation.InboxTombstoned)
	}
	if got := e.process(t, inboxDoc(t, "Undo", carol+"#undo-1", carol, inboxAlice+"#follow-later")); got != appfederation.InboxTombstoned {
		t.Fatalf("Undo of an unknown activity of the same origin = %q, want %q", got, appfederation.InboxTombstoned)
	}
	if got := e.process(t, inboxDoc(t, "Create", note+"/activity", "", inboxNote(note))); got != appfederation.InboxHandled || !e.notes.live[federation.IRI(note)] {
		t.Errorf("alice's Create after another actor's Delete = %q, want %q", got, appfederation.InboxHandled)
	}
	local := "https://" + contractHost + "/ap/actors/x"
	if got := e.process(t, inboxDoc(t, "Follow", inboxAlice+"#follow-later", "", local)); got != appfederation.InboxHandled {
		t.Errorf("alice's Follow after another actor's Undo = %q, want %q", got, appfederation.InboxHandled)
	}
	// The deleting actor's own Create of the URI stays held back.
	carolNote := inboxNote(note)
	carolNote["attributedTo"] = carol
	if got := e.process(t, inboxDoc(t, "Create", note+"/activity-carol", carol, carolNote)); got != appfederation.InboxDropped {
		t.Errorf("Create by the deleting actor = %q, want %q", got, appfederation.InboxDropped)
	}
}

func inboxDropsRestrictedActors(t *testing.T, e *inboxProcessEnv) {
	e.domains["remote.example"] = true
	note := inboxAlice + "/notes/1"
	if got := e.process(t, inboxDoc(t, "Create", note+"/activity", "", inboxNote(note))); got != appfederation.InboxDropped {
		t.Errorf("Create of a suspended domain = %q, want %q", got, appfederation.InboxDropped)
	}
	// What takes something back still goes through.
	if got := e.process(t, inboxDoc(t, "Delete", note+"#delete", "", note)); got != appfederation.InboxTombstoned {
		t.Errorf("Delete of a suspended domain = %q, want %q", got, appfederation.InboxTombstoned)
	}
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo", "", inboxAlice+"#follow")); got != appfederation.InboxTombstoned {
		t.Errorf("Undo of a suspended domain = %q, want %q", got, appfederation.InboxTombstoned)
	}
	e.suspended[NamedID("account:"+inboxBob)] = true
	bobNote := inboxNote("https://other.example/notes/1")
	bobNote["attributedTo"] = inboxBob
	if got := e.process(t, inboxDoc(t, "Create", "https://other.example/notes/1/activity", inboxBob, bobNote)); got != appfederation.InboxDropped {
		t.Errorf("Create of a suspended account = %q, want %q", got, appfederation.InboxDropped)
	}
	if len(e.notes.live) != 0 {
		t.Errorf("notes = %v, want none", e.notes.live)
	}
	// An unknown actor that cannot be fetched is dropped too.
	if got := e.process(t, inboxDoc(t, "Create", "https://gone.example/notes/1/activity", "https://gone.example/users/x", inboxNote("https://gone.example/notes/1"))); got != appfederation.InboxDropped {
		t.Errorf("Create of an actor that cannot be resolved = %q, want %q", got, appfederation.InboxDropped)
	}
}

func inboxConfirmsRefetched(t *testing.T, e *inboxProcessEnv) {
	refetch := federation.TrustRefetch

	// A relayed Create uses the note as its origin serves it, not as embedded.
	note := inboxAlice + "/notes/relayed"
	served := inboxNote(note)
	served["content"] = "as served"
	e.fetcher.serve(t, note, served)
	forged := inboxNote(note)
	forged["content"] = "forged"
	if got := e.processAs(t, inboxDoc(t, "Create", note+"/activity", "", forged), refetch); got != appfederation.InboxHandled {
		t.Fatalf("relayed Create = %q, want %q", got, appfederation.InboxHandled)
	}
	if html := e.notes.seen[federation.IRI(note)]; html != "as served" {
		t.Errorf("relayed Create stored %q, want the content served by the origin", html)
	}

	// A relayed Create whose note its origin does not serve is dropped.
	missing := inboxAlice + "/notes/missing"
	if got := e.processAs(t, inboxDoc(t, "Create", missing+"/activity", "", inboxNote(missing)), refetch); got != appfederation.InboxDropped {
		t.Errorf("relayed Create of a missing note = %q, want %q", got, appfederation.InboxDropped)
	}

	// A temporary failure is retried.
	flaky := inboxAlice + "/notes/flaky"
	e.fetcher.errs[flaky] = fmt.Errorf("%w: %s", appfederation.ErrFetchFailed, flaky)
	uri := e.receive(t, inboxDoc(t, "Create", flaky+"/activity", "", inboxNote(flaky)), refetch)
	if r := e.job(t, uri, refetch); !r.IsRetry() {
		t.Errorf("job of a relayed Create whose fetch failed = %+v, want Retry", r)
	}

	// A relayed Delete of a note that still exists is forged; one of a gone note stands.
	if got := e.processAs(t, inboxDoc(t, "Delete", note+"#delete", "", note), refetch); got != appfederation.InboxDropped || !e.notes.live[federation.IRI(note)] {
		t.Errorf("relayed Delete of a note that exists = %q, want %q and the note kept", got, appfederation.InboxDropped)
	}
	gone := inboxAlice + "/notes/gone"
	e.fetcher.errs[gone] = fmt.Errorf("%w: %s", appfederation.ErrFetchGone, gone)
	if got := e.processAs(t, inboxDoc(t, "Delete", gone+"#delete", "", gone), refetch); got != appfederation.InboxTombstoned {
		t.Errorf("relayed Delete of a gone note = %q, want %q", got, appfederation.InboxTombstoned)
	}

	// Other activities are fetched by their id and must be the same activity.
	local := "https://" + contractHost + "/ap/actors/x"
	follow := inboxAlice + "#follow-relayed"
	e.fetcher.serve(t, follow, map[string]any{"id": follow, "type": "Follow", "actor": inboxAlice, "object": local})
	if got := e.processAs(t, inboxDoc(t, "Follow", follow, "", local), refetch); got != appfederation.InboxHandled {
		t.Errorf("relayed Follow served by its origin = %q, want %q", got, appfederation.InboxHandled)
	}
	unconfirmed := inboxAlice + "#follow-unconfirmed"
	if got := e.processAs(t, inboxDoc(t, "Follow", unconfirmed, "", local), refetch); got != appfederation.InboxDropped {
		t.Errorf("relayed Follow its origin does not serve = %q, want %q", got, appfederation.InboxDropped)
	}

	// An Announce that cannot be fetched is kept, with its embedded object untrusted.
	var announced *appfederation.InboxActivity
	e.processor.Handle(federation.ActivityAnnounce, func(_ context.Context, a *appfederation.InboxActivity) error {
		announced = a
		return nil
	})
	announce := inboxDoc(t, "Announce", inboxAlice+"/notes/relayed/announce", "", inboxNote(note))
	if got := e.processAs(t, announce, refetch); got != appfederation.InboxHandled || announced == nil {
		t.Fatalf("relayed Announce = %q, want %q", got, appfederation.InboxHandled)
	}
	if obj := announced.Activity.Object(); obj.ID != federation.IRI(note) || obj.Trusted() != nil {
		t.Errorf("relayed Announce object = %s (trusted %v), want %s untrusted", obj.ID, obj.Trusted() != nil, note)
	}
}

// inboxReleasesUnconfirmed checks that an activity of trust refetch that its origin does not
// confirm gives up its record, so that the activity arriving later is taken in, while an activity
// dropped by a rule after it was confirmed keeps its record.
func inboxReleasesUnconfirmed(t *testing.T, e *inboxProcessEnv) {
	ctx := t.Context()
	note := inboxAlice + "/notes/later"
	activity := note + "/activity"
	if got := e.processAs(t, inboxDoc(t, "Create", activity, "", inboxNote(note)), federation.TrustRefetch); got != appfederation.InboxDropped {
		t.Fatalf("Create its origin does not serve = %q, want %q", got, appfederation.InboxDropped)
	}
	if _, err := e.h.Inbox.FindReceivedActivity(ctx, federation.IRI(activity)); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindReceivedActivity after the drop = %v, want ErrNotFound", err)
	}
	if _, err := e.h.Inbox.FindReceivedDocument(ctx, federation.IRI(activity)); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindReceivedDocument after the drop = %v, want ErrNotFound", err)
	}
	// The same activity signed by its actor is taken in.
	if got := e.process(t, inboxDoc(t, "Create", activity, "", inboxNote(note))); got != appfederation.InboxHandled || !e.notes.live[federation.IRI(note)] {
		t.Errorf("Create signed by its actor after the drop = %q, want %q", got, appfederation.InboxHandled)
	}

	// A signed activity dropped by a rule keeps its record.
	e.domains["remote.example"] = true
	dropped := inboxAlice + "/notes/dropped"
	if got := e.process(t, inboxDoc(t, "Create", dropped+"/activity", "", inboxNote(dropped))); got != appfederation.InboxDropped {
		t.Fatalf("Create of a suspended domain = %q, want %q", got, appfederation.InboxDropped)
	}
	if _, err := e.h.Inbox.FindReceivedActivity(ctx, federation.IRI(dropped+"/activity")); err != nil {
		t.Errorf("FindReceivedActivity of a signed activity dropped by a rule = %v, want the record kept", err)
	}
}

func inboxFailsUnprocessable(t *testing.T, e *inboxProcessEnv) {
	// A record whose document cannot be normalized.
	ctx := t.Context()
	uri := federation.IRI(inboxAlice + "#broken")
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := e.h.Inbox.InsertReceivedActivity(ctx, tx, federation.ReceivedActivity{
			ActivityURI: uri, ActorURI: inboxAlice, Type: "Create", Scheme: "cavage", Trust: federation.TrustSigned, ReceivedAt: e.clock.Now(),
		}); err != nil {
			return err
		}
		return e.h.Inbox.InsertReceivedDocument(ctx, tx, uri, []byte(`["not an object"]`))
	})
	if err != nil {
		t.Fatal(err)
	}
	if r := e.job(t, uri, federation.TrustSigned); !r.IsFail() || !errors.Is(r.Err(), appfederation.ErrUnprocessable) {
		t.Errorf("job of a document that cannot be normalized = %+v, want Fail", r)
	}
	// An activity that was never recorded, and a payload that is not one.
	if r := e.job(t, inboxAlice+"#never", federation.TrustSigned); !r.IsFail() {
		t.Errorf("job of an activity never recorded = %+v, want Fail", r)
	}
	if r := e.processor.HandleJob(ctx, queue.Job{Type: appfederation.JobInbox, Payload: json.RawMessage(`{}`)}); !r.IsFail() {
		t.Errorf("job without an activity = %+v, want Fail", r)
	}
	// An activity of another actor than the one recorded.
	other := federation.IRI(inboxAlice + "#other-actor")
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := e.h.Inbox.InsertReceivedActivity(ctx, tx, federation.ReceivedActivity{
			ActivityURI: other, ActorURI: inboxAlice, Type: "Follow", Scheme: "cavage", Trust: federation.TrustSigned, ReceivedAt: e.clock.Now(),
		}); err != nil {
			return err
		}
		return e.h.Inbox.InsertReceivedDocument(ctx, tx, other, inboxDoc(t, "Follow", string(other), inboxBob, inboxAlice))
	})
	if err != nil {
		t.Fatal(err)
	}
	if r := e.job(t, other, federation.TrustSigned); !r.IsFail() {
		t.Errorf("job of an activity of another actor = %+v, want Fail", r)
	}
}

func inboxRoutesOwnActor(t *testing.T, e *inboxProcessEnv) {
	var updates, deletes int
	e.processor.HandleOwnActor(federation.ActivityUpdate, func(_ context.Context, a *appfederation.InboxActivity) error {
		updates++
		if a.Actor.Account.URI != inboxAlice {
			t.Errorf("own-actor Update of %s, want alice", a.Actor.Account.URI)
		}
		return nil
	})
	e.processor.HandleOwnActor(federation.ActivityDelete, func(context.Context, *appfederation.InboxActivity) error {
		deletes++
		return nil
	})
	actor := map[string]any{"id": inboxAlice, "type": "Person", "inbox": inboxAlice + "/inbox", "preferredUsername": "alice"}
	if got := e.process(t, inboxDoc(t, "Update", inboxAlice+"#update-1", "", actor)); got != appfederation.InboxHandled || updates != 1 {
		t.Errorf("Update of alice = %q (%d calls), want handled by the own-actor handler", got, updates)
	}
	if got := e.process(t, inboxDoc(t, "Delete", inboxAlice+"#delete-1", "", inboxAlice)); got != appfederation.InboxHandled || deletes != 1 {
		t.Errorf("Delete of alice = %q (%d calls), want handled by the own-actor handler", got, deletes)
	}
	// The Delete of a note still goes to the Delete handler.
	if got := e.process(t, inboxDoc(t, "Delete", inboxAlice+"#delete-2", "", inboxAlice+"/notes/9")); got != appfederation.InboxTombstoned || deletes != 1 {
		t.Errorf("Delete of a note = %q (%d own-actor calls), want tombstoned by the Delete handler", got, deletes)
	}
	defer func() {
		if recover() == nil {
			t.Error("HandleOwnActor(Follow) did not panic")
		}
	}()
	e.processor.HandleOwnActor(federation.ActivityFollow, func(context.Context, *appfederation.InboxActivity) error { return nil })
}

func inboxRoutesUndo(t *testing.T, e *inboxProcessEnv) {
	// The stand-ins take back follows; a second handler takes back likes it knows.
	var likes []federation.IRI
	known := federation.IRI(inboxAlice + "#like-1")
	e.processor.HandleUndo(federation.ActivityLike, func(_ context.Context, a *appfederation.InboxActivity) error {
		likes = append(likes, a.Activity.Object().ID)
		if a.Activity.Object().ID != known {
			return appfederation.ErrUnknownObject
		}
		return nil
	})
	local := "https://" + contractHost + "/ap/objects/x"
	// An embedded Like goes to the Like handler alone.
	embedded := map[string]any{"id": string(known), "type": "Like", "actor": inboxAlice, "object": local}
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo-like-1", "", embedded)); got != appfederation.InboxHandled || len(likes) != 1 {
		t.Errorf("Undo of an embedded Like = %q (%d Like calls), want handled by the Like handler", got, len(likes))
	}
	// An IRI alone goes to the handlers in order until one knows it.
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo-like-2", "", string(known))); got != appfederation.InboxHandled || len(likes) != 2 {
		t.Errorf("Undo of a known IRI = %q (%d Like calls), want handled by the Like handler", got, len(likes))
	}
	unknown := inboxAlice + "#like-2"
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo-like-3", "", unknown)); got != appfederation.InboxTombstoned || !e.tombstoned(t, federation.IRI(unknown)) {
		t.Errorf("Undo of an IRI no handler knows = %q, want %q", got, appfederation.InboxTombstoned)
	}
	// An embedded activity of a type without an Undo handler is unknown.
	block := inboxAlice + "#block-1"
	if got := e.process(t, inboxDoc(t, "Undo", inboxAlice+"#undo-block", "", map[string]any{"id": block, "type": "Block", "actor": inboxAlice, "object": local})); got != appfederation.InboxTombstoned || len(likes) != 3 {
		t.Errorf("Undo of an embedded Block = %q (%d Like calls), want %q without the Like handler", got, len(likes), appfederation.InboxTombstoned)
	}
	defer func() {
		if recover() == nil {
			t.Error("Handle(Undo) after HandleUndo did not panic")
		}
	}()
	e.processor.Handle(federation.ActivityUndo, func(context.Context, *appfederation.InboxActivity) error { return nil })
}

// inboxOrderCase is one case of the order and duplicate property test: activities that must
// leave the same rows in whatever order and however often their jobs run. The tasks that add
// inbox handlers append cases with their handlers and rows.
type inboxOrderCase struct {
	name string
	// activities returns the documents, all recorded as signed before any job runs.
	activities func(t *testing.T) [][]byte
	// wire registers the handlers on e; nil registers the stand-ins of this file.
	wire func(t *testing.T, e *inboxProcessEnv)
	// rows returns the final rows that must not depend on the order.
	rows func(t *testing.T, e *inboxProcessEnv) any
	// want is the expected value of rows.
	want any
}

// inboxNoteRows are the stand-in notes: the live ones, and the URIs that are gone (deleted, or
// tombstoned before their Create arrived), among uris.
type inboxNoteRows struct {
	Live, Gone []federation.IRI
}

func noteRows(uris ...string) func(t *testing.T, e *inboxProcessEnv) any {
	return func(t *testing.T, e *inboxProcessEnv) any {
		r := inboxNoteRows{Live: []federation.IRI{}, Gone: []federation.IRI{}}
		for _, u := range uris {
			uri := federation.IRI(u)
			switch {
			case e.notes.live[uri]:
				r.Live = append(r.Live, uri)
			case e.notes.deleted[uri] || e.tombstoned(t, uri):
				r.Gone = append(r.Gone, uri)
			}
		}
		return r
	}
}

// inboxFollowRows are the stand-in follows among uris: active, and ended or undone first.
type inboxFollowRows struct {
	Active, Ended []federation.IRI
}

func followRows(uris ...string) func(t *testing.T, e *inboxProcessEnv) any {
	return func(t *testing.T, e *inboxProcessEnv) any {
		r := inboxFollowRows{Active: []federation.IRI{}, Ended: []federation.IRI{}}
		for _, u := range uris {
			uri := federation.IRI(u)
			switch {
			case e.follows.active[uri]:
				r.Active = append(r.Active, uri)
			case e.follows.ended[uri] || e.tombstoned(t, uri):
				r.Ended = append(r.Ended, uri)
			}
		}
		return r
	}
}

var inboxOrderCases = []inboxOrderCase{
	{
		// An Update of the actor before or after its Delete never brings it back, and the
		// Delete is recorded once (federation.md §3.4).
		name: "OwnActorUpdatesAndDelete",
		activities: func(t *testing.T) [][]byte {
			return [][]byte{
				inboxDoc(t, "Update", inboxAlice+"#updates/1", "", inboxActorDoc("Alice 1")),
				inboxDoc(t, "Delete", inboxAlice+"#delete", "", inboxAlice),
				inboxDoc(t, "Update", inboxAlice+"#updates/2", "", inboxActorDoc("Alice 2")),
			}
		},
		wire: wireOwnActor,
		rows: ownActorRows,
		want: inboxOwnActorRows{Deleted: true, RemoteDeletions: 1},
	},
	{
		name: "CreateAndDelete",
		activities: func(t *testing.T) [][]byte {
			n := inboxAlice + "/notes/1"
			return [][]byte{
				inboxDoc(t, "Create", n+"/activity", "", inboxNote(n)),
				inboxDoc(t, "Delete", n+"#delete", "", map[string]any{"id": n, "type": "Tombstone"}),
			}
		},
		rows: noteRows(inboxAlice + "/notes/1"),
		want: inboxNoteRows{Live: []federation.IRI{}, Gone: []federation.IRI{inboxAlice + "/notes/1"}},
	},
	{
		name: "FollowAndUndo",
		activities: func(t *testing.T) [][]byte {
			f := inboxAlice + "#follow-1"
			local := "https://" + contractHost + "/ap/actors/x"
			return [][]byte{
				inboxDoc(t, "Follow", f, "", local),
				inboxDoc(t, "Undo", inboxAlice+"#undo-1", "", map[string]any{"id": f, "type": "Follow", "actor": inboxAlice, "object": local}),
			}
		},
		rows: followRows(inboxAlice + "#follow-1"),
		want: inboxFollowRows{Active: []federation.IRI{}, Ended: []federation.IRI{inboxAlice + "#follow-1"}},
	},
	{
		name: "NotesWithAnUnknownType",
		activities: func(t *testing.T) [][]byte {
			n1, n2 := inboxAlice+"/notes/1", inboxAlice+"/notes/2"
			return [][]byte{
				inboxDoc(t, "Create", n1+"/activity", "", inboxNote(n1)),
				inboxDoc(t, "Create", n2+"/activity", "", inboxNote(n2)),
				inboxDoc(t, "Delete", n1+"#delete", "", n1),
				inboxDoc(t, "Move", inboxAlice+"#move", "", inboxAlice),
			}
		},
		rows: noteRows(inboxAlice+"/notes/1", inboxAlice+"/notes/2"),
		want: inboxNoteRows{Live: []federation.IRI{inboxAlice + "/notes/2"}, Gone: []federation.IRI{inboxAlice + "/notes/1"}},
	},
	{
		name:       "RemoteStatusCreateUpdateDelete",
		activities: remoteStatusActivities("create", "update-1", "delete"),
		wire:       wireStatusHandlers,
		rows:       remoteStatusRows(inboxAlice + "/notes/1"),
		want:       inboxStatusRows{Live: []inboxLiveStatus{}, Gone: []federation.IRI{inboxAlice + "/notes/1"}},
	},
	{
		name:       "RemoteStatusCreateAndUpdates",
		activities: remoteStatusActivities("create", "update-1", "update-2"),
		wire:       wireStatusHandlers,
		rows:       remoteStatusRows(inboxAlice + "/notes/1"),
		want: inboxStatusRows{
			Live: []inboxLiveStatus{{URI: inboxAlice + "/notes/1", HTML: "<p>update-2</p>", RevisedAt: remoteOrderPublished.Add(2 * time.Minute)}},
			Gone: []federation.IRI{},
		},
	},
}

// inboxOrderAndDuplicates runs every inboxOrderCases case in each of inboxSequences, each on a
// new database, and requires that every job completes and that the rows are the case's.
func inboxOrderAndDuplicates(t *testing.T, newHarness InboxFactory) {
	for _, c := range inboxOrderCases {
		t.Run(c.name, func(t *testing.T) {
			docs := c.activities(t)
			for _, seq := range inboxSequences(len(docs)) {
				e := newInboxProcessEnv(t, newHarness)
				if c.wire != nil {
					c.wire(t, e)
				} else {
					e.wireStandIns()
				}
				uris := make([]federation.IRI, len(docs))
				for i, doc := range docs {
					uris[i] = e.receive(t, doc, federation.TrustSigned)
				}
				for _, i := range seq {
					if r := e.job(t, uris[i], federation.TrustSigned); !r.IsDone() {
						t.Fatalf("order %v: job of %s = %+v, want Done", seq, uris[i], r)
					}
				}
				if got := c.rows(t, e); !reflect.DeepEqual(got, c.want) {
					t.Errorf("order %v: rows = %+v, want %+v", seq, got, c.want)
				}
			}
		})
	}
}

// inboxSequences returns the orders in which to run n jobs: every permutation for up to three
// jobs, or six seeded random ones; then each of the first and the reversed order with every job
// run twice (at-least-once delivery), once right after itself and once after all the others.
func inboxSequences(n int) [][]int {
	identity := make([]int, n)
	for i := range identity {
		identity[i] = i
	}
	var orders [][]int
	if n <= 3 {
		orders = permutations(identity)
	} else {
		r := rand.New(rand.NewPCG(245, 33)) //nolint:gosec // G404: a seeded, reproducible test order
		orders = append(orders, slices.Clone(identity))
		for len(orders) < 6 {
			p := slices.Clone(identity)
			r.Shuffle(n, func(i, j int) { p[i], p[j] = p[j], p[i] })
			orders = append(orders, p)
		}
	}
	reversed := slices.Clone(identity)
	slices.Reverse(reversed)
	for _, o := range [][]int{identity, reversed} {
		var twice []int
		for _, i := range o {
			twice = append(twice, i, i)
		}
		orders = append(orders, twice, append(slices.Clone(o), o...))
	}
	return orders
}

func permutations(s []int) [][]int {
	if len(s) <= 1 {
		return [][]int{slices.Clone(s)}
	}
	var out [][]int
	for i := range s {
		rest := append(slices.Clone(s[:i]), s[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]int{s[i]}, p...))
		}
	}
	return out
}

// inboxActorDoc returns the actor document of alice named name.
func inboxActorDoc(name string) map[string]any {
	return map[string]any{
		"id": inboxAlice, "type": "Person", "inbox": inboxAlice + "/inbox", "preferredUsername": "alice", "name": name,
	}
}

// wireOwnActor registers the stand-ins and the own-actor handlers of RemoteActors on the account
// module of the harness's database, with alice stored as she was first fetched. The network
// answers nothing.
func wireOwnActor(t *testing.T, e *inboxProcessEnv) {
	t.Helper()
	e.wireStandIns()
	appaccount.RegisterProjectors(e.projectors, e.h.Accounts)
	ids := NewIDGenerator(e.clock)
	e.accounts = appaccount.NewService(appaccount.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Accounts, Clock: e.clock, IDs: ids,
		Sealer: kernel.NewSealer(NewKeyManager("inbox")), Host: contractHost, GenerateKey: cachedKey,
		Sanitizer:         remoteSanitizer{},
		RecordRemoteImage: func(context.Context, uow.Tx, string, string, account.ProfileProperty) (string, error) { return "", nil },
	})
	net := &remoteNet{routes: map[string]remoteResponse{}}
	kv := NewKeyValueStore(e.clock)
	fetcher := appfederation.NewFetcher(appfederation.FetcherDeps{
		Client: appfederation.NewSignedHTTPClient(net, remoteSigner{}, kv, e.clock), KV: kv, Clock: e.clock,
		Domains: appfederation.NoDomainRestrictions{}, Instances: e.h.Federation,
		Key: appfederation.InstanceKey(e.accounts), Host: contractHost,
	})
	jobs := &remoteJobs{seen: map[string]bool{}}
	actors := appfederation.NewRemoteActors(appfederation.RemoteActorsDeps{
		UnitOfWork: e.h.UnitOfWork, Accounts: e.accounts, Instances: e.h.Instances, Fetcher: fetcher, HTTP: net,
		Jobs: jobs, Outbox: queue.AfterCommit(e.projectors, jobs, nil), Followers: remoteFollowers{},
		Clock: e.clock, IDs: ids,
	})
	actors.RegisterInboxHandlers(e.processor)
	if _, err := e.accounts.SaveRemoteActor(t.Context(), appaccount.RemoteActorInput{
		URI: inboxAlice, Domain: federation.IRI(inboxAlice).Host(), Type: "Person", PreferredUsername: "alice",
		Name: "Alice", InboxURL: inboxAlice + "/inbox", Document: []byte(`{}`),
	}); err != nil {
		t.Fatalf("SaveRemoteActor(alice): %v", err)
	}
}

// inboxOwnActorRows are what the own-actor handlers leave of alice: whether she is deleted, and
// how many remote_deleted events she has.
type inboxOwnActorRows struct {
	Deleted         bool
	RemoteDeletions int
}

func ownActorRows(t *testing.T, e *inboxProcessEnv) any {
	t.Helper()
	ctx := t.Context()
	a, err := e.accounts.FindByURI(ctx, inboxAlice)
	if err != nil {
		t.Fatalf("FindByURI(alice): %v", err)
	}
	r := inboxOwnActorRows{Deleted: a.Deleted()}
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		events, err := e.h.Accounts.ListLifecycleEvents(ctx, tx)
		for _, ev := range events {
			if ev.AccountID == a.Account.ID && ev.Kind == account.LifecycleRemoteDeleted {
				r.RemoteDeletions++
			}
		}
		return err
	})
	if err != nil {
		t.Fatalf("ListLifecycleEvents: %v", err)
	}
	return r
}
