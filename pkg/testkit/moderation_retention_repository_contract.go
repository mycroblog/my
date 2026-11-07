// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// ModerationRetentionContract verifies the retention of the moderation records of a database
// adapter's moderation repository by its methods alone, the port contract of ADR-0153 §1: a hold
// is read back as inserted and released once, the holds of an account list newest first, and the
// accounts held are those with a hold in effect; a status is retained while a report or an action
// points at it or a hold preserves its author; the purge lists the actions that ended (at their
// issue for those without lasting effect, at the earlier of their lift and expiry for the others,
// and not before the decision on their appeal), the reports resolved that no action follows from,
// and the holds released before a time, oldest first, in batches, and skipping the accounts that
// a hold preserves, and deletes each with its children; the audit log entries older than a time
// are deleted in batches, except those on a held account and on its reports and appeals; and it
// tells which voice chat rooms a report points at. The use cases on top of it are
// ModerationRetentionFlows, which internal/flowtest/moderation runs once on SQLite.
func ModerationRetentionContract(t *testing.T, newHarness ModerationFactory) {
	t.Helper()
	runModRepo(t, newHarness, []modRepoCase{
		{"HoldsAreReleasedOnce", retentionRepoHolds},
		{"RetainsTheEvidenceOfStatuses", retentionRepoStatuses},
		{"PurgesTheActionsThatEnded", retentionRepoActions},
		{"PurgesTheResolvedReports", retentionRepoReports},
		{"PurgesTheReleasedHolds", retentionRepoReleasedHolds},
		{"DeletesTheOldAuditLog", retentionRepoAuditLog},
		{"TellsTheReportedVoiceRooms", retentionRepoVoiceRooms},
	})
}

// hold inserts a hold on the account.
func (r *modRepo) hold(t *testing.T, accountID string) moderation.RetentionHold {
	t.Helper()
	h := moderation.RetentionHold{ID: r.ids.NewID(), AccountID: accountID, Reason: "court order", CreatedByUserID: NamedID("mod-1"), CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.InsertRetentionHold(ctx, tx, h) })
	r.clock.Advance(time.Second)
	return h
}

// release releases a hold.
func (r *modRepo) release(t *testing.T, holdID string) moderation.RetentionHoldRelease {
	t.Helper()
	rel := moderation.RetentionHoldRelease{HoldID: holdID, ReleasedByUserID: NamedID("mod-2"), ReleasedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertRetentionHoldRelease(ctx, tx, rel)
	})
	r.clock.Advance(time.Second)
	return rel
}

func (r *modRepo) held(t *testing.T) []string {
	t.Helper()
	got, err := r.h.Moderation.HeldAccountIDs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var inTx []string
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		inTx, err = r.h.Moderation.HeldAccountIDsTx(ctx, tx)
		return err
	})
	if !slices.Equal(got, inTx) {
		t.Errorf("HeldAccountIDsTx = %v, HeldAccountIDs = %v; want the same", inTx, got)
	}
	return got
}

func retentionRepoHolds(t *testing.T, r *modRepo) {
	ctx := t.Context()
	alice, bob := r.ids.NewID(), r.ids.NewID()
	first, second, other := r.hold(t, alice), r.hold(t, alice), r.hold(t, bob)
	if got, want := r.held(t), sortedIDs(alice, bob); !slices.Equal(got, want) {
		t.Errorf("HeldAccountIDs = %v, want %v", got, want)
	}
	if got, err := r.h.Moderation.ListRetentionHolds(ctx, alice); err != nil ||
		!reflect.DeepEqual(got, []moderation.RetentionHoldRecord{{Hold: second}, {Hold: first}}) {
		t.Errorf("ListRetentionHolds = %+v, %v; want the account's two holds newest first", got, err)
	}
	if got, err := r.h.Moderation.ListRetentionHolds(ctx, ""); err != nil || len(got) != 3 || got[0].Hold != other {
		t.Errorf("ListRetentionHolds(every account) = %+v, %v; want the three holds newest first", got, err)
	}

	rel := r.release(t, first.ID)
	var rec moderation.RetentionHoldRecord
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		rec, err = r.h.Moderation.RetentionHold(ctx, tx, first.ID)
		return err
	})
	if rec.Hold != first || rec.Release == nil || *rec.Release != rel {
		t.Errorf("RetentionHold = %+v, want the hold with its release %+v", rec, rel)
	}
	err := r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertRetentionHoldRelease(ctx, tx, moderation.RetentionHoldRelease{HoldID: first.ID, ReleasedAt: r.clock.Now()})
	})
	if !errors.Is(err, appmoderation.ErrAlreadyRecorded) {
		t.Errorf("InsertRetentionHoldRelease again = %v, want ErrAlreadyRecorded", err)
	}
	if got, want := r.held(t), sortedIDs(alice, bob); !slices.Equal(got, want) {
		t.Errorf("HeldAccountIDs with one hold of alice released = %v, want %v", got, want)
	}
	r.release(t, second.ID)
	if got := r.held(t); !slices.Equal(got, []string{bob}) {
		t.Errorf("HeldAccountIDs with alice's holds released = %v, want bob only", got)
	}
	err = r.txErr(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Moderation.RetentionHold(ctx, tx, r.ids.NewID())
		return err
	})
	if !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("RetentionHold(unknown) = %v, want ErrNotFound", err)
	}
}

func retentionRepoStatuses(t *testing.T, r *modRepo) {
	author, held := r.ids.NewID(), r.ids.NewID()
	reported, acted, free, ofHeld := r.ids.NewID(), r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	r.report(t, r.ids.NewID(), author, func(rep *moderation.Report) { rep.StatusIDs = []string{reported} })
	r.action(t, author, moderation.ActionDeleteStatuses, func(a *moderation.Action) { a.StatusIDs = []string{acted} })
	r.hold(t, held)
	for _, c := range []struct {
		name, status, author string
		want                 bool
	}{
		{"reported", reported, author, true},
		{"acted on", acted, author, true},
		{"of a held author", ofHeld, held, true},
		{"free", free, author, false},
	} {
		var got bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			got, err = r.h.Moderation.StatusRetained(ctx, tx, c.status, c.author)
			return err
		})
		if got != c.want {
			t.Errorf("StatusRetained(%s) = %t, want %t", c.name, got, c.want)
		}
	}
}

func retentionRepoActions(t *testing.T, r *modRepo) {
	ctx := t.Context()
	account, held := r.ids.NewID(), r.ids.NewID()
	warn := r.action(t, account, moderation.ActionWarn, nil)
	lifted := r.action(t, account, moderation.ActionLimit, nil)
	r.lift(t, lifted.ID)
	expired := r.action(t, account, moderation.ActionSuspend, func(a *moderation.Action) { a.ExpiresAt = r.at(time.Second) })
	indefinite := r.action(t, account, moderation.ActionSuspend, nil)
	pending := r.action(t, account, moderation.ActionLimit, func(a *moderation.Action) { a.ExpiresAt = r.at(time.Second) })
	r.appeal(t, pending)
	decided := r.action(t, account, moderation.ActionLimit, func(a *moderation.Action) { a.ExpiresAt = r.at(time.Second) })
	decidedAppeal := r.appeal(t, decided)
	ofHeld := r.action(t, held, moderation.ActionWarn, nil)
	r.hold(t, held)
	r.clock.Advance(time.Minute)
	r.decide(t, decidedAppeal.ID, moderation.AppealDecisionUpheld)
	afterDecision := r.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertAccountRestriction(ctx, tx, moderation.RestrictionRow{
			Target: account, Restriction: moderation.RestrictLogin, SourceActionID: expired.ID, EffectiveUntil: expired.ExpiresAt,
		})
	})

	list := func(before time.Time, limit int) []string {
		t.Helper()
		var ids []string
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ids, err = r.h.Moderation.ListActionsToPurge(ctx, tx, before, limit)
			return err
		})
		return ids
	}
	if got := list(afterDecision.Add(-time.Second), 10); !slices.Equal(got, []string{warn.ID, lifted.ID, expired.ID}) {
		t.Errorf("ListActionsToPurge before the decision = %v, want warn, lifted, and expired", got)
	}
	if got := list(afterDecision, 2); !slices.Equal(got, []string{warn.ID, lifted.ID}) {
		t.Errorf("ListActionsToPurge(limit 2) = %v, want the two oldest", got)
	}
	ended := []string{warn.ID, lifted.ID, expired.ID, decided.ID}
	if got := list(afterDecision.Add(time.Second), 10); !slices.Equal(got, ended) {
		t.Errorf("ListActionsToPurge after the decision = %v, want %v", got, ended)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Moderation.PurgeActions(ctx, tx, ended) })
	for _, id := range ended {
		if _, err := r.h.Moderation.FindActionRecord(ctx, id); !errors.Is(err, appmoderation.ErrNotFound) {
			t.Errorf("FindActionRecord(purged) = %v, want ErrNotFound", err)
		}
	}
	if _, err := r.h.Moderation.FindAppeal(ctx, decidedAppeal.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindAppeal(of a purged action) = %v, want ErrNotFound", err)
	}
	if n, err := r.h.Moderation.CountAppeals(ctx, moderation.AppealUpheld); err != nil || n != 0 {
		t.Errorf("CountAppeals(upheld) after the purge = %d, %v; want 0", n, err)
	}
	var rows []moderation.RestrictionRow
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		rows, err = r.h.Moderation.AccountRestrictionRows(ctx, tx, account)
		return err
	})
	if len(rows) != 0 {
		t.Errorf("AccountRestrictionRows after the purge = %+v, want none", rows)
	}
	for _, a := range []moderation.Action{indefinite, pending, ofHeld} {
		if _, err := r.h.Moderation.FindActionRecord(ctx, a.ID); err != nil {
			t.Errorf("FindActionRecord(%s) = %v, want it kept", a.ID, err)
		}
	}
	if got := list(r.clock.Now().Add(time.Hour), 10); len(got) != 0 {
		t.Errorf("ListActionsToPurge after the purge = %v, want none", got)
	}
}

func retentionRepoReports(t *testing.T, r *modRepo) {
	ctx := t.Context()
	reporter, target, held := r.ids.NewID(), r.ids.NewID(), r.ids.NewID()
	resolved := r.report(t, reporter, target, func(rep *moderation.Report) {
		rep.StatusIDs, rep.RuleIDs = []string{r.ids.NewID()}, []string{r.ids.NewID()}
		rep.VoiceRoom = &moderation.ReportVoiceRoom{RoomID: r.ids.NewID(), RevisionID: r.ids.NewID()}
	})
	r.assign(t, resolved.ID, NamedID("mod-1"))
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.InsertReportNote(ctx, tx, moderation.ReportNote{ID: r.ids.NewID(), ReportID: resolved.ID, Body: "spam", WrittenAt: r.clock.Now()})
	})
	r.resolve(t, resolved.ID, moderation.OutcomeDismissed)
	open := r.report(t, reporter, target, nil)
	actioned := r.report(t, reporter, target, nil)
	r.action(t, target, moderation.ActionWarn, func(a *moderation.Action) { a.ReportID = actioned.ID })
	r.resolve(t, actioned.ID, moderation.OutcomeActioned)
	byHeld := r.report(t, held, target, nil)
	r.resolve(t, byHeld.ID, moderation.OutcomeDismissed)
	r.hold(t, held)
	second := r.report(t, reporter, target, nil)
	r.resolve(t, second.ID, moderation.OutcomeDismissed)
	resolvedLast := r.clock.Now()

	list := func(before time.Time, limit int) []string {
		t.Helper()
		var ids []string
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ids, err = r.h.Moderation.ListReportsToPurge(ctx, tx, before, limit)
			return err
		})
		return ids
	}
	if got := list(resolvedLast, 10); !slices.Equal(got, []string{resolved.ID, second.ID}) {
		t.Errorf("ListReportsToPurge = %v, want the two resolved reports that nothing keeps", got)
	}
	if got := list(resolvedLast, 1); !slices.Equal(got, []string{resolved.ID}) {
		t.Errorf("ListReportsToPurge(limit 1) = %v, want the oldest", got)
	}
	if got := list(resolved.FiledAt, 10); len(got) != 0 {
		t.Errorf("ListReportsToPurge before any resolution = %v, want none", got)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.PurgeReports(ctx, tx, []string{resolved.ID})
	})
	if _, err := r.h.Moderation.FindReport(ctx, resolved.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindReport(purged) = %v, want ErrNotFound", err)
	}
	if _, err := r.h.Moderation.FindReportStatus(ctx, resolved.ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("FindReportStatus(purged) = %v, want ErrNotFound", err)
	}
	if notes, err := r.h.Moderation.ListReportNotes(ctx, resolved.ID); err != nil || len(notes) != 0 {
		t.Errorf("ListReportNotes(purged) = %+v, %v; want none", notes, err)
	}
	if h, err := r.h.Moderation.ReportHistory(ctx, resolved.ID); err != nil || len(h.Assignments)+len(h.Resolutions)+len(h.Reopenings) != 0 {
		t.Errorf("ReportHistory(purged) = %+v, %v; want none", h, err)
	}
	if rooms, err := r.h.Moderation.ReportedVoiceRooms(ctx, []string{resolved.VoiceRoom.RoomID}); err != nil || rooms[resolved.VoiceRoom.RoomID] {
		t.Errorf("ReportedVoiceRooms(of a purged report) = %v, %v; want none", rooms, err)
	}
	for _, rep := range []moderation.Report{open, actioned, byHeld, second} {
		if _, err := r.h.Moderation.FindReport(ctx, rep.ID); err != nil {
			t.Errorf("FindReport(%s) = %v, want it kept", rep.ID, err)
		}
	}
}

func retentionRepoReleasedHolds(t *testing.T, r *modRepo) {
	ctx := t.Context()
	account := r.ids.NewID()
	first, second, active := r.hold(t, account), r.hold(t, account), r.hold(t, account)
	r.release(t, second.ID)
	r.release(t, first.ID)
	released := r.clock.Now()
	list := func(before time.Time, limit int) []string {
		t.Helper()
		var ids []string
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			ids, err = r.h.Moderation.ListRetentionHoldsToPurge(ctx, tx, before, limit)
			return err
		})
		return ids
	}
	if got := list(released, 10); !slices.Equal(got, []string{second.ID, first.ID}) {
		t.Errorf("ListRetentionHoldsToPurge = %v, want the released holds, the first released first", got)
	}
	if got := list(released, 1); !slices.Equal(got, []string{second.ID}) {
		t.Errorf("ListRetentionHoldsToPurge(limit 1) = %v, want the first released", got)
	}
	if got := list(released.Add(-2*time.Second), 10); len(got) != 0 {
		t.Errorf("ListRetentionHoldsToPurge before the releases = %v, want none", got)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.h.Moderation.PurgeRetentionHolds(ctx, tx, []string{first.ID, second.ID})
	})
	if got, err := r.h.Moderation.ListRetentionHolds(ctx, account); err != nil || !reflect.DeepEqual(got, []moderation.RetentionHoldRecord{{Hold: active}}) {
		t.Errorf("ListRetentionHolds after the purge = %+v, %v; want the hold in effect", got, err)
	}
}

func retentionRepoAuditLog(t *testing.T, r *modRepo) {
	held := r.ids.NewID()
	heldReport := r.report(t, r.ids.NewID(), held, nil)
	heldAction := r.action(t, held, moderation.ActionSuspend, nil)
	heldAppeal := r.appeal(t, heldAction)
	r.hold(t, held)
	onTarget := func(kind moderation.AuditTargetKind, id string) func(*moderation.AuditEntry) {
		return func(e *moderation.AuditEntry) {
			e.ActorKind, e.Action, e.TargetKind, e.TargetID = moderation.AuditActorCLI, moderation.AuditSettingChanged, kind, id
		}
	}
	var gone []moderation.AuditEntry
	for range 3 {
		gone = append(gone, r.auditEntry(t, nil))
	}
	kept := []moderation.AuditEntry{
		r.auditEntry(t, onTarget(moderation.AuditTargetAccount, held)),
		r.auditEntry(t, onTarget(moderation.AuditTargetReport, heldReport.ID)),
		r.auditEntry(t, onTarget(moderation.AuditTargetAppeal, heldAppeal.ID)),
	}
	cutoff := r.clock.Now()
	recent := r.auditEntry(t, nil)

	del := func(limit int) int64 {
		t.Helper()
		var n int64
		r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
			n, err = r.h.Moderation.DeleteAuditEntriesBefore(ctx, tx, cutoff, limit)
			return err
		})
		return n
	}
	if n := del(2); n != 2 {
		t.Errorf("DeleteAuditEntriesBefore(limit 2) = %d, want 2", n)
	}
	if got := r.auditLog(t, appmoderation.AuditQuery{}, 100); len(got) != 5 || got[4].ID != gone[2].ID {
		t.Errorf("audit log after a batch = %+v; want the oldest two deleted", got)
	}
	if n := del(10); n != 1 {
		t.Errorf("DeleteAuditEntriesBefore = %d, want the last old entry", n)
	}
	if n := del(10); n != 0 {
		t.Errorf("DeleteAuditEntriesBefore again = %d, want 0", n)
	}
	want := []moderation.AuditEntry{recent, kept[2], kept[1], kept[0]}
	if got := r.auditLog(t, appmoderation.AuditQuery{}, 100); !reflect.DeepEqual(got, want) {
		t.Errorf("audit log = %+v, want the recent entry and those on the held account, its report, and its appeal", got)
	}
}

func retentionRepoVoiceRooms(t *testing.T, r *modRepo) {
	room, other := r.ids.NewID(), r.ids.NewID()
	r.report(t, r.ids.NewID(), r.ids.NewID(), func(rep *moderation.Report) {
		rep.VoiceRoom = &moderation.ReportVoiceRoom{RoomID: room, RevisionID: r.ids.NewID()}
	})
	got, err := r.h.Moderation.ReportedVoiceRooms(t.Context(), []string{room, other})
	if err != nil || !got[room] || got[other] {
		t.Errorf("ReportedVoiceRooms = %v, %v; want the reported room only", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		got, err = r.h.Moderation.ReportedVoiceRoomsTx(ctx, tx, []string{other, room})
		return err
	})
	if !got[room] || got[other] {
		t.Errorf("ReportedVoiceRoomsTx = %v; want the reported room only", got)
	}
}
