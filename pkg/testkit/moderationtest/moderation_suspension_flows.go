// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"reflect"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// moderationSuspensions checks AccountSuspensionChanged (federation.md §11.1): an action that
// suspends an account that was not suspended, local or remote, tells it with the action, the lift or the expiry of its last
// suspension tells the end once, whichever removes the suspension first, and the expiry job
// removes the expired restrictions of every kind.
func moderationSuspensions(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	issue := func(kind moderation.ActionKind, d time.Duration) moderation.Action {
		t.Helper()
		a, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
			TargetAccountID: e.local, Kind: kind, ReasonCategory: moderation.CategoryOther, Duration: d,
		})
		if err != nil {
			t.Fatalf("IssueAccountAction(%s) = %v", kind, err)
		}
		return a
	}
	lift := func(a moderation.Action) {
		t.Helper()
		if err := e.svc.LiftAccountAction(ctx, a.ID, "", ""); err != nil {
			t.Fatalf("LiftAccountAction(%s) = %v", a.Kind, err)
		}
	}
	expire := func(want int) {
		t.Helper()
		n, err := e.svc.ExpireAccountRestrictions(ctx)
		if err != nil || n != want {
			t.Fatalf("ExpireAccountRestrictions = %d, %v; want %d removed", n, err, want)
		}
	}
	told := func(what string, want ...appmoderation.AccountSuspensionChanged) {
		t.Helper()
		got := e.suspensions.Take()
		if len(got) == 0 && len(want) == 0 {
			return
		}
		ws := make([]any, len(want))
		for i, w := range want {
			ws[i] = w
		}
		gs := make([]any, len(got))
		for i, g := range got {
			gs[i] = g
		}
		if !reflect.DeepEqual(gs, ws) {
			t.Errorf("%s: suspension events = %+v, want %+v", what, gs, ws)
		}
	}
	// suspendedBy is the event of a suspension by a; ended, the event of the end of the last one.
	suspendedBy := func(a moderation.Action) appmoderation.AccountSuspensionChanged {
		return appmoderation.AccountSuspensionChanged{AccountID: a.TargetAccountID, Suspended: true, ActionID: a.ID, ChangedAt: e.clock.Now()}
	}
	ended := func(accountID string) appmoderation.AccountSuspensionChanged {
		return appmoderation.AccountSuspensionChanged{AccountID: accountID, Suspended: false, ChangedAt: e.clock.Now()}
	}
	suspendRows := len(moderation.Action{Kind: moderation.ActionSuspend, ExpiresAt: &time.Time{}}.Rows())

	// The suspension of a remote account is told like that of a local one, with its action.
	remote, err := e.svc.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: e.remote, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction(remote suspend) = %v", err)
	}
	told("remote suspension", suspendedBy(remote))
	if err := e.svc.LiftAccountAction(ctx, remote.ID, "", ""); err != nil {
		t.Fatalf("LiftAccountAction(remote suspend) = %v", err)
	}
	told("lift of the remote suspension", ended(e.remote))

	// The first suspension is told; a second one, and the lift of one of the two, are not.
	first := issue(moderation.ActionSuspend, time.Hour)
	told("first suspension", suspendedBy(first))
	forever := issue(moderation.ActionSuspend, 0)
	told("second suspension")
	lift(forever)
	told("lift of one of two")
	// Another kind of action tells nothing.
	issue(moderation.ActionDisableLogin, time.Hour)
	told("disable_login")
	expire(0)

	// The expiry of the last suspension is told once, by the first run after it; the run removes
	// the expired rows of every kind.
	e.clock.Advance(2 * time.Hour)
	expire(suspendRows + 1)
	told("expiry", ended(e.local))
	expire(0)
	told("expiry again")
	var rows []moderation.RestrictionRow
	err = e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		rows, err = e.h.Moderation.AccountRestrictionRows(ctx, tx, e.local)
		return err
	})
	if err != nil || len(rows) != 0 {
		t.Errorf("rows after the expiry = %+v, %v; want none", rows, err)
	}

	// A lift after the expiry and before the run tells the end, and the run then has nothing
	// to tell.
	a := issue(moderation.ActionSuspend, time.Hour)
	told("suspension", suspendedBy(a))
	e.clock.Advance(2 * time.Hour)
	lift(a)
	told("lift of an expired suspension", ended(e.local))
	expire(0)
	told("expiry after the lift")

	// A suspension over an expired one that the run has not removed is told, and removing the
	// expired one tells nothing.
	a = issue(moderation.ActionSuspend, time.Hour)
	told("suspension", suspendedBy(a))
	e.clock.Advance(2 * time.Hour)
	a = issue(moderation.ActionSuspend, time.Hour)
	told("suspension over an expired one", suspendedBy(a))
	expire(suspendRows)
	told("removal of the expired suspension under another")
	if ok, err := e.policy.AccountFederationSuspended(ctx, e.local); err != nil || !ok {
		t.Errorf("AccountFederationSuspended after the removal = %v, %v; want true", ok, err)
	}
}

// moderationExpiresDomainRestrictions checks that the expiry job removes the expired rows of a domain
// and leaves those still in effect, of the same domain and of another.
func moderationExpiresDomainRestrictions(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	issue := func(domain string, d time.Duration) {
		t.Helper()
		if _, err := e.svc.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: domain, Kind: moderation.DomainLimit, Duration: d}); err != nil {
			t.Fatalf("IssueDomainAction(%s) = %v", domain, err)
		}
	}
	rowsOf := func(domain string) int {
		t.Helper()
		var rows []moderation.RestrictionRow
		if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			rows, err = e.h.Moderation.DomainRestrictionRows(ctx, tx, domain)
			return err
		}); err != nil {
			t.Fatalf("DomainRestrictionRows = %v", err)
		}
		return len(rows)
	}
	issue("short.example", time.Hour)
	issue("mixed.example", time.Hour)
	issue("mixed.example", 3*time.Hour)
	issue("forever.example", 0)
	limitRows := rowsOf("short.example")

	if n, err := e.svc.ExpireDomainRestrictions(ctx); err != nil || n != 0 {
		t.Errorf("ExpireDomainRestrictions before the end = %d, %v; want none", n, err)
	}
	e.clock.Advance(2 * time.Hour)
	if n, err := e.svc.ExpireDomainRestrictions(ctx); err != nil || n != 2*limitRows {
		t.Errorf("ExpireDomainRestrictions = %d, %v; want %d", n, err, 2*limitRows)
	}
	if n := rowsOf("short.example"); n != 0 {
		t.Errorf("rows of the expired domain = %d, want none", n)
	}
	if n := rowsOf("mixed.example"); n != limitRows {
		t.Errorf("rows of the domain with a longer action = %d, want %d", n, limitRows)
	}
	if n := rowsOf("forever.example"); n != limitRows {
		t.Errorf("rows of the domain without an end = %d, want %d", n, limitRows)
	}
	if n, err := e.svc.ExpireDomainRestrictions(ctx); err != nil || n != 0 {
		t.Errorf("ExpireDomainRestrictions again = %d, %v; want none", n, err)
	}
}
