// SPDX-License-Identifier: AGPL-3.0-only

package hook_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
)

// plugin answers what a test sets and records its calls.
type plugin struct {
	answer   apphook.Answer
	err      error
	requests []apphook.Request
}

func (p *plugin) Answer(a apphook.Answer) { p.answer, p.err = a, nil }

func (p *plugin) Fail(err error) { p.err = err }

func (p *plugin) Requests() []apphook.Request { return p.requests }

func (p *plugin) Check(_ context.Context, r apphook.Request) (apphook.Answer, error) {
	p.requests = append(p.requests, r)
	return p.answer, p.err
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

type ids struct{ n int }

func (i *ids) NewID() string { i.n++; return fmt.Sprintf("id-%d", i.n) }

func (i *ids) NewIDAt(time.Time) string { return i.NewID() }

// settings are the instance settings of a test.
type settings struct {
	mode      hook.Mode
	onFailure hook.OnFailure
	reads     int
}

func (s *settings) HookMode(context.Context, hook.Name) (hook.Mode, error) {
	s.reads++
	return s.mode, nil
}

func (s *settings) HookOnFailure(context.Context, hook.Name) (hook.OnFailure, error) {
	return s.onFailure, nil
}

func (s *settings) HookRetention(context.Context) (time.Duration, time.Duration, error) {
	return 90 * 24 * time.Hour, 30 * 24 * time.Hour, nil
}

// fakeTx collects the events of a transaction.
type fakeTx struct {
	uow.Tx
	events *[]uow.Event
}

func (t fakeTx) Emit(_ context.Context, e uow.Event) error {
	*t.events = append(*t.events, e)
	return nil
}

// memory is the persistence of a test: a Repository and a UnitOfWork over maps.
type memory struct {
	uow.UnitOfWork
	records    map[string]*hook.Record
	statuses   map[string][]string
	appeals    map[string]*hook.Appeal
	reviews    map[string]hook.AppealDecision
	exemptions map[string]bool
	events     []uow.Event
	reads      int
}

func newMemory() *memory {
	return &memory{
		records: map[string]*hook.Record{}, statuses: map[string][]string{},
		appeals: map[string]*hook.Appeal{}, reviews: map[string]hook.AppealDecision{}, exemptions: map[string]bool{},
	}
}

func (m *memory) Tx(_ context.Context, f func(uow.Tx) error) error {
	return f(fakeTx{events: &m.events})
}

func (m *memory) InsertDecision(_ context.Context, _ uow.Tx, d hook.Decision) (bool, error) {
	for _, r := range m.records {
		if r.Decision.Hook == d.Hook && r.Decision.SubjectKey == d.SubjectKey {
			return false, nil
		}
	}
	m.records[d.ID] = &hook.Record{Decision: d}
	return true, nil
}

func (m *memory) RecordBySubject(_ context.Context, n hook.Name, key string) (hook.Record, error) {
	m.reads++
	for _, r := range m.records {
		if r.Decision.Hook == n && r.Decision.SubjectKey == key {
			return *r, nil
		}
	}
	return hook.Record{}, apphook.ErrNotFound
}

func (m *memory) RecordTx(_ context.Context, _ uow.Tx, id string) (hook.Record, error) {
	if r, ok := m.records[id]; ok {
		return *r, nil
	}
	return hook.Record{}, apphook.ErrNotFound
}

func (m *memory) InsertDecisionStatus(_ context.Context, _ uow.Tx, decisionID, statusID string) error {
	m.statuses[decisionID] = append(m.statuses[decisionID], statusID)
	return nil
}

func (m *memory) InsertOverride(_ context.Context, _ uow.Tx, o hook.Override) (bool, error) {
	r := m.records[o.DecisionID]
	if r.Override != nil {
		return false, nil
	}
	r.Override = &o
	return true, nil
}

func (m *memory) ListRecords(context.Context, apphook.ListQuery) ([]hook.Record, error) {
	return nil, nil
}

func (m *memory) Summarize(context.Context, time.Time) ([]apphook.SummaryRow, error) {
	return nil, nil
}

func (m *memory) ListExpired(_ context.Context, q apphook.ExpiredQuery) ([]hook.Expired, error) {
	if q.AfterID != "" {
		return nil, nil
	}
	var out []hook.Expired
	for _, r := range m.records {
		d := r.Decision
		if d.DecidedAt.Before(q.Before) {
			out = append(out, hook.Expired{
				ID: d.ID, Hook: d.Hook, AccountID: d.AccountID, Mode: d.Mode, Outcome: d.Outcome,
				DecidedAt: d.DecidedAt, Overridden: r.Override != nil, StatusIDs: m.statuses[d.ID], AppealPending: m.appealPending(d.ID),
			})
		}
	}
	return out, nil
}

func (m *memory) DecisionIDsOfAccount(_ context.Context, _ uow.Tx, accountID string, limit int) ([]string, error) {
	var out []string
	for id, r := range m.records {
		if r.Decision.AccountID == accountID && len(out) < limit {
			out = append(out, id)
		}
	}
	return out, nil
}

func (m *memory) DeleteDecisions(_ context.Context, _ uow.Tx, ids []string) error {
	for _, id := range ids {
		delete(m.records, id)
		delete(m.statuses, id)
		for appealID, a := range m.appeals {
			if a.DecisionID == id {
				delete(m.appeals, appealID)
				delete(m.reviews, appealID)
			}
		}
	}
	return nil
}

type harness struct {
	s        *apphook.Service
	mem      *memory
	plugin   *plugin
	settings *settings
	clock    *clock
}

func newHarness(mode hook.Mode, onFailure hook.OnFailure) *harness {
	h := &harness{
		mem: newMemory(), plugin: &plugin{answer: apphook.Answer{Verdict: hook.Allow}}, settings: &settings{mode: mode, onFailure: onFailure},
		clock: &clock{now: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)},
	}
	h.s = apphook.NewService(apphook.Deps{
		UnitOfWork: h.mem, Repository: h.mem, Settings: h.settings, Clock: h.clock, IDs: &ids{},
		Plugins: map[hook.Name]apphook.Plugin{hook.InboundStatus: h.plugin},
	})
	return h
}

func (h *harness) decide(t *testing.T, key string) apphook.Result {
	t.Helper()
	r, err := h.s.Decide(t.Context(), apphook.Call{Hook: hook.InboundStatus, SubjectKey: key, AccountID: "account"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	return r
}

func TestObserveRecordsTheVerdictAndNeverActsOnIt(t *testing.T) {
	h := newHarness(hook.ModeObserve, hook.PassOnFailure)
	h.plugin.Answer(apphook.Answer{Verdict: hook.Drop, Reason: hook.ReasonSpam, Note: "n"})
	r := h.decide(t, "a")
	if r.Effect != hook.Allow {
		t.Errorf("Effect = %s, want allow", r.Effect)
	}
	got := h.mem.records[r.DecisionID].Decision
	if got.Outcome != hook.Drop || got.Mode != hook.ModeObserve || got.Reason != hook.ReasonSpam {
		t.Errorf("recorded %+v", got)
	}
}

func TestEnforceActsOnTheVerdict(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
	h.plugin.Answer(apphook.Answer{Verdict: hook.Drop})
	if r := h.decide(t, "a"); r.Effect != hook.Drop {
		t.Errorf("Effect = %s, want drop", r.Effect)
	}
}

func TestARecordedDecisionDoesNotCallThePluginAgain(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
	h.plugin.Answer(apphook.Answer{Verdict: hook.Drop})
	first := h.decide(t, "a")
	h.plugin.Answer(apphook.Answer{Verdict: hook.Allow})
	second := h.decide(t, "a")
	if n := len(h.plugin.Requests()); n != 1 {
		t.Errorf("the plugin got %d calls", n)
	}
	if second.DecisionID != first.DecisionID || second.Effect != hook.Drop {
		t.Errorf("second = %+v, first = %+v", second, first)
	}
	h.decide(t, "b")
	if n := len(h.plugin.Requests()); n != 2 {
		t.Errorf("another subject did not call the plugin: %d calls", n)
	}
}

func TestFailuresAreOutcomesNotVerdicts(t *testing.T) {
	for _, c := range []struct {
		err  error
		want hook.Outcome
	}{
		{apphook.ErrPluginTimeout, hook.FailedTimeout},
		{apphook.ErrPluginFailed, hook.FailedError},
		{apphook.ErrPluginInvalid, hook.FailedInvalid},
	} {
		h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
		h.plugin.Fail(c.err)
		r := h.decide(t, "a")
		if r.Effect != hook.Allow {
			t.Errorf("%v: Effect = %s, want allow", c.err, r.Effect)
		}
		if got := h.mem.records[r.DecisionID].Decision.Outcome; got != c.want {
			t.Errorf("%v: recorded %s, want %s", c.err, got, c.want)
		}
	}
}

func TestAVerdictTheHookDoesNotTakeIsInvalid(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
	h.plugin.Answer(apphook.Answer{Verdict: hook.Reject})
	r := h.decide(t, "a")
	if got := h.mem.records[r.DecisionID].Decision.Outcome; got != hook.FailedInvalid || r.Effect != hook.Allow {
		t.Errorf("recorded %s, effect %s", got, r.Effect)
	}
}

func TestRetryRecordsNothingAndCallsAgain(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.RetryOnFailure)
	h.plugin.Fail(apphook.ErrPluginFailed)
	_, err := h.s.Decide(t.Context(), apphook.Call{Hook: hook.InboundStatus, SubjectKey: "a"})
	if !errors.Is(err, apphook.ErrRetryLater) {
		t.Fatalf("err = %v", err)
	}
	if len(h.mem.records) != 0 {
		t.Errorf("recorded %d decisions", len(h.mem.records))
	}
	h.plugin.Answer(apphook.Answer{Verdict: hook.Flag})
	if r := h.decide(t, "a"); r.Effect != hook.Flag || len(h.plugin.Requests()) != 2 {
		t.Errorf("effect %s after %d calls", r.Effect, len(h.plugin.Requests()))
	}
}

func TestObserveNeverHoldsAJobOnFailure(t *testing.T) {
	h := newHarness(hook.ModeObserve, hook.RetryOnFailure)
	h.plugin.Fail(apphook.ErrPluginFailed)
	if r := h.decide(t, "a"); r.Effect != hook.Allow || r.DecisionID == "" {
		t.Errorf("result %+v", r)
	}
}

func TestTheBreakerSkipsThePluginAndComesBack(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
	h.plugin.Fail(apphook.ErrPluginFailed)
	for i := range apphook.BreakerThreshold {
		h.decide(t, fmt.Sprintf("failing-%d", i))
	}
	calls := len(h.plugin.Requests())
	if calls != apphook.BreakerThreshold {
		t.Fatalf("%d calls before the breaker opened", calls)
	}
	failing := 0
	for _, e := range h.mem.events {
		if _, ok := e.(apphook.Failing); ok {
			failing++
		}
	}
	if failing != 1 {
		t.Errorf("the moderators are told %d times", failing)
	}
	r := h.decide(t, "skipped")
	if len(h.plugin.Requests()) != calls {
		t.Error("an open breaker called the plugin")
	}
	if got := h.mem.records[r.DecisionID].Decision.Outcome; got != hook.FailedBreakerOpen || r.Effect != hook.Allow {
		t.Errorf("recorded %s, effect %s", got, r.Effect)
	}

	h.clock.now = h.clock.now.Add(apphook.BreakerOpenFor)
	h.plugin.Answer(apphook.Answer{Verdict: hook.Drop})
	if r := h.decide(t, "test-call"); r.Effect != hook.Drop {
		t.Errorf("the test call: %+v", r)
	}
	if r := h.decide(t, "closed"); r.Effect != hook.Drop {
		t.Errorf("after the breaker closed: %+v", r)
	}
}

func TestABreakerFollowsTheFailureBehaviour(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.RetryOnFailure)
	h.plugin.Fail(apphook.ErrPluginFailed)
	for i := range apphook.BreakerThreshold {
		if _, err := h.s.Decide(t.Context(), apphook.Call{Hook: hook.InboundStatus, SubjectKey: fmt.Sprintf("k%d", i)}); !errors.Is(err, apphook.ErrRetryLater) {
			t.Fatalf("err = %v", err)
		}
	}
	calls := len(h.plugin.Requests())
	if _, err := h.s.Decide(t.Context(), apphook.Call{Hook: hook.InboundStatus, SubjectKey: "skipped"}); !errors.Is(err, apphook.ErrRetryLater) {
		t.Fatalf("err = %v", err)
	}
	if len(h.plugin.Requests()) != calls || len(h.mem.records) != 0 {
		t.Errorf("%d calls, %d decisions", len(h.plugin.Requests()), len(h.mem.records))
	}
}

func TestACancelledCallerRecordsNothing(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.s.Decide(ctx, apphook.Call{Hook: hook.InboundStatus, SubjectKey: "a"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if len(h.mem.records) != 0 {
		t.Errorf("recorded %d decisions", len(h.mem.records))
	}
}

func TestNoHookConfiguredCallsNothingAndReadsNothing(t *testing.T) {
	mem := newMemory()
	st := &settings{mode: hook.ModeEnforce, onFailure: hook.PassOnFailure}
	s := apphook.NewService(apphook.Deps{UnitOfWork: mem, Repository: mem, Settings: st, Clock: &clock{now: time.Unix(1, 0)}, IDs: &ids{}})
	r, err := s.Decide(t.Context(), apphook.Call{Hook: hook.InboundStatus, SubjectKey: "a"})
	if err != nil || r.Effect != hook.Allow || r.DecisionID != "" {
		t.Errorf("result %+v, %v", r, err)
	}
	if st.reads != 0 || mem.reads != 0 || len(mem.records) != 0 {
		t.Errorf("%d settings reads, %d record reads, %d decisions", st.reads, mem.reads, len(mem.records))
	}
	if s.Configured(hook.InboundStatus) {
		t.Error("Configured is true")
	}
	used, err := s.InUse(t.Context())
	if err != nil || len(used) != 0 || st.reads != 0 {
		t.Errorf("InUse = %v, %v after %d reads", used, err, st.reads)
	}
}

func TestOffCallsNothing(t *testing.T) {
	h := newHarness(hook.ModeOff, hook.PassOnFailure)
	if r := h.decide(t, "a"); r.Effect != hook.Allow || r.DecisionID != "" {
		t.Errorf("result %+v", r)
	}
	if len(h.plugin.Requests()) != 0 {
		t.Error("off called the plugin")
	}
	used, err := h.s.InUse(t.Context())
	if err != nil || len(used) != 0 {
		t.Errorf("InUse = %v, %v", used, err)
	}
}

func TestAppliedAuditsOnlyWhatLeavesState(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
	for _, c := range []struct {
		verdict hook.Outcome
		audited bool
	}{{hook.MarkSensitive, true}, {hook.Flag, true}, {hook.Drop, false}, {hook.Allow, false}} {
		h.mem.events = nil
		h.plugin.Answer(apphook.Answer{Verdict: c.verdict})
		r := h.decide(t, string(c.verdict))
		err := h.mem.Tx(t.Context(), func(tx uow.Tx) error { return h.s.Applied(t.Context(), tx, r) })
		if err != nil {
			t.Fatal(err)
		}
		if got := len(h.mem.events) == 1; got != c.audited {
			t.Errorf("%s: audited = %v", c.verdict, got)
		}
	}
}

func TestOverrideIsItsOwnEventAndRunsTheEffects(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
	undone := 0
	h.s.RegisterOverrideEffect(hook.InboundStatus, func(context.Context, uow.Tx, hook.Record) error {
		undone++
		return nil
	})
	h.plugin.Answer(apphook.Answer{Verdict: hook.Drop})
	r := h.decide(t, "a")
	rec, err := h.s.Override(t.Context(), r.DecisionID, "moderator")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Override == nil || rec.Override.ByUserID != "moderator" || undone != 1 {
		t.Errorf("record %+v, undone %d", rec, undone)
	}
	if len(h.mem.events) != 1 {
		t.Fatalf("events %+v", h.mem.events)
	}
	if ev, ok := h.mem.events[0].(apphook.DecisionOverridden); !ok || ev.ByUserID != "moderator" {
		t.Errorf("event %+v", h.mem.events[0])
	}
	// The decision reads as allow when its job runs again, without calling the plugin.
	if again := h.decide(t, "a"); again.Effect != hook.Allow || len(h.plugin.Requests()) != 1 {
		t.Errorf("effect %s after %d calls", again.Effect, len(h.plugin.Requests()))
	}
	if _, err := h.s.Override(t.Context(), r.DecisionID, "moderator"); !errors.Is(err, apphook.ErrAlreadyOverridden) {
		t.Errorf("a second override: %v", err)
	}
	if _, err := h.s.Override(t.Context(), "none", ""); !errors.Is(err, apphook.ErrNotFound) {
		t.Errorf("an unknown decision: %v", err)
	}
}

func TestOverrideRefusesWhatHadNoEffect(t *testing.T) {
	h := newHarness(hook.ModeObserve, hook.PassOnFailure)
	h.plugin.Answer(apphook.Answer{Verdict: hook.Drop})
	r := h.decide(t, "a")
	if _, err := h.s.Override(t.Context(), r.DecisionID, "m"); !errors.Is(err, apphook.ErrNotOverridable) {
		t.Errorf("err = %v", err)
	}
}

type heldAccounts []string

func (h heldAccounts) HeldAccountIDs(context.Context) ([]string, error) { return h, nil }

type statuses []string

func (s statuses) ExistingStatusIDs(context.Context, []string) ([]string, error) { return s, nil }

func TestPurgeKeepsWhatTheDecisionLists(t *testing.T) {
	mem := newMemory()
	now := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	old := now.Add(-100 * 24 * time.Hour)
	add := func(id string, d hook.Decision) {
		d.ID, d.SubjectKey, d.Mode, d.DecidedAt = id, id, hook.ModeEnforce, old
		mem.records[id] = &hook.Record{Decision: d}
	}
	add("plain", hook.Decision{Hook: hook.InboundStatus, Outcome: hook.Drop})
	add("held", hook.Decision{Hook: hook.InboundStatus, Outcome: hook.Drop, AccountID: "held-account"})
	add("marks-a-status", hook.Decision{Hook: hook.InboundStatus, Outcome: hook.MarkSensitive})
	mem.statuses["marks-a-status"] = []string{"existing"}
	add("marks-a-gone-status", hook.Decision{Hook: hook.InboundStatus, Outcome: hook.MarkSensitive})
	mem.statuses["marks-a-gone-status"] = []string{"gone"}
	mem.records["recent"] = &hook.Record{Decision: hook.Decision{
		ID: "recent", SubjectKey: "recent", Hook: hook.InboundStatus, Mode: hook.ModeEnforce, Outcome: hook.Drop, DecidedAt: now,
	}}
	s := apphook.NewService(apphook.Deps{
		UnitOfWork: mem, Repository: mem, Settings: &settings{}, Clock: &clock{now: now}, IDs: &ids{},
		Holds: heldAccounts{"held-account"}, Statuses: statuses{"existing"},
	})
	n, err := s.PurgeDecisions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("removed %d", n)
	}
	for _, id := range []string{"held", "marks-a-status", "recent"} {
		if mem.records[id] == nil {
			t.Errorf("%s was removed", id)
		}
	}
	for _, id := range []string{"plain", "marks-a-gone-status"} {
		if mem.records[id] != nil {
			t.Errorf("%s stayed", id)
		}
	}
}

func TestEraseAccountData(t *testing.T) {
	h := newHarness(hook.ModeEnforce, hook.PassOnFailure)
	h.decide(t, "a")
	h.decide(t, "b")
	step, err := h.s.EraseAccountData(t.Context(), appaccount.ErasureTarget{AccountID: "account"}, "", 1)
	if err != nil || step.Done || len(h.mem.records) != 1 {
		t.Fatalf("step %+v, %v, %d left", step, err, len(h.mem.records))
	}
	step, err = h.s.EraseAccountData(t.Context(), appaccount.ErasureTarget{AccountID: "account"}, "", 2)
	if err != nil || !step.Done || len(h.mem.records) != 0 {
		t.Fatalf("step %+v, %v, %d left", step, err, len(h.mem.records))
	}
}
