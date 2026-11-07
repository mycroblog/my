// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/identity"
)

// errMailDeferred is a temporary failure of a Mailer, which the jobs retry.
var errMailDeferred = errors.New("smtp: RCPT TO: reply 451 4.3.0")

// wantNoSecrets fails when what, the payload or the error of a job, contains one of secrets,
// compared without case.
func wantNoSecrets(t *testing.T, what, text string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(strings.ToLower(text), strings.ToLower(s)) {
			t.Errorf("%s %q contains %q", what, text, s)
		}
	}
}

// onlyJob returns the one job of jobType that the env's mail jobs ran, failing unless there is
// exactly one.
func (e *accountEnv) onlyJob(t *testing.T, jobType string) InlineResult {
	t.Helper()
	var found []InlineResult
	for _, r := range e.mailJobs.Results() {
		if r.Job.Type == jobType {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("ran %d jobs of %s, want 1", len(found), jobType)
	}
	return found[0]
}

// passwordResetEmailJobs sends a reset email from its job (ADR-0086 §1): the request enqueues a
// payload without the address or a token, the handler's errors name neither, a temporary
// failure is retried under the transactional policy, a rejected message fails the job (a
// rejected recipient also suppresses the address: emailHardBounceStopsMail), and a run after
// PasswordResetEmailDeadline finishes without sending.
func passwordResetEmailJobs(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	m := &Mailer{Err: errMailDeferred}
	e.withMailer(m)
	e.sessionUser(t)
	e.requestReset(t, sessionEmail)

	run := e.onlyJob(t, appidentity.JobPasswordResetEmail)
	if !run.Result.IsRetry() {
		t.Fatalf("result of a temporary failure = %+v, want Retry", run.Result)
	}
	secrets := []string{sessionEmail, appidentity.PasswordResetTokenPrefix}
	wantNoSecrets(t, "payload", string(run.Job.Payload), secrets...)
	wantNoSecrets(t, "error", run.Result.Err().Error(), secrets...)
	handle, policy, _ := e.mailJobs.Registry.Lookup(appidentity.JobPasswordResetEmail)
	if policy.MaxAttempts != queue.TransactionalMailPolicy.MaxAttempts || policy.BaseDelay != 30*time.Second {
		t.Errorf("retry policy = %+v, want the transactional one", policy)
	}

	m.Err = fmt.Errorf("smtp: DATA: reply 554 5.7.1: %w", kernel.ErrMailRejected)
	if r := handle(ctx, run.Job); !r.IsFail() {
		t.Errorf("result of a rejected message = %+v, want Fail", r)
	} else {
		wantNoSecrets(t, "error", r.Err().Error(), secrets...)
	}

	// Every run issues its own link.
	m.Err = nil
	for range 2 {
		if r := handle(ctx, run.Job); !r.IsDone() {
			t.Fatalf("result of a sent email = %+v, want Done", r)
		}
	}
	sent := m.Sent()
	if len(sent) != 2 || resetToken(t, m, "Alice@Example.com") == "" || sent[0].Text == sent[1].Text {
		t.Fatalf("sent %d emails, want 2 with different links", len(sent))
	}

	e.clock.Advance(appidentity.PasswordResetEmailDeadline + time.Second)
	if r := handle(ctx, run.Job); !r.IsDone() {
		t.Errorf("result after the deadline = %+v, want Done", r)
	}
	if n := len(m.Sent()); n != 2 {
		t.Errorf("sent %d emails after the deadline, want none more", n-2)
	}
}

// verificationEmailJobs sends a verification email from its job (ADR-0086 §1): the sign-up
// enqueues a payload without the address or a token and does not tell whether the email went
// out, and the handler sends nothing once the address is verified or after
// VerificationEmailDeadline.
func verificationEmailJobs(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	r := e.registration(identity.RegistrationOpen, true)
	r.mailer.Err = errMailDeferred
	in := signUpInput("carol")
	got := r.signUp(t, in)

	run := e.onlyJob(t, appidentity.JobVerificationEmail)
	if !run.Result.IsRetry() {
		t.Fatalf("result of a temporary failure = %+v, want Retry", run.Result)
	}
	secrets := []string{in.Email, appidentity.EmailVerificationTokenPrefix}
	wantNoSecrets(t, "payload", string(run.Job.Payload), secrets...)
	wantNoSecrets(t, "error", run.Result.Err().Error(), secrets...)
	handle, _, _ := e.mailJobs.Registry.Lookup(appidentity.JobVerificationEmail)

	r.mailer.Err = fmt.Errorf("%w: the recipient must be a bare address", kernel.ErrInvalidMail)
	if res := handle(ctx, run.Job); !res.IsFail() {
		t.Errorf("result of an invalid message = %+v, want Fail", res)
	}
	r.mailer.Err = nil
	if res := handle(ctx, run.Job); !res.IsDone() {
		t.Fatalf("result of a sent email = %+v, want Done", res)
	}
	if _, err := r.VerifyEmail(ctx, r.lastToken(t, in.Email)); err != nil {
		t.Fatalf("VerifyEmail = %v", err)
	}
	if res := handle(ctx, run.Job); !res.IsDone() || len(r.mailer.Sent()) != 1 {
		t.Errorf("run for a verified address = %+v, %d emails; want Done and none more", res, len(r.mailer.Sent())-1)
	}

	// A request for an address that is still unverified expires too.
	late := e.registration(identity.RegistrationOpen, true)
	late.mailer.Err = errMailDeferred
	dave := late.signUp(t, signUpInput("dave"))
	run = e.onlyJob(t, appidentity.JobVerificationEmail)
	handle, _, _ = e.mailJobs.Registry.Lookup(appidentity.JobVerificationEmail)
	late.mailer.Err = nil
	e.clock.Advance(appidentity.VerificationEmailDeadline + time.Second)
	if res := handle(ctx, run.Job); !res.IsDone() || len(late.mailer.Sent()) != 0 {
		t.Errorf("run after the deadline = %+v, %d emails; want Done and none", res, len(late.mailer.Sent()))
	}
	if a := late.access(t, dave.User.ID); a.EmailVerified || got.User.ID == dave.User.ID {
		t.Errorf("access of dave = %+v", a)
	}
}
