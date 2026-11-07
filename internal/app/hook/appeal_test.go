// SPDX-License-Identifier: AGPL-3.0-only

package hook_test

import (
	"context"
	"errors"
	"testing"
	"time"

	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/hook"
)

// The appeal half of memory.

func (m *memory) appealPending(decisionID string) bool {
	for id, a := range m.appeals {
		if _, reviewed := m.reviews[id]; a.DecisionID == decisionID && !reviewed {
			return true
		}
	}
	return false
}

func (m *memory) appealRecord(a *hook.Appeal) hook.AppealRecord {
	out := hook.AppealRecord{Appeal: *a, Decision: *m.records[a.DecisionID]}
	if r, ok := m.reviews[a.ID]; ok {
		out.Review = &r
	}
	return out
}

func (m *memory) InsertAppeal(_ context.Context, _ uow.Tx, a hook.Appeal) (bool, error) {
	for _, other := range m.appeals {
		if other.DecisionID == a.DecisionID {
			return false, nil
		}
	}
	m.appeals[a.ID] = &a
	return true, nil
}

func (m *memory) AppealTx(_ context.Context, _ uow.Tx, id string) (hook.AppealRecord, error) {
	a, ok := m.appeals[id]
	if !ok {
		return hook.AppealRecord{}, apphook.ErrAppealNotFound
	}
	return m.appealRecord(a), nil
}

func (m *memory) Appeal(ctx context.Context, id string) (hook.AppealRecord, error) {
	return m.AppealTx(ctx, nil, id)
}

func (m *memory) AppealOfDecision(_ context.Context, decisionID string) (hook.AppealRecord, error) {
	for _, a := range m.appeals {
		if a.DecisionID == decisionID {
			return m.appealRecord(a), nil
		}
	}
	return hook.AppealRecord{}, apphook.ErrAppealNotFound
}

func (m *memory) ListAppeals(_ context.Context, q apphook.AppealQuery) ([]hook.AppealRecord, error) {
	var out []hook.AppealRecord
	for id, a := range m.appeals {
		if _, reviewed := m.reviews[id]; q.Pending && reviewed {
			continue
		}
		out = append(out, m.appealRecord(a))
	}
	return out, nil
}

func (m *memory) InsertAppealDecision(_ context.Context, _ uow.Tx, d hook.AppealDecision) (bool, error) {
	if _, ok := m.reviews[d.AppealID]; ok {
		return false, nil
	}
	m.reviews[d.AppealID] = d
	return true, nil
}

func (m *memory) InsertExemption(_ context.Context, _ uow.Tx, e hook.Exemption) error {
	m.exemptions[string(e.Hook)+" "+e.AccountID] = true
	return nil
}

func (m *memory) Exempt(_ context.Context, n hook.Name, accountID string) (bool, error) {
	return m.exemptions[string(n)+" "+accountID], nil
}

func (m *memory) DeleteAppealsOfAccount(_ context.Context, _ uow.Tx, accountID string) error {
	for id, a := range m.appeals {
		if a.AccountID == accountID {
			delete(m.appeals, id)
			delete(m.reviews, id)
		}
	}
	for key := range m.exemptions {
		if key == string(hook.LocalStatus)+" "+accountID {
			delete(m.exemptions, key)
		}
	}
	return nil
}

// localHarness is a harness whose plugin serves local.status.
func localHarness(mode hook.Mode) *harness {
	h := &harness{
		mem: newMemory(), plugin: &plugin{answer: apphook.Answer{Verdict: hook.Reject, Reason: hook.ReasonSpam}},
		settings: &settings{mode: mode, onFailure: hook.PassOnFailure}, clock: &clock{now: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)},
	}
	h.s = apphook.NewService(apphook.Deps{
		UnitOfWork: h.mem, Repository: h.mem, Settings: h.settings, Clock: h.clock, IDs: &ids{},
		Plugins: map[hook.Name]apphook.Plugin{hook.LocalStatus: h.plugin},
	})
	return h
}

func (h *harness) post(t *testing.T, key, accountID string) apphook.Result {
	t.Helper()
	r, err := h.s.Decide(t.Context(), apphook.Call{Hook: hook.LocalStatus, SubjectKey: key, AccountID: accountID})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	return r
}

func TestAnAppealIsReviewedByPeopleAndUpholdingKeepsTheDecision(t *testing.T) {
	h := localHarness(hook.ModeEnforce)
	r := h.post(t, "status-1", "author")
	if r.Effect != hook.Reject {
		t.Fatalf("effect %s", r.Effect)
	}
	calls := len(h.plugin.Requests())
	in := apphook.AppealInput{AccountID: "author", DecisionID: r.DecisionID, StatusText: "hello", Comment: " not spam "}
	if _, err := h.s.FileAppeal(t.Context(), apphook.AppealInput{AccountID: "other", DecisionID: r.DecisionID, StatusText: "x"}); !errors.Is(err, apphook.ErrAppealNotFound) {
		t.Errorf("another account's appeal: %v", err)
	}
	a, err := h.s.FileAppeal(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if a.Appeal.Comment != "not spam" || a.Review != nil {
		t.Errorf("appeal %+v", a)
	}
	if _, err := h.s.FileAppeal(t.Context(), in); !errors.Is(err, apphook.ErrAlreadyAppealed) {
		t.Errorf("a second appeal: %v", err)
	}
	rec, err := h.s.DecideAppeal(t.Context(), a.Appeal.ID, hook.AppealUpheld, "moderator")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Review == nil || rec.Review.Outcome != hook.AppealUpheld || rec.Decision.Override != nil {
		t.Errorf("upheld %+v", rec)
	}
	// Filing tells the moderators; upholding tells the author and is audited, and changes nothing else.
	if len(h.mem.events) != 2 || len(h.mem.exemptions) != 0 {
		t.Fatalf("upholding did something: %v %v", h.mem.events, h.mem.exemptions)
	}
	if filed, ok := h.mem.events[0].(apphook.AppealFiled); !ok || filed.Appeal.ID != a.Appeal.ID {
		t.Errorf("filing: %+v", h.mem.events[0])
	}
	if decided, ok := h.mem.events[1].(apphook.AppealDecided); !ok || decided.Review.Outcome != hook.AppealUpheld ||
		decided.Appeal.ID != a.Appeal.ID || decided.AuditID == "" || decided.Label != "local.status status-1" {
		t.Errorf("upholding: %+v", h.mem.events[1])
	}
	if _, err := h.s.DecideAppeal(t.Context(), a.Appeal.ID, hook.AppealUpheld, "moderator"); err != nil {
		t.Errorf("the same review again: %v", err)
	}
	if _, err := h.s.DecideAppeal(t.Context(), a.Appeal.ID, hook.AppealOverturned, "moderator"); !errors.Is(err, apphook.ErrAppealDecided) {
		t.Errorf("the other review: %v", err)
	}
	if len(h.plugin.Requests()) != calls {
		t.Error("an appeal called the plugin")
	}
	if again := h.post(t, "status-2", "author"); again.Effect != hook.Reject {
		t.Errorf("after upholding, %s", again.Effect)
	}
}

func TestOverturningCancelsAndExemptsTheAccount(t *testing.T) {
	h := localHarness(hook.ModeEnforce)
	r := h.post(t, "status-1", "author")
	a, err := h.s.FileAppeal(t.Context(), apphook.AppealInput{AccountID: "author", DecisionID: r.DecisionID, StatusText: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := h.s.DecideAppeal(t.Context(), a.Appeal.ID, hook.AppealOverturned, "moderator")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Decision.Override == nil || rec.Decision.Override.ByUserID != "moderator" {
		t.Errorf("not cancelled: %+v", rec.Decision)
	}
	if len(h.mem.events) != 3 {
		t.Fatalf("events %v", h.mem.events)
	}
	if _, ok := h.mem.events[1].(apphook.DecisionOverridden); !ok {
		t.Errorf("event %+v", h.mem.events[1])
	}
	if decided, ok := h.mem.events[2].(apphook.AppealDecided); !ok || decided.Review.Outcome != hook.AppealOverturned {
		t.Errorf("event %+v", h.mem.events[2])
	}
	calls := len(h.plugin.Requests())
	if again := h.post(t, "status-2", "author"); again.Effect != hook.Allow || again.DecisionID != "" {
		t.Errorf("after overturning: %+v", again)
	}
	if len(h.plugin.Requests()) != calls {
		t.Error("an exempt account was sent to the plugin")
	}
	if other := h.post(t, "status-3", "other"); other.Effect != hook.Reject {
		t.Errorf("another account: %s", other.Effect)
	}
}

func TestOnlyARejectInForceInsideTheWindowCanBeAppealed(t *testing.T) {
	h := localHarness(hook.ModeObserve)
	observed := h.post(t, "observed", "author")
	if _, err := h.s.FileAppeal(t.Context(), apphook.AppealInput{AccountID: "author", DecisionID: observed.DecisionID, StatusText: "x"}); !errors.Is(err, hook.ErrNotAppealable) {
		t.Errorf("observe: %v", err)
	}
	h.settings.mode = hook.ModeEnforce
	r := h.post(t, "late", "author")
	h.clock.now = h.clock.now.Add(31 * 24 * time.Hour)
	if _, err := h.s.FileAppeal(t.Context(), apphook.AppealInput{AccountID: "author", DecisionID: r.DecisionID, StatusText: "x"}); !errors.Is(err, hook.ErrAppealWindowClosed) {
		t.Errorf("late: %v", err)
	}
	var textErr *hook.AppealTextError
	if _, err := h.s.FileAppeal(t.Context(), apphook.AppealInput{AccountID: "author", DecisionID: r.DecisionID, StatusText: " "}); !errors.As(err, &textErr) {
		t.Errorf("blank: %v", err)
	}
}

func TestPurgeKeepsAnAppealUnderReview(t *testing.T) {
	h := localHarness(hook.ModeEnforce)
	pending := h.post(t, "pending", "author")
	decided := h.post(t, "decided", "author")
	for _, id := range []string{pending.DecisionID, decided.DecisionID} {
		if _, err := h.s.FileAppeal(t.Context(), apphook.AppealInput{AccountID: "author", DecisionID: id, StatusText: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	upheld, err := h.mem.AppealOfDecision(t.Context(), decided.DecisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.DecideAppeal(t.Context(), upheld.Appeal.ID, hook.AppealUpheld, ""); err != nil {
		t.Fatal(err)
	}
	h.post(t, "never-appealed", "author")
	h.clock.now = h.clock.now.Add(100 * 24 * time.Hour)
	n, err := h.s.PurgeDecisions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || h.mem.records[pending.DecisionID] == nil {
		t.Errorf("removed %d; the pending one stays: %v", n, h.mem.records[pending.DecisionID] != nil)
	}
	if len(h.mem.appeals) != 1 {
		t.Errorf("%d appeals left", len(h.mem.appeals))
	}
}

func TestErasureRemovesAppealsAndExemptions(t *testing.T) {
	h := localHarness(hook.ModeEnforce)
	r := h.post(t, "status-1", "author")
	a, err := h.s.FileAppeal(t.Context(), apphook.AppealInput{AccountID: "author", DecisionID: r.DecisionID, StatusText: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.DecideAppeal(t.Context(), a.Appeal.ID, hook.AppealOverturned, ""); err != nil {
		t.Fatal(err)
	}
	done, err := h.s.EraseAccount(t.Context(), "author", 10)
	if err != nil || !done {
		t.Fatalf("done %v, %v", done, err)
	}
	if len(h.mem.records) != 0 || len(h.mem.appeals) != 0 || len(h.mem.reviews) != 0 || len(h.mem.exemptions) != 0 {
		t.Errorf("left: %d decisions, %d appeals, %d reviews, %d exemptions",
			len(h.mem.records), len(h.mem.appeals), len(h.mem.reviews), len(h.mem.exemptions))
	}
}
