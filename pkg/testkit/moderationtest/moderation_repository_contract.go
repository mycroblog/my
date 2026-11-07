// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"cmp"
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// ModerationContract verifies the moderation repository of a database adapter by its methods
// alone, the port contract of ADR-0153 §1: the actions on accounts and on domains with their
// statuses and their lifts, each lift recorded once; the restriction rows of accounts and of
// domains, the distinct restrictions in effect in restriction order, and the deletes of an
// action's rows and of the expired ones in batches, the earliest first; the reports with their
// statuses, rule citations, and voice chat room, filed once per URI; the states of the reports,
// read for update, with their assignments, resolutions, counts, and the oldest one in a state;
// the lists of reports, of actions, and of domain actions newest first with their filters and
// keyset pages; the notes and reopenings of a report and the counts of a target's history; what
// the rebuild of the projections reads and deletes; and the reads in a transaction that see what
// it wrote. The moderation use cases on top of it are ModerationFlows, which
// internal/flowtest/moderation runs once on SQLite.
func ModerationContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	runModRepo(t, newHarness, []modRepoCase{
		{"RecordsActionsAndTheirLiftsOnce", modRepoActions},
		{"RecordsDomainActionsAndTheirLiftsOnce", modRepoDomainActions},
		{"KeepsTheRestrictionsOfAccounts", modRepoAccountRestrictions},
		{"KeepsTheRestrictionsOfDomains", modRepoDomainRestrictions},
		{"FilesAReportOncePerURI", modRepoReports},
		{"KeepsTheStatesOfReports", modRepoReportStates},
		{"ListsReportsWithFiltersAndPages", modRepoListsReports},
		{"ListsActionsWithFiltersAndPages", modRepoListsActions},
		{"KeepsNotesAndCountsTheHistory", modRepoNotes},
		{"ReadsAndDeletesForTheRebuild", modRepoRebuild},
		{"ReadsWhatItsTransactionWrote", modRepoTxReads},
	})
}

type modRepoCase struct {
	name string
	run  func(t *testing.T, r *modRepo)
}

// runModRepo runs the cases of a port contract of the moderation repository, each on a new
// database.
func runModRepo(t *testing.T, newHarness ModerationFactory, cases []modRepoCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &modRepo{h: h, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

// modRepo writes the rows of the moderation repository directly, as its use cases would.
type modRepo struct {
	h     ModerationHarness
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *modRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// txErr runs fn in a transaction and returns its error.
func (r *modRepo) txErr(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) error {
	t.Helper()
	return r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) })
}

// at returns the time d from now, for an end.
func (r *modRepo) at(d time.Duration) *time.Time {
	at := r.clock.Now().Add(d)
	return &at
}

// action inserts an action of kind on the account, changed by edit, a second before the next.
func (r *modRepo) action(t *testing.T, accountID string, kind moderation.ActionKind, edit func(*moderation.Action)) moderation.Action {
	t.Helper()
	a := moderation.Action{
		ID: r.ids.NewID(), TargetAccountID: accountID, Kind: kind, ReasonCategory: moderation.CategorySpam,
		Issuer: moderation.IssuerModerator, IssuedByUserID: testkit.NamedID("mod-1"), IssuedAt: r.clock.Now(),
	}
	if edit != nil {
		edit(&a)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertAction(ctx, tx, a) })
	r.clock.Advance(time.Second)
	return a
}

// lift lifts an action on an account.
func (r *modRepo) lift(t *testing.T, actionID string) moderation.Lift {
	t.Helper()
	l := moderation.Lift{ActionID: actionID, LiftedByUserID: testkit.NamedID("mod-2"), Reason: "appealed", LiftedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertActionLift(ctx, tx, l) })
	r.clock.Advance(time.Second)
	return l
}

// domainAction inserts an action of kind on the domain, changed by edit.
func (r *modRepo) domainAction(t *testing.T, domain string, kind moderation.DomainActionKind, edit func(*moderation.DomainAction)) moderation.DomainAction {
	t.Helper()
	a := moderation.DomainAction{ID: r.ids.NewID(), Domain: domain, Kind: kind, IssuedByUserID: testkit.NamedID("mod-1"), IssuedAt: r.clock.Now()}
	if edit != nil {
		edit(&a)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertDomainAction(ctx, tx, a) })
	r.clock.Advance(time.Second)
	return a
}

// liftDomain lifts an action on a domain.
func (r *modRepo) liftDomain(t *testing.T, actionID string) moderation.DomainLift {
	t.Helper()
	l := moderation.DomainLift{ActionID: actionID, LiftedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertDomainActionLift(ctx, tx, l) })
	r.clock.Advance(time.Second)
	return l
}

// report inserts a report of the target by the reporter, changed by edit, with its open state.
func (r *modRepo) report(t *testing.T, reporter, target string, edit func(*moderation.Report)) moderation.Report {
	t.Helper()
	rep := moderation.Report{
		ID: r.ids.NewID(), ReporterAccountID: reporter, TargetAccountID: target, Category: moderation.CategorySpam,
		Comment: "look", FiledAt: r.clock.Now(),
	}
	if edit != nil {
		edit(&rep)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		ok, err := r.h.Moderation.InsertReport(ctx, tx, rep)
		if err == nil && !ok {
			err = errors.New("InsertReport inserted nothing")
		}
		if err != nil {
			return err
		}
		return r.h.Moderation.UpsertReportStatus(ctx, tx, moderation.ReportStatus{ReportID: rep.ID, State: moderation.ReportOpen, Since: rep.FiledAt})
	})
	r.clock.Advance(time.Second)
	return rep
}

// assign records an assignment of a report and moves its state to it.
func (r *modRepo) assign(t *testing.T, reportID, userID string) {
	t.Helper()
	now := r.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Moderation.InsertReportAssignment(ctx, tx, moderation.ReportAssignment{
				ID: r.ids.NewID(), ReportID: reportID, AssigneeUserID: userID, AssignedByUserID: testkit.NamedID("mod-1"), AssignedAt: now,
			}),
			r.h.Moderation.UpsertReportStatus(ctx, tx, moderation.ReportStatus{ReportID: reportID, State: moderation.ReportOpen, AssigneeUserID: userID, Since: now}),
		)
	})
	r.clock.Advance(time.Second)
}

// resolve records a resolution of a report and moves its state to it.
func (r *modRepo) resolve(t *testing.T, reportID string, outcome moderation.ReportOutcome) moderation.ReportResolution {
	t.Helper()
	res := moderation.ReportResolution{ID: r.ids.NewID(), ReportID: reportID, Outcome: outcome, ResolvedByUserID: testkit.NamedID("mod-1"), ResolvedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Moderation.InsertReportResolution(ctx, tx, res),
			r.h.Moderation.UpsertReportStatus(ctx, tx, moderation.ReportStatus{ReportID: reportID, State: moderation.ReportResolved, Outcome: outcome, Since: res.ResolvedAt}),
		)
	})
	r.clock.Advance(time.Second)
	return res
}

func modRepoActions(t *testing.T, r *modRepo) {
	ctx := t.Context()
	account, report := r.ids.NewID(), r.ids.NewID()
	statuses := []string{r.ids.NewID(), r.ids.NewID()}
	a := r.action(t, account, moderation.ActionDeleteStatuses, func(a *moderation.Action) {
		a.PublicReason, a.PrivateNote, a.ReportID, a.NotifyTarget = "spam", "seen before", report, true
		a.ExpiresAt, a.StatusIDs = r.at(time.Hour), statuses
	})
	var got moderation.Action
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		got, err = r.h.Moderation.Action(ctx, tx, a.ID)
		return err
	})
	if !reflect.DeepEqual(got, a) {
		t.Errorf("Action = %+v, want %+v", got, a)
	}
	rec, err := r.h.Moderation.FindActionRecord(ctx, a.ID)
	if err != nil || rec.Lift != nil || rec.Action.ID != a.ID || rec.Action.Kind != a.Kind || rec.Action.PrivateNote != a.PrivateNote ||
		rec.Action.ReportID != report || !rec.Action.NotifyTarget || rec.Action.ExpiresAt == nil || !rec.Action.ExpiresAt.Equal(*a.ExpiresAt) {
		t.Errorf("FindActionRecord = %+v, %v; want the action without a lift", rec, err)
	}

	l := r.lift(t, a.ID)
	if rec, err := r.h.Moderation.FindActionRecord(ctx, a.ID); err != nil || rec.Lift == nil || *rec.Lift != l {
		t.Errorf("FindActionRecord after the lift = %+v, %v; want the lift %+v", rec, err, l)
	}
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertActionLift(ctx, tx, moderation.Lift{ActionID: a.ID, LiftedAt: r.clock.Now()})
	})
	if !errors.Is(err, appmoderation.ErrAlreadyRecorded) {
		t.Errorf("InsertActionLift again = %v, want ErrAlreadyRecorded", err)
	}

	unknown := r.ids.NewID()
	if _, err := r.h.Moderation.FindActionRecord(ctx, unknown); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindActionRecord(unknown) = %v, want ErrNotFound", err)
	}
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Moderation.Action(ctx, tx, unknown)
		return err
	})
	if !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("Action(unknown) = %v, want ErrNotFound", err)
	}
}

func modRepoDomainActions(t *testing.T, r *modRepo) {
	ctx := t.Context()
	a := r.domainAction(t, "bad.example", moderation.DomainSuspend, func(a *moderation.DomainAction) {
		a.PublicComment, a.PrivateNote, a.Obfuscate, a.ExpiresAt = "spam", "seen before", true, r.at(time.Hour)
	})
	var got moderation.DomainAction
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		got, err = r.h.Moderation.DomainAction(ctx, tx, a.ID)
		return err
	})
	if !reflect.DeepEqual(got, a) {
		t.Errorf("DomainAction = %+v, want %+v", got, a)
	}
	if rec, err := r.h.Moderation.FindDomainActionRecord(ctx, a.ID); err != nil || rec.Lift != nil || !reflect.DeepEqual(rec.Action, a) {
		t.Errorf("FindDomainActionRecord = %+v, %v; want the action without a lift", rec, err)
	}
	l := r.liftDomain(t, a.ID)
	if rec, err := r.h.Moderation.FindDomainActionRecord(ctx, a.ID); err != nil || rec.Lift == nil || *rec.Lift != l {
		t.Errorf("FindDomainActionRecord after the lift = %+v, %v; want the lift %+v", rec, err, l)
	}
	err := r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertDomainActionLift(ctx, tx, moderation.DomainLift{ActionID: a.ID, LiftedAt: r.clock.Now()})
	})
	if !errors.Is(err, appmoderation.ErrAlreadyRecorded) {
		t.Errorf("InsertDomainActionLift again = %v, want ErrAlreadyRecorded", err)
	}
	unknown := r.ids.NewID()
	if _, err := r.h.Moderation.FindDomainActionRecord(ctx, unknown); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindDomainActionRecord(unknown) = %v, want ErrNotFound", err)
	}
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Moderation.DomainAction(ctx, tx, unknown)
		return err
	})
	if !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("DomainAction(unknown) = %v, want ErrNotFound", err)
	}
}

// sortRows sorts restriction rows by target, restriction, and source, for comparison.
func sortRows(rows []moderation.RestrictionRow) []moderation.RestrictionRow {
	rows = slices.Clone(rows)
	slices.SortFunc(rows, func(a, b moderation.RestrictionRow) int {
		return cmp.Or(cmp.Compare(a.Target, b.Target), cmp.Compare(a.Restriction, b.Restriction), cmp.Compare(a.SourceActionID, b.SourceActionID))
	})
	return rows
}

func modRepoAccountRestrictions(t *testing.T, r *modRepo) {
	ctx := t.Context()
	account, other := r.ids.NewID(), r.ids.NewID()
	suspend, limit, expiring := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	soon, later := r.at(time.Minute), r.at(time.Hour)
	rows := []moderation.RestrictionRow{
		{Target: account, Restriction: moderation.RestrictPost, SourceActionID: suspend},
		{Target: account, Restriction: moderation.RestrictLogin, SourceActionID: suspend},
		{Target: account, Restriction: moderation.RestrictPost, SourceActionID: limit, EffectiveUntil: later},
		{Target: account, Restriction: moderation.RestrictPublicDiscovery, SourceActionID: expiring, EffectiveUntil: soon},
		{Target: other, Restriction: moderation.RestrictInteract, SourceActionID: r.ids.NewID(), EffectiveUntil: later},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Moderation.LockAccountRestrictions(ctx, tx, account); err != nil {
			return err
		}
		for _, row := range rows {
			if err := r.h.Moderation.InsertAccountRestriction(ctx, tx, row); err != nil {
				return err
			}
		}
		return nil
	})
	inEffect := func(name string, at time.Time, want ...moderation.Restriction) {
		t.Helper()
		got, err := r.h.Moderation.AccountRestrictionsInEffect(ctx, account, at)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: AccountRestrictionsInEffect = %v, %v; want %v", name, got, err, want)
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			got, err = r.h.Moderation.AccountRestrictionsInEffectTx(ctx, tx, account, at)
			return err
		})
		if !slices.Equal(got, want) {
			t.Errorf("%s: AccountRestrictionsInEffectTx = %v; want %v", name, got, want)
		}
	}
	now := r.clock.Now()
	inEffect("now", now, moderation.RestrictLogin, moderation.RestrictPost, moderation.RestrictPublicDiscovery)
	inEffect("at the first end", *soon, moderation.RestrictLogin, moderation.RestrictPost)
	if got, err := r.h.Moderation.AccountRestrictionsInEffect(ctx, r.ids.NewID(), now); err != nil || len(got) != 0 {
		t.Errorf("AccountRestrictionsInEffect(unknown) = %v, %v; want none", got, err)
	}

	var got []moderation.RestrictionRow
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		got, err = r.h.Moderation.AccountRestrictionRows(ctx, tx, account)
		return err
	})
	if !reflect.DeepEqual(sortRows(got), sortRows(rows[:4])) {
		t.Errorf("AccountRestrictionRows = %+v, want %+v", got, rows[:4])
	}

	// The expired rows go in batches, the earliest end first, and only when they ended.
	var deleted []moderation.RestrictionRow
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		deleted, err = r.h.Moderation.DeleteExpiredAccountRestrictions(ctx, tx, now, 10)
		return err
	})
	if len(deleted) != 0 {
		t.Errorf("DeleteExpiredAccountRestrictions before any end = %+v, want none", deleted)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		deleted, err = r.h.Moderation.DeleteExpiredAccountRestrictions(ctx, tx, *later, 1)
		return err
	})
	if !reflect.DeepEqual(deleted, []moderation.RestrictionRow{rows[3]}) {
		t.Errorf("DeleteExpiredAccountRestrictions(limit 1) = %+v, want the earliest %+v", deleted, rows[3])
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		deleted, err = r.h.Moderation.DeleteExpiredAccountRestrictions(ctx, tx, *later, 10)
		return err
	})
	if !reflect.DeepEqual(sortRows(deleted), sortRows([]moderation.RestrictionRow{rows[2], rows[4]})) {
		t.Errorf("DeleteExpiredAccountRestrictions = %+v, want the rows that ended at %v", deleted, *later)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.DeleteAccountRestrictionsOf(ctx, tx, suspend)
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		got, err = r.h.Moderation.AccountRestrictionRows(ctx, tx, account)
		return err
	})
	if len(got) != 0 {
		t.Errorf("AccountRestrictionRows after the deletes = %+v, want none", got)
	}
}

func modRepoDomainRestrictions(t *testing.T, r *modRepo) {
	ctx := t.Context()
	suspend, media := r.ids.NewID(), r.ids.NewID()
	soon := r.at(time.Minute)
	rows := []moderation.RestrictionRow{
		{Target: "bad.example", Restriction: moderation.RestrictFederation, SourceActionID: suspend},
		{Target: "bad.example", Restriction: moderation.RestrictProfileView, SourceActionID: suspend},
		{Target: "sub.bad.example", Restriction: moderation.RestrictMediaFetch, SourceActionID: media, EffectiveUntil: soon},
		{Target: "other.example", Restriction: moderation.RestrictReportIntake, SourceActionID: r.ids.NewID()},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Moderation.LockDomainRestrictions(ctx, tx, "bad.example"); err != nil {
			return err
		}
		for _, row := range rows {
			if err := r.h.Moderation.InsertDomainRestriction(ctx, tx, row); err != nil {
				return err
			}
		}
		return nil
	})
	now := r.clock.Now()
	inEffect := func(name string, domains []string, at time.Time, want ...moderation.Restriction) {
		t.Helper()
		got, err := r.h.Moderation.DomainRestrictionsInEffect(ctx, domains, at)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: DomainRestrictionsInEffect = %v, %v; want %v", name, got, err, want)
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			got, err = r.h.Moderation.DomainRestrictionsInEffectTx(ctx, tx, domains, at)
			return err
		})
		if !slices.Equal(got, want) {
			t.Errorf("%s: DomainRestrictionsInEffectTx = %v; want %v", name, got, want)
		}
	}
	inEffect("a subdomain and its parents", []string{"sub.bad.example", "bad.example", "example"}, now,
		moderation.RestrictFederation, moderation.RestrictMediaFetch, moderation.RestrictProfileView)
	inEffect("after the end", []string{"sub.bad.example"}, *soon)
	inEffect("one domain", []string{"other.example"}, now, moderation.RestrictReportIntake)
	inEffect("none", nil, now)

	var got []moderation.RestrictionRow
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		got, err = r.h.Moderation.DomainRestrictionRows(ctx, tx, "bad.example")
		return err
	})
	if !reflect.DeepEqual(sortRows(got), sortRows(rows[:2])) {
		t.Errorf("DomainRestrictionRows = %+v, want the rows of the domain only %+v", got, rows[:2])
	}
	var deleted []moderation.RestrictionRow
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		deleted, err = r.h.Moderation.DeleteExpiredDomainRestrictions(ctx, tx, *soon, 10)
		return err
	})
	if !reflect.DeepEqual(deleted, []moderation.RestrictionRow{rows[2]}) {
		t.Errorf("DeleteExpiredDomainRestrictions = %+v, want %+v", deleted, rows[2])
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.DeleteDomainRestrictionsOf(ctx, tx, suspend)
	})
	inEffect("after the delete", []string{"bad.example", "other.example"}, now, moderation.RestrictReportIntake)
}

func modRepoReports(t *testing.T, r *modRepo) {
	ctx := t.Context()
	reporter, target := r.ids.NewID(), r.ids.NewID()
	statuses := []string{r.ids.NewID(), r.ids.NewID()}
	rules := []string{r.ids.NewID()}
	remote := r.report(t, reporter, target, func(rep *moderation.Report) {
		rep.URI, rep.Forwarded, rep.StatusIDs = "https://remote.example/flags/1", true, statuses
	})
	local := r.report(t, reporter, target, func(rep *moderation.Report) {
		rep.Category, rep.RuleIDs = moderation.CategoryViolation, rules
		rep.VoiceRoom = &moderation.ReportVoiceRoom{RoomID: r.ids.NewID(), RevisionID: r.ids.NewID()}
	})
	// A second local report: reports without a URI do not collide.
	r.report(t, reporter, target, nil)

	if got, err := r.h.Moderation.FindReport(ctx, remote.ID); err != nil || !sameReport(got, remote) {
		t.Errorf("FindReport = %+v, %v; want %+v", got, err, remote)
	}
	if got, err := r.h.Moderation.FindReport(ctx, local.ID); err != nil || !sameReport(got, local) {
		t.Errorf("FindReport(local) = %+v, %v; want %+v", got, err, local)
	}
	if got, err := r.h.Moderation.FindReportByURI(ctx, remote.URI); err != nil || !sameReport(got, remote) {
		t.Errorf("FindReportByURI = %+v, %v; want %+v", got, err, remote)
	}
	again := remote
	again.ID = r.ids.NewID()
	var inserted bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		inserted, err = r.h.Moderation.InsertReport(ctx, tx, again)
		return err
	})
	if inserted {
		t.Error("InsertReport of a URI already filed = true, want false")
	}
	if _, err := r.h.Moderation.FindReport(ctx, again.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindReport(the second report of a URI) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Moderation.FindReportByURI(ctx, "https://remote.example/flags/2"); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindReportByURI(unknown) = %v, want ErrNotFound", err)
	}
	list, err := r.h.Moderation.ListReportsOf(ctx, target)
	if err != nil || len(list) != 3 || !sameReport(list[0], remote) || !sameReport(list[1], local) {
		t.Errorf("ListReportsOf = %+v, %v; want the three reports oldest first", list, err)
	}
	if list, err := r.h.Moderation.ListReportsOf(ctx, reporter); err != nil || len(list) != 0 {
		t.Errorf("ListReportsOf(the reporter) = %+v, %v; want none", list, err)
	}
}

func modRepoReportStates(t *testing.T, r *modRepo) {
	ctx := t.Context()
	reporter, target := r.ids.NewID(), r.ids.NewID()
	if _, err := r.h.Moderation.OldestReportFiledAt(ctx, moderation.ReportOpen); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("OldestReportFiledAt with no report = %v, want ErrNotFound", err)
	}
	first := r.report(t, reporter, target, nil)
	second := r.report(t, reporter, target, nil)
	r.report(t, reporter, r.ids.NewID(), nil)

	hasOther := func(reportID string) bool {
		t.Helper()
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ok, err = r.h.Moderation.HasOtherOpenReportOf(ctx, tx, target, reportID)
			return err
		})
		return ok
	}
	if !hasOther(first.ID) {
		t.Error("HasOtherOpenReportOf(first) = false, want true")
	}

	r.assign(t, first.ID, testkit.NamedID("mod-2"))
	st, err := r.h.Moderation.FindReportStatus(ctx, first.ID)
	if err != nil || st.State != moderation.ReportOpen || st.AssigneeUserID != testkit.NamedID("mod-2") || st.Outcome != "" {
		t.Errorf("FindReportStatus after the assignment = %+v, %v", st, err)
	}
	res := r.resolve(t, first.ID, moderation.OutcomeActioned)
	want := moderation.ReportStatus{ReportID: first.ID, State: moderation.ReportResolved, Outcome: moderation.OutcomeActioned, Since: res.ResolvedAt}
	if st, err := r.h.Moderation.FindReportStatus(ctx, first.ID); err != nil || st != want {
		t.Errorf("FindReportStatus after the resolution = %+v, %v; want %+v", st, err, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		st, err = r.h.Moderation.ReportStatusForUpdate(ctx, tx, first.ID)
		return err
	})
	if st != want {
		t.Errorf("ReportStatusForUpdate = %+v, want %+v", st, want)
	}
	if hasOther(second.ID) {
		t.Error("HasOtherOpenReportOf(second) with the first resolved = true, want false")
	}
	if !hasOther(first.ID) {
		t.Error("HasOtherOpenReportOf(first) with the second open = false, want true")
	}

	for state, n := range map[moderation.ReportState]int64{moderation.ReportOpen: 2, moderation.ReportResolved: 1} {
		if got, err := r.h.Moderation.CountReports(ctx, state); err != nil || got != n {
			t.Errorf("CountReports(%s) = %d, %v; want %d", state, got, err, n)
		}
	}
	if at, err := r.h.Moderation.OldestReportFiledAt(ctx, moderation.ReportOpen); err != nil || !at.Equal(second.FiledAt) {
		t.Errorf("OldestReportFiledAt(open) = %v, %v; want %v", at, err, second.FiledAt)
	}
	if at, err := r.h.Moderation.OldestReportFiledAt(ctx, moderation.ReportResolved); err != nil || !at.Equal(first.FiledAt) {
		t.Errorf("OldestReportFiledAt(resolved) = %v, %v; want %v", at, err, first.FiledAt)
	}

	unknown := r.ids.NewID()
	if _, err := r.h.Moderation.FindReportStatus(ctx, unknown); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindReportStatus(unknown) = %v, want ErrNotFound", err)
	}
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Moderation.ReportStatusForUpdate(ctx, tx, unknown)
		return err
	})
	if !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("ReportStatusForUpdate(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Moderation.FindReport(ctx, unknown); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindReport(unknown) = %v, want ErrNotFound", err)
	}
}

// sameReport reports whether two reports are equal, taking no statuses or rules as nil or empty.
func sameReport(a, b moderation.Report) bool {
	for _, r := range []*moderation.Report{&a, &b} {
		if len(r.StatusIDs) == 0 {
			r.StatusIDs = nil
		}
		if len(r.RuleIDs) == 0 {
			r.RuleIDs = nil
		}
	}
	return reflect.DeepEqual(a, b)
}

func reportIDs(recs []moderation.ReportRecord) []string {
	var out []string
	for _, rec := range recs {
		out = append(out, rec.Report.ID)
	}
	return out
}

func modRepoListsReports(t *testing.T, r *modRepo) {
	ctx := t.Context()
	alice, bob, target := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	spam := r.report(t, alice, target, nil)
	legal := r.report(t, bob, target, func(rep *moderation.Report) { rep.Category = moderation.CategoryLegal })
	assigned := r.report(t, alice, target, nil)
	resolved := r.report(t, alice, target, nil)
	r.assign(t, assigned.ID, testkit.NamedID("mod-2"))
	r.resolve(t, resolved.ID, moderation.OutcomeDismissed)

	for _, c := range []struct {
		name  string
		q     appmoderation.ReportQuery
		limit int
		want  []string
	}{
		{"every report", appmoderation.ReportQuery{}, 10, []string{resolved.ID, assigned.ID, legal.ID, spam.ID}},
		{"open", appmoderation.ReportQuery{State: moderation.ReportOpen}, 10, []string{assigned.ID, legal.ID, spam.ID}},
		{"resolved", appmoderation.ReportQuery{State: moderation.ReportResolved}, 10, []string{resolved.ID}},
		{"legal", appmoderation.ReportQuery{Category: moderation.CategoryLegal}, 10, []string{legal.ID}},
		{"assigned", appmoderation.ReportQuery{AssigneeUserID: testkit.NamedID("mod-2")}, 10, []string{assigned.ID}},
		{"unassigned and open", appmoderation.ReportQuery{State: moderation.ReportOpen, Unassigned: true}, 10, []string{legal.ID, spam.ID}},
		{"a page", appmoderation.ReportQuery{}, 2, []string{resolved.ID, assigned.ID}},
		{"the next page", appmoderation.ReportQuery{After: assigned.ID}, 2, []string{legal.ID, spam.ID}},
		{"oldest first", appmoderation.ReportQuery{OldestFirst: true}, 3, []string{spam.ID, legal.ID, assigned.ID}},
		{"oldest first, the next page", appmoderation.ReportQuery{OldestFirst: true, After: legal.ID}, 3, []string{assigned.ID, resolved.ID}},
	} {
		got, err := r.h.Moderation.ListReports(ctx, c.q, c.limit)
		if err != nil || !slices.Equal(reportIDs(got), c.want) {
			t.Errorf("ListReports(%s) = %v, %v; want %v", c.name, reportIDs(got), err, c.want)
		}
	}
	got, err := r.h.Moderation.ListReports(ctx, appmoderation.ReportQuery{AssigneeUserID: testkit.NamedID("mod-2")}, 1)
	if err != nil || len(got) != 1 || !sameReport(got[0].Report, assigned) || got[0].Status.AssigneeUserID != testkit.NamedID("mod-2") ||
		got[0].Status.State != moderation.ReportOpen {
		t.Errorf("ListReports(assigned) = %+v, %v; want the report with its state", got, err)
	}

	for _, c := range []struct {
		reporter, before string
		limit            int
		want             []string
	}{
		{alice, "", 10, []string{resolved.ID, assigned.ID, spam.ID}},
		{alice, "", 1, []string{resolved.ID}},
		{alice, resolved.ID, 10, []string{assigned.ID, spam.ID}},
		{bob, "", 10, []string{legal.ID}},
		{target, "", 10, nil},
	} {
		got, err := r.h.Moderation.ListReportsByReporter(ctx, c.reporter, c.before, c.limit)
		if err != nil || !slices.Equal(reportIDs(got), c.want) {
			t.Errorf("ListReportsByReporter(before %q, %d) = %v, %v; want %v", c.before, c.limit, reportIDs(got), err, c.want)
		}
	}
	if got, err := r.h.Moderation.ListReportsByReporter(ctx, alice, "", 1); err != nil || len(got) != 1 ||
		got[0].Status.State != moderation.ReportResolved || got[0].Status.Outcome != moderation.OutcomeDismissed {
		t.Errorf("ListReportsByReporter = %+v, %v; want the resolved report with its state", got, err)
	}
}

func modRepoListsActions(t *testing.T, r *modRepo) {
	ctx := t.Context()
	alice, bob := r.ids.NewID(), r.ids.NewID()
	warn := r.action(t, alice, moderation.ActionWarn, nil)
	lifted := r.action(t, alice, moderation.ActionSuspend, nil)
	expiring := r.action(t, bob, moderation.ActionLimit, func(a *moderation.Action) { a.ExpiresAt = r.at(time.Hour) })
	latest := r.action(t, alice, moderation.ActionLimit, nil)
	lift := r.lift(t, lifted.ID)

	ids := func(recs []moderation.ActionRecord) []string {
		var out []string
		for _, rec := range recs {
			out = append(out, rec.Action.ID)
		}
		return out
	}
	now := r.clock.Now()
	later := now.Add(time.Hour)
	for _, c := range []struct {
		name  string
		q     appmoderation.ActionQuery
		limit int
		want  []string
	}{
		{"every action", appmoderation.ActionQuery{}, 10, []string{latest.ID, expiring.ID, lifted.ID, warn.ID}},
		{"of an account", appmoderation.ActionQuery{AccountID: alice}, 10, []string{latest.ID, lifted.ID, warn.ID}},
		{"in effect", appmoderation.ActionQuery{InEffectAt: &now}, 10, []string{latest.ID, expiring.ID, warn.ID}},
		{"in effect after the end", appmoderation.ActionQuery{InEffectAt: &later}, 10, []string{latest.ID, warn.ID}},
		{"a page", appmoderation.ActionQuery{}, 2, []string{latest.ID, expiring.ID}},
		{"the next page", appmoderation.ActionQuery{Before: expiring.ID}, 2, []string{lifted.ID, warn.ID}},
	} {
		got, err := r.h.Moderation.ListActions(ctx, c.q, c.limit)
		if err != nil || !slices.Equal(ids(got), c.want) {
			t.Errorf("ListActions(%s) = %v, %v; want %v", c.name, ids(got), err, c.want)
		}
	}
	got, err := r.h.Moderation.ListActions(ctx, appmoderation.ActionQuery{AccountID: alice, Before: latest.ID}, 1)
	if err != nil || len(got) != 1 || got[0].Lift == nil || *got[0].Lift != lift || got[0].Action.Kind != moderation.ActionSuspend {
		t.Errorf("ListActions = %+v, %v; want the action with its lift", got, err)
	}

	domainSuspend := r.domainAction(t, "bad.example", moderation.DomainSuspend, nil)
	domainMedia := r.domainAction(t, "bad.example", moderation.DomainRejectMedia, func(a *moderation.DomainAction) { a.ExpiresAt = r.at(time.Hour) })
	domainLifted := r.domainAction(t, "other.example", moderation.DomainLimit, nil)
	domainLift := r.liftDomain(t, domainLifted.ID)
	domainIDs := func(recs []moderation.DomainActionRecord) []string {
		var out []string
		for _, rec := range recs {
			out = append(out, rec.Action.ID)
		}
		return out
	}
	now = r.clock.Now()
	later = now.Add(time.Hour)
	for _, c := range []struct {
		name  string
		q     appmoderation.DomainActionQuery
		limit int
		want  []string
	}{
		{"every action", appmoderation.DomainActionQuery{}, 10, []string{domainLifted.ID, domainMedia.ID, domainSuspend.ID}},
		{"of a domain", appmoderation.DomainActionQuery{Domain: "bad.example"}, 10, []string{domainMedia.ID, domainSuspend.ID}},
		{"of kinds", appmoderation.DomainActionQuery{Kinds: []moderation.DomainActionKind{moderation.DomainSuspend, moderation.DomainLimit}}, 10,
			[]string{domainLifted.ID, domainSuspend.ID}},
		{"in effect", appmoderation.DomainActionQuery{InEffectAt: &now}, 10, []string{domainMedia.ID, domainSuspend.ID}},
		{"in effect after the end", appmoderation.DomainActionQuery{InEffectAt: &later}, 10, []string{domainSuspend.ID}},
		{"a page", appmoderation.DomainActionQuery{}, 1, []string{domainLifted.ID}},
		{"the next page", appmoderation.DomainActionQuery{Before: domainLifted.ID}, 1, []string{domainMedia.ID}},
	} {
		got, err := r.h.Moderation.ListDomainActions(ctx, c.q, c.limit)
		if err != nil || !slices.Equal(domainIDs(got), c.want) {
			t.Errorf("ListDomainActions(%s) = %v, %v; want %v", c.name, domainIDs(got), err, c.want)
		}
	}
	gotDomain, err := r.h.Moderation.ListDomainActions(ctx, appmoderation.DomainActionQuery{Domain: "other.example"}, 10)
	if err != nil || len(gotDomain) != 1 || gotDomain[0].Lift == nil || *gotDomain[0].Lift != domainLift {
		t.Errorf("ListDomainActions(other.example) = %+v, %v; want the action with its lift", gotDomain, err)
	}
}

func modRepoNotes(t *testing.T, r *modRepo) {
	ctx := t.Context()
	reporter, target := r.ids.NewID(), r.ids.NewID()
	rep := r.report(t, reporter, target, nil)
	other := r.report(t, reporter, target, nil)
	r.report(t, reporter, r.ids.NewID(), nil)
	r.action(t, target, moderation.ActionWarn, nil)
	lifted := r.action(t, target, moderation.ActionSuspend, nil)
	r.lift(t, lifted.ID)
	r.action(t, reporter, moderation.ActionWarn, nil)

	var notes []moderation.ReportNote
	for _, body := range []string{"first", "second"} {
		n := moderation.ReportNote{ID: r.ids.NewID(), ReportID: rep.ID, AuthorUserID: testkit.NamedID("mod-1"), Body: body, WrittenAt: r.clock.Now()}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertReportNote(ctx, tx, n) })
		notes = append(notes, n)
		r.clock.Advance(time.Second)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertReportNote(ctx, tx, moderation.ReportNote{ID: r.ids.NewID(), ReportID: other.ID, Body: "elsewhere", WrittenAt: r.clock.Now()})
	})
	if got, err := r.h.Moderation.ListReportNotes(ctx, rep.ID); err != nil || !reflect.DeepEqual(got, notes) {
		t.Errorf("ListReportNotes = %+v, %v; want %+v oldest first", got, err, notes)
	}
	if got, err := r.h.Moderation.ListReportNotes(ctx, r.ids.NewID()); err != nil || len(got) != 0 {
		t.Errorf("ListReportNotes(unknown) = %+v, %v; want none", got, err)
	}

	res := r.resolve(t, rep.ID, moderation.OutcomeDismissed)
	reopening := moderation.ReportReopening{ID: r.ids.NewID(), ReportID: rep.ID, ReopenedByUserID: testkit.NamedID("mod-2"), ReopenedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertReportReopening(ctx, tx, reopening)
	})
	h, err := r.h.Moderation.ReportHistory(ctx, rep.ID)
	if err != nil || len(h.Assignments) != 0 || !reflect.DeepEqual(h.Resolutions, []moderation.ReportResolution{res}) ||
		!reflect.DeepEqual(h.Reopenings, []moderation.ReportReopening{reopening}) {
		t.Errorf("ReportHistory = %+v, %v; want the resolution and the reopening", h, err)
	}

	if n, err := r.h.Moderation.CountOtherReportsOf(ctx, target, rep.ID); err != nil || n != 1 {
		t.Errorf("CountOtherReportsOf = %d, %v; want 1", n, err)
	}
	if n, err := r.h.Moderation.CountActionsOf(ctx, target); err != nil || n != 2 {
		t.Errorf("CountActionsOf = %d, %v; want 2, the lifted one included", n, err)
	}
	if n, err := r.h.Moderation.CountActionsOf(ctx, r.ids.NewID()); err != nil || n != 0 {
		t.Errorf("CountActionsOf(unknown) = %d, %v; want 0", n, err)
	}
}

func modRepoRebuild(t *testing.T, r *modRepo) {
	account := r.ids.NewID()
	statuses := []string{r.ids.NewID(), r.ids.NewID()}
	sensitive := r.action(t, account, moderation.ActionMarkStatusesSensitive, func(a *moderation.Action) { a.StatusIDs = statuses })
	expired := r.action(t, account, moderation.ActionLimit, func(a *moderation.Action) { a.ExpiresAt = r.at(time.Second) })
	lifted := r.action(t, account, moderation.ActionMarkStatusesSensitive, func(a *moderation.Action) { a.StatusIDs = []string{r.ids.NewID()} })
	r.lift(t, lifted.ID)
	domain := r.domainAction(t, "bad.example", moderation.DomainSuspend, func(a *moderation.DomainAction) { a.ExpiresAt = r.at(time.Hour) })
	liftedDomain := r.domainAction(t, "other.example", moderation.DomainLimit, nil)
	r.liftDomain(t, liftedDomain.ID)
	accountRows := []moderation.RestrictionRow{{Target: account, Restriction: moderation.RestrictPublicDiscovery, SourceActionID: expired.ID, EffectiveUntil: expired.ExpiresAt}}
	domainRows := []moderation.RestrictionRow{{Target: "bad.example", Restriction: moderation.RestrictFederation, SourceActionID: domain.ID, EffectiveUntil: domain.ExpiresAt}}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(r.h.Moderation.InsertAccountRestriction(ctx, tx, accountRows[0]), r.h.Moderation.InsertDomainRestriction(ctx, tx, domainRows[0]))
	})

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		actions, err := r.h.Moderation.ListUnliftedActions(ctx, tx)
		if err != nil {
			return err
		}
		if len(actions) != 2 || actions[0].ID != sensitive.ID || actions[1].ID != expired.ID || actions[1].TargetAccountID != account ||
			actions[1].Kind != moderation.ActionLimit || actions[1].ExpiresAt == nil || !actions[1].ExpiresAt.Equal(*expired.ExpiresAt) {
			t.Errorf("ListUnliftedActions = %+v; want the two unlifted actions, the expired one included", actions)
		}
		domains, err := r.h.Moderation.ListUnliftedDomainActions(ctx, tx)
		if err != nil {
			return err
		}
		if len(domains) != 1 || domains[0].ID != domain.ID || domains[0].Domain != "bad.example" || domains[0].Kind != moderation.DomainSuspend ||
			domains[0].ExpiresAt == nil || !domains[0].ExpiresAt.Equal(*domain.ExpiresAt) {
			t.Errorf("ListUnliftedDomainActions = %+v; want the unlifted action", domains)
		}
		marked, err := r.h.Moderation.ListUnliftedActionStatuses(ctx, tx, moderation.ActionMarkStatusesSensitive)
		if err != nil {
			return err
		}
		want := []appmoderation.ActionStatus{{ActionID: sensitive.ID, StatusID: statuses[0]}, {ActionID: sensitive.ID, StatusID: statuses[1]}}
		if !reflect.DeepEqual(marked, want) {
			t.Errorf("ListUnliftedActionStatuses = %+v, want %+v", marked, want)
		}
		if other, err := r.h.Moderation.ListUnliftedActionStatuses(ctx, tx, moderation.ActionDeleteStatuses); err != nil || len(other) != 0 {
			t.Errorf("ListUnliftedActionStatuses(delete_statuses) = %+v, %v; want none", other, err)
		}
		rows, err := r.h.Moderation.ListAllAccountRestrictionRows(ctx, tx)
		if err != nil || !reflect.DeepEqual(rows, accountRows) {
			t.Errorf("ListAllAccountRestrictionRows = %+v, %v; want %+v", rows, err, accountRows)
		}
		rows, err = r.h.Moderation.ListAllDomainRestrictionRows(ctx, tx)
		if err != nil || !reflect.DeepEqual(rows, domainRows) {
			t.Errorf("ListAllDomainRestrictionRows = %+v, %v; want %+v", rows, err, domainRows)
		}
		return errors.Join(r.h.Moderation.DeleteAllAccountRestrictions(ctx, tx), r.h.Moderation.DeleteAllDomainRestrictions(ctx, tx))
	})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		accounts, err := r.h.Moderation.ListAllAccountRestrictionRows(ctx, tx)
		if err != nil {
			return err
		}
		domains, err := r.h.Moderation.ListAllDomainRestrictionRows(ctx, tx)
		if len(accounts)+len(domains) != 0 {
			t.Errorf("rows after DeleteAll = %+v, %+v; want none", accounts, domains)
		}
		return err
	})
}

func modRepoTxReads(t *testing.T, r *modRepo) {
	account := r.ids.NewID()
	rep := moderation.Report{ID: r.ids.NewID(), ReporterAccountID: r.ids.NewID(), TargetAccountID: account, Category: moderation.CategoryOther, FiledAt: r.clock.Now()}
	st := moderation.ReportStatus{ReportID: rep.ID, State: moderation.ReportOpen, Since: rep.FiledAt}
	a := moderation.Action{ID: r.ids.NewID(), TargetAccountID: account, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
		Issuer: moderation.IssuerModerator, IssuedAt: r.clock.Now()}
	da := moderation.DomainAction{ID: r.ids.NewID(), Domain: "bad.example", Kind: moderation.DomainLimit, IssuedAt: r.clock.Now()}
	appeal := moderation.Appeal{ID: r.ids.NewID(), ActionID: a.ID, AccountID: account, Text: "it was not spam", SubmittedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.h.Moderation.InsertReport(ctx, tx, rep); err != nil {
			return err
		}
		if err := errors.Join(
			r.h.Moderation.UpsertReportStatus(ctx, tx, st),
			r.h.Moderation.InsertAction(ctx, tx, a),
			r.h.Moderation.InsertDomainAction(ctx, tx, da),
			r.h.Moderation.InsertAppeal(ctx, tx, appeal),
			r.h.Moderation.InsertAccountRestriction(ctx, tx, moderation.RestrictionRow{Target: account, Restriction: moderation.RestrictLogin, SourceActionID: a.ID}),
			r.h.Moderation.InsertDomainRestriction(ctx, tx, moderation.RestrictionRow{Target: "bad.example", Restriction: moderation.RestrictPublicDiscovery, SourceActionID: da.ID}),
		); err != nil {
			return err
		}
		if got, err := r.h.Moderation.FindReportTx(ctx, tx, rep.ID); err != nil || got.ID != rep.ID || got.TargetAccountID != account {
			t.Errorf("FindReportTx = %+v, %v; want the report of the transaction", got, err)
		}
		if got, err := r.h.Moderation.FindReportStatusTx(ctx, tx, rep.ID); err != nil || got != st {
			t.Errorf("FindReportStatusTx = %+v, %v; want %+v", got, err, st)
		}
		if got, err := r.h.Moderation.FindActionRecordTx(ctx, tx, a.ID); err != nil || got.Action.ID != a.ID || got.Lift != nil {
			t.Errorf("FindActionRecordTx = %+v, %v; want the action", got, err)
		}
		if got, err := r.h.Moderation.FindDomainActionRecordTx(ctx, tx, da.ID); err != nil || !reflect.DeepEqual(got.Action, da) {
			t.Errorf("FindDomainActionRecordTx = %+v, %v; want %+v", got, err, da)
		}
		if got, err := r.h.Moderation.FindAppealTx(ctx, tx, appeal.ID); err != nil || got.Appeal != appeal || got.Decision != nil {
			t.Errorf("FindAppealTx = %+v, %v; want the pending appeal", got, err)
		}
		if got, err := r.h.Moderation.AccountRestrictionsInEffectTx(ctx, tx, account, r.clock.Now()); err != nil || !slices.Equal(got, []moderation.Restriction{moderation.RestrictLogin}) {
			t.Errorf("AccountRestrictionsInEffectTx = %v, %v", got, err)
		}
		if got, err := r.h.Moderation.DomainRestrictionsInEffectTx(ctx, tx, []string{"bad.example"}, r.clock.Now()); err != nil ||
			!slices.Equal(got, []moderation.Restriction{moderation.RestrictPublicDiscovery}) {
			t.Errorf("DomainRestrictionsInEffectTx = %v, %v", got, err)
		}
		return nil
	})
	unknown := r.ids.NewID()
	_ = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		for name, f := range map[string]func() error{
			"FindReportTx":             func() error { _, err := r.h.Moderation.FindReportTx(ctx, tx, unknown); return err },
			"FindReportStatusTx":       func() error { _, err := r.h.Moderation.FindReportStatusTx(ctx, tx, unknown); return err },
			"FindActionRecordTx":       func() error { _, err := r.h.Moderation.FindActionRecordTx(ctx, tx, unknown); return err },
			"FindDomainActionRecordTx": func() error { _, err := r.h.Moderation.FindDomainActionRecordTx(ctx, tx, unknown); return err },
			"FindAppealTx":             func() error { _, err := r.h.Moderation.FindAppealTx(ctx, tx, unknown); return err },
		} {
			if err := f(); !errors.Is(err, appmoderation.ErrNotFound) {
				t.Errorf("%s(unknown) = %v, want ErrNotFound", name, err)
			}
		}
		return nil
	})
}
