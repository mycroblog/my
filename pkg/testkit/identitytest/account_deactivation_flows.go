// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

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
	"github.com/mycroblog/my/pkg/testkit"
)

// otherAdmin creates another administrator, so that deactivating the session user leaves one.
func (e *AccountEnv) otherAdmin(t *testing.T) {
	t.Helper()
	if _, err := e.Identity.CreateAdmin(t.Context(), appidentity.CreateAdminInput{
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
func (e *AccountEnv) lifecycleOf(t *testing.T, accountID string) account.LifecycleState {
	t.Helper()
	a, err := e.Accounts.FindByID(t.Context(), accountID)
	if err != nil {
		t.Fatalf("FindByID = %v", err)
	}
	return a.Lifecycle
}

// deactivationConfirmsAndEndsSessions checks the deactivation through the use case (ADR-0089 §11):
// a wrong confirmation changes nothing, the password or a code of the second factor confirms, and
// every session and token ends in its transaction.
func deactivationConfirmsAndEndsSessions(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	appidentity.RegisterLifecycleProjectors(e.Projectors, e.H.Identity, e.Accounts, e.Clock)
	reg := e.SessionUser(t)
	e.otherAdmin(t)
	signedIn := e.SignIn(t, appidentity.SignInInput{})
	tokens, _ := e.WebTokensFor(t, reg.User.ID)

	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{Password: "wrong password"}); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Fatalf("DeactivateOwnAccount(wrong password) = %v, want ErrWrongPassword", err)
	}
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{TwoFactorCode: "000000"}); !errors.Is(err, appidentity.ErrTwoFactorNotEnabled) {
		t.Fatalf("DeactivateOwnAccount(a code without a second factor) = %v, want ErrTwoFactorNotEnabled", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Active() {
		t.Fatalf("state after the wrong confirmations = %+v, want active", s)
	}

	totp := e.EnableTOTP(t, reg)
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{TwoFactorCode: "000000"}); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Fatalf("DeactivateOwnAccount(wrong code) = %v, want ErrInvalidSecondFactor", err)
	}
	due, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{TwoFactorCode: totp.codes[0]})
	if err != nil {
		t.Fatalf("DeactivateOwnAccount(a recovery code) = %v", err)
	}
	s := e.lifecycleOf(t, reg.AccountID)
	if !s.Deactivated() || !due.Equal(s.Since.Add(account.DeactivationPeriod)) {
		t.Fatalf("state = %+v, due = %v; want deactivated, deleted 30 days later", s, due)
	}
	e.wantInvalidSession(t, signedIn.Token, "the session of a deactivated account")
	e.WantAccess(t, tokens.AccessToken, false, "the token of a deactivated account")

	// Again: refused before the confirmation is spent.
	if at := wantRefused(t, errorOf(e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed)), appaccount.DeactivationRefusedAlready); at != nil {
		t.Errorf("AvailableAt = %v, want none", at)
	}

	// A new sign-in reaches the status page only; the old session and token stay ended after the
	// reactivation.
	if _, err := e.Identity.PreviewDeactivation(ctx, reg.User.ID); err != nil {
		t.Fatalf("PreviewDeactivation = %v", err)
	}
	if err := e.Identity.ReactivateOwnAccount(ctx, reg.User.ID); err != nil {
		t.Fatalf("ReactivateOwnAccount = %v", err)
	}
	e.wantInvalidSession(t, signedIn.Token, "the old session after the reactivation")
	e.WantAccess(t, tokens.AccessToken, false, "the old token after the reactivation")
}

// errorOf drops the value of a call that returns one with its error.
func errorOf[T any](_ T, err error) error { return err }

// deactivationRefusals checks each refusal of a deactivation (ADR-0089 §11, §18).
func deactivationRefusals(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.SessionUser(t)
	deactivate := func(s *appidentity.Service) error {
		return errorOf(s.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed))
	}

	// The only administrator who can sign in stays, or the deletion would leave the instance without one.
	wantRefused(t, deactivate(e.Identity), appaccount.DeactivationRefusedLastAdministrator)
	e.otherAdmin(t)

	// Moderation keeps the account from logging in, and it is told in the preview too.
	restricted := e.newIdentityWith(func(d *appidentity.Deps) { d.Logins = loginPolicy{reg.AccountID: true} })
	wantRefused(t, deactivate(restricted), appaccount.DeactivationRefusedRestricted)
	switch p, err := restricted.PreviewDeactivation(ctx, reg.User.ID); {
	case err != nil:
		t.Fatalf("PreviewDeactivation = %v", err)
	case p.Refused == nil || p.Refused.Reason != appaccount.DeactivationRefusedRestricted:
		t.Errorf("PreviewDeactivation = %+v, want the refusal", p)
	case !p.DeletionDueAt.Equal(e.Clock.Now().Add(account.DeactivationPeriod)):
		t.Errorf("PreviewDeactivation = %+v, want the date 30 days from now", p)
	}

	// A paid subscription goes on.
	subscribed := e.newIdentityWith(func(d *appidentity.Deps) { d.Subscriptions = liveSubscribers{reg.User.ID: true} })
	wantRefused(t, deactivate(subscribed), appaccount.DeactivationRefusedSubscription)

	// The account moved.
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.H.Accounts.UpsertMoveState(ctx, tx, account.MoveState{AccountID: reg.AccountID, MoveID: e.IDs.NewID(), TargetAccountID: e.IDs.NewID(), Since: e.Clock.Now()})
	}); err != nil {
		t.Fatalf("UpsertMoveState = %v", err)
	}
	wantRefused(t, deactivate(e.Identity), appaccount.DeactivationRefusedMoved)
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.H.Accounts.DeleteMoveState(ctx, tx, reg.AccountID) }); err != nil {
		t.Fatalf("DeleteMoveState = %v", err)
	}

	// The registration waits for review.
	pending := e.Registration(identity.RegistrationApproval, false)
	applicant, err := pending.SignUp(ctx, appidentity.SignUpInput{
		Username: "applicant", Email: "applicant@example.com", Password: sessionPassword, Reason: "hello",
	})
	if err != nil {
		t.Fatalf("SignUp = %v", err)
	}
	wantRefused(t, errorOf(pending.DeactivateOwnAccount(ctx, applicant.Session.Session.UserID, deactivationConfirmed)), appaccount.DeactivationRefusedPending)

	// A deletion is scheduled.
	gone := e.Register(t, "gone")
	due := e.Clock.Now().Add(30 * 24 * time.Hour)
	if _, err := e.Accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: gone.AccountID, Kind: account.LifecycleDeletionScheduled, InitiatedBy: account.InitiatedByModerator, DeletionDueAt: &due,
	}); err != nil {
		t.Fatal(err)
	}
	wantRefused(t, e.Accounts.DeactivationRefused(ctx, gone.AccountID), appaccount.DeactivationRefusedDeletionScheduled)

	// The account is reactivated: 24 hours pass before the next deactivation, which counts anew.
	if err := deactivate(e.Identity); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	e.Clock.Advance(time.Hour)
	if err := e.Identity.ReactivateOwnAccount(ctx, reg.User.ID); err != nil {
		t.Fatalf("ReactivateOwnAccount = %v", err)
	}
	back := e.Clock.Now()
	if at := wantRefused(t, deactivate(e.Identity), appaccount.DeactivationRefusedRecentlyReactivated); at == nil || !at.Equal(back.Add(account.DeactivationGap)) {
		t.Errorf("AvailableAt = %v, want 24 hours after %v", at, back)
	}
	e.Clock.Advance(account.DeactivationGap - time.Millisecond)
	wantRefused(t, deactivate(e.Identity), appaccount.DeactivationRefusedRecentlyReactivated)
	e.Clock.Advance(time.Millisecond)
	again, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed)
	if err != nil {
		t.Fatalf("DeactivateOwnAccount after the gap = %v", err)
	}
	if !again.Equal(e.Clock.Now().Add(account.DeactivationPeriod)) {
		t.Errorf("the date after the gap = %v, want 30 days from now", again)
	}
}

// liveSubscribers is an identity.LiveSubscriptions with a live subscription of the users in it.
type liveSubscribers map[string]bool

func (l liveSubscribers) HasLiveSubscription(_ context.Context, userID string) (bool, error) {
	return l[userID], nil
}

// deactivationDeletesAfterThePeriod checks the end of the period (ADR-0089 §12): the job deletes a
// deactivated account once its date passes, not before, once, and leaves one that came back; a
// reactivation is refused from the date on, also before the job records the deletion.
func deactivationDeletesAfterThePeriod(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.SessionUser(t)
	e.otherAdmin(t)
	back := e.Register(t, "back")
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return errorOf(e.Accounts.DeactivateTx(ctx, tx, back.AccountID, e.IDs.NewID())) }); err != nil {
		t.Fatalf("DeactivateTx = %v", err)
	}
	e.Clock.Advance(account.DeactivationPeriod - time.Millisecond)
	if n, err := e.Accounts.ExecuteDueDeletions(ctx); err != nil || n != 0 {
		t.Fatalf("ExecuteDueDeletions before the date = %d, %v; want none", n, err)
	}
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.Accounts.ReactivateTx(ctx, tx, back.AccountID) }); err != nil {
		t.Fatalf("ReactivateTx just before the date = %v", err)
	}
	e.Clock.Advance(time.Millisecond)

	// The gap between the date and the job.
	if err := e.Identity.ReactivateOwnAccount(ctx, reg.User.ID); !errors.Is(err, appaccount.ErrDeactivationExpired) {
		t.Fatalf("ReactivateOwnAccount at the date = %v, want ErrDeactivationExpired", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Deactivated() {
		t.Fatalf("state after the refused reactivation = %+v", s)
	}
	for _, want := range []int{1, 0} { // a second run finds nothing
		if n, err := e.Accounts.ExecuteDueDeletions(ctx); err != nil || n != want {
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
	if err := e.Identity.ReactivateOwnAccount(ctx, reg.User.ID); !errors.Is(err, appaccount.ErrNotDeactivated) {
		t.Errorf("ReactivateOwnAccount of a deleted account = %v, want ErrNotDeactivated", err)
	}
}

// lastLifecycleEvent returns the newest lifecycle event of an account.
func lastLifecycleEvent(t *testing.T, e *AccountEnv, accountID string) account.LifecycleEvent {
	t.Helper()
	var last account.LifecycleEvent
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		events, err := e.H.Accounts.ListLifecycleEvents(t.Context(), tx)
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

// deactivationYieldsToABan checks the precedence of a ban (ADR-0089 §12): it moves a deactivated
// account to its own date, which the reactivation and the period no longer touch; cancelling it
// makes the account active.
func deactivationYieldsToABan(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.SessionUser(t)
	e.otherAdmin(t)
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	due := e.Clock.Now().Add(60 * 24 * time.Hour)
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return e.Accounts.ScheduleDeletionByModeration(ctx, tx, reg.AccountID, e.IDs.NewID(), due)
	}); err != nil {
		t.Fatalf("ScheduleDeletionByModeration = %v", err)
	}
	if err := e.Identity.ReactivateOwnAccount(ctx, reg.User.ID); !errors.Is(err, appaccount.ErrNotDeactivated) {
		t.Fatalf("ReactivateOwnAccount of a banned account = %v, want ErrNotDeactivated", err)
	}
	e.Clock.Advance(account.DeactivationPeriod + time.Hour)
	if n, err := e.Accounts.ExecuteDueDeletions(ctx); err != nil || n != 0 {
		t.Fatalf("ExecuteDueDeletions past the period of the deactivation = %d, %v; want none", n, err)
	}
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return errorOf(e.Accounts.CancelDeletionByModeration(ctx, tx, reg.AccountID, e.IDs.NewID(), e.IDs.NewID()))
	}); err != nil {
		t.Fatalf("CancelDeletionByModeration = %v", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Active() {
		t.Errorf("state after the canceled ban = %+v, want active", s)
	}
}

// deactivationSendsOneEmail checks the email (ADR-0089 §13): one job of the deactivation, in the
// user's time zone with the date, none without email delivery, and none once the account is back.
func deactivationSendsOneEmail(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	m := &testkit.Mailer{}
	e.withMailer(m)
	reg := e.SessionUser(t)
	e.otherAdmin(t)
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
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
	if err := e.Identity.ReactivateOwnAccount(ctx, reg.User.ID); err != nil {
		t.Fatalf("ReactivateOwnAccount = %v", err)
	}
	e.runMailJob(t, appidentity.JobDeactivationEmail, map[string]any{"user_id": reg.User.ID, "deactivated_at": s.Since})
	if n := len(m.Sent()); n != 2 {
		t.Errorf("sent %d emails after the late job, want the first and the reset", n)
	}

	// The period itself sends nothing more.
	e.Clock.Advance(account.DeactivationGap)
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	before := len(m.Sent())
	e.Clock.Advance(account.DeactivationPeriod)
	if _, err := e.Accounts.ExecuteDueDeletions(ctx); err != nil {
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
func deactivationStatusPageDelete(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.SessionUser(t)
	e.otherAdmin(t)
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount = %v", err)
	}
	signedIn := e.SignIn(t, appidentity.SignInInput{})
	if err := e.Identity.DeleteDeactivatedAccount(ctx, signedIn.Session, "wrong password"); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Fatalf("DeleteDeactivatedAccount(wrong password) = %v, want ErrWrongPassword", err)
	}
	if err := e.Identity.DeleteDeactivatedAccount(ctx, signedIn.Session, sessionPassword); err != nil {
		t.Fatalf("DeleteDeactivatedAccount = %v", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); s.State != account.StateDeleted {
		t.Errorf("state = %+v, want deleted", s)
	}

	// Without a password, the sign-in confirms while it is recent.
	other, err := e.Identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "nopassword", Email: "nopassword@example.com", Password: sessionPassword})
	if err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	p, _ := e.RegisterPasskey(t, other.User.ID, "laptop")
	if err := e.Identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: other.User.ID, Passkey: e.confirmWithPasskey(t, other.User.ID, p).Passkey}); err != nil {
		t.Fatalf("RemovePassword = %v", err)
	}
	if err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return errorOf(e.Accounts.DeactivateTx(ctx, tx, other.AccountID, e.IDs.NewID()))
	}); err != nil {
		t.Fatalf("DeactivateTx = %v", err)
	}
	if has, err := e.Identity.HasPassword(ctx, other.User.ID); err != nil || has {
		t.Fatalf("HasPassword = %v, %v; want none", has, err)
	}
	session := identity.Session{UserID: other.User.ID, CreatedAt: e.Clock.Now()}
	e.Clock.Advance(appidentity.DeleteConfirmationFreshness + time.Millisecond)
	if err := e.Identity.DeleteDeactivatedAccount(ctx, session, ""); !errors.Is(err, appidentity.ErrSignInAgain) {
		t.Fatalf("DeleteDeactivatedAccount(old session) = %v, want ErrSignInAgain", err)
	}
	session.CreatedAt = e.Clock.Now()
	if err := e.Identity.DeleteDeactivatedAccount(ctx, session, ""); err != nil {
		t.Fatalf("DeleteDeactivatedAccount(new session) = %v", err)
	}
}

// deactivationConfirmedByACode checks the confirmation with a TOTP code: a valid one deactivates,
// and a code that was used already does not, since this path ends in a deletion.
func deactivationConfirmedByACode(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.SessionUser(t)
	e.otherAdmin(t)
	totp := e.EnableTOTP(t, reg)

	used := totp.code(e)
	if _, err := e.Identity.RegenerateRecoveryCodes(ctx, reg.User.ID, used); err != nil {
		t.Fatalf("RegenerateRecoveryCodes = %v", err)
	}
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{TwoFactorCode: used}); !errors.Is(err, appidentity.ErrInvalidSecondFactor) {
		t.Fatalf("DeactivateOwnAccount(a used code) = %v, want ErrInvalidSecondFactor", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Active() {
		t.Fatalf("state after the used code = %+v, want active", s)
	}

	due, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, appidentity.Confirmation{TwoFactorCode: totp.code(e)})
	if err != nil {
		t.Fatalf("DeactivateOwnAccount(a valid code) = %v", err)
	}
	s := e.lifecycleOf(t, reg.AccountID)
	if !s.Deactivated() || !due.Equal(s.DeactivationDueAt()) {
		t.Errorf("state = %+v, due = %v; want deactivated with that date", s, due)
	}
}

// deactivationConfirmedByAPasskey checks the confirmation with a passkey: another user's passkey
// does not confirm, a confirmation works once, and a user without a password confirms with one.
func deactivationConfirmedByAPasskey(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.SessionUser(t)
	e.otherAdmin(t)
	own, _ := e.RegisterPasskey(t, reg.User.ID, "laptop")
	other, err := e.Identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "other", Email: "other@example.com", Password: sessionPassword})
	if err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	foreign, _ := e.RegisterPasskey(t, other.User.ID, "theirs")

	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, e.confirmWithPasskey(t, reg.User.ID, foreign)); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Fatalf("DeactivateOwnAccount(the passkey of another user) = %v, want ErrInvalidPasskey", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Active() {
		t.Fatalf("state after the wrong passkey = %+v, want active", s)
	}

	confirmed := e.confirmWithPasskey(t, reg.User.ID, own)
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, confirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount(a passkey) = %v", err)
	}
	if s := e.lifecycleOf(t, reg.AccountID); !s.Deactivated() {
		t.Fatalf("state = %+v, want deactivated", s)
	}
	if err := e.Identity.ReactivateOwnAccount(ctx, reg.User.ID); err != nil {
		t.Fatalf("ReactivateOwnAccount = %v", err)
	}
	e.Clock.Advance(account.DeactivationGap)
	if _, err := e.Identity.DeactivateOwnAccount(ctx, reg.User.ID, confirmed); !errors.Is(err, appidentity.ErrPasskeyConfirmationExpired) {
		t.Fatalf("DeactivateOwnAccount(a used confirmation) = %v, want ErrPasskeyConfirmationExpired", err)
	}

	// Without a password, the passkey is what confirms.
	if err := e.Identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: other.User.ID, Passkey: e.confirmWithPasskey(t, other.User.ID, foreign).Passkey}); err != nil {
		t.Fatalf("RemovePassword = %v", err)
	}
	if _, err := e.Identity.DeactivateOwnAccount(ctx, other.User.ID, deactivationConfirmed); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Fatalf("DeactivateOwnAccount(a password, without one) = %v, want ErrWrongPassword", err)
	}
	if _, err := e.Identity.DeactivateOwnAccount(ctx, other.User.ID, e.confirmWithPasskey(t, other.User.ID, foreign)); err != nil {
		t.Fatalf("DeactivateOwnAccount(a passkey, without a password) = %v", err)
	}
}

// deactivationProtectsTheLastAdministrator checks that a deactivated administrator does not count
// as one who can sign in, and that the deletion at the end of the period waits while it would take
// the last administrator, until there is another one.
func deactivationProtectsTheLastAdministrator(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	e.Accounts.SetDeletionHold(e.Identity.LastAdministratorAccountTx)
	alice := e.SessionUser(t)
	bob, err := e.Identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "bob", Email: "bob@example.com", Password: sessionPassword})
	if err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	if _, err := e.Identity.DeactivateOwnAccount(ctx, alice.User.ID, deactivationConfirmed); err != nil {
		t.Fatalf("DeactivateOwnAccount(alice) = %v", err)
	}

	// alice is deactivated, so bob is the last one who can sign in.
	wantRefused(t, errorOf(e.Identity.DeactivateOwnAccount(ctx, bob.User.ID, deactivationConfirmed)), appaccount.DeactivationRefusedLastAdministrator)
	p, err := e.Identity.PreviewDeactivation(ctx, bob.User.ID)
	if err != nil || p.Refused == nil || p.Refused.Reason != appaccount.DeactivationRefusedLastAdministrator {
		t.Errorf("PreviewDeactivation(bob) = %+v, %v; want the refusal", p, err)
	}
	if err := e.Identity.DeleteOwnAccount(ctx, bob.User.ID, deactivationConfirmed); !errors.Is(err, appidentity.ErrLastAdmin) {
		t.Errorf("DeleteOwnAccount(bob) = %v, want ErrLastAdmin", err)
	}

	// bob goes some other way: at the date, the job holds back and leaves alice as she is, however
	// often it runs.
	if _, err := e.Accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: bob.AccountID, Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySystem,
	}); err != nil {
		t.Fatal(err)
	}
	e.Clock.Advance(account.DeactivationPeriod)
	for range 2 {
		if n, err := e.Accounts.ExecuteDueDeletions(ctx); err != nil || n != 0 {
			t.Fatalf("ExecuteDueDeletions of the last administrator = %d, %v; want none", n, err)
		}
	}
	if s := e.lifecycleOf(t, alice.AccountID); !s.Deactivated() {
		t.Fatalf("state of the last administrator = %+v, want deactivated", s)
	}

	// With another administrator, the next run deletes her.
	if _, err := e.Identity.CreateAdmin(ctx, appidentity.CreateAdminInput{Username: "carol", Email: "carol@example.com", Password: sessionPassword}); err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	if n, err := e.Accounts.ExecuteDueDeletions(ctx); err != nil || n != 1 {
		t.Fatalf("ExecuteDueDeletions with another administrator = %d, %v; want 1", n, err)
	}
	if s := e.lifecycleOf(t, alice.AccountID); s.State != account.StateDeleted {
		t.Errorf("state = %+v, want deleted", s)
	}
}
