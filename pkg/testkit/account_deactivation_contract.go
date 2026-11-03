// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
)

// otherAdmin creates another administrator, so that deactivating the session user leaves one.
func (e *accountEnv) otherAdmin(t *testing.T) {
	t.Helper()
	if _, err := e.identity.CreateAdmin(t.Context(), appidentity.CreateAdminInput{
		Username: "backup", Email: "backup@example.com", Password: sessionPassword,
	}); err != nil {
		t.Fatalf("CreateAdmin(backup) = %v", err)
	}
}

// deactivationConfirmed is the confirmation of the deactivation by the user of sessionUser.
var deactivationConfirmed = appidentity.Confirmation{Password: sessionPassword}

// wantRefused checks that err refuses a deactivation for reason, and returns when the account can
// be deactivated again.
func wantRefused(t *testing.T, err error, reason appaccount.DeactivationRefusal) *time.Time {
	t.Helper()
	var refused *appaccount.DeactivationRefusedError
	if !errors.As(err, &refused) || refused.Reason != reason {
		t.Fatalf("err = %v, want the refusal %s", err, reason)
	}
	return refused.AvailableAt
}

// lifecycleOf returns the current lifecycle state of an account.
func (e *accountEnv) lifecycleOf(t *testing.T, accountID string) account.LifecycleState {
	t.Helper()
	a, err := e.accounts.FindByID(t.Context(), accountID)
	if err != nil {
		t.Fatalf("FindByID = %v", err)
	}
	return a.Lifecycle
}

// deactivationConfirmsAndEndsSessions checks the deactivation through the use case (ADR-0145 §1):
// a wrong confirmation changes nothing, the password or a code of the second factor confirms, and
// every session and token ends in its transaction.
func deactivationConfirmsAndEndsSessions(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	appidentity.RegisterLifecycleProjectors(e.projectors, e.h.Identity, e.accounts, e.clock)
	reg := e.sessionUser(t)
	e.otherAdmin(t)
	signedIn := e.signIn(t, appidentity.SignInInput{})
	tokens, _ := e.webTokensFor(t, reg.User.ID)

	if _, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{Password: "wrong password"}); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Fatalf("DeactivateOwnAccount(wrong password) = %v, want ErrWrongPassword", err)
	}
	if _, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{TwoFactorCode: "000000"}); !errors.Is(err, appidentity.ErrTwoFactorNotEnabled) {
		t.Fatalf("DeactivateOwnAccount(a code without a second factor) = %v, want ErrTwoFactorNotEnabled", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Active() {
		t.Fatalf("state after the wrong confirmations = %+v, want active", s)
	}

	totp := e.enableTOTP(t, reg)
	if _, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{TwoFactorCode: "000000"}); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Fatalf("DeactivateOwnAccount(wrong code) = %v, want ErrInvalidSecondFactor", err)
	}
	due, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{TwoFactorCode: totp.codes[0]})
	if err != nil {
		t.Fatalf("DeactivateOwnAccount(a recovery code) = %v", err)
	}
	s := e.lifecycleOf(t, reg.AccountID)
	if !s.Deactivated() || !due.Equal(s.Since.Add(account.DeactivationPeriod)) {
		t.Fatalf("state = %+v, due = %v; want deactivated, deleted 30 days later", s, due)
	}
	e.wantInvalidSession(t, signedIn.Token, "the session of a deactivated account")
	e.wantAccess(t, tokens.AccessToken, false, "the token of a deactivated account")

	// Again: refused before the confirmation is spent.
	if at := wantRefused(t, errorOf(e.identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed)), appaccount.DeactivationRefusedAlready); at != nil {
		t.Errorf("AvailableAt = %v, want none", at)
	}

	// A new sign-in reaches the status page only; the old session and token stay ended after the
	// reactivation.
	if _, err := e.identity.PreviewDeactivation(ctx, reg.User.ID); err != nil {
		t.Fatalf("PreviewDeactivation = %v", err)
	}
	if err := e.identity.ReactivateOwnAccount(ctx, reg.User.ID); err != nil {
		t.Fatalf("ReactivateOwnAccount = %v", err)
	}
	e.wantInvalidSession(t, signedIn.Token, "the old session after the reactivation")
	e.wantAccess(t, tokens.AccessToken, false, "the old token after the reactivation")
}

// errorOf drops the value of a call that returns one with its error.
func errorOf[T any](_ T, err error) error { return err }

// deactivationRefusals checks each refusal of a deactivation (ADR-0145 §1, §8).
func deactivationRefusals(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	deactivate := func(s *appidentity.Service) error {
		return errorOf(s.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed))
	}

	// The only administrator who can sign in stays, or the deletion would leave the instance without one.
	wantRefused(t, deactivate(e.identity), appaccount.DeactivationRefusedLastAdministrator)
	e.otherAdmin(t)

	// Moderation keeps the account from logging in, and it is told in the preview too.
	restricted := e.newIdentityWith(func(d *appidentity.Deps) { d.Logins = loginPolicy{reg.AccountID: true} })
	wantRefused(t, deactivate(restricted), appaccount.DeactivationRefusedRestricted)
	switch p, err := restricted.PreviewDeactivation(ctx, reg.User.ID); {
	case err != nil:
		t.Fatalf("PreviewDeactivation = %v", err)
	case p.Refused == nil || p.Refused.Reason != appaccount.DeactivationRefusedRestricted:
		t.Errorf("PreviewDeactivation = %+v, want the refusal", p)
	case !p.DeletionDueAt.Equal(e.clock.Now().Add(account.DeactivationPeriod)):
		t.Errorf("PreviewDeactivation = %+v, want the date 30 days from now", p)
	}

	// A paid subscription goes on.
	subscribed := e.newIdentityWith(func(d *appidentity.Deps) { d.Subscriptions = liveSubscribers{reg.User.ID: true} })
	wantRefused(t, deactivate(subscribed), appaccount.DeactivationRefusedSubscription)

	// The account moved.
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.h.Accounts.UpsertMoveState(ctx, tx, account.MoveState{AccountID: reg.AccountID, MoveID: e.ids.NewID(), TargetAccountID: e.ids.NewID(), Since: e.clock.Now()})
	}); err != nil {
		t.Fatalf("UpsertMoveState = %v", err)
	}
	wantRefused(t, deactivate(e.identity), appaccount.DeactivationRefusedMoved)
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Accounts.DeleteMoveState(ctx, tx, reg.AccountID) }); err != nil {
		t.Fatalf("DeleteMoveState = %v", err)
	}

	// The registration waits for review.
	pending := e.registration(identity.RegistrationApproval, false)
	applicant, err := pending.SignUp(ctx, appidentity.SignUpInput{
		Username: "applicant", Email: "applicant@example.com", Password: sessionPassword, Reason: "hello",
	})
	if err != nil {
		t.Fatalf("SignUp = %v", err)
	}
	wantRefused(t, errorOf(pending.DeactivateOwnAccount(ctx, applicant.Session.Session.UserID, deactivationConfirmed)), appaccount.DeactivationRefusedPending)

	// A deletion is scheduled.
	gone := e.register(t, "gone")
	due := e.clock.Now().Add(30 * 24 * time.Hour)
	if _, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: gone.AccountID, Kind: account.LifecycleDeletionScheduled, InitiatedBy: account.InitiatedByModerator, DeletionDueAt: &due,
	}); err != nil {
		t.Fatal(err)
	}
	wantRefused(t, e.accounts.DeactivationRefused(ctx, gone.AccountID), appaccount.DeactivationRefusedDeletionScheduled)

	// The account is reactivated: 24 hours pass before the next deactivation, which counts anew.
	if err := deactivate(e.identity); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	e.clock.Advance(time.Hour)
	if err := e.identity.ReactivateOwnAccount(ctx, reg.User.ID); err != nil {
		t.Fatalf("ReactivateOwnAccount = %v", err)
	}
	back := e.clock.Now()
	if at := wantRefused(t, deactivate(e.identity), appaccount.DeactivationRefusedRecentlyReactivated); at == nil || !at.Equal(back.Add(account.DeactivationGap)) {
		t.Errorf("AvailableAt = %v, want 24 hours after %v", at, back)
	}
	e.clock.Advance(account.DeactivationGap - time.Millisecond)
	wantRefused(t, deactivate(e.identity), appaccount.DeactivationRefusedRecentlyReactivated)
	e.clock.Advance(time.Millisecond)
	again, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed)
	if err != nil {
		t.Fatalf("DeactivateOwnAccount after the gap = %v", err)
	}
	if !again.Equal(e.clock.Now().Add(account.DeactivationPeriod)) {
		t.Errorf("the date after the gap = %v, want 30 days from now", again)
	}
}

// liveSubscribers is an identity.LiveSubscriptions with a live subscription of the users in it.
type liveSubscribers map[string]bool

func (l liveSubscribers) HasLiveSubscription(_ context.Context, userID string) (bool, error) {
	return l[userID], nil
}

// deactivationDeletesAfterThePeriod checks the end of the period (ADR-0145 §2): the job deletes a
// deactivated account once its date passes, not before, once, and leaves one that came back; a
// reactivation is refused from the date on, also before the job records the deletion.
func deactivationDeletesAfterThePeriod(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	e.otherAdmin(t)
	back := e.register(t, "back")
	if _, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return errorOf(e.accounts.DeactivateTx(ctx, tx, back.AccountID, e.ids.NewID())) }); err != nil {
		t.Fatalf("DeactivateTx = %v", err)
	}
	e.clock.Advance(account.DeactivationPeriod - time.Millisecond)
	if n, err := e.accounts.ExecuteDueDeletions(ctx); err != nil || n != 0 {
		t.Fatalf("ExecuteDueDeletions before the date = %d, %v; want none", n, err)
	}
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.accounts.ReactivateTx(ctx, tx, back.AccountID) }); err != nil {
		t.Fatalf("ReactivateTx just before the date = %v", err)
	}
	e.clock.Advance(time.Millisecond)

	// The gap between the date and the job.
	if err := e.identity.ReactivateOwnAccount(ctx, reg.User.ID); !errors.Is(err, appaccount.ErrDeactivationExpired) {
		t.Fatalf("ReactivateOwnAccount at the date = %v, want ErrDeactivationExpired", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Deactivated() {
		t.Fatalf("state after the refused reactivation = %+v", s)
	}
	for _, want := range []int{1, 0} { // a second run finds nothing
		if n, err := e.accounts.ExecuteDueDeletions(ctx); err != nil || n != want {
			t.Fatalf("ExecuteDueDeletions = %d, %v; want %d", n, err, want)
		}
	}
	if s := e.lifecycleOf(t, reg.AccountID); s.State != account.StateDeleted {
		t.Fatalf("state after the job = %+v, want deleted", s)
	}
	if s := e.lifecycleOf(t, back.AccountID); !s.Active() {
		t.Fatalf("state of the account that came back = %+v, want active", s)
	}
	last := lastLifecycleEvent(t, e, reg.AccountID)
	if last.Kind != account.LifecycleDeleted || last.InitiatedBy != account.InitiatedBySystem {
		t.Errorf("last event = %+v, want deleted by the system", last)
	}
	if err := e.identity.ReactivateOwnAccount(ctx, reg.User.ID); !errors.Is(err, appaccount.ErrNotDeactivated) {
		t.Errorf("ReactivateOwnAccount of a deleted account = %v, want ErrNotDeactivated", err)
	}
}

// lastLifecycleEvent returns the newest lifecycle event of an account.
func lastLifecycleEvent(t *testing.T, e *accountEnv, accountID string) account.LifecycleEvent {
	t.Helper()
	var last account.LifecycleEvent
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		events, err := e.h.Accounts.ListLifecycleEvents(t.Context(), tx)
		for _, ev := range events {
			if ev.AccountID == accountID {
				last = ev
			}
		}
		return err
	})
	if err != nil {
		t.Fatalf("ListLifecycleEvents = %v", err)
	}
	return last
}

// deactivationYieldsToABan checks the precedence of a ban (ADR-0145 §2): it moves a deactivated
// account to its own date, which the reactivation and the period no longer touch; cancelling it
// makes the account active.
func deactivationYieldsToABan(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	e.otherAdmin(t)
	if _, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	due := e.clock.Now().Add(60 * 24 * time.Hour)
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.accounts.ScheduleDeletionByModeration(ctx, tx, reg.AccountID, e.ids.NewID(), due)
	}); err != nil {
		t.Fatalf("ScheduleDeletionByModeration = %v", err)
	}
	if err := e.identity.ReactivateOwnAccount(ctx, reg.User.ID); !errors.Is(err, appaccount.ErrNotDeactivated) {
		t.Fatalf("ReactivateOwnAccount of a banned account = %v, want ErrNotDeactivated", err)
	}
	e.clock.Advance(account.DeactivationPeriod + time.Hour)
	if n, err := e.accounts.ExecuteDueDeletions(ctx); err != nil || n != 0 {
		t.Fatalf("ExecuteDueDeletions past the period of the deactivation = %d, %v; want none", n, err)
	}
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return errorOf(e.accounts.CancelDeletionByModeration(ctx, tx, reg.AccountID, e.ids.NewID(), e.ids.NewID()))
	}); err != nil {
		t.Fatalf("CancelDeletionByModeration = %v", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Active() {
		t.Errorf("state after the cancelled ban = %+v, want active", s)
	}
}

// deactivationSendsOneEmail checks the email (ADR-0145 §3): one job of the deactivation, in the
// user's time zone with the date, none without email delivery, and none once the account is back.
func deactivationSendsOneEmail(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	m := &Mailer{}
	e.withMailer(m)
	reg := e.sessionUser(t)
	e.otherAdmin(t)
	if _, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	e.onlyJob(t, appidentity.JobDeactivationEmail)
	sent := m.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d emails, want 1", len(sent))
	}
	s := e.lifecycleOf(t, reg.AccountID)
	date := appidentity.DeletionDateText(s.DeactivationDueAt(), "")
	if !strings.EqualFold(sent[0].To, sessionEmail) || !strings.Contains(sent[0].Text, date) {
		t.Errorf("email to %s:\n%s\nwant the date %s", sent[0].To, sent[0].Text, date)
	}

	// The password can be reset while the account is deactivated.
	e.requestReset(t, sessionEmail)
	if !strings.Contains(m.Sent()[len(m.Sent())-1].Text, appidentity.PathResetPassword) {
		t.Errorf("no reset email for a deactivated account: %+v", m.Sent())
	}

	// A job that runs after the user came back tells nothing that is still true.
	if err := e.identity.ReactivateOwnAccount(ctx, reg.User.ID); err != nil {
		t.Fatalf("ReactivateOwnAccount = %v", err)
	}
	e.runMailJob(t, appidentity.JobDeactivationEmail, map[string]any{"user_id": reg.User.ID, "deactivated_at": s.Since})
	if n := len(m.Sent()); n != 2 {
		t.Errorf("sent %d emails after the late job, want the first and the reset", n)
	}

	// The period itself sends nothing more.
	e.clock.Advance(account.DeactivationGap)
	if _, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	before := len(m.Sent())
	e.clock.Advance(account.DeactivationPeriod)
	if _, err := e.accounts.ExecuteDueDeletions(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(m.Sent()); n != before {
		t.Errorf("sent %d more emails by the deletion, want none", n-before)
	}

	// Without email delivery there is no job.
	quiet := e.newIdentityWith(func(d *appidentity.Deps) { d.Mailer, d.EmailDelivery = m, false })
	other, err := quiet.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "quiet", Email: "quiet@example.com", Password: sessionPassword})
	if err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	jobs := len(e.mailJobs.Results())
	if _, err := quiet.DeactivateOwnAccount(ctx, other.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	if n := len(e.mailJobs.Results()); n != jobs {
		t.Errorf("ran %d jobs without email delivery, want none", n-jobs)
	}
}

// deactivationStatusPageDelete checks the deletion from the status page: the password confirms for
// a user who has one, and a fresh sign-in for one who has none.
func deactivationStatusPageDelete(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	e.otherAdmin(t)
	if _, err := e.identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	signedIn := e.signIn(t, appidentity.SignInInput{})
	if err := e.identity.DeleteDeactivatedAccount(ctx, signedIn.Session, "wrong password"); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Fatalf("DeleteDeactivatedAccount(wrong password) = %v, want ErrWrongPassword", err)
	}
	if err := e.identity.DeleteDeactivatedAccount(ctx, signedIn.Session, sessionPassword); err != nil {
		t.Fatalf("DeleteDeactivatedAccount = %v", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); s.State != account.StateDeleted {
		t.Errorf("state = %+v, want deleted", s)
	}

	// Without a password, the sign-in confirms while it is recent.
	other, err := e.identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "nopassword", Email: "nopassword@example.com", Password: sessionPassword})
	if err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	p, _ := e.registerPasskey(t, other.User.ID, "laptop")
	if err := e.identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: other.User.ID, Passkey: e.confirmWithPasskey(t, other.User.ID, p).Passkey}); err != nil {
		t.Fatalf("RemovePassword = %v", err)
	}
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return errorOf(e.accounts.DeactivateTx(ctx, tx, other.AccountID, e.ids.NewID()))
	}); err != nil {
		t.Fatalf("DeactivateTx = %v", err)
	}
	if has, err := e.identity.HasPassword(ctx, other.User.ID); err != nil || has {
		t.Fatalf("HasPassword = %v, %v; want none", has, err)
	}
	session := identity.Session{UserID: other.User.ID, CreatedAt: e.clock.Now()}
	e.clock.Advance(appidentity.DeleteConfirmationFreshness + time.Millisecond)
	if err := e.identity.DeleteDeactivatedAccount(ctx, session, ""); !errors.Is(err, appidentity.ErrSignInAgain) {
		t.Fatalf("DeleteDeactivatedAccount(old session) = %v, want ErrSignInAgain", err)
	}
	session.CreatedAt = e.clock.Now()
	if err := e.identity.DeleteDeactivatedAccount(ctx, session, ""); err != nil {
		t.Fatalf("DeleteDeactivatedAccount(new session) = %v", err)
	}
}
