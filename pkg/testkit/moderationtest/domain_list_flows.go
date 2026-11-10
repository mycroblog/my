// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// DomainListFlows verifies the subscriptions to domain block lists of a database adapter's
// moderation repository (ADR-0159 §4, §5) through the moderation use cases: a fetch reads the list
// through the HTTP client with its validators and is idempotent (fetching it twice issues nothing
// twice), the automatic mode issues what no action in effect covers and lifts, on the next fetch,
// what the list no longer names, an action a person issued on the same domain is left as it is
// and a lift by a person sticks, a failed fetch is recorded and lifts nothing and the third in a
// row is told, a severity the server does not know and a fetch that would lift too much stay
// drafts, drafts are accepted and rejected with their audit, the next subscription issues what an
// earlier one lifted, a deleted subscription keeps or lifts its actions, and an action names the
// subscription that issued it. These are the flows of ADR-0153 §1, which internal/flowtest/moderation
// runs once on SQLite.
func DomainListFlows(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *domainListEnv)
	}{
		{"FetchIsIdempotent", domainListIdempotent},
		{"AutomaticModeLiftsWhatTheListRemoves", domainListAutomaticLifts},
		{"LeavesThePeoplesActionsAlone", domainListLeavesPeople},
		{"FailuresAreRecordedAndLiftNothing", domainListFailures},
		{"UnknownSeveritiesAndMassLiftsStayDrafts", domainListHeld},
		{"DraftsAreAcceptedAndRejected", domainListDrafts},
		{"NextSubscriptionIssuesWhatAnEarlierOneLifted", domainListOverlap},
		{"DeletionKeepsOrLiftsTheActions", domainListDeletion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newDomainListEnv(t, newHarness))
		})
	}
}

// listServer stands in for the servers that publish the lists, one body per URL.
type listServer struct {
	mu    sync.Mutex
	lists map[string]*servedList
	// requests are the requests received, in order.
	requests []kernel.HTTPRequest
}

type servedList struct {
	status int
	body   string
	etag   string
	err    error
}

func (s *listServer) serve(url string, l servedList) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.status == 0 {
		l.status = http.StatusOK
	}
	s.lists[url] = &l
}

// Do implements kernel.HTTPClient.
func (s *listServer) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, *req)
	l, ok := s.lists[req.URL]
	if !ok {
		return &kernel.HTTPResponse{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	if l.err != nil {
		return nil, l.err
	}
	h := http.Header{}
	if l.etag != "" {
		h.Set("ETag", l.etag)
		if req.Header.Get("If-None-Match") == l.etag {
			return &kernel.HTTPResponse{StatusCode: http.StatusNotModified, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
	}
	return &kernel.HTTPResponse{StatusCode: l.status, Header: h, URL: req.URL, Body: io.NopCloser(strings.NewReader(l.body))}, nil
}

type domainListEnv struct {
	h       ModerationHarness
	clock   *testkit.Clock
	ids     *testkit.IDGenerator
	svc     *appmoderation.Service
	server  *listServer
	mu      sync.Mutex
	failing []appmoderation.DomainListFailing
	// moderator is a user who acts.
	moderator string
}

func newDomainListEnv(t *testing.T, newHarness ModerationFactory) *domainListEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appmoderation.RegisterAuditProjector(projectors, h.Moderation)
	clock := testkit.NewClock(time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	ids := testkit.NewIDGenerator(clock)
	e := &domainListEnv{h: h, clock: clock, ids: ids, server: &listServer{lists: map[string]*servedList{}}, moderator: ids.NewID()}
	projectors.Register(appmoderation.EventDomainListFailing, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.failing = append(e.failing, ev.(appmoderation.DomainListFailing))
		return nil
	})
	e.svc = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Moderation, Accounts: &identitytest.StubActors{Actors: map[string]appaccount.Actor{}},
		Clock: clock, IDs: ids, HTTP: e.server, LocalDomains: []string{"local.example"},
	})
	return e
}

// subscribe subscribes to url in mode, with the default settings otherwise.
func (e *domainListEnv) subscribe(t *testing.T, url string, format moderation.DomainListFormat, mode moderation.DomainListMode) moderation.DomainListSubscriptionRecord {
	t.Helper()
	st := moderation.DefaultDomainListSettings("List " + url)
	st.Mode = mode
	rec, err := e.svc.CreateDomainListSubscription(t.Context(), appmoderation.DomainListSubscriptionInput{
		URL: url, Format: format, Settings: st, CreatedByUserID: e.moderator,
	})
	if err != nil {
		t.Fatalf("CreateDomainListSubscription(%s) = %v", url, err)
	}
	return rec
}

func (e *domainListEnv) fetch(t *testing.T, id string, want moderation.DomainListFetchOutcome) moderation.DomainListFetch {
	t.Helper()
	e.clock.Advance(time.Minute)
	f, err := e.svc.FetchDomainList(t.Context(), id)
	if err != nil || f.Outcome != want {
		t.Fatalf("FetchDomainList(%s) = %+v, %v; want %s", id, f, err, want)
	}
	return f
}

// inEffect returns the actions in effect as "domain kind [subscription]" lines, sorted, where
// subscription is the label of the subscription in names ("-" for a person's).
func (e *domainListEnv) inEffect(t *testing.T, names map[string]string) []string {
	t.Helper()
	vs, err := e.svc.DomainActions(t.Context(), appmoderation.DomainActionFilter{Active: true}, 1000)
	if err != nil {
		t.Fatalf("DomainActions = %v", err)
	}
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		by := "-"
		if v.SubscriptionID != "" {
			by = names[v.SubscriptionID]
		}
		out = append(out, fmt.Sprintf("%s %s %s", v.Action.Domain, v.Action.Kind, by))
	}
	slices.Sort(out)
	return out
}

func (e *domainListEnv) wantInEffect(t *testing.T, names map[string]string, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := e.inEffect(t, names); !slices.Equal(got, want) {
		t.Errorf("actions in effect = %q, want %q", got, want)
	}
}

func (e *domainListEnv) drafts(t *testing.T, id string) []string {
	t.Helper()
	ds, err := e.svc.DomainListDrafts(t.Context(), id)
	if err != nil {
		t.Fatalf("DomainListDrafts(%s) = %v", id, err)
	}
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Key() + " " + string(d.Held)
	}
	return out
}

func (e *domainListEnv) audits(t *testing.T, action moderation.AuditAction) []moderation.AuditEntry {
	t.Helper()
	es, err := e.svc.AuditLog(t.Context(), appmoderation.AuditQuery{Action: action}, 1000)
	if err != nil {
		t.Fatalf("AuditLog(%s) = %v", action, err)
	}
	return es
}

const listCSV = "#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate\n"

func domainListIdempotent(t *testing.T, e *domainListEnv) {
	const url = "https://lists.example/blocks.csv"
	e.server.serve(url, servedList{body: listCSV + "spam.example,suspend,false,false,Spam,false\nloud.example,silence,true,false,,false\nlocal.example,suspend,,,,\n"})
	rec := e.subscribe(t, url, moderation.DomainListMastodonCSV, moderation.DomainListAutomatic)
	names := map[string]string{rec.Subscription.ID: "A"}
	f := e.fetch(t, rec.Subscription.ID, moderation.DomainListApplied)
	if f.EntryCount != 2 || f.HTTPStatus != http.StatusOK {
		t.Errorf("first fetch = %+v; want 2 entries (this instance's domain left out) and 200", f)
	}
	want := []string{"spam.example suspend A", "loud.example limit A", "loud.example reject_media A"}
	e.wantInEffect(t, names, want...)
	// The same list again, without validators: nothing changes and nothing is issued twice.
	e.fetch(t, rec.Subscription.ID, moderation.DomainListNotModified)
	e.wantInEffect(t, names, want...)
	if n := len(e.audits(t, moderation.AuditDomainActionIssued)); n != 3 {
		t.Errorf("domain_action_issued entries = %d, want 3", n)
	}
	for _, a := range e.audits(t, moderation.AuditDomainActionIssued) {
		if a.ActorKind != moderation.AuditActorSystem {
			t.Errorf("audit of an automatic action = %+v; want the system as the actor", a)
		}
	}
	// With an ETag, the next fetch asks for it and the 304 changes nothing.
	e.server.serve(url, servedList{body: listCSV + "spam.example,suspend,false,false,Spam,false\nloud.example,silence,true,false,,false\n", etag: `"v2"`})
	e.fetch(t, rec.Subscription.ID, moderation.DomainListApplied)
	e.fetch(t, rec.Subscription.ID, moderation.DomainListNotModified)
	e.server.mu.Lock()
	last := e.server.requests[len(e.server.requests)-1]
	e.server.mu.Unlock()
	if last.Header.Get("If-None-Match") != `"v2"` || last.Purpose != kernel.HTTPPurposeDomainList {
		t.Errorf("last request = %+v; want If-None-Match \"v2\" for the domain list purpose", last)
	}
	e.wantInEffect(t, names, want...)
	fetches, err := e.svc.DomainListFetches(t.Context(), rec.Subscription.ID, "", 10)
	if err != nil || len(fetches) != 4 {
		t.Errorf("DomainListFetches = %d, %v; want 4", len(fetches), err)
	}
	// An action names the subscription that issued it.
	vs, _ := e.svc.DomainActions(t.Context(), appmoderation.DomainActionFilter{Domain: "spam.example"}, 10)
	if len(vs) != 1 || vs[0].SubscriptionID != rec.Subscription.ID {
		t.Errorf("DomainActions(spam.example) = %+v; want the action of the subscription", vs)
	}
	if v, err := e.svc.DomainAction(t.Context(), vs[0].Action.ID); err != nil || v.SubscriptionID != rec.Subscription.ID {
		t.Errorf("DomainAction = %+v, %v; want the subscription", v, err)
	}
}

func domainListAutomaticLifts(t *testing.T, e *domainListEnv) {
	const url = "https://lists.example/plain.txt"
	e.server.serve(url, servedList{body: "a.example\nb.example\nc.example\n"})
	rec := e.subscribe(t, url, moderation.DomainListPlainText, moderation.DomainListAutomatic)
	names := map[string]string{rec.Subscription.ID: "A"}
	e.fetch(t, rec.Subscription.ID, moderation.DomainListApplied)
	e.wantInEffect(t, names, "a.example suspend A", "b.example suspend A", "c.example suspend A")
	e.server.serve(url, servedList{body: "a.example\nc.example\n"})
	e.fetch(t, rec.Subscription.ID, moderation.DomainListApplied)
	e.wantInEffect(t, names, "a.example suspend A", "c.example suspend A")
	if lifts := e.audits(t, moderation.AuditDomainActionLifted); len(lifts) != 1 || lifts[0].TargetID != "b.example" || lifts[0].ActorKind != moderation.AuditActorSystem {
		t.Errorf("domain_action_lifted entries = %+v; want the lift of b.example by the system", lifts)
	}
	// New settings apply at the next fetch: suspend as limit replaces the suspensions.
	st := rec.Revision.Settings
	st.SuspendAs = moderation.DomainListAsLimit
	if _, err := e.svc.UpdateDomainListSubscription(t.Context(), rec.Subscription.ID, st, e.moderator); err != nil {
		t.Fatalf("UpdateDomainListSubscription = %v", err)
	}
	e.fetch(t, rec.Subscription.ID, moderation.DomainListNotModified)
	e.wantInEffect(t, names, "a.example limit A", "c.example limit A")
	if n := len(e.audits(t, moderation.AuditDomainListSubscriptionUpdated)); n != 1 {
		t.Errorf("domain_list_subscription_updated entries = %d, want 1", n)
	}
}

func domainListLeavesPeople(t *testing.T, e *domainListEnv) {
	ctx := t.Context()
	human, err := e.svc.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "human.example", Kind: moderation.DomainSuspend, IssuedByUserID: e.moderator})
	if err != nil {
		t.Fatalf("IssueDomainAction = %v", err)
	}
	const url = "https://lists.example/people.txt"
	e.server.serve(url, servedList{body: "human.example\nsub.human.example\nlisted.example\n"})
	rec := e.subscribe(t, url, moderation.DomainListPlainText, moderation.DomainListAutomatic)
	names := map[string]string{rec.Subscription.ID: "A"}
	e.fetch(t, rec.Subscription.ID, moderation.DomainListApplied)
	// The person's suspension covers the domain and its subdomain: the subscription issues
	// nothing there.
	e.wantInEffect(t, names, "human.example suspend -", "listed.example suspend A")
	// The list drops the domain: the person's action stays.
	e.server.serve(url, servedList{body: "listed.example\n"})
	e.fetch(t, rec.Subscription.ID, moderation.DomainListApplied)
	e.wantInEffect(t, names, "human.example suspend -", "listed.example suspend A")
	if v, err := e.svc.DomainAction(ctx, human.ID); err != nil || v.Lift != nil || v.SubscriptionID != "" {
		t.Errorf("the person's action = %+v, %v; want it in effect, of nobody's subscription", v, err)
	}
	// A person lifts the subscription's action: the subscription does not issue it again
	// while the list names the domain alike.
	vs, _ := e.svc.DomainActions(ctx, appmoderation.DomainActionFilter{Domain: "listed.example", Active: true}, 10)
	if len(vs) != 1 {
		t.Fatalf("DomainActions(listed.example) = %+v", vs)
	}
	if err := e.svc.LiftDomainAction(ctx, vs[0].Action.ID, e.moderator); err != nil {
		t.Fatalf("LiftDomainAction = %v", err)
	}
	e.server.serve(url, servedList{body: "listed.example\n\n"})
	e.fetch(t, rec.Subscription.ID, moderation.DomainListApplied)
	e.wantInEffect(t, names, "human.example suspend -")
	if d := e.drafts(t, rec.Subscription.ID); len(d) != 0 {
		t.Errorf("drafts = %v; want none", d)
	}
}

func domainListFailures(t *testing.T, e *domainListEnv) {
	const url = "https://lists.example/fails.csv"
	e.server.serve(url, servedList{body: listCSV + "spam.example,suspend,,,,\n"})
	rec := e.subscribe(t, url, moderation.DomainListMastodonCSV, moderation.DomainListAutomatic)
	id := rec.Subscription.ID
	names := map[string]string{id: "A"}
	e.fetch(t, id, moderation.DomainListApplied)
	for i, l := range []servedList{
		{status: http.StatusInternalServerError, body: "oops"},
		{body: "#domain\n\"broken\n"},
		{err: fmt.Errorf("dial: %w", kernel.ErrForbiddenDestination)},
		{status: http.StatusNotFound},
	} {
		e.server.serve(url, l)
		f := e.fetch(t, id, moderation.DomainListFailed)
		if f.EntryCount != 1 || f.ErrorCode == "" {
			t.Errorf("failed fetch %d = %+v; want the error and the 1 entry kept", i, f)
		}
		e.wantInEffect(t, names, "spam.example suspend A")
	}
	got, err := e.svc.DomainListSubscription(t.Context(), id)
	if err != nil || got.Head.ConsecutiveFailures != 4 || got.LastFetch == nil || got.LastFetch.ErrorCode != moderation.DomainListErrorHTTPStatus || got.LastFetch.HTTPStatus != http.StatusNotFound {
		t.Errorf("DomainListSubscription = %+v, %v; want 4 failures, the last a 404", got, err)
	}
	fetches, _ := e.svc.DomainListFetches(t.Context(), id, "", 10)
	codes := make([]moderation.DomainListFetchError, 0, 4)
	for _, f := range fetches[:4] {
		codes = append(codes, f.ErrorCode)
	}
	if want := []moderation.DomainListFetchError{"http_status", "forbidden_destination", "malformed", "http_status"}; !slices.Equal(codes, want) {
		t.Errorf("error codes, newest first = %v, want %v", codes, want)
	}
	e.mu.Lock()
	failing := slices.Clone(e.failing)
	e.mu.Unlock()
	if len(failing) != 1 || failing[0].SubscriptionID != id {
		t.Errorf("DomainListFailing = %+v; want one, at the third failure", failing)
	}
	// A fetch that works again resets the count.
	e.server.serve(url, servedList{body: listCSV + "spam.example,suspend,,,,\n"})
	e.fetch(t, id, moderation.DomainListNotModified)
	if got, _ := e.svc.DomainListSubscription(t.Context(), id); got.Head.ConsecutiveFailures != 0 {
		t.Errorf("failures after a good fetch = %d, want 0", got.Head.ConsecutiveFailures)
	}
}

func domainListHeld(t *testing.T, e *domainListEnv) {
	const url = "https://lists.example/held.json"
	var b strings.Builder
	b.WriteString(`[{"domain":"odd.example","severity":"block"}`)
	for i := range 12 {
		fmt.Fprintf(&b, `,{"domain":"d%02d.example","severity":"suspend"}`, i)
	}
	b.WriteString("]")
	e.server.serve(url, servedList{body: b.String()})
	rec := e.subscribe(t, url, moderation.DomainListMastodonJSON, moderation.DomainListAutomatic)
	id := rec.Subscription.ID
	e.fetch(t, id, moderation.DomainListApplied)
	if got := e.inEffect(t, nil); len(got) != 12 || slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "odd.example") }) {
		t.Errorf("actions in effect = %v; want the 12 suspends and nothing on the unknown severity", got)
	}
	if d := e.drafts(t, id); !slices.Equal(d, []string{"issue:suspend:odd.example unknown_severity"}) {
		t.Errorf("drafts = %v; want the unknown severity", d)
	}
	// An empty answer would lift every action: the lifts stay drafts.
	e.server.serve(url, servedList{body: "[]"})
	e.fetch(t, id, moderation.DomainListApplied)
	if got := e.inEffect(t, nil); len(got) != 12 {
		t.Errorf("actions in effect after an empty list = %d, want the 12 kept", len(got))
	}
	d := e.drafts(t, id)
	if len(d) != 12 || !strings.HasSuffix(d[0], " mass_lift") {
		t.Errorf("drafts after an empty list = %v; want 12 held lifts", d)
	}
}

func domainListDrafts(t *testing.T, e *domainListEnv) {
	ctx := t.Context()
	const url = "https://lists.example/drafts.csv"
	e.server.serve(url, servedList{body: listCSV + "a.example,suspend,,,From the list,\nb.example,silence,,,,\nc.example,suspend,,,,\n"})
	rec := e.subscribe(t, url, moderation.DomainListMastodonCSV, moderation.DomainListDraft)
	id := rec.Subscription.ID
	names := map[string]string{id: "A"}
	e.fetch(t, id, moderation.DomainListApplied)
	e.wantInEffect(t, names)
	want := []string{"issue:suspend:a.example draft_mode", "issue:limit:b.example draft_mode", "issue:suspend:c.example draft_mode"}
	if d := e.drafts(t, id); !slices.Equal(d, want) {
		t.Errorf("drafts = %v, want %v", d, want)
	}
	res, err := e.svc.RejectDomainListDrafts(ctx, id, []string{"issue:limit:b.example", "issue:limit:nothing.example"}, e.moderator)
	if err != nil || len(res.Done) != 1 || !slices.Equal(res.Missing, []string{"issue:limit:nothing.example"}) {
		t.Errorf("RejectDomainListDrafts = %+v, %v; want one rejected and one missing", res, err)
	}
	res, err = e.svc.AcceptDomainListDrafts(ctx, id, []string{"issue:suspend:a.example"}, e.moderator)
	if err != nil || len(res.Done) != 1 {
		t.Errorf("AcceptDomainListDrafts = %+v, %v; want one", res, err)
	}
	e.wantInEffect(t, names, "a.example suspend A")
	if d := e.drafts(t, id); !slices.Equal(d, []string{"issue:suspend:c.example draft_mode"}) {
		t.Errorf("drafts after = %v; want c.example only", d)
	}
	accepted := e.audits(t, moderation.AuditDomainListDraftAccepted)
	issued := e.audits(t, moderation.AuditDomainActionIssued)
	if len(accepted) != 1 || accepted[0].ActorUserID != e.moderator || len(issued) != 1 || issued[0].ActorUserID != e.moderator || accepted[0].SourceEventID != issued[0].SourceEventID {
		t.Errorf("audit = accepted %+v, issued %+v; want both by the moderator, on the same action", accepted, issued)
	}
	if n := len(e.audits(t, moderation.AuditDomainListDraftRejected)); n != 1 {
		t.Errorf("domain_list_draft_rejected entries = %d, want 1", n)
	}
	// The rejected draft comes back when the list changes the domain's row.
	e.server.serve(url, servedList{body: listCSV + "a.example,suspend,,,From the list,\nb.example,silence,,,Now with a reason,\nc.example,suspend,,,,\n"})
	e.fetch(t, id, moderation.DomainListApplied)
	if d := e.drafts(t, id); !slices.Equal(d, []string{"issue:limit:b.example draft_mode", "issue:suspend:c.example draft_mode"}) {
		t.Errorf("drafts after the list changed b.example = %v", d)
	}
	// Accepting everything; a removal is then a lift draft.
	if _, err := e.svc.AcceptDomainListDrafts(ctx, id, nil, e.moderator); err != nil {
		t.Fatalf("AcceptDomainListDrafts(all) = %v", err)
	}
	e.wantInEffect(t, names, "a.example suspend A", "b.example limit A", "c.example suspend A")
	e.server.serve(url, servedList{body: listCSV + "a.example,suspend,,,From the list,\n"})
	e.fetch(t, id, moderation.DomainListApplied)
	if d := e.drafts(t, id); !slices.Equal(d, []string{"lift:limit:b.example draft_mode", "lift:suspend:c.example draft_mode"}) {
		t.Errorf("drafts after removals = %v", d)
	}
	e.wantInEffect(t, names, "a.example suspend A", "b.example limit A", "c.example suspend A")
}

func domainListOverlap(t *testing.T, e *domainListEnv) {
	const first, second = "https://lists.example/first.txt", "https://lists.example/second.txt"
	e.server.serve(first, servedList{body: "shared.example\n"})
	e.server.serve(second, servedList{body: "shared.example\nother.example\n"})
	a := e.subscribe(t, first, moderation.DomainListPlainText, moderation.DomainListAutomatic)
	e.clock.Advance(time.Second)
	b := e.subscribe(t, second, moderation.DomainListPlainText, moderation.DomainListAutomatic)
	names := map[string]string{a.Subscription.ID: "A", b.Subscription.ID: "B"}
	e.fetch(t, a.Subscription.ID, moderation.DomainListApplied)
	e.fetch(t, b.Subscription.ID, moderation.DomainListApplied)
	e.wantInEffect(t, names, "shared.example suspend A", "other.example suspend B")
	e.server.serve(first, servedList{body: "\n"})
	e.fetch(t, a.Subscription.ID, moderation.DomainListApplied)
	e.wantInEffect(t, names, "shared.example suspend B", "other.example suspend B")
}

func domainListDeletion(t *testing.T, e *domainListEnv) {
	ctx := t.Context()
	const kept, lifted = "https://lists.example/kept.txt", "https://lists.example/lifted.txt"
	e.server.serve(kept, servedList{body: "kept.example\n"})
	e.server.serve(lifted, servedList{body: "lifted.example\n"})
	a := e.subscribe(t, kept, moderation.DomainListPlainText, moderation.DomainListAutomatic)
	b := e.subscribe(t, lifted, moderation.DomainListPlainText, moderation.DomainListAutomatic)
	e.fetch(t, a.Subscription.ID, moderation.DomainListApplied)
	e.fetch(t, b.Subscription.ID, moderation.DomainListApplied)
	if err := e.svc.DeleteDomainListSubscription(ctx, a.Subscription.ID, false, e.moderator); err != nil {
		t.Fatalf("DeleteDomainListSubscription(keep) = %v", err)
	}
	if err := e.svc.DeleteDomainListSubscription(ctx, b.Subscription.ID, true, e.moderator); err != nil {
		t.Fatalf("DeleteDomainListSubscription(lift) = %v", err)
	}
	names := map[string]string{a.Subscription.ID: "A"}
	e.wantInEffect(t, names, "kept.example suspend A")
	if _, err := e.svc.DomainListSubscription(ctx, a.Subscription.ID); err == nil {
		t.Error("DomainListSubscription(deleted) = nil error; want ErrNotFound")
	}
	if err := e.svc.DeleteDomainListSubscription(ctx, a.Subscription.ID, false, e.moderator); err == nil {
		t.Error("DeleteDomainListSubscription twice = nil error; want ErrNotFound")
	}
	if _, err := e.svc.FetchDomainList(ctx, a.Subscription.ID); err == nil {
		t.Error("FetchDomainList(deleted) = nil error; want ErrNotFound")
	}
	recs, err := e.svc.DomainListSubscriptions(ctx, "", 10)
	if err != nil || len(recs) != 0 {
		t.Errorf("DomainListSubscriptions = %d, %v; want none left", len(recs), err)
	}
	if n := len(e.audits(t, moderation.AuditDomainListSubscriptionDeleted)); n != 2 {
		t.Errorf("domain_list_subscription_deleted entries = %d, want 2", n)
	}
	if n := len(e.audits(t, moderation.AuditDomainListSubscriptionCreated)); n != 2 {
		t.Errorf("domain_list_subscription_created entries = %d, want 2", n)
	}
}
