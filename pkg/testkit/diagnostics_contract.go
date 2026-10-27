// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

// DiagnosticsHarness is what DiagnosticsRepositoryContract needs from a database adapter: its
// unit of work and the diagnostics repository.
type DiagnosticsHarness struct {
	UnitOfWork  uow.UnitOfWork
	Diagnostics appdiagnostics.Repository
}

// DiagnosticsFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type DiagnosticsFactory func(t *testing.T, opts uow.Options) DiagnosticsHarness

// DiagnosticsRepositoryContract verifies the diagnostics repository of a database adapter
// through the Recorder and the use cases of the error groups (ADR-0103 §4 (2), §7): the first
// occurrence of a fingerprint creates its group, open, and emits GroupCreated once however many
// processes see it at the same time; the occurrences are counted by hour, each once, also when
// the flushes of several processes run concurrently, and the latest one is the sample; a
// resolved group that recurs with the version it was resolved with is only counted, and one
// that recurs with another version regresses to open and emits GroupRegressed; a group that a web report without a session
// created is confirmed once, by the first report with one; resolve, mute,
// and unmute record only the changes they make; the counts older than 90 days are pruned; the
// recorder of a process writes what it holds when it closes; and the projection of the states
// rebuilds to what the events wrote.
func DiagnosticsRepositoryContract(t *testing.T, newHarness DiagnosticsFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *diagnosticsEnv)
	}{
		{"CreatesAGroupOnce", diagnosticsNewGroup},
		{"CountsByHour", diagnosticsCountsByHour},
		{"ConfirmsAGroupOfAReportWithoutASession", diagnosticsConfirm},
		{"CountsConcurrentFlushesOnce", diagnosticsConcurrentFlushes},
		{"CountsASameVersionRecurrence", diagnosticsSameVersion},
		{"RegressesWithANewVersion", diagnosticsNewVersion},
		{"ResolvesMutesAndUnmutes", diagnosticsStates},
		{"PrunesOldCounts", diagnosticsPrune},
		{"WritesWhatItHoldsOnClose", diagnosticsClose},
		{"WritesReportsOfEndedContexts", diagnosticsEndedContext},
		{"RebuildsItsProjection", diagnosticsRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newDiagnosticsEnv(t, newHarness)) })
	}
}

type diagnosticsEnv struct {
	h     DiagnosticsHarness
	clock *Clock
	svc   *appdiagnostics.Service

	mu        sync.Mutex
	created   []appdiagnostics.GroupCreated
	regressed []appdiagnostics.GroupRegressed
	confirmed []appdiagnostics.GroupConfirmed
}

func newDiagnosticsEnv(t *testing.T, newHarness DiagnosticsFactory) *diagnosticsEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 50}, Projectors: projectors})
	clock := NewClock(time.Date(2026, 10, 20, 12, 30, 0, 0, time.UTC))
	appdiagnostics.RegisterProjectors(projectors, h.Diagnostics)
	e := &diagnosticsEnv{h: h, clock: clock}
	// The subscribers of the notifications and the webhooks see the events after the commit.
	projectors.OnCommit(appdiagnostics.EventGroupCreated, func(_ context.Context, ev uow.Event) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.created = append(e.created, ev.(appdiagnostics.GroupCreated))
	})
	projectors.OnCommit(appdiagnostics.EventGroupRegressed, func(_ context.Context, ev uow.Event) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.regressed = append(e.regressed, ev.(appdiagnostics.GroupRegressed))
	})
	projectors.OnCommit(appdiagnostics.EventGroupConfirmed, func(_ context.Context, ev uow.Event) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.confirmed = append(e.confirmed, ev.(appdiagnostics.GroupConfirmed))
	})
	e.svc = appdiagnostics.NewService(appdiagnostics.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Diagnostics, Clock: clock, IDs: NewIDGenerator(clock),
	})
	return e
}

// recorder returns the Recorder of a new process, which writes on Flush only.
func (e *diagnosticsEnv) recorder(t *testing.T) *appdiagnostics.Recorder {
	t.Helper()
	r := appdiagnostics.NewRecorder(appdiagnostics.RecorderOptions{Service: e.svc, Clock: e.clock, Manual: true})
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	return r
}

func (e *diagnosticsEnv) events() ([]appdiagnostics.GroupCreated, []appdiagnostics.GroupRegressed) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]appdiagnostics.GroupCreated(nil), e.created...), append([]appdiagnostics.GroupRegressed(nil), e.regressed...)
}

// event returns a scrubbed report of fingerprint fp with version at the clock's time.
func (e *diagnosticsEnv) event(fp, version string) kernel.ErrorEvent {
	return kernel.ErrorEvent{
		Time: e.clock.Now(), Source: kernel.ErrorSourceServer, Kind: kernel.ErrorKindPanic,
		Type: "*fmt.wrapError > *pgconn.PgError", Message: "query failed: <id>\nsecond line",
		Stack: "goroutine 1 [running]:\ngithub.com/mycroblog/my/internal/app/status.(*Service).Get(...)\n",
		Where: "client_api GET /api/v1/statuses/{id}", Fingerprint: fp,
		RequestID: "req-1", TraceID: "0123456789abcdef0123456789abcdef", Version: version, Role: "all",
	}
}

// group returns the group of fp with its state, through the first created event.
func (e *diagnosticsEnv) group(t *testing.T, fp string) (appdiagnostics.Group, appdiagnostics.Head) {
	t.Helper()
	created, _ := e.events()
	for _, c := range created {
		if c.Group.Fingerprint == fp {
			g, h, err := e.svc.Group(t.Context(), c.Group.ID)
			if err != nil {
				t.Fatalf("Group: %v", err)
			}
			return g, h
		}
	}
	t.Fatalf("no group of %s was created", fp)
	return appdiagnostics.Group{}, appdiagnostics.Head{}
}

func (e *diagnosticsEnv) total(t *testing.T, groupID string) int64 {
	t.Helper()
	counts, err := e.svc.HourCounts(t.Context(), groupID)
	if err != nil {
		t.Fatalf("HourCounts: %v", err)
	}
	var n int64
	for _, c := range counts {
		n += c.Occurrences
	}
	return n
}

func diagnosticsNewGroup(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	// Two processes see the same new error at the same time.
	a, b := e.recorder(t), e.recorder(t)
	a.ReceiveError(ctx, e.event("fp-new", "1.0.0"))
	b.ReceiveError(ctx, e.event("fp-new", "1.0.0"))
	var wg sync.WaitGroup
	for _, r := range []*appdiagnostics.Recorder{a, b} {
		wg.Go(func() { r.Flush(ctx) })
	}
	wg.Wait()
	created, regressed := e.events()
	if len(created) != 1 || len(regressed) != 0 {
		t.Fatalf("created %d, regressed %d; want 1 and 0", len(created), len(regressed))
	}
	g, h := e.group(t, "fp-new")
	want := appdiagnostics.Group{
		ID: g.ID, Fingerprint: "fp-new", Source: "server", Kind: "panic", ErrorType: "*pgconn.PgError",
		Message: "query failed: <id>", Where: "client_api GET /api/v1/statuses/{id}", FirstVersion: "1.0.0",
		FirstSeenAt: e.clock.Now(),
	}
	if !g.FirstSeenAt.Equal(want.FirstSeenAt) {
		t.Errorf("first seen at %v, want %v", g.FirstSeenAt, want.FirstSeenAt)
	}
	g.FirstSeenAt = want.FirstSeenAt
	if g != want {
		t.Errorf("group = %+v, want %+v", g, want)
	}
	if created[0].Group.ID != g.ID {
		t.Errorf("GroupCreated carries %s, want %s", created[0].Group.ID, g.ID)
	}
	if h.State != diagnostics.StateOpen || h.ResolvedVersion != "" {
		t.Errorf("state = %+v, want open", h)
	}
	if n := e.total(t, g.ID); n != 2 {
		t.Errorf("counted %d, want 2", n)
	}
	s, err := e.svc.Sample(ctx, g.ID)
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if s.Fingerprint != "fp-new" || s.ErrorType != "*fmt.wrapError > *pgconn.PgError" || s.Message != "query failed: <id>\nsecond line" ||
		s.RequestID != "req-1" || s.TraceID == "" || s.Version != "1.0.0" || s.Role != "all" || s.Stack == "" {
		t.Errorf("sample = %+v", s)
	}
	if _, _, err := e.svc.Group(ctx, OrderedID(999)); !errors.Is(err, appdiagnostics.ErrNotFound) {
		t.Errorf("Group of an unknown ID: %v, want ErrNotFound", err)
	}
	// A report without a fingerprint is not grouped.
	a.ReceiveError(ctx, kernel.ErrorEvent{Message: "no fingerprint"})
	a.Flush(ctx)
	if created, _ := e.events(); len(created) != 1 {
		t.Errorf("a report without a fingerprint created a group")
	}
}

func diagnosticsCountsByHour(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	r := e.recorder(t)
	for range 3 {
		r.ReceiveError(ctx, e.event("fp-hours", "1.0.0"))
	}
	r.Flush(ctx)
	first := e.clock.Now()
	e.clock.Advance(time.Hour)
	for range 2 {
		r.ReceiveError(ctx, e.event("fp-hours", "1.0.1"))
	}
	r.Flush(ctx)
	r.Flush(ctx) // nothing more
	g, _ := e.group(t, "fp-hours")
	counts, err := e.svc.HourCounts(ctx, g.ID)
	if err != nil {
		t.Fatalf("HourCounts: %v", err)
	}
	if len(counts) != 2 {
		t.Fatalf("counts = %+v, want 2 hours", counts)
	}
	if !counts[0].Hour.Equal(diagnostics.Hour(first)) || counts[0].Occurrences != 3 || counts[0].LastVersion != "1.0.0" {
		t.Errorf("first hour = %+v", counts[0])
	}
	if !counts[1].Hour.Equal(diagnostics.Hour(e.clock.Now())) || counts[1].Occurrences != 2 || counts[1].LastVersion != "1.0.1" ||
		!counts[1].LastSeenAt.Equal(e.clock.Now()) {
		t.Errorf("second hour = %+v", counts[1])
	}
	s, err := e.svc.Sample(ctx, g.ID)
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if s.Version != "1.0.1" || !s.OccurredAt.Equal(e.clock.Now()) {
		t.Errorf("sample = %+v, want the latest", s)
	}
	// An older occurrence that arrives late counts but does not replace the sample.
	late := e.event("fp-hours", "0.9.0")
	late.Time = first
	r.ReceiveError(ctx, late)
	r.Flush(ctx)
	if s, _ := e.svc.Sample(ctx, g.ID); s.Version != "1.0.1" {
		t.Errorf("a late occurrence replaced the sample: %+v", s)
	}
	if n := e.total(t, g.ID); n != 6 {
		t.Errorf("counted %d, want 6", n)
	}
}

func diagnosticsConfirm(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	web := func(unauthenticated bool) kernel.ErrorEvent {
		ev := e.event("fp-web", "0.3.0")
		ev.Source, ev.Kind, ev.Unauthenticated = kernel.ErrorSourceWeb, kernel.ErrorKindError, unauthenticated
		return ev
	}
	r := e.recorder(t)
	r.ReceiveError(ctx, web(true))
	r.ReceiveError(ctx, web(true))
	r.Flush(ctx)
	created, _ := e.events()
	if len(created) != 1 || !created[0].Group.Unauthenticated || created[0].Group.Source != "web" {
		t.Fatalf("created = %+v, want one unauthenticated web group", created)
	}
	if g, _ := e.group(t, "fp-web"); !g.Unauthenticated {
		t.Errorf("stored group = %+v, want unauthenticated", g)
	}
	e.mu.Lock()
	n := len(e.confirmed)
	e.mu.Unlock()
	if n != 0 {
		t.Fatalf("confirmed %d times before a report with a session", n)
	}
	// Two processes get a report with a session: one confirmation.
	a, b := e.recorder(t), e.recorder(t)
	a.ReceiveError(ctx, web(false))
	b.ReceiveError(ctx, web(false))
	var wg sync.WaitGroup
	for _, rec := range []*appdiagnostics.Recorder{a, b} {
		wg.Go(func() { rec.Flush(ctx) })
	}
	wg.Wait()
	r.ReceiveError(ctx, web(false))
	r.Flush(ctx)
	e.mu.Lock()
	confirmed := append([]appdiagnostics.GroupConfirmed(nil), e.confirmed...)
	e.mu.Unlock()
	if len(confirmed) != 1 || confirmed[0].Group.Fingerprint != "fp-web" || !confirmed[0].ConfirmedAt.Equal(e.clock.Now()) {
		t.Errorf("confirmed = %+v, want one", confirmed)
	}
	g, _ := e.group(t, "fp-web")
	if n := e.total(t, g.ID); n != 5 {
		t.Errorf("counted %d, want 5", n)
	}
	// A group created with a session is never confirmed.
	c := e.recorder(t)
	c.ReceiveError(ctx, e.event("fp-session", "1.0.0"))
	c.Flush(ctx)
	if created, _ := e.events(); len(created) != 2 || created[1].Group.Unauthenticated {
		t.Errorf("created = %+v", created)
	}
}

func diagnosticsConcurrentFlushes(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	const processes, reports = 3, 40
	var wg sync.WaitGroup
	for range processes {
		r := e.recorder(t)
		wg.Go(func() {
			var inner sync.WaitGroup
			for i := range reports {
				inner.Go(func() {
					r.ReceiveError(ctx, e.event("fp-concurrent", "1.0.0"))
					if i%5 == 0 {
						r.Flush(ctx)
					}
				})
			}
			inner.Wait()
			r.Flush(ctx)
		})
	}
	wg.Wait()
	created, _ := e.events()
	if len(created) != 1 {
		t.Fatalf("created %d groups, want 1", len(created))
	}
	if n := e.total(t, created[0].Group.ID); n != processes*reports {
		t.Errorf("counted %d, want %d", n, processes*reports)
	}
}

// resolved returns the group of fp, created at version and resolved by the CLI.
func (e *diagnosticsEnv) resolved(t *testing.T, fp, version string) appdiagnostics.Group {
	t.Helper()
	r := e.recorder(t)
	r.ReceiveError(t.Context(), e.event(fp, version))
	r.Flush(t.Context())
	g, _ := e.group(t, fp)
	h, err := e.svc.Resolve(t.Context(), g.ID, appdiagnostics.Actor{Kind: diagnostics.ActorCLI})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if h.State != diagnostics.StateResolved || h.ResolvedVersion != version {
		t.Fatalf("resolved state = %+v", h)
	}
	return g
}

func diagnosticsSameVersion(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	g := e.resolved(t, "fp-same", "1.0.0")
	e.clock.Advance(time.Minute)
	// Another process of the same version still runs the old code.
	r := e.recorder(t)
	r.ReceiveError(ctx, e.event("fp-same", "1.0.0"))
	r.ReceiveError(ctx, e.event("fp-same", "1.0.0"))
	r.Flush(ctx)
	if _, regressed := e.events(); len(regressed) != 0 {
		t.Errorf("a recurrence of the same version regressed: %+v", regressed)
	}
	if _, h, _ := e.svc.Group(ctx, g.ID); h.State != diagnostics.StateResolved {
		t.Errorf("state = %s, want resolved", h.State)
	}
	if n := e.total(t, g.ID); n != 3 {
		t.Errorf("counted %d, want 3", n)
	}
	events, err := e.svc.Events(ctx, g.ID)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 || events[0].Kind != diagnostics.EventResolved {
		t.Errorf("events = %+v, want only resolved", events)
	}
}

func diagnosticsNewVersion(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	g := e.resolved(t, "fp-regress", "1.0.0")
	e.clock.Advance(time.Minute)
	// The new version recurs in two processes at the same time: one regression.
	a, b := e.recorder(t), e.recorder(t)
	a.ReceiveError(ctx, e.event("fp-regress", "1.1.0"))
	b.ReceiveError(ctx, e.event("fp-regress", "1.1.0"))
	var wg sync.WaitGroup
	for _, r := range []*appdiagnostics.Recorder{a, b} {
		wg.Go(func() { r.Flush(ctx) })
	}
	wg.Wait()
	_, regressed := e.events()
	if len(regressed) != 1 {
		t.Fatalf("regressed %d times, want 1", len(regressed))
	}
	ev := regressed[0]
	if ev.Group.ID != g.ID || ev.Event.Kind != diagnostics.EventRegressed || ev.Event.Version != "1.1.0" ||
		ev.Event.Actor.Kind != diagnostics.ActorSystem || !ev.Event.OccurredAt.Equal(e.clock.Now()) {
		t.Errorf("GroupRegressed = %+v", ev)
	}
	_, h, err := e.svc.Group(ctx, g.ID)
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	if h.State != diagnostics.StateOpen || h.ResolvedVersion != "" || !h.Since.Equal(e.clock.Now()) {
		t.Errorf("state = %+v, want open since now", h)
	}
	// Resolved again with the new version, the next version regresses again.
	if _, err := e.svc.Resolve(ctx, g.ID, appdiagnostics.Actor{Kind: diagnostics.ActorUser, UserID: NamedID("admin")}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	c := e.recorder(t)
	c.ReceiveError(ctx, e.event("fp-regress", "1.1.0"))
	c.ReceiveError(ctx, e.event("fp-regress", "1.2.0"))
	c.Flush(ctx)
	if _, regressed := e.events(); len(regressed) != 2 || regressed[1].Event.Version != "1.2.0" {
		t.Errorf("regressions = %+v, want a second one with 1.2.0", regressed)
	}
	events, _ := e.svc.Events(ctx, g.ID)
	kinds := make([]diagnostics.EventKind, len(events))
	for i, ev := range events {
		kinds[i] = ev.Kind
	}
	want := []diagnostics.EventKind{diagnostics.EventResolved, diagnostics.EventRegressed, diagnostics.EventResolved, diagnostics.EventRegressed}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Errorf("events = %v, want %v", kinds, want)
	}
	if events[2].Actor.UserID != NamedID("admin") || events[2].Version != "1.1.0" {
		t.Errorf("second resolve = %+v", events[2])
	}
}

func diagnosticsStates(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	r := e.recorder(t)
	r.ReceiveError(ctx, e.event("fp-states", "1.0.0"))
	r.Flush(ctx)
	g, _ := e.group(t, "fp-states")
	cli := appdiagnostics.Actor{Kind: diagnostics.ActorCLI}
	steps := []struct {
		do   func(context.Context, string, appdiagnostics.Actor) (appdiagnostics.Head, error)
		want diagnostics.State
	}{
		{e.svc.Unmute, diagnostics.StateOpen}, // nothing to unmute
		{e.svc.Mute, diagnostics.StateMuted},
		{e.svc.Mute, diagnostics.StateMuted}, // already muted
		{e.svc.Resolve, diagnostics.StateResolved},
		{e.svc.Mute, diagnostics.StateMuted},
		{e.svc.Unmute, diagnostics.StateOpen},
	}
	for i, s := range steps {
		e.clock.Advance(time.Second)
		h, err := s.do(ctx, g.ID, cli)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if h.State != s.want {
			t.Errorf("step %d: state %s, want %s", i, h.State, s.want)
		}
		if _, stored, _ := e.svc.Group(ctx, g.ID); !sameHead(stored, h) {
			t.Errorf("step %d: stored %+v, returned %+v", i, stored, h)
		}
	}
	events, _ := e.svc.Events(ctx, g.ID)
	if len(events) != 4 {
		t.Errorf("recorded %d events, want 4 (the changes only)", len(events))
	}
	// A muted group that recurs with another version is counted, not regressed.
	if _, err := e.svc.Mute(ctx, g.ID, cli); err != nil {
		t.Fatal(err)
	}
	m := e.recorder(t)
	m.ReceiveError(ctx, e.event("fp-states", "2.0.0"))
	m.Flush(ctx)
	if _, regressed := e.events(); len(regressed) != 0 {
		t.Errorf("a muted group regressed")
	}
	if _, err := e.svc.Resolve(ctx, OrderedID(999), cli); !errors.Is(err, appdiagnostics.ErrNotFound) {
		t.Errorf("Resolve of an unknown group: %v, want ErrNotFound", err)
	}
	for _, a := range []appdiagnostics.Actor{
		{Kind: diagnostics.ActorUser}, {Kind: diagnostics.ActorCLI, UserID: NamedID("admin")}, {Kind: diagnostics.ActorSystem},
	} {
		if _, err := e.svc.Resolve(ctx, g.ID, a); !errors.Is(err, appdiagnostics.ErrInvalidActor) {
			t.Errorf("Resolve by %+v: %v, want ErrInvalidActor", a, err)
		}
	}
}

func diagnosticsPrune(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	r := e.recorder(t)
	r.ReceiveError(ctx, e.event("fp-prune", "1.0.0"))
	old := e.event("fp-prune", "1.0.0")
	old.Time = e.clock.Now().Add(-diagnostics.CountRetention - 2*time.Hour)
	r.ReceiveError(ctx, old)
	r.Flush(ctx)
	g, _ := e.group(t, "fp-prune")
	n, err := e.svc.PruneCounts(ctx)
	if err != nil {
		t.Fatalf("PruneCounts: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1", n)
	}
	counts, _ := e.svc.HourCounts(ctx, g.ID)
	if len(counts) != 1 || !counts[0].Hour.Equal(diagnostics.Hour(e.clock.Now())) {
		t.Errorf("counts after the prune = %+v", counts)
	}
	if n, err := e.svc.PruneCounts(ctx); err != nil || n != 0 {
		t.Errorf("second prune: %d, %v", n, err)
	}
}

func diagnosticsClose(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	// The recorder of a running process writes in its own goroutine, also the reports made with
	// a context that has ended, such as the panic of a goroutine that was stopping.
	ended, cancel := context.WithCancel(ctx)
	cancel()
	r := appdiagnostics.NewRecorder(appdiagnostics.RecorderOptions{Service: e.svc, Clock: e.clock, Interval: time.Hour})
	for range 4 {
		r.ReceiveError(ended, e.event("fp-close", "1.0.0"))
	}
	if err := r.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	r.ReceiveError(ctx, e.event("fp-close", "1.0.0")) // after Close: dropped
	if err := r.Close(ctx); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	g, _ := e.group(t, "fp-close")
	if n := e.total(t, g.ID); n != 4 {
		t.Errorf("counted %d, want 4", n)
	}
}

func diagnosticsEndedContext(t *testing.T, e *diagnosticsEnv) {
	// An HTTP report can arrive after the response, with the request's context cancelled; the
	// flush itself may run with an ended context too.
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	r := e.recorder(t)
	for range 3 {
		r.ReceiveError(ended, e.event("fp-ended", "1.0.0"))
	}
	r.Flush(ended)
	g, _ := e.group(t, "fp-ended")
	if n := e.total(t, g.ID); n != 3 {
		t.Errorf("counted %d, want 3", n)
	}
}

func diagnosticsRebuild(t *testing.T, e *diagnosticsEnv) {
	ctx := t.Context()
	open := e.recorder(t)
	open.ReceiveError(ctx, e.event("fp-open", "1.0.0"))
	open.Flush(ctx)
	resolved := e.resolved(t, "fp-resolved", "1.0.0")
	muted := e.resolved(t, "fp-muted", "1.0.0")
	e.clock.Advance(time.Second)
	if _, err := e.svc.Mute(ctx, muted.ID, appdiagnostics.Actor{Kind: diagnostics.ActorCLI}); err != nil {
		t.Fatal(err)
	}
	regressed := e.resolved(t, "fp-regressed", "1.0.0")
	e.clock.Advance(time.Second)
	again := e.recorder(t)
	again.ReceiveError(ctx, e.event("fp-regressed", "2.0.0"))
	again.Flush(ctx)
	openGroup, _ := e.group(t, "fp-open")
	ids := []string{openGroup.ID, resolved.ID, muted.ID, regressed.ID}
	before := map[string]appdiagnostics.Head{}
	for _, id := range ids {
		_, h, err := e.svc.Group(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		before[id] = h
	}
	if err := appdiagnostics.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Diagnostics); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}
	for _, id := range ids {
		_, h, err := e.svc.Group(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !sameHead(h, before[id]) {
			t.Errorf("%s: rebuilt %+v, want %+v", id, h, before[id])
		}
	}
	if before[resolved.ID].State != diagnostics.StateResolved || before[muted.ID].State != diagnostics.StateMuted ||
		before[regressed.ID].State != diagnostics.StateOpen || before[openGroup.ID].State != diagnostics.StateOpen {
		t.Errorf("states = %+v", before)
	}
}

func sameHead(a, b appdiagnostics.Head) bool {
	return a.GroupID == b.GroupID && a.State == b.State && a.ResolvedVersion == b.ResolvedVersion && a.Since.Equal(b.Since)
}
