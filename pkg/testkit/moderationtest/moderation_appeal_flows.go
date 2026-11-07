// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/contenttest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// AppealFlows verifies the appeals of a database adapter's moderation repository through the
// moderation use cases (FR-707, ADR-0058): a local account appeals an action on it once, within the
// instance's appeal window (a ban, also only before the account's deletion: no later than
// moderation.BanDeletionDelay and never once the account is deleted), never an action on another
// account, with a text of at most moderation.MaxAppealText characters; its own actions show the
// appeal, the deadline, and whether it can still appeal; a moderator upholds an appeal, keeping the
// action, or overturns it, lifting the action in the same transaction (the statuses that
// delete_statuses deleted stay deleted; a ban's scheduled deletion is canceled and its suspension
// ends, and a ban whose account is deleted is not overturned); each decision is audited and emitted
// once, and a decided appeal is not decided otherwise; the moderators list the appeals newest first
// by state, in keyset pages, and count the pending ones. The accounts and statuses are in-memory
// stand-ins. These are the flows of ADR-0153 §1, which internal/flowtest/moderation runs once on
// SQLite; AppealContract checks the repository's methods on every adapter.
func AppealFlows(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *appealEnv)
	}{
		{"FilesOnceWithinTheWindow", appealFiles},
		{"UpholdKeepsTheAction", appealUphold},
		{"OverturnLiftsTheAction", appealOverturn},
		{"BanIsAppealedUntilTheDeletion", appealBan},
		{"ListsAndCounts", appealLists},
		{"ReadsAnAppealInATransaction", appealReadsInTransaction},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newAppealEnv(t, newHarness)) })
	}
}

// stubAppealWindow stands in for the instance setting appeal_window_days.
type stubAppealWindow struct {
	mu sync.Mutex
	d  time.Duration
}

func (w *stubAppealWindow) set(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.d = d
}

// AppealWindow implements appmoderation.AppealWindow.
func (w *stubAppealWindow) AppealWindow(context.Context) (time.Duration, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.d, nil
}

type appealEnv struct {
	h          ModerationHarness
	projectors *uow.Projectors
	clock      *testkit.Clock
	ids        *testkit.IDGenerator
	svc        *appmoderation.Service
	policy     *appmoderation.AccountPolicy
	statuses   *stubModerationStatuses
	window     *stubAppealWindow
	actors     *identitytest.StubActors
	// lifecycles stands in for the account module's deletions.
	lifecycles *stubLifecycles
	events     *testkit.EventLog
	// local and other are local accounts; moderator is a moderator's user.
	local, other, moderator string
}

func newAppealEnv(t *testing.T, newHarness ModerationFactory) *appealEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appmoderation.RegisterAuditProjector(projectors, h.Moderation)
	clock := testkit.NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	ids := testkit.NewIDGenerator(clock)
	actors := &identitytest.StubActors{Actors: map[string]appaccount.Actor{}}
	e := &appealEnv{
		h: h, projectors: projectors, clock: clock, ids: ids, events: &testkit.EventLog{}, window: &stubAppealWindow{d: moderation.DefaultAppealWindow},
		statuses: &stubModerationStatuses{live: map[string]appcontent.Status{}, deleted: map[string]string{}, marked: map[string][]string{}},
		local:    ids.NewID(), other: ids.NewID(), moderator: ids.NewID(), actors: actors,
		lifecycles: &stubLifecycles{due: map[string]time.Time{}, deleted: map[string]bool{}},
	}
	for _, typ := range []string{appmoderation.EventAppealFiled, appmoderation.EventAppealDecided, appmoderation.EventAccountSuspensionChanged} {
		projectors.Register(typ, e.events.Record)
	}
	for id, name := range map[string]string{e.local: "alice", e.other: "carol"} {
		actors.Set(appaccount.Actor{
			Account: account.Account{ID: id, Domain: account.LocalDomain},
			Local:   &account.LocalAccount{AccountID: id, Username: name},
		})
	}
	e.svc = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Moderation, Accounts: actors, Clock: clock, IDs: ids,
		Statuses: e.statuses, Lifecycles: e.lifecycles, AppealWindow: e.window,
	})
	e.policy = appmoderation.NewAccountPolicy(h.Moderation, actors, clock)
	return e
}

func (e *appealEnv) issue(t *testing.T, accountID string, kind moderation.ActionKind, statusIDs ...string) moderation.Action {
	t.Helper()
	a, err := e.svc.IssueAccountAction(t.Context(), appmoderation.AccountActionInput{
		TargetAccountID: accountID, Kind: kind, ReasonCategory: moderation.CategorySpam, PublicReason: "spam",
		PrivateNote: "the moderators' note", NotifyTarget: true, StatusIDs: statusIDs,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction(%s) = %v", kind, err)
	}
	return a
}

func (e *appealEnv) appeal(t *testing.T, accountID string, a moderation.Action) appmoderation.TargetActionView {
	t.Helper()
	v, err := e.svc.FileAppeal(t.Context(), accountID, a.ID, "I did not spam.")
	if err != nil {
		t.Fatalf("FileAppeal(%s) = %v", a.ID, err)
	}
	if v.Appeal == nil {
		t.Fatalf("FileAppeal(%s) = %+v, want its appeal", a.ID, v)
	}
	return v
}

// audited returns the audit log entries of an appeal, newest first.
func (e *appealEnv) audited(t *testing.T, kind moderation.AuditTargetKind, id string) []moderation.AuditAction {
	t.Helper()
	entries, err := e.svc.AuditLog(t.Context(), appmoderation.AuditQuery{TargetKind: kind, TargetID: id}, 10)
	if err != nil {
		t.Fatalf("AuditLog(%s %s) = %v", kind, id, err)
	}
	out := make([]moderation.AuditAction, len(entries))
	for i, en := range entries {
		out[i] = en.Action
	}
	return out
}

func appealFiles(t *testing.T, e *appealEnv) {
	ctx := t.Context()
	suspend := e.issue(t, e.local, moderation.ActionSuspend)
	deadline := suspend.IssuedAt.Add(moderation.DefaultAppealWindow)
	e.events.Take()

	// Before the appeal, the account sees that it can appeal until the deadline.
	v, err := e.svc.OwnAction(ctx, e.local, suspend.ID)
	if err != nil || !v.Appealable || v.Appeal != nil || v.AppealDeadline == nil || !v.AppealDeadline.Equal(deadline) {
		t.Fatalf("OwnAction before the appeal = %+v, %v; want appealable until %v", v, err, deadline)
	}

	for _, c := range []struct {
		name, accountID, actionID, text string
		want                            error
	}{
		{"another account's action", e.other, suspend.ID, "mine", appmoderation.ErrNotFound},
		{"an unknown action", e.local, e.ids.NewID(), "mine", appmoderation.ErrNotFound},
		{"an empty text", e.local, suspend.ID, " \n", moderation.ErrInvalidAppeal},
		{"a text too long", e.local, suspend.ID, strings.Repeat("あ", moderation.MaxAppealText+1), moderation.ErrInvalidAppeal},
		{"a text with NUL", e.local, suspend.ID, "a\x00b", moderation.ErrInvalidAppeal},
	} {
		if _, err := e.svc.FileAppeal(ctx, c.accountID, c.actionID, c.text); !errors.Is(err, c.want) {
			t.Errorf("FileAppeal(%s) = %v, want %v", c.name, err, c.want)
		}
	}
	if evs := e.events.Take(); len(evs) != 0 {
		t.Errorf("events of refused appeals = %v, want none", contenttest.EventTypes(evs))
	}

	// The longest text is accepted, at the deadline.
	e.clock.Set(deadline)
	v, err = e.svc.FileAppeal(ctx, e.local, suspend.ID, strings.Repeat("あ", moderation.MaxAppealText))
	if err != nil {
		t.Fatalf("FileAppeal at the deadline = %v", err)
	}
	if v.Appealable || v.Appeal == nil || v.Appeal.State != moderation.AppealPending || !v.Appeal.SubmittedAt.Equal(deadline) || v.Appeal.DecidedAt != nil {
		t.Errorf("FileAppeal = %+v, want a pending appeal that cannot be repeated", v)
	}
	evs := e.events.Take()
	if len(evs) != 1 {
		t.Fatalf("events of the appeal = %v, want one AppealFiled", contenttest.EventTypes(evs))
	}
	if f, ok := evs[0].(appmoderation.AppealFiled); !ok || f.Appeal.ID != v.Appeal.ID || f.Appeal.AccountID != e.local || f.Appeal.ActionID != suspend.ID {
		t.Errorf("event = %+v, want AppealFiled of %s", evs[0], v.Appeal.ID)
	}
	if _, err := e.svc.FileAppeal(ctx, e.local, suspend.ID, "again"); !errors.Is(err, appmoderation.ErrAlreadyAppealed) {
		t.Errorf("a second FileAppeal = %v, want ErrAlreadyAppealed", err)
	}

	// After the window, an action cannot be appealed; the window is the instance's.
	e.window.set(20 * 24 * time.Hour)
	warn := e.issue(t, e.local, moderation.ActionWarn)
	e.clock.Advance(20*24*time.Hour + time.Millisecond)
	if _, err := e.svc.FileAppeal(ctx, e.local, warn.ID, "late"); !errors.Is(err, appmoderation.ErrAppealWindowClosed) {
		t.Errorf("FileAppeal after the window = %v, want ErrAppealWindowClosed", err)
	}
	if v, err := e.svc.OwnAction(ctx, e.local, warn.ID); err != nil || v.Appealable || v.Appeal != nil {
		t.Errorf("OwnAction after the window = %+v, %v; want not appealable", v, err)
	}

	// The list of the account's own actions shows the appeals.
	own, err := e.svc.OwnActions(ctx, e.local, appmoderation.OwnActionFilter{}, 10)
	if err != nil || len(own) != 2 || own[0].ID != warn.ID || own[1].ID != suspend.ID || own[0].Appeal != nil || own[1].Appeal == nil {
		t.Errorf("OwnActions = %+v, %v; want the warning without an appeal and the suspension with one", own, err)
	}
}

func appealUphold(t *testing.T, e *appealEnv) {
	ctx := t.Context()
	limit := e.issue(t, e.local, moderation.ActionLimit)
	appeal := e.appeal(t, e.local, limit).Appeal
	e.events.Take()

	for _, c := range []struct {
		name, id, reason string
		decision         moderation.AppealDecisionKind
		want             error
	}{
		{"an unknown appeal", e.ids.NewID(), "", moderation.AppealDecisionUpheld, appmoderation.ErrNotFound},
		{"an unknown decision", appeal.ID, "", "pending", moderation.ErrInvalidAppeal},
		{"a reason too long", appeal.ID, strings.Repeat("a", moderation.MaxAppealText+1), moderation.AppealDecisionUpheld, moderation.ErrInvalidAppeal},
	} {
		if _, err := e.svc.DecideAppeal(ctx, c.id, c.decision, c.reason, e.moderator); !errors.Is(err, c.want) {
			t.Errorf("DecideAppeal(%s) = %v, want %v", c.name, err, c.want)
		}
	}

	e.clock.Advance(time.Hour)
	v, err := e.svc.DecideAppeal(ctx, appeal.ID, moderation.AppealDecisionUpheld, "The posts were spam.", e.moderator)
	if err != nil {
		t.Fatalf("DecideAppeal(upheld) = %v", err)
	}
	d := v.Decision
	if v.State() != moderation.AppealUpheld || d == nil || d.DecidedByUserID != e.moderator || d.PublicReason != "The posts were spam." ||
		!d.DecidedAt.Equal(e.clock.Now()) || v.Action.Action.ID != limit.ID || v.Action.State != moderation.ActionActive {
		t.Errorf("DecideAppeal(upheld) = %+v, want upheld with the action in effect", v)
	}
	if rs, err := e.h.Moderation.AccountRestrictionsInEffect(ctx, e.local, e.clock.Now()); err != nil || len(rs) != len(moderation.ActionLimit.Restrictions()) {
		t.Errorf("restrictions after upholding = %v, %v; want those of the limit", rs, err)
	}
	if got := contenttest.EventTypes(e.events.Take()); len(got) != 1 || got[0] != appmoderation.EventAppealDecided {
		t.Errorf("events of the decision = %v, want one AppealDecided", got)
	}
	if got := e.audited(t, moderation.AuditTargetAppeal, appeal.ID); len(got) != 1 || got[0] != moderation.AuditAppealUpheld {
		t.Errorf("audit log of the appeal = %v, want appeal_upheld", got)
	}

	// The account sees the decision and its reason, never who decided.
	own, err := e.svc.OwnAction(ctx, e.local, limit.ID)
	if err != nil || own.Appeal == nil || own.Appeal.State != moderation.AppealUpheld || own.Appeal.DecisionReason != "The posts were spam." || own.Appeal.DecidedAt == nil {
		t.Errorf("OwnAction after upholding = %+v, %v", own, err)
	}
	if own, err := e.svc.OwnAppealAction(ctx, e.local, appeal.ID); err != nil || own.ID != limit.ID {
		t.Errorf("OwnAppealAction = %+v, %v; want the limit", own, err)
	}
	if _, err := e.svc.OwnAppealAction(ctx, e.other, appeal.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("OwnAppealAction(another account) = %v, want ErrNotFound", err)
	}

	// Upholding again changes nothing; overturning the upheld appeal is refused.
	if _, err := e.svc.DecideAppeal(ctx, appeal.ID, moderation.AppealDecisionUpheld, "again", ""); err != nil {
		t.Errorf("DecideAppeal(upheld) again = %v", err)
	}
	if _, err := e.svc.DecideAppeal(ctx, appeal.ID, moderation.AppealDecisionOverturned, "", ""); !errors.Is(err, appmoderation.ErrAppealDecided) {
		t.Errorf("DecideAppeal(overturned) of an upheld appeal = %v, want ErrAppealDecided", err)
	}
	if got := contenttest.EventTypes(e.events.Take()); len(got) != 0 {
		t.Errorf("events of the repeated decisions = %v, want none", got)
	}
	if got := e.audited(t, moderation.AuditTargetAppeal, appeal.ID); len(got) != 1 {
		t.Errorf("audit log after the repeated decisions = %v, want one entry", got)
	}
	if v, err := e.svc.Appeal(ctx, appeal.ID); err != nil || v.Decision == nil || v.Decision.PublicReason != "The posts were spam." {
		t.Errorf("Appeal after the repeated decisions = %+v, %v; want the first decision", v, err)
	}
}

func appealOverturn(t *testing.T, e *appealEnv) {
	ctx := t.Context()
	suspend := e.issue(t, e.local, moderation.ActionSuspend)
	st := content.Status{ID: e.ids.NewID(), URI: "https://social.example/statuses/" + e.ids.NewID(), AuthorAccountID: e.local, IsLocal: true}
	e.statuses.add(st)
	deletion := e.issue(t, e.local, moderation.ActionDeleteStatuses, st.ID)
	first := e.appeal(t, e.local, suspend).Appeal
	second := e.appeal(t, e.local, deletion).Appeal
	e.events.Take()

	// Overturning lifts the action at once, with the lift audited too.
	v, err := e.svc.DecideAppeal(ctx, first.ID, moderation.AppealDecisionOverturned, "", "")
	if err != nil {
		t.Fatalf("DecideAppeal(overturned) = %v", err)
	}
	if v.State() != moderation.AppealOverturned || v.Decision.DecidedByUserID != "" || v.Action.State != moderation.ActionLifted || v.Action.Lift == nil {
		t.Errorf("DecideAppeal(overturned) = %+v, want overturned with the action lifted", v)
	}
	if rs, err := e.h.Moderation.AccountRestrictionsInEffect(ctx, e.local, e.clock.Now()); err != nil || len(rs) != 0 {
		t.Errorf("restrictions after overturning = %v, %v; want none", rs, err)
	}
	if restricted, err := e.policy.LoginRestricted(ctx, e.local); err != nil || restricted {
		t.Errorf("LoginRestricted after overturning the suspension = %v, %v; want false", restricted, err)
	}
	if got := e.audited(t, moderation.AuditTargetAppeal, first.ID); len(got) != 1 || got[0] != moderation.AuditAppealOverturned {
		t.Errorf("audit log of the appeal = %v, want appeal_overturned", got)
	}
	if got := e.audited(t, moderation.AuditTargetAccount, e.local); len(got) == 0 || got[0] != moderation.AuditAccountActionLifted {
		t.Errorf("audit log of the account = %v, want account_action_lifted first", got)
	}
	got := e.events.Take()
	if d, ok := lastEvent[appmoderation.AppealDecided](got); !ok || d.Appeal.ID != first.ID || d.Decision.Decision != moderation.AppealDecisionOverturned {
		t.Errorf("events of overturning = %v, want AppealDecided of %s", contenttest.EventTypes(got), first.ID)
	}

	// Overturning delete_statuses lifts it, and the status stays deleted.
	if _, err := e.svc.DecideAppeal(ctx, second.ID, moderation.AppealDecisionOverturned, "Sorry.", e.moderator); err != nil {
		t.Fatalf("DecideAppeal(overturned delete_statuses) = %v", err)
	}
	if own, err := e.svc.OwnAction(ctx, e.local, deletion.ID); err != nil || own.State != moderation.ActionLifted || own.Appeal.State != moderation.AppealOverturned {
		t.Errorf("OwnAction(delete_statuses) = %+v, %v; want lifted and overturned", own, err)
	}
	if _, err := e.statuses.Find(ctx, st.ID); !errors.Is(err, appcontent.ErrNotFound) {
		t.Errorf("Find(deleted status) after overturning = %v, want it still deleted", err)
	}

	// Overturning again changes nothing; upholding an overturned appeal is refused.
	if _, err := e.svc.DecideAppeal(ctx, first.ID, moderation.AppealDecisionOverturned, "", e.moderator); err != nil {
		t.Errorf("DecideAppeal(overturned) again = %v", err)
	}
	if _, err := e.svc.DecideAppeal(ctx, first.ID, moderation.AppealDecisionUpheld, "", e.moderator); !errors.Is(err, appmoderation.ErrAppealDecided) {
		t.Errorf("DecideAppeal(upheld) of an overturned appeal = %v, want ErrAppealDecided", err)
	}
	if got := e.audited(t, moderation.AuditTargetAppeal, first.ID); len(got) != 1 {
		t.Errorf("audit log after the repeated decisions = %v, want one entry", got)
	}

	// Overturning an appeal of an action lifted meanwhile records the decision only.
	warn := e.issue(t, e.local, moderation.ActionWarn)
	third := e.appeal(t, e.local, warn).Appeal
	if err := e.svc.LiftAccountAction(ctx, warn.ID, e.moderator, "a mistake"); err != nil {
		t.Fatal(err)
	}
	before := len(e.audited(t, moderation.AuditTargetAccount, e.local))
	if v, err := e.svc.DecideAppeal(ctx, third.ID, moderation.AppealDecisionOverturned, "", e.moderator); err != nil || v.Action.Lift.Reason != "a mistake" {
		t.Errorf("DecideAppeal(overturned) of a lifted action = %+v, %v; want the first lift kept", v, err)
	}
	if after := len(e.audited(t, moderation.AuditTargetAccount, e.local)); after != before {
		t.Errorf("account audit entries = %d, want %d: the action was already lifted", after, before)
	}
}

// lastEvent returns the last event of type E in events.
func lastEvent[E uow.Event](events []uow.Event) (E, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if ev, ok := events[i].(E); ok {
			return ev, true
		}
	}
	var zero E
	return zero, false
}

func appealLists(t *testing.T, e *appealEnv) {
	ctx := t.Context()
	var appeals []string
	for range 3 {
		a := e.issue(t, e.local, moderation.ActionWarn)
		appeals = append(appeals, e.appeal(t, e.local, a).Appeal.ID)
		e.clock.Advance(time.Second)
	}
	if _, err := e.svc.DecideAppeal(ctx, appeals[1], moderation.AppealDecisionUpheld, "", ""); err != nil {
		t.Fatal(err)
	}
	ids := func(vs []appmoderation.AppealView) []string {
		out := make([]string, len(vs))
		for i, v := range vs {
			out[i] = v.Appeal.ID
		}
		return out
	}
	for _, c := range []struct {
		name string
		f    appmoderation.AppealFilter
		want []string
	}{
		{"every appeal", appmoderation.AppealFilter{}, []string{appeals[2], appeals[1], appeals[0]}},
		{"the pending ones", appmoderation.AppealFilter{State: moderation.AppealPending}, []string{appeals[2], appeals[0]}},
		{"the upheld ones", appmoderation.AppealFilter{State: moderation.AppealUpheld}, []string{appeals[1]}},
		{"the overturned ones", appmoderation.AppealFilter{State: moderation.AppealOverturned}, []string{}},
		{"after a cursor", appmoderation.AppealFilter{Before: appeals[2]}, []string{appeals[1], appeals[0]}},
	} {
		got, err := e.svc.Appeals(ctx, c.f, 10)
		if err != nil || strings.Join(ids(got), ",") != strings.Join(c.want, ",") {
			t.Errorf("Appeals(%s) = %v, %v; want %v", c.name, ids(got), err, c.want)
		}
	}
	if got, err := e.svc.Appeals(ctx, appmoderation.AppealFilter{}, 1); err != nil || len(got) != 1 || got[0].Appeal.ID != appeals[2] {
		t.Errorf("Appeals(limit 1) = %v, %v", ids(got), err)
	} else if got[0].Action.Action.Kind != moderation.ActionWarn || got[0].Action.Action.PrivateNote == "" {
		t.Errorf("Appeals(limit 1) = %+v, want the action with its private note for the moderators", got[0])
	}
	if _, err := e.svc.Appeals(ctx, appmoderation.AppealFilter{State: "closed"}, 10); !errors.Is(err, appmoderation.ErrInvalidQuery) {
		t.Errorf("Appeals(unknown state) = %v, want ErrInvalidQuery", err)
	}
	if n, err := e.svc.CountPendingAppeals(ctx); err != nil || n != 2 {
		t.Errorf("CountPendingAppeals = %d, %v; want 2", n, err)
	}
}

func appealBan(t *testing.T, e *appealEnv) {
	ctx := t.Context()
	ban := e.issue(t, e.local, moderation.ActionBan)
	deadline := ban.IssuedAt.Add(moderation.BanDeletionDelay)

	// A ban is appealed until the deletion of its account, sooner than the window.
	v, err := e.svc.OwnAction(ctx, e.local, ban.ID)
	if err != nil || !v.Appealable || v.AppealDeadline == nil || !v.AppealDeadline.Equal(deadline) {
		t.Fatalf("OwnAction(ban) = %+v, %v; want appealable until %v", v, err, deadline)
	}
	if err := e.svc.LiftAccountAction(ctx, ban.ID, e.moderator, ""); !errors.Is(err, appmoderation.ErrNotLiftable) {
		t.Errorf("LiftAccountAction(ban) = %v, want ErrNotLiftable", err)
	}
	// The ban lets its account sign in to appeal it (identity.LoginPolicy); no other account.
	if banned, err := e.policy.Banned(ctx, e.local); err != nil || !banned {
		t.Errorf("Banned(the banned account) = %v, %v; want true", banned, err)
	}
	if banned, err := e.policy.Banned(ctx, e.other); err != nil || banned {
		t.Errorf("Banned(another account) = %v, %v; want false", banned, err)
	}
	e.clock.Set(deadline)
	appeal := e.appeal(t, e.local, ban).Appeal
	e.events.Take()

	// Overturning cancels the deletion and ends the suspension, in one transaction.
	got, err := e.svc.DecideAppeal(ctx, appeal.ID, moderation.AppealDecisionOverturned, "", e.moderator)
	if err != nil {
		t.Fatalf("DecideAppeal(overturned ban) = %v", err)
	}
	if got.Action.State != moderation.ActionLifted {
		t.Errorf("the ban after overturning = %+v, want lifted", got.Action)
	}
	if e.lifecycles.canceled[e.local] != ban.ID {
		t.Errorf("canceled deletions = %v, want the ban's", e.lifecycles.canceled)
	}
	if restricted, err := e.policy.LoginRestricted(ctx, e.local); err != nil || restricted {
		t.Errorf("LoginRestricted after overturning the ban = %v, %v; want false", restricted, err)
	}
	if banned, err := e.policy.Banned(ctx, e.local); err != nil || banned {
		t.Errorf("Banned after overturning the ban = %v, %v; want false", banned, err)
	}
	if s, ok := lastEvent[appmoderation.AccountSuspensionChanged](e.events.Take()); !ok || s.Suspended || s.AccountID != e.local {
		t.Errorf("suspension change = %+v, %v; want the end of the suspension", s, ok)
	}
	if got := e.audited(t, moderation.AuditTargetAccount, e.local); len(got) < 2 ||
		got[0] != moderation.AuditAccountActionLifted || got[1] != moderation.AuditAccountDeletionCanceled {
		t.Errorf("audit log of the account = %v, want the lift after the canceled deletion", got)
	}

	// After the deletion, a ban is not appealed, and a pending appeal of it is not overturned.
	second := e.issue(t, e.other, moderation.ActionBan)
	third := e.appeal(t, e.other, second).Appeal
	e.lifecycles.mu.Lock()
	e.lifecycles.deleted[e.other] = true
	e.lifecycles.mu.Unlock()
	a, err := e.actors.FindByID(ctx, e.other)
	if err != nil {
		t.Fatal(err)
	}
	a.Lifecycle = account.LifecycleState{AccountID: e.other, State: account.StateDeleted}
	e.actors.Set(a)
	if _, err := e.svc.DecideAppeal(ctx, third.ID, moderation.AppealDecisionOverturned, "", e.moderator); !errors.Is(err, appmoderation.ErrNotAppealable) {
		t.Errorf("DecideAppeal(overturned) of a deleted account's ban = %v, want ErrNotAppealable", err)
	}
	if v, err := e.svc.Appeal(ctx, third.ID); err != nil || v.State() != moderation.AppealPending || v.Action.State != moderation.ActionActive {
		t.Errorf("the refused overturning = %+v, %v; want the appeal pending and the ban in effect", v, err)
	}
	if _, err := e.svc.DecideAppeal(ctx, third.ID, moderation.AppealDecisionUpheld, "", e.moderator); err != nil {
		t.Errorf("DecideAppeal(upheld) of a deleted account's ban = %v", err)
	}
	fourth := e.issue(t, e.other, moderation.ActionBan)
	if _, err := e.svc.FileAppeal(ctx, e.other, fourth.ID, "please"); !errors.Is(err, appmoderation.ErrNotAppealable) {
		t.Errorf("FileAppeal(ban of a deleted account) = %v, want ErrNotAppealable", err)
	}
	if v, err := e.svc.OwnAction(ctx, e.other, fourth.ID); err != nil || v.Appealable {
		t.Errorf("OwnAction(ban of a deleted account) = %+v, %v; want not appealable", v, err)
	}

	// Past the deletion's due time, a ban is not appealed even within the window.
	late := e.issue(t, e.local, moderation.ActionBan)
	e.clock.Advance(moderation.BanDeletionDelay + time.Millisecond)
	if _, err := e.svc.FileAppeal(ctx, e.local, late.ID, "please"); !errors.Is(err, appmoderation.ErrAppealWindowClosed) {
		t.Errorf("FileAppeal(ban past its deletion) = %v, want ErrAppealWindowClosed", err)
	}
}
