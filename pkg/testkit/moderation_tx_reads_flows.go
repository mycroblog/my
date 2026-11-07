// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"testing"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// moderationReadsInTransaction reads a report, an action on an account and its restrictions, and
// an action on a domain and its restrictions in the transactions that record them.
func moderationReadsInTransaction(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	reports := inTransactionOf(e.projectors, appmoderation.EventReportFiled, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		id := ev.(appmoderation.ReportFiled).Report.ID
		if rec, err := e.svc.ReportRecordTx(ctx, tx, id); err != nil || rec.Report.ID != id || rec.Status.State != moderation.ReportOpen {
			t.Errorf("ReportRecordTx = %+v, %v; want the open report", rec, err)
		}
	})
	if _, _, err := e.svc.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/tx", ReporterAccountID: e.reports, TargetAccountID: e.local,
	}); err != nil {
		t.Fatalf("FileRemoteReport = %v", err)
	}
	wantRan(t, "FileRemoteReport", reports)

	actions := inTransactionOf(e.projectors, appmoderation.EventAccountActionIssued, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		a := ev.(appmoderation.AccountActionIssued).Action
		if v, err := e.svc.AccountActionTx(ctx, tx, a.ID); err != nil || v.Action.ID != a.ID || v.State != moderation.ActionActive {
			t.Errorf("AccountActionTx = %+v, %v; want the active action", v, err)
		}
		if rs, err := e.accounts.OwnRestrictionsTx(ctx, tx, a.TargetAccountID); err != nil || len(rs) == 0 {
			t.Errorf("OwnRestrictionsTx = %v, %v; want the restrictions of the action", rs, err)
		}
	})
	a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction = %v", err)
	}
	wantRan(t, "IssueAccountAction", actions)
	if v, err := e.svc.AccountAction(ctx, a.ID); err != nil || v.State != moderation.ActionActive {
		t.Errorf("AccountAction after the commit = %+v, %v", v, err)
	}

	domains := inTransactionOf(e.projectors, appmoderation.EventDomainActionIssued, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		d := ev.(appmoderation.DomainActionIssued).Action
		if v, err := e.svc.DomainActionTx(ctx, tx, d.ID); err != nil || v.Action.ID != d.ID || v.State != moderation.ActionActive {
			t.Errorf("DomainActionTx = %+v, %v; want the active action", v, err)
		}
		if rejected, err := e.policy.RejectsMediaTx(ctx, tx, "cdn.media.example"); err != nil || !rejected {
			t.Errorf("RejectsMediaTx = %v, %v; want true", rejected, err)
		}
		if rejected, err := e.policy.RejectsMediaTx(ctx, tx, "other.example"); err != nil || rejected {
			t.Errorf("RejectsMediaTx(another domain) = %v, %v; want false", rejected, err)
		}
	})
	e.domainAction(t, "media.example", moderation.DomainRejectMedia, 0)
	wantRan(t, "IssueDomainAction", domains)
}

// appealReadsInTransaction reads an appeal with its action in the transaction that files it.
func appealReadsInTransaction(t *testing.T, e *appealEnv) {
	ctx := t.Context()
	suspend := e.issue(t, e.local, moderation.ActionSuspend)
	ran := inTransactionOf(e.projectors, appmoderation.EventAppealFiled, func(ctx context.Context, tx uow.Tx, ev uow.Event) {
		id := ev.(appmoderation.AppealFiled).Appeal.ID
		v, err := e.svc.AppealTx(ctx, tx, id)
		if err != nil || v.Appeal.ID != id || v.Action.Action.ID != suspend.ID || v.State() != moderation.AppealPending {
			t.Errorf("AppealTx = %+v, %v; want the pending appeal of the suspension", v, err)
		}
	})
	got := e.appeal(t, e.local, suspend)
	wantRan(t, "FileAppeal", ran)
	if v, err := e.svc.Appeal(ctx, got.Appeal.ID); err != nil || v.Action.Action.ID != suspend.ID {
		t.Errorf("Appeal after the commit = %+v, %v", v, err)
	}
}
