// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"reflect"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The suppression cases of AccountRepositoryContract and AdminAccountContract (ADR-0086 §5).

// errRecipientUnknown is the error of a mail server that does not know the recipient.
var errRecipientUnknown = &kernel.StatusError{Status: "5.1.1", Err: fmt.Errorf("smtp: RCPT TO: reply 550 5.1.1: %w", kernel.ErrRecipientRejected)}

// primaryEmailID returns the ID of the primary address of userID.
func (e *accountEnv) primaryEmailID(t *testing.T, userID string) string {
	t.Helper()
	email, _, err := e.h.Identity.FindPrimaryEmail(t.Context(), userID)
	if err != nil {
		t.Fatalf("FindPrimaryEmail = %v", err)
	}
	return email.ID
}

// suppression returns the suppression of an address and whether there is one.
func (e *accountEnv) suppression(t *testing.T, emailAddressID string) (identity.EmailSuppression, bool) {
	t.Helper()
	sup, ok, err := e.identity.EmailSuppression(t.Context(), emailAddressID)
	if err != nil {
		t.Fatalf("EmailSuppression = %v", err)
	}
	return sup, ok
}

// runMailJob runs the handler of jobType on a job with payload, as attempt 1 of a new job.
func (e *accountEnv) runMailJob(t *testing.T, jobType string, payload any) queue.Result {
	t.Helper()
	handle, _, ok := e.mailJobs.Registry.Lookup(jobType)
	if !ok {
		t.Fatalf("no handler of %s", jobType)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return handle(t.Context(), queue.Job{ID: e.ids.NewID(), Type: jobType, Payload: b, Attempt: 1})
}

// verifyToken returns the token of the newest verification email in m.
func verifyToken(t *testing.T, m *Mailer) string {
	t.Helper()
	sent := m.Sent()
	for i := len(sent) - 1; i >= 0; i-- {
		if match := verifyLink.FindStringSubmatch(sent[i].Text); match != nil {
			token, err := url.QueryUnescape(match[1])
			if err != nil {
				t.Fatal(err)
			}
			return token
		}
	}
	t.Fatalf("no verification email among %d", len(sent))
	return ""
}

// emailHardBounceStopsMail records a rejected recipient as a hard bounce once per job run,
// stops the reset and notice emails after it while a requested verification email still goes
// out, and lifts the suppression (reverified) when the user verifies the address with it.
func emailHardBounceStopsMail(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	m := &Mailer{Err: errRecipientUnknown}
	e.withMailer(m)
	reg := e.sessionUser(t)
	addressID := e.primaryEmailID(t, reg.User.ID)

	e.requestReset(t, sessionEmail)
	run := e.onlyJob(t, appidentity.JobPasswordResetEmail)
	if !run.Result.IsFail() {
		t.Fatalf("result of a rejected recipient = %+v, want Fail", run.Result)
	}
	sup, ok := e.suppression(t, addressID)
	if !ok || sup.Kind != identity.EmailHardBounce || !sup.Since.Equal(e.clock.Now()) {
		t.Fatalf("suppression = %+v, %v; want a hard bounce now", sup, ok)
	}
	// The same run recorded again is the same failure: an earlier time does not replace it.
	again := appidentity.DeliveryFailure{
		EmailAddressID: addressID, Kind: identity.EmailHardBounce, Source: identity.EmailFailureRejection,
		FeedbackKey: "job:" + run.Job.ID + ":1", OccurredAt: e.clock.Now().Add(-time.Hour),
	}
	if err := e.identity.RecordDeliveryFailure(ctx, again); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.suppression(t, addressID); got != sup {
		t.Errorf("suppression after the same failure again = %+v, want %+v", got, sup)
	}

	// No reset or notice email goes to the address; the answer to a request stays the same.
	m.Err = nil
	e.clock.Advance(time.Second)
	e.requestReset(t, sessionEmail)
	notice := map[string]string{"user_id": reg.User.ID, "notice": "changed", "credential_id": e.ids.NewID()}
	if r := e.runMailJob(t, appidentity.JobPasswordNoticeEmail, notice); !r.IsDone() {
		t.Errorf("notice = %+v, want Done", r)
	}
	if n := len(m.Sent()); n != 0 {
		t.Fatalf("sent %d emails to a hard-bounced address, want none", n)
	}
	if _, ok, err := e.identity.TransactionalAddress(ctx, reg.User.ID); err != nil || ok {
		t.Errorf("TransactionalAddress after a hard bounce = %v, %v; want none", ok, err)
	}

	// The user asks for the verification email, which goes out, and verifying lifts the
	// suppression.
	if err := e.identity.ResendVerificationEmail(ctx, reg.User.ID); err != nil {
		t.Fatalf("ResendVerificationEmail of a hard-bounced address = %v", err)
	}
	if n := len(m.Sent()); n != 1 {
		t.Fatalf("sent %d verification emails, want 1", n)
	}
	e.clock.Advance(time.Second)
	if _, err := e.identity.VerifyEmail(ctx, verifyToken(t, m)); err != nil {
		t.Fatalf("VerifyEmail = %v", err)
	}
	if sup, ok := e.suppression(t, addressID); ok {
		t.Fatalf("suppression after verifying again = %+v, want none", sup)
	}
	if err := e.identity.ResendVerificationEmail(ctx, reg.User.ID); !errors.Is(err, appidentity.ErrAlreadyVerified) {
		t.Errorf("ResendVerificationEmail after the lift = %v, want ErrAlreadyVerified", err)
	}
	e.clock.Advance(time.Second)
	e.requestReset(t, sessionEmail)
	if n := len(m.Sent()); n != 2 {
		t.Errorf("sent %d emails after the lift, want the reset email too", n)
	}
}

// emailComplaintStopsNotifications keeps sending the transactional emails to an address with a
// complaint but no notification email.
func emailComplaintStopsNotifications(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	m := &Mailer{}
	e.withMailer(m)
	reg := e.sessionUser(t)
	addressID := e.primaryEmailID(t, reg.User.ID)
	if _, ok, err := e.identity.NotificationMailbox(ctx, reg.User.ID); err != nil || !ok {
		t.Fatalf("NotificationMailbox before the complaint = %v, %v; want ok", ok, err)
	}

	if err := e.identity.RecordDeliveryFailure(ctx, appidentity.DeliveryFailure{
		EmailAddressID: addressID, Kind: identity.EmailComplaint, Source: identity.EmailFailureSES, FeedbackKey: "ses:feedback-1",
		OccurredAt: e.clock.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if sup, ok := e.suppression(t, addressID); !ok || sup.Kind != identity.EmailComplaint {
		t.Fatalf("suppression = %+v, %v; want a complaint", sup, ok)
	}
	if _, ok, err := e.identity.NotificationMailbox(ctx, reg.User.ID); err != nil || ok {
		t.Errorf("NotificationMailbox after a complaint = %v, %v; want none", ok, err)
	}
	if _, ok, err := e.identity.NotificationAddress(ctx, reg.User.ID); err != nil || ok {
		t.Errorf("NotificationAddress after a complaint = %v, %v; want none", ok, err)
	}
	if _, ok, err := e.identity.TransactionalAddress(ctx, reg.User.ID); err != nil || !ok {
		t.Errorf("TransactionalAddress after a complaint = %v, %v; want the address", ok, err)
	}
	e.requestReset(t, sessionEmail)
	notice := map[string]string{"user_id": reg.User.ID, "notice": "changed", "credential_id": e.ids.NewID()}
	if r := e.runMailJob(t, appidentity.JobPasswordNoticeEmail, notice); !r.IsDone() {
		t.Errorf("notice = %+v, want Done", r)
	}
	if n := len(m.Sent()); n != 2 {
		t.Errorf("sent %d transactional emails after a complaint, want 2", n)
	}
	// A verified address with a complaint does not take a verification email.
	if err := e.identity.ResendVerificationEmail(ctx, reg.User.ID); !errors.Is(err, appidentity.ErrAlreadyVerified) {
		t.Errorf("ResendVerificationEmail with a complaint = %v, want ErrAlreadyVerified", err)
	}
}

// emailAddressFailureRecordsOnce records an SES notification by the address it names: the
// primary address matches in another case and after the forms that match nothing, the same
// feedback key again is the same failure, and an unknown address records nothing.
func emailAddressFailureRecordsOnce(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	addressID := e.primaryEmailID(t, reg.User.ID)
	in := appidentity.AddressFailure{
		Addresses: []string{"nobody@example.com", "ALICE@Example.com"}, Kind: identity.EmailHardBounce,
		Source: identity.EmailFailureSES, FeedbackKey: "0100017f8ab5f2f6-feedback", EnhancedStatus: "5.1.1", OccurredAt: e.clock.Now(),
	}
	if ok, err := e.identity.RecordAddressFailure(ctx, in); err != nil || !ok {
		t.Fatalf("RecordAddressFailure = %v, %v; want a match", ok, err)
	}
	sup, ok := e.suppression(t, addressID)
	if !ok || sup.Kind != identity.EmailHardBounce || !sup.Since.Equal(e.clock.Now()) {
		t.Fatalf("suppression = %+v, %v; want a hard bounce now", sup, ok)
	}
	// SNS delivers the notification again: an earlier time does not replace the failure.
	in.OccurredAt = e.clock.Now().Add(-time.Hour)
	if ok, err := e.identity.RecordAddressFailure(ctx, in); err != nil || !ok {
		t.Fatalf("RecordAddressFailure again = %v, %v; want a match", ok, err)
	}
	if got, _ := e.suppression(t, addressID); got != sup {
		t.Errorf("suppression after the same notification again = %+v, want %+v", got, sup)
	}
	unknown := in
	unknown.Addresses, unknown.FeedbackKey = []string{"nobody@example.com"}, "other-feedback"
	if ok, err := e.identity.RecordAddressFailure(ctx, unknown); err != nil || ok {
		t.Errorf("RecordAddressFailure of an unknown address = %v, %v; want no match", ok, err)
	}
}

// emailSuppressionEqualsRebuild records random failures and lifts on a few addresses and checks
// that the suppressions equal a rebuild of the projection.
func emailSuppressionEqualsRebuild(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(86, 20261025)) //nolint:gosec // deterministic test input, not a secret
	admin := appidentity.NewAccountAdmin(appidentity.AccountAdminDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Identity, Accounts: e.accounts, Clock: e.clock, IDs: e.ids,
	})
	type user struct{ accountID, addressID string }
	var users []user
	for i := range 4 {
		reg := e.register(t, fmt.Sprintf("suppressed%d", i))
		email := identity.EmailAddress{
			ID: e.ids.NewID(), UserID: reg.User.ID, Address: fmt.Sprintf("s%d@example.com", i),
			AddressNormalized: fmt.Sprintf("s%d@example.com", i), AddedAt: e.clock.Now(),
		}
		if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			if err := e.h.Identity.InsertEmailAddress(ctx, tx, email); err != nil {
				return err
			}
			return e.h.Identity.InsertPrimaryEmail(ctx, tx, email)
		}); err != nil {
			t.Fatal(err)
		}
		users = append(users, user{reg.AccountID, email.ID})
	}
	snapshot := func() []any {
		out := make([]any, len(users))
		for i, u := range users {
			sup, ok := e.suppression(t, u.addressID)
			out[i] = [2]any{sup, ok}
		}
		return out
	}
	kinds := []identity.EmailFailureKind{identity.EmailHardBounce, identity.EmailComplaint}
	for round := range 3 {
		for step := range 30 {
			u := users[rng.IntN(len(users))]
			switch op := rng.IntN(10); {
			case op < 6:
				// Failures may arrive out of order, and some repeat a key.
				at := e.clock.Now().Add(-time.Duration(rng.IntN(3000)) * time.Millisecond)
				key := fmt.Sprintf("key-%d-%d", round, step)
				if rng.IntN(5) == 0 {
					key = "key-repeated"
				}
				if err := e.identity.RecordDeliveryFailure(ctx, appidentity.DeliveryFailure{
					EmailAddressID: u.addressID, Kind: kinds[rng.IntN(2)], Source: identity.EmailFailureSES, FeedbackKey: key, OccurredAt: at,
				}); err != nil {
					t.Fatal(err)
				}
			default:
				err := admin.LiftEmailSuppression(ctx, u.accountID, "")
				if err != nil && !errors.Is(err, appidentity.ErrNotSuppressed) {
					t.Fatalf("LiftEmailSuppression = %v", err)
				}
			}
			e.clock.Advance(time.Duration(rng.IntN(2000)+1) * time.Millisecond)
		}
		incremental := snapshot()
		if err := appidentity.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Identity); err != nil {
			t.Fatal(err)
		}
		if rebuilt := snapshot(); !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: suppressions differ from a rebuild:\nincremental %+v\nrebuilt     %+v", round, incremental, rebuilt)
		}
	}
}

// adminAccountLiftsEmailSuppressions shows the suppression of alice's address in her account's
// details and lifts it for a moderator, which the audit log records.
func adminAccountLiftsEmailSuppressions(t *testing.T, e *adminAccountEnv) {
	ctx := t.Context()
	if err := e.admin.LiftEmailSuppression(ctx, e.alice, NamedID("mod-user")); !errors.Is(err, appidentity.ErrNotSuppressed) {
		t.Errorf("LiftEmailSuppression(not suppressed) = %v, want ErrNotSuppressed", err)
	}
	if err := e.admin.LiftEmailSuppression(ctx, e.alfred, NamedID("mod-user")); !errors.Is(err, appidentity.ErrNotLocalAccount) {
		t.Errorf("LiftEmailSuppression(remote) = %v, want ErrNotLocalAccount", err)
	}
	if err := e.admin.LiftEmailSuppression(ctx, e.ids.NewID(), NamedID("mod-user")); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("LiftEmailSuppression(unknown) = %v, want ErrNotFound", err)
	}
	addressID := e.primaryEmailID(t, e.aliceUser)
	if err := e.identity.RecordDeliveryFailure(ctx, appidentity.DeliveryFailure{
		EmailAddressID: addressID, Kind: identity.EmailHardBounce, Source: identity.EmailFailureRejection,
		FeedbackKey: "job:x:1", EnhancedStatus: "5.1.1", OccurredAt: e.clock.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	d, err := e.admin.Detail(ctx, e.alice)
	if err != nil || d.EmailSuppression == nil || d.EmailSuppression.Kind != identity.EmailHardBounce || !d.EmailSuppression.Since.Equal(e.clock.Now()) {
		t.Fatalf("Detail = %+v, %v; want the hard bounce", d.EmailSuppression, err)
	}

	e.clock.Advance(time.Second)
	if err := e.admin.LiftEmailSuppression(ctx, e.alice, NamedID("mod-user")); err != nil {
		t.Fatalf("LiftEmailSuppression = %v", err)
	}
	if d, err := e.admin.Detail(ctx, e.alice); err != nil || d.EmailSuppression != nil {
		t.Errorf("Detail after the lift = %+v, %v; want no suppression", d.EmailSuppression, err)
	}
	got, err := e.mod.AuditLog(ctx, appmoderation.AuditQuery{Action: moderation.AuditEmailSuppressionLifted}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ActorUserID != NamedID("mod-user") || got[0].TargetKind != moderation.AuditTargetAccount ||
		got[0].TargetID != e.alice || got[0].TargetLabel != "@alice" || got[0].SourceEventID == "" || !got[0].OccurredAt.Equal(e.clock.Now()) {
		t.Errorf("audit log = %+v, want one email_suppression_lifted entry on @alice", got)
	}
}
