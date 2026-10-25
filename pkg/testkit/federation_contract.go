// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
)

// FederationHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the federation repository on it.
type FederationHarness struct {
	UnitOfWork uow.UnitOfWork
	Federation appfederation.Repository
}

// FederationFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type FederationFactory func(t *testing.T, opts uow.Options) FederationHarness

// FederationRepositoryContract verifies the federation repository of a database adapter, alone
// and through the outbound use cases that drive it (federation.md §7): the outbound activities
// and their lookups, the first document kept, one delivery per inbox, the state of remote
// servers, the inboxes each visibility reaches (data-model §7), the Update and Delete that follow
// the Create, a fan-out run again without a delivery twice, the servers left out, the boosts that
// are not recorded, the Update and Delete of a local actor (to its followers, and for the
// Delete to every known shared inbox), the Update of a local actor's suspension and of its end
// (to its followers and every known shared inbox, with the emptied actor marked suspended while
// the suspension is in effect), the Block and Undo Block of a local block of a remote account,
// and the Flag of the instance actor that forwards a report to the reported account's inbox.
// The documents carry the body of the revision they present
// (federation.md §7.4); a local status leaves the remote accounts that do not follow its author
// out of tag (ADR-0035); the Delete is stored with the deletion, so that it reaches the
// inboxes of the Create after the status's rows are gone; and the automatic deletion of an old
// post (FR-118) sends the same Delete as its author's deletion. It also covers one delivery (federation.md §7.3): the classification of
// responses, the circuit breaker, the failure days that make a server unavailable and the
// successes, signed requests, and probes that bring it back, and the dead job after the retry
// schedule. It covers the admin views too: the servers with a health row by state and page, the
// pages of availability events, and an operator's clearing of a server. The other modules are
// in-memory stand-ins; the fan-out reads its recipients in batches of two, so that every source
// of recipients spans several jobs.
func FederationRepositoryContract(t *testing.T, newHarness FederationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *fedEnv)
	}{
		{"RecordsOutboundActivities", fedRecordsOutboundActivities},
		{"KeepsTheFirstDocument", fedKeepsTheFirstDocument},
		{"DeliversToAnInboxOnce", fedDeliversToAnInboxOnce},
		{"ReadsTheStateOfServers", fedReadsTheStateOfServers},
		{"DeliversEachVisibilityToItsAudience", fedDeliversEachVisibility},
		{"UpdateAndDeleteFollowTheCreate", fedUpdateAndDeleteFollowTheCreate},
		{"FanOutAgainDeliversNothingTwice", fedFanOutAgain},
		{"LeavesOutSuspendedAndUnavailableServers", fedLeavesOutServers},
		{"RecordsOnlyWhatFederates", fedRecordsOnlyWhatFederates},
		{"DeliversLocalBlocksOfRemoteAccounts", fedDeliversBlocks},
		{"ForwardsReportsAsFlags", fedForwardsReports},
		{"UpdatesAndDeletesActors", fedUpdatesAndDeletesActors},
		{"PropagatesSuspensionsOfLocalActors", fedPropagatesSuspensions},
		{"BuildsTheNoteOfTheRevision", fedBuildsTheNote},
		{"SendsVideoAndAudioWithPosterAndDuration", fedSendsVideoAndAudio},
		{"LeavesNonFollowersOutOfTheTagOfLocalStatuses", fedLeavesNonFollowersOutOfLocalTags},
		{"DeliversTheStoredDeleteAfterThePurge", fedDeliversTheStoredDelete},
		{"DeliversTheDeleteOfAnAutomaticDeletion", fedDeliversAutoDelete},
		{"ClassifiesDeliveryResponses", fedClassifiesResponses},
		{"SignsIntegrityProofs", fedSignsIntegrityProofs},
		{"BreaksTheCircuitOfAHost", fedBreaksTheCircuit},
		{"SkipsWhatTheFederationModeDoesNotSend", fedSkipsWhatTheModeDoesNotSend},
		{"SendsOnlyTheClosureDeletes", fedSendsOnlyTheClosureDeletes},
		{"MarksServersUnavailableAndBack", fedMarksServersUnavailable},
		{"BuriesExhaustedDeliveries", fedBuriesExhaustedDeliveries},
		{"ProbesUnavailableServers", fedProbesUnavailableServers},
		{"ListsAndClearsServersForOperators", fedListsServersForOperators},
		{"DeliversTheClosureDeletes", fedDeliversTheClosureDeletes},
		{"CountsPendingMoves", fedCountsPendingMoves},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newFedEnv(t, newHarness)) })
	}
}

type fedEnv struct {
	h        FederationHarness
	clock    *Clock
	ids      *IDGenerator
	jobs     *fedJobs
	handlers *queue.JobHandlerRegistry
	statuses *fedStatuses
	parts    *fedParts
	accounts *fedAccounts
	graph    *fedGraph
	blocks   *fedBlocks
	reports  *fedReports
	domains  fedDomains
	// suspensions are the suspended accounts.
	suspensions *fedSuspensions
	// handled are the fan-out jobs run so far, to run them again.
	handled []queue.Job
	// projectors are the projectors of the harness's UnitOfWork, and outbox enqueues on jobs
	// after their commits.
	projectors *uow.Projectors
	outbox     queue.Outbox
}

func newFedEnv(t *testing.T, newHarness FederationFactory) *fedEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	clock := NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	e := &fedEnv{
		h: h, projectors: projectors, clock: clock, ids: NewIDGenerator(clock), jobs: &fedJobs{seen: map[string]bool{}},
		handlers:    &queue.JobHandlerRegistry{},
		statuses:    &fedStatuses{statuses: map[string]content.Status{}, deleted: map[string]bool{}, revisions: map[string]content.Revision{}, boosts: map[string]content.Boost{}},
		accounts:    &fedAccounts{actors: map[string]appaccount.Actor{}, inboxes: map[string]appaccount.RemoteInbox{}},
		graph:       &fedGraph{followers: map[string][]string{}, blocks: map[[2]string]bool{}},
		blocks:      &fedBlocks{blocks: map[string]graph.Block{}},
		domains:     fedDomains{},
		suspensions: &fedSuspensions{accounts: map[string]bool{}},
		reports:     &fedReports{reports: map[string]appmoderation.Report{}},
	}
	e.parts = &fedParts{accounts: e.accounts, tags: map[string]content.Tag{}, media: map[string][]appcontent.MediaView{}}
	outbox := queue.AfterCommit(projectors, e.jobs, nil)
	e.outbox = outbox
	appfederation.RegisterOutboundProjectors(projectors, appfederation.RecorderDeps{
		Repository: h.Federation, Outbox: outbox, IDs: e.ids, Host: contractHost,
	})
	appfederation.RegisterBlockProjectors(projectors, appfederation.RecorderDeps{
		Repository: h.Federation, Outbox: outbox, IDs: e.ids, Host: contractHost, Accounts: e.accounts,
	})
	appfederation.RegisterSuspensionProjectors(projectors, appfederation.RecorderDeps{
		Repository: h.Federation, Outbox: outbox, IDs: e.ids, Host: contractHost, Accounts: e.accounts,
	})
	appfederation.RegisterReportProjectors(projectors, appfederation.RecorderDeps{
		Repository: h.Federation, Outbox: outbox, IDs: e.ids, Host: contractHost,
	}, e.accounts)
	appfederation.NewFanOut(appfederation.FanOutDeps{
		UnitOfWork: h.UnitOfWork, Repository: h.Federation, Outbox: outbox,
		Statuses: e.statuses, Accounts: e.accounts, Graph: e.graph, Domains: e.domains, Parts: e.parts, Blocks: e.blocks, Reports: e.reports,
		Suspensions: e.suspensions,
		Clock:       clock, IDs: e.ids, Host: contractHost, AccountDomain: contractAccountDomain, Batch: 2,
	}).RegisterJobs(e.handlers)
	return e
}

// fedJobs is a queue.JobQueue that keeps the jobs given to it and skips a job whose idempotency
// key it has seen. deliveries counts every JobDeliver given to it, skipped or not.
type fedJobs struct {
	mu         sync.Mutex
	pending    []queue.NewJob
	seen       map[string]bool
	deliveries int
}

func (q *fedJobs) Enqueue(_ context.Context, jobs ...queue.NewJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range jobs {
		if j.Type == appfederation.JobDeliver {
			q.deliveries++
		}
		if j.IdempotencyKey != "" && q.seen[j.IdempotencyKey] {
			continue
		}
		q.seen[j.IdempotencyKey] = true
		q.pending = append(q.pending, j)
	}
	return nil
}

func (q *fedJobs) pop() (queue.NewJob, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return queue.NewJob{}, false
	}
	j := q.pending[0]
	q.pending = q.pending[1:]
	return j, true
}

func (q *fedJobs) delivered() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.deliveries
}

// drain runs the pending fan-out jobs, and those they enqueue, until none is left. Deliveries
// are only counted.
func (e *fedEnv) drain(t *testing.T) {
	t.Helper()
	fanOut, _, ok := e.handlers.Lookup(appfederation.JobFanOut)
	if !ok {
		t.Fatal("no handler for " + appfederation.JobFanOut)
	}
	for n := 0; ; n++ {
		if n > 1000 {
			t.Fatal("the fan-out does not end")
		}
		nj, ok := e.jobs.pop()
		if !ok {
			return
		}
		if nj.Type != appfederation.JobFanOut {
			continue
		}
		j, err := nj.Build(e.ids.NewID(), e.clock.Now())
		if err != nil {
			t.Fatalf("build job: %v", err)
		}
		if r := fanOut(t.Context(), j); !r.IsDone() {
			t.Fatalf("fan-out job %s = %+v (%v), want Done", j.Payload, r, r.Err())
		}
		e.handled = append(e.handled, j)
	}
}

// fedStatuses stands in for the content module.
type fedStatuses struct {
	mu           sync.Mutex
	statuses     map[string]content.Status
	deleted      map[string]bool
	revisions    map[string]content.Revision
	boosts       map[string]content.Boost
	restrictions map[string]content.AuthorRestrictions
}

func (s *fedStatuses) FindStatus(_ context.Context, id string) (appcontent.Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.statuses[id]
	if !ok {
		return appcontent.Status{}, fmt.Errorf("status %s: %w", id, appcontent.ErrNotFound)
	}
	return st, nil
}

func (s *fedStatuses) LiveRevision(_ context.Context, statusID, revisionID string) (appcontent.Status, appcontent.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.statuses[statusID]
	rev, found := s.revisions[revisionID]
	if !ok || !found || s.deleted[statusID] || rev.StatusID != statusID {
		return appcontent.Status{}, appcontent.Revision{}, fmt.Errorf("status %s: %w", statusID, appcontent.ErrNotFound)
	}
	return st, rev, nil
}

func (s *fedStatuses) AuthorRestrictions(_ context.Context, accountID string) (appcontent.AuthorRestrictions, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restrictions[accountID], nil
}

// PollVoterIDs returns no voters: the contract's statuses have no polls.
func (s *fedStatuses) PollVoterIDs(context.Context, string, string, int) ([]string, error) {
	return nil, nil
}

func (s *fedStatuses) FindBoost(_ context.Context, id string) (appcontent.Boost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.boosts[id]
	if !ok {
		return appcontent.Boost{}, fmt.Errorf("boost %s: %w", id, appcontent.ErrNotFound)
	}
	return b, nil
}

// fedAccounts stands in for the account module.
type fedAccounts struct {
	mu      sync.Mutex
	actors  map[string]appaccount.Actor
	inboxes map[string]appaccount.RemoteInbox
}

func (a *fedAccounts) FindByID(_ context.Context, id string) (appaccount.Actor, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	actor, ok := a.actors[id]
	if !ok {
		return appaccount.Actor{}, fmt.Errorf("account %s: %w", id, appaccount.ErrNotFound)
	}
	return actor, nil
}

func (a *fedAccounts) RemoteInbox(_ context.Context, id string) (appaccount.RemoteInbox, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	in, ok := a.inboxes[id]
	if !ok {
		return appaccount.RemoteInbox{}, fmt.Errorf("remote inbox of %s: %w", id, appaccount.ErrNotFound)
	}
	return in, nil
}

func (a *fedAccounts) KnownSharedInboxes(_ context.Context, after string, limit int) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var all []string
	for _, in := range a.inboxes {
		if in.SharedInboxURL != "" && in.SharedInboxURL > after && !slices.Contains(all, in.SharedInboxURL) {
			all = append(all, in.SharedInboxURL)
		}
	}
	slices.Sort(all)
	return all[:min(limit, len(all))], nil
}

// fedGraph stands in for the graph module. followers are newest first; the cursor of a batch is
// the last follower's account ID.
type fedGraph struct {
	mu        sync.Mutex
	followers map[string][]string
	// blocks holds {blocker, blocked}.
	blocks map[[2]string]bool
}

func (g *fedGraph) ActiveFollowerIDs(_ context.Context, accountID, before string, limit int) ([]string, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	all := g.followers[accountID]
	start := 0
	if before != "" {
		start = slices.Index(all, before) + 1
	}
	end := min(start+limit, len(all))
	ids := slices.Clone(all[start:end])
	next := ""
	if len(ids) == limit {
		next = ids[len(ids)-1]
	}
	return ids, next, nil
}

func (g *fedGraph) Relationship(_ context.Context, accountID, targetID string) (appgraph.Relationship, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := appgraph.Relationship{AccountID: accountID, TargetAccountID: targetID, Following: graph.StateNone, FollowedBy: graph.StateNone}
	if slices.Contains(g.followers[targetID], accountID) {
		r.Following = graph.StateActive
	}
	r.Blocking, r.BlockedBy = g.blocks[[2]string{accountID, targetID}], g.blocks[[2]string{targetID, accountID}]
	return r, nil
}

// fedDomains is a DomainPolicy that suspends the domains it holds.
type fedDomains map[string]bool

func (d fedDomains) FederationSuspended(_ context.Context, domain string) (bool, error) {
	return d[domain], nil
}

// local registers a local account and returns its ID.
func (e *fedEnv) local() string {
	id := e.ids.NewID()
	e.accounts.mu.Lock()
	defer e.accounts.mu.Unlock()
	username := "l" + id[len(id)-6:]
	e.accounts.actors[id] = appaccount.Actor{
		Account:   account.Account{ID: id, URI: account.ActorURI(contractHost, id), Domain: account.LocalDomain, ActorType: account.ActorPerson},
		Local:     &account.LocalAccount{AccountID: id, Username: username, UsernameNormalized: username},
		Handle:    &account.Handle{AccountID: id, Username: username, UsernameNormalized: username},
		Lifecycle: account.LifecycleState{AccountID: id, State: account.StateActive},
	}
	return id
}

// remote registers an actor of domain, with the server's shared inbox when shared, and
// returns its ID.
func (e *fedEnv) remote(domain string, shared bool) string {
	id := e.ids.NewID()
	uri := "https://" + domain + "/users/" + id
	e.accounts.mu.Lock()
	defer e.accounts.mu.Unlock()
	e.accounts.actors[id] = appaccount.Actor{
		Account:   account.Account{ID: id, URI: uri, Domain: domain, ActorType: account.ActorPerson},
		Handle:    &account.Handle{AccountID: id, Username: "r" + id[len(id)-6:], UsernameNormalized: "r" + id[len(id)-6:], Domain: domain},
		Lifecycle: account.LifecycleState{AccountID: id, State: account.StateActive},
	}
	in := appaccount.RemoteInbox{InboxURL: uri + "/inbox"}
	if shared {
		in.SharedInboxURL = "https://" + domain + "/inbox"
	}
	e.accounts.inboxes[id] = in
	return id
}

func (e *fedEnv) uri(id string) string {
	e.accounts.mu.Lock()
	defer e.accounts.mu.Unlock()
	return e.accounts.actors[id].Account.URI
}

// follow makes follower an active follower of followee, the newest one.
func (e *fedEnv) follow(follower, followee string) {
	e.graph.mu.Lock()
	defer e.graph.mu.Unlock()
	e.graph.followers[followee] = append([]string{follower}, e.graph.followers[followee]...)
}

// emit runs one transaction that emits ev, as the content module's commands do.
func (e *fedEnv) emit(t *testing.T, ev uow.Event) {
	t.Helper()
	if err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return tx.Emit(t.Context(), ev) }); err != nil {
		t.Fatalf("emit %s: %v", ev.EventType(), err)
	}
}

// post creates a local status of author mentioning mentions, emits its creation, and returns it.
func (e *fedEnv) post(t *testing.T, author string, v content.Visibility, mentions ...string) content.Status {
	t.Helper()
	return e.postWith(t, author, v, func(r *content.Revision) { r.MentionedAccountIDs = mentions })
}

// postWith creates a local status of author whose first revision edit changes, emits its
// creation, and returns it.
func (e *fedEnv) postWith(t *testing.T, author string, v content.Visibility, edit func(*content.Revision)) content.Status {
	t.Helper()
	e.clock.Advance(time.Second)
	id := e.ids.NewID()
	s := content.Status{
		ID: id, URI: content.StatusURI(contractHost, id),
		AuthorAccountID: author, IsLocal: true, Visibility: v, PublishedAt: e.clock.Now(),
	}
	rev := content.Revision{
		ID: e.ids.NewID(), StatusID: id, Seq: 1, Language: "en", RevisedAt: s.PublishedAt,
		Bodies: []content.Body{{Language: "en", ContentHTML: "<p>hello</p>"}},
	}
	edit(&rev)
	e.statuses.mu.Lock()
	e.statuses.statuses[id], e.statuses.revisions[rev.ID] = s, rev
	e.statuses.mu.Unlock()
	e.emit(t, appcontent.StatusCreated{Status: s, Revision: rev})
	return s
}

func (e *fedEnv) revise(t *testing.T, s content.Status) {
	t.Helper()
	e.reviseWith(t, s, func(*content.Revision) {})
}

// reviseWith adds a revision to s that edit changes and emits it.
func (e *fedEnv) reviseWith(t *testing.T, s content.Status, edit func(*content.Revision)) {
	t.Helper()
	e.clock.Advance(time.Second)
	rev := content.Revision{ID: e.ids.NewID(), StatusID: s.ID, Seq: 2, Language: "en", RevisedAt: e.clock.Now()}
	edit(&rev)
	e.statuses.mu.Lock()
	e.statuses.revisions[rev.ID] = rev
	e.statuses.mu.Unlock()
	e.emit(t, appcontent.StatusRevised{Status: s, Revision: rev})
}

func (e *fedEnv) delete(t *testing.T, s content.Status) {
	t.Helper()
	e.deleteBy(t, s, content.DeletedByAuthor)
}

// deleteBy deletes s with the deletion's origin.
func (e *fedEnv) deleteBy(t *testing.T, s content.Status, origin content.DeletionOrigin) {
	t.Helper()
	e.clock.Advance(time.Second)
	e.statuses.mu.Lock()
	e.statuses.deleted[s.ID] = true
	e.statuses.mu.Unlock()
	e.emit(t, appcontent.StatusDeleted{
		Status:   s,
		Deletion: content.Deletion{StatusID: s.ID, URI: s.URI, AuthorAccountID: s.AuthorAccountID, Origin: origin, DeletedAt: e.clock.Now()},
	})
}

// activity returns the only outbound activity of type typ about an object.
func (e *fedEnv) activity(t *testing.T, kind federation.OutboundObjectKind, objectID string, typ federation.ActivityType) federation.OutboundActivity {
	t.Helper()
	all, err := e.h.Federation.ListOutboundActivitiesOf(t.Context(), kind, objectID)
	if err != nil {
		t.Fatalf("ListOutboundActivitiesOf: %v", err)
	}
	var out []federation.OutboundActivity
	for _, a := range all {
		if a.Type == typ {
			out = append(out, a)
		}
	}
	if len(out) != 1 {
		t.Fatalf("%d %s activities about %s %s, want 1 (all: %+v)", len(out), typ, kind, objectID, all)
	}
	return out[0]
}

// inboxes returns the inboxes an outbound activity is delivered to, sorted.
func (e *fedEnv) inboxes(t *testing.T, outboundID string) []string {
	t.Helper()
	ds, err := e.h.Federation.ListDeliveries(t.Context(), outboundID, "", 1000)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	out := []string{}
	for _, d := range ds {
		out = append(out, string(d.InboxURL))
	}
	slices.Sort(out)
	return out
}

// document returns the stored document of an outbound activity.
func (e *fedEnv) document(t *testing.T, outboundID string) map[string]any {
	t.Helper()
	b, err := e.h.Federation.FindOutboundDocument(t.Context(), outboundID)
	if err != nil {
		t.Fatalf("FindOutboundDocument: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("document %s: %v", b, err)
	}
	return doc
}

func strs(v any) []string {
	out := []string{}
	list, _ := v.([]any)
	for _, s := range list {
		str, _ := s.(string)
		out = append(out, str)
	}
	return out
}

func fedRecordsOutboundActivities(t *testing.T, e *fedEnv) {
	ctx := t.Context()
	at := e.clock.Now()
	create := federation.OutboundActivity{
		ID: e.ids.NewID(), ActivityURI: "https://social.example/ap/objects/s1/activity", Type: federation.ActivityCreate,
		ActorAccountID: NamedID("a1"), ObjectKind: federation.OutboundStatus, ObjectID: NamedID("s1"), CreatedAt: at,
	}
	update := create
	update.ID, update.ActivityURI, update.Type, update.CreatedAt = e.ids.NewID(), "https://social.example/ap/activities/u1", federation.ActivityUpdate, at.Add(time.Second)
	other := create
	other.ID, other.ActivityURI, other.ObjectKind, other.ObjectID, other.Type = e.ids.NewID(), "https://social.example/ap/activities/b1", federation.OutboundBoost, NamedID("s1"), federation.ActivityAnnounce
	err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		for _, a := range []federation.OutboundActivity{create, update, other} {
			if err := e.h.Federation.InsertOutboundActivity(ctx, tx, a); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("InsertOutboundActivity: %v", err)
	}
	if got, err := e.h.Federation.FindOutboundActivity(ctx, update.ID); err != nil || !reflect.DeepEqual(got, update) {
		t.Errorf("FindOutboundActivity = %+v, %v; want %+v", got, err, update)
	}
	if _, err := e.h.Federation.FindOutboundActivity(ctx, e.ids.NewID()); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindOutboundActivity(missing) = %v, want ErrNotFound", err)
	}
	got, err := e.h.Federation.ListOutboundActivitiesOf(ctx, federation.OutboundStatus, NamedID("s1"))
	if err != nil || !reflect.DeepEqual(got, []federation.OutboundActivity{create, update}) {
		t.Errorf("ListOutboundActivitiesOf = %+v, %v; want the Create and the Update", got, err)
	}
	dup := create
	dup.ID = e.ids.NewID()
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Federation.InsertOutboundActivity(ctx, tx, dup) })
	if err == nil {
		t.Error("a second activity with the same URI was accepted")
	}
}

func fedKeepsTheFirstDocument(t *testing.T, e *fedEnv) {
	ctx := t.Context()
	insert := func(doc string) bool {
		var inserted bool
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			inserted, err = e.h.Federation.InsertOutboundDocument(ctx, tx, NamedID("o1"), []byte(doc), e.clock.Now())
			return err
		})
		if err != nil {
			t.Fatalf("InsertOutboundDocument: %v", err)
		}
		return inserted
	}
	if !insert(`{"id":"first"}`) {
		t.Error("the first document was not inserted")
	}
	if insert(`{"id":"second"}`) {
		t.Error("a second document was inserted")
	}
	b, err := e.h.Federation.FindOutboundDocument(ctx, NamedID("o1"))
	var doc map[string]any
	if err != nil || json.Unmarshal(b, &doc) != nil || doc["id"] != "first" {
		t.Errorf("FindOutboundDocument = %s, %v; want the first document", b, err)
	}
	if _, err := e.h.Federation.FindOutboundDocument(ctx, NamedID("o2")); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("FindOutboundDocument(missing) = %v, want ErrNotFound", err)
	}
}

func fedDeliversToAnInboxOnce(t *testing.T, e *fedEnv) {
	ctx := t.Context()
	var want []federation.Delivery
	for _, inbox := range []string{"https://a.example/inbox", "https://b.example/inbox", "https://c.example/inbox"} {
		e.clock.Advance(time.Millisecond)
		want = append(want, federation.Delivery{
			ID: e.ids.NewID(), OutboundActivityID: NamedID("o1"), InboxURL: federation.IRI(inbox), Domain: federation.IRI(inbox).Host(), CreatedAt: e.clock.Now(),
		})
	}
	insert := func(d federation.Delivery) bool {
		var inserted bool
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			inserted, err = e.h.Federation.InsertDelivery(ctx, tx, d)
			return err
		})
		if err != nil {
			t.Fatalf("InsertDelivery: %v", err)
		}
		return inserted
	}
	for _, d := range want {
		if !insert(d) {
			t.Errorf("delivery to %s was not inserted", d.InboxURL)
		}
	}
	again := want[1]
	again.ID = e.ids.NewID()
	if insert(again) {
		t.Error("a second delivery to the same inbox was inserted")
	}
	other := again
	other.OutboundActivityID = NamedID("o2")
	if !insert(other) {
		t.Error("the delivery of another activity to the same inbox was not inserted")
	}
	first, err := e.h.Federation.ListDeliveries(ctx, NamedID("o1"), "", 2)
	if err != nil || !reflect.DeepEqual(first, want[:2]) {
		t.Errorf("ListDeliveries(first 2) = %+v, %v; want %+v", first, err, want[:2])
	}
	rest, err := e.h.Federation.ListDeliveries(ctx, NamedID("o1"), want[1].ID, 2)
	if err != nil || !reflect.DeepEqual(rest, want[2:]) {
		t.Errorf("ListDeliveries(after 2) = %+v, %v; want %+v", rest, err, want[2:])
	}
}

func fedReadsTheStateOfServers(t *testing.T, e *fedEnv) {
	ctx := t.Context()
	if s, err := e.h.Federation.InstanceState(ctx, "new.example"); err != nil || s != federation.InstanceAvailable {
		t.Errorf("InstanceState(unknown) = %q, %v; want available", s, err)
	}
	e.markUnavailable(t, "down.example")
	if s, err := e.h.Federation.InstanceState(ctx, "down.example"); err != nil || s != federation.InstanceUnavailable {
		t.Errorf("InstanceState(down) = %q, %v; want unavailable", s, err)
	}
}

func (e *fedEnv) markUnavailable(t *testing.T, domain string) {
	t.Helper()
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return e.h.Federation.UpsertInstanceHealth(t.Context(), tx, federation.InstanceHealth{
			Domain: domain, State: federation.InstanceUnavailable, Since: e.clock.Now(), FailureDays: 7, LastFailureDay: "20261017",
		})
	})
	if err != nil {
		t.Fatalf("UpsertInstanceHealth: %v", err)
	}
}

// fedAudience is an author with followers and mentionable accounts of every kind.
type fedAudience struct {
	author string
	// sharedA and sharedB follow on a.example, which has a shared inbox; ownInbox follows on
	// b.example, which has none; local follows from this server.
	sharedA, sharedB, ownInbox, local string
	// mentioned is a remote account on c.example that does not follow; localMentioned a local
	// one; blocker a remote account that blocks the author.
	mentioned, localMentioned, blocker string
}

func (e *fedEnv) audience() fedAudience {
	a := fedAudience{
		author: e.local(), sharedA: e.remote("a.example", true), sharedB: e.remote("a.example", true),
		ownInbox: e.remote("b.example", false), local: e.local(),
		mentioned: e.remote("c.example", true), localMentioned: e.local(), blocker: e.remote("d.example", false),
	}
	for _, f := range []string{a.sharedA, a.sharedB, a.ownInbox, a.local} {
		e.follow(f, a.author)
	}
	e.graph.mu.Lock()
	e.graph.blocks[[2]string{a.blocker, a.author}] = true
	e.graph.mu.Unlock()
	return a
}

func fedDeliversEachVisibility(t *testing.T, e *fedEnv) {
	a := e.audience()
	followers := e.uri(a.author) + "/followers"
	public := federation.PublicIRI
	mentions := []string{e.uri(a.mentioned), e.uri(a.localMentioned), e.uri(a.blocker)}
	followersInboxes := []string{"https://a.example/inbox", "https://b.example/users/" + a.ownInbox + "/inbox"}
	everyone := append(slices.Clone(followersInboxes), "https://c.example/inbox")
	slices.Sort(everyone)

	// data-model §7 and federation.md §7.4: who each visibility reaches and how it is addressed.
	cases := []struct {
		visibility content.Visibility
		inboxes    []string
		to, cc     []string
	}{
		{content.VisibilityPublic, everyone, []string{string(public)}, append([]string{followers}, mentions...)},
		{content.VisibilityUnlisted, everyone, []string{followers}, append([]string{string(public)}, mentions...)},
		{content.VisibilityPrivate, everyone, []string{followers}, mentions},
		{content.VisibilityLocal, followersInboxes, []string{followers}, []string{}},
		{content.VisibilityDirect, []string{"https://c.example/inbox"}, mentions, []string{}},
	}
	for _, tc := range cases {
		t.Run(string(tc.visibility), func(t *testing.T) {
			s := e.post(t, a.author, tc.visibility, a.mentioned, a.localMentioned, a.blocker)
			e.drain(t)
			create := e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityCreate)
			if create.ActivityURI != federation.IRI(s.URI+"/activity") || create.ActorAccountID != a.author {
				t.Errorf("Create = %+v, want the activity {status}/activity of the author", create)
			}
			if got := e.inboxes(t, create.ID); !slices.Equal(got, tc.inboxes) {
				t.Errorf("inboxes = %v, want %v", got, tc.inboxes)
			}
			doc := e.document(t, create.ID)
			note, _ := doc["object"].(map[string]any)
			if doc["type"] != "Create" || doc["actor"] != e.uri(a.author) || note["id"] != s.URI || note["attributedTo"] != e.uri(a.author) {
				t.Errorf("document = %v, want the author's Create of the status", doc)
			}
			if got := strs(doc["to"]); !slices.Equal(got, tc.to) {
				t.Errorf("to = %v, want %v", got, tc.to)
			}
			if got := strs(doc["cc"]); !slices.Equal(got, tc.cc) {
				t.Errorf("cc = %v, want %v", got, tc.cc)
			}
			if !slices.Equal(strs(note["to"]), tc.to) || !slices.Equal(strs(note["cc"]), tc.cc) {
				t.Errorf("note to = %v, cc = %v; want the activity's", note["to"], note["cc"])
			}
		})
	}
}

func fedUpdateAndDeleteFollowTheCreate(t *testing.T, e *fedEnv) {
	a := e.audience()
	for _, tc := range []struct {
		visibility content.Visibility
		// later says whether a follower who arrives after the Create gets the Update and Delete.
		later bool
	}{
		{content.VisibilityPublic, true},
		{content.VisibilityUnlisted, true},
		{content.VisibilityPrivate, false},
		{content.VisibilityLocal, false},
		{content.VisibilityDirect, false},
	} {
		t.Run(string(tc.visibility), func(t *testing.T) {
			s := e.post(t, a.author, tc.visibility, a.mentioned)
			e.drain(t)
			created := e.inboxes(t, e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityCreate).ID)

			newcomer := e.remote("e.example", false)
			e.follow(newcomer, a.author)
			want := slices.Clone(created)
			if tc.later {
				want = append(want, "https://e.example/users/"+newcomer+"/inbox")
				slices.Sort(want)
			}

			e.revise(t, s)
			e.drain(t)
			update := e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityUpdate)
			if got := e.inboxes(t, update.ID); !slices.Equal(got, want) {
				t.Errorf("Update inboxes = %v, want %v", got, want)
			}
			if doc := e.document(t, update.ID); doc["type"] != "Update" || doc["id"] != string(update.ActivityURI) {
				t.Errorf("Update document = %v", doc)
			}

			e.delete(t, s)
			e.drain(t)
			del := e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityDelete)
			if got := e.inboxes(t, del.ID); !slices.Equal(got, want) {
				t.Errorf("Delete inboxes = %v, want %v", got, want)
			}
			doc := e.document(t, del.ID)
			obj, _ := doc["object"].(map[string]any)
			if doc["type"] != "Delete" || obj["type"] != "Tombstone" || obj["id"] != s.URI {
				t.Errorf("Delete document = %v, want a Tombstone of the status", doc)
			}
		})
	}
}

func fedFanOutAgain(t *testing.T, e *fedEnv) {
	a := e.audience()
	for i := range 3 {
		e.follow(e.remote(fmt.Sprintf("f%d.example", i), false), a.author)
	}
	s := e.post(t, a.author, content.VisibilityPublic, a.mentioned)
	e.drain(t)
	create := e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityCreate)
	inboxes := e.inboxes(t, create.ID)
	delivered := e.jobs.delivered()
	if delivered != len(inboxes) || len(inboxes) != 6 {
		t.Fatalf("%d JobDeliver for %d inboxes %v, want one per inbox and 6 inboxes", delivered, len(inboxes), inboxes)
	}
	doc := e.document(t, create.ID)

	fanOut, _, _ := e.handlers.Lookup(appfederation.JobFanOut)
	for _, j := range slices.Clone(e.handled) {
		if r := fanOut(t.Context(), j); !r.IsDone() {
			t.Fatalf("fan-out again = %+v (%v), want Done", r, r.Err())
		}
	}
	e.drain(t)
	if got := e.inboxes(t, create.ID); !slices.Equal(got, inboxes) {
		t.Errorf("inboxes after the fan-out again = %v, want %v", got, inboxes)
	}
	if got := e.jobs.delivered(); got != delivered {
		t.Errorf("%d JobDeliver after the fan-out again, want %d", got, delivered)
	}
	if again := e.document(t, create.ID); !reflect.DeepEqual(again, doc) {
		t.Errorf("document after the fan-out again = %v, want %v", again, doc)
	}
}

func fedLeavesOutServers(t *testing.T, e *fedEnv) {
	a := e.audience()
	e.markUnavailable(t, "b.example")
	e.domains["c.example"] = true
	s := e.post(t, a.author, content.VisibilityPublic, a.mentioned)
	e.drain(t)
	create := e.activity(t, federation.OutboundStatus, s.ID, federation.ActivityCreate)
	if got, want := e.inboxes(t, create.ID), []string{"https://a.example/inbox"}; !slices.Equal(got, want) {
		t.Errorf("inboxes = %v, want only %v: b.example is unavailable and c.example suspended", got, want)
	}
}

// reviseProfile emits the revision seq of an account's profile, of a remote actor when remote.
func (e *fedEnv) reviseProfile(t *testing.T, accountID string, seq int, remote bool) {
	t.Helper()
	e.clock.Advance(time.Second)
	rev := account.ProfileRevision{ID: e.ids.NewID(), AccountID: accountID, Seq: seq, DisplayName: "Author", RevisedAt: e.clock.Now()}
	if remote {
		rev.Remote = &account.RemoteProfileSource{SnapshotID: e.ids.NewID(), HTMLPolicyVersion: 1}
	}
	e.emit(t, appaccount.ProfileRevised{Revision: rev})
}

// lifecycle emits a lifecycle event of an account.
func (e *fedEnv) lifecycle(t *testing.T, accountID string, kind account.LifecycleKind) {
	t.Helper()
	e.clock.Advance(time.Second)
	e.emit(t, appaccount.LifecycleEventRecorded{Event: account.LifecycleEvent{
		ID: e.ids.NewID(), AccountID: accountID, Kind: kind, InitiatedBy: account.InitiatedBySelf, OccurredAt: e.clock.Now(),
	}})
}

func fedUpdatesAndDeletesActors(t *testing.T, e *fedEnv) {
	author := e.local()
	e.accounts.mu.Lock()
	actor := e.accounts.actors[author]
	actor.Local = &account.LocalAccount{AccountID: author, Username: "author"}
	actor.Profile = &account.ProfileRevision{AccountID: author, Seq: 2, DisplayName: "Author", NoteHTML: "<p>bio</p>"}
	e.accounts.actors[author] = actor
	e.accounts.mu.Unlock()
	for _, f := range []string{e.remote("a.example", true), e.remote("a.example", true), e.remote("b.example", false), e.remote("c.example", true), e.local()} {
		e.follow(f, author)
	}
	// Servers that no follower is on: d.example has a shared inbox, e.example does not.
	e.remote("d.example", true)
	e.remote("e.example", false)
	followerInboxes := []string{"https://a.example/inbox", "https://b.example/users/" + e.graph.followers[author][2] + "/inbox", "https://c.example/inbox"}
	slices.Sort(followerInboxes)

	// The first revision, which a new account is created with, and remote revisions send nothing.
	e.reviseProfile(t, author, 1, false)
	e.reviseProfile(t, e.graph.followers[author][1], 2, true)
	e.lifecycle(t, e.graph.followers[author][1], account.LifecycleRemoteDeleted)
	e.drain(t)
	if all, err := e.h.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundAccount, author); err != nil || len(all) != 0 {
		t.Fatalf("activities about the author = %+v, %v; want none", all, err)
	}

	// A profile revision sends the actor's document to its followers, shared inboxes merged.
	e.reviseProfile(t, author, 2, false)
	e.drain(t)
	update := e.activity(t, federation.OutboundAccount, author, federation.ActivityUpdate)
	if update.ActorAccountID != author || !strings.HasPrefix(string(update.ActivityURI), "https://"+contractHost+"/ap/activities/") {
		t.Errorf("Update = %+v, want the author's /ap/activities/{id}", update)
	}
	if got := e.inboxes(t, update.ID); !slices.Equal(got, followerInboxes) {
		t.Errorf("Update inboxes = %v, want %v", got, followerInboxes)
	}
	doc := e.document(t, update.ID)
	person, _ := doc["object"].(map[string]any)
	if doc["type"] != "Update" || doc["actor"] != e.uri(author) || !slices.Equal(strs(doc["to"]), []string{string(federation.PublicIRI)}) {
		t.Errorf("Update document = %v, want the author's Update to the public", doc)
	}
	if person["type"] != "Person" || person["id"] != e.uri(author) || person["name"] != "Author" || person["summary"] != "<p>bio</p>" ||
		person["preferredUsername"] != "author" || person["inbox"] != e.uri(author)+"/inbox" || person["@context"] != nil {
		t.Errorf("Update object = %v, want the author's embedded actor document", person)
	}

	// The account's deletion reaches its followers and every known shared inbox.
	e.lifecycle(t, author, account.LifecycleDeleted)
	e.drain(t)
	del := e.activity(t, federation.OutboundAccount, author, federation.ActivityDelete)
	want := append(slices.Clone(followerInboxes), "https://d.example/inbox")
	slices.Sort(want)
	if got := e.inboxes(t, del.ID); !slices.Equal(got, want) {
		t.Errorf("Delete inboxes = %v, want %v", got, want)
	}
	doc = e.document(t, del.ID)
	if doc["type"] != "Delete" || doc["actor"] != e.uri(author) || doc["object"] != e.uri(author) || !slices.Equal(strs(doc["to"]), []string{string(federation.PublicIRI)}) {
		t.Errorf("Delete document = %v, want the Delete of the author's id to the public", doc)
	}

	// The erasure's deletions of the account's statuses send nothing: the Delete of the actor
	// stands for them.
	s := e.post(t, author, content.VisibilityPublic)
	e.clock.Advance(time.Second)
	e.emit(t, appcontent.StatusDeleted{
		Status: s,
		Deletion: content.Deletion{
			StatusID: s.ID, URI: s.URI, AuthorAccountID: author, Origin: content.DeletedByAccountDeletion, DeletedAt: e.clock.Now(),
		},
	})
	e.drain(t)
	statusActs, _ := e.h.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundStatus, s.ID)
	for _, a := range statusActs {
		if a.Type == federation.ActivityDelete {
			t.Errorf("erasure of a status recorded %+v, want no Delete", a)
		}
	}

	// An Update of an account that is no longer active has nothing to send.
	e.accounts.mu.Lock()
	actor = e.accounts.actors[author]
	actor.Lifecycle.State = account.StateDeleted
	e.accounts.actors[author] = actor
	e.accounts.mu.Unlock()
	e.reviseProfile(t, author, 3, false)
	e.drain(t)
	var updates []federation.OutboundActivity
	all, _ := e.h.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundAccount, author)
	for _, a := range all {
		if a.Type == federation.ActivityUpdate && a.ID != update.ID {
			updates = append(updates, a)
		}
	}
	if len(updates) != 1 {
		t.Fatalf("updates = %+v, want the new one recorded", updates)
	}
	if got := e.inboxes(t, updates[0].ID); len(got) != 0 {
		t.Errorf("Update of a deleted account delivered to %v, want nowhere", got)
	}
}

func fedRecordsOnlyWhatFederates(t *testing.T, e *fedEnv) {
	a := e.audience()
	booster := e.local()
	follower := e.remote("g.example", true)
	e.follow(follower, booster)
	remoteAuthor := e.remote("r.example", false)

	boost := func(s content.Status) content.Boost {
		e.clock.Advance(time.Second)
		id := e.ids.NewID()
		b := content.Boost{
			ID: id, URI: content.ActivityURI(contractHost, id), BoosterAccountID: booster, StatusID: s.ID,
			Visibility: s.Visibility, IsLocal: true, BoostedAt: e.clock.Now(),
		}
		e.statuses.mu.Lock()
		e.statuses.boosts[id] = b
		e.statuses.mu.Unlock()
		e.emit(t, appcontent.Boosted{Boost: b, Status: appcontent.RefOf(s)})
		e.drain(t)
		return b
	}

	// A boost of a local status stays on this server (ADR-0017).
	local := e.post(t, a.author, content.VisibilityLocal)
	e.drain(t)
	b := boost(local)
	if got, err := e.h.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundBoost, b.ID); err != nil || len(got) != 0 {
		t.Errorf("activities about the boost of a local status = %+v, %v; want none", got, err)
	}

	// A remote status is not this server's to deliver.
	e.clock.Advance(time.Second)
	remoteID := e.ids.NewID()
	remote := content.Status{
		ID: remoteID, URI: "https://r.example/notes/" + remoteID, AuthorAccountID: remoteAuthor,
		Visibility: content.VisibilityPublic, PublishedAt: e.clock.Now(),
	}
	e.statuses.mu.Lock()
	e.statuses.statuses[remoteID] = remote
	e.statuses.mu.Unlock()
	e.emit(t, appcontent.StatusCreated{Status: remote, Revision: content.Revision{ID: e.ids.NewID(), StatusID: remoteID, Seq: 1, RevisedAt: e.clock.Now()}})
	if got, err := e.h.Federation.ListOutboundActivitiesOf(t.Context(), federation.OutboundStatus, remoteID); err != nil || len(got) != 0 {
		t.Errorf("activities about a remote status = %+v, %v; want none", got, err)
	}

	// A boost of a remote public status goes to the booster's followers and the status's author.
	b = boost(remote)
	announce := e.activity(t, federation.OutboundBoost, b.ID, federation.ActivityAnnounce)
	want := []string{"https://g.example/inbox", "https://r.example/users/" + remoteAuthor + "/inbox"}
	if got := e.inboxes(t, announce.ID); !slices.Equal(got, want) {
		t.Errorf("Announce inboxes = %v, want %v", got, want)
	}
	doc := e.document(t, announce.ID)
	if doc["type"] != "Announce" || doc["id"] != b.URI || doc["object"] != remote.URI ||
		!slices.Equal(strs(doc["to"]), []string{string(federation.PublicIRI)}) ||
		!slices.Equal(strs(doc["cc"]), []string{e.uri(booster) + "/followers", e.uri(remoteAuthor)}) {
		t.Errorf("Announce document = %v", doc)
	}

	e.clock.Advance(time.Second)
	e.emit(t, appcontent.BoostRetracted{
		Boost: b, Retraction: content.BoostRetraction{BoostID: b.ID, Origin: content.RetractedByBooster, RetractedAt: e.clock.Now()},
		Status: appcontent.RefOf(remote),
	})
	e.drain(t)
	undo := e.activity(t, federation.OutboundBoost, b.ID, federation.ActivityUndo)
	if got := e.inboxes(t, undo.ID); !slices.Equal(got, want) {
		t.Errorf("Undo inboxes = %v, want %v", got, want)
	}
	inner, _ := e.document(t, undo.ID)["object"].(map[string]any)
	if inner["type"] != "Announce" || inner["id"] != b.URI || inner["@context"] != nil {
		t.Errorf("Undo object = %v, want the embedded Announce", inner)
	}
}
