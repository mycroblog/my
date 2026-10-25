// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The emails of the identity module (ADR-0086 §1). A use case never calls the Mailer: it
// enqueues a job of the email's purpose through Deps.Outbox in the transaction of the fact that
// the email tells, and the job's handler builds the message and sends it. A payload holds IDs
// and the time of the request only, never the address, the body, or a secret. The handlers of
// the verification and the reset emails issue a new token on every run, commit it, and only
// then send it, so that the secret appears in the email's body only; a retried or repeated run
// may send a second email with another link that works too, which is harmless, since both work
// once and expire. Before it sends, a handler checks that there is still a reason to, and
// finishes without sending when there is none. The errors of the handlers name the job's type,
// the user's ID, and what the Mailer reported, never the address, the token, or the body.

// The job types of the identity module's emails.
const (
	// JobVerificationEmail sends the verification email of an unverified primary address.
	JobVerificationEmail = "identity.verification_email"
	// JobPasswordResetEmail sends the email of a password reset link.
	JobPasswordResetEmail = "identity.password_reset_email"
	// JobPasswordNoticeEmail tells a user that their password changed or was removed.
	JobPasswordNoticeEmail = "identity.password_notice_email"
)

// How long after its request a transactional email is still sent (ADR-0086 §1). A handler that
// runs later finishes without sending, so that an old link does not arrive late.
const (
	VerificationEmailDeadline  = time.Hour
	PasswordResetEmailDeadline = 15 * time.Minute
)

// verificationEmail is the payload of JobVerificationEmail.
type verificationEmail struct {
	// RequestID identifies the request, a sign-up or a request to send the email again; it
	// keys the job's idempotency.
	RequestID      string    `json:"request_id"`
	UserID         string    `json:"user_id"`
	EmailAddressID string    `json:"email_address_id"`
	RequestedAt    time.Time `json:"requested_at"`
}

// passwordResetEmail is the payload of JobPasswordResetEmail.
type passwordResetEmail struct {
	RequestID   string    `json:"request_id"`
	UserID      string    `json:"user_id"`
	RequestedAt time.Time `json:"requested_at"`
}

// passwordNotice is what happened to a user's password, for the email that tells them.
type passwordNotice string

const (
	passwordChanged passwordNotice = "changed"
	passwordRemoved passwordNotice = "removed"
)

// passwordNoticeEmail is the payload of JobPasswordNoticeEmail.
type passwordNoticeEmail struct {
	UserID string         `json:"user_id"`
	Notice passwordNotice `json:"notice"`
	// CredentialID is the password credential that was set or removed; it keys the job's
	// idempotency.
	CredentialID string `json:"credential_id"`
}

// RegisterMailJobs registers the handlers of the identity module's emails on reg: the
// verification and the reset emails with queue.TransactionalMailPolicy, the notices of password
// changes with the runner's default policy.
func (s *Service) RegisterMailJobs(reg *queue.JobHandlerRegistry) {
	reg.RegisterWithPolicy(JobVerificationEmail, mailHandler(s.sendVerificationEmail), queue.TransactionalMailPolicy)
	reg.RegisterWithPolicy(JobPasswordResetEmail, mailHandler(s.sendPasswordResetEmail), queue.TransactionalMailPolicy)
	reg.Register(JobPasswordNoticeEmail, mailHandler(s.sendPasswordNoticeEmail))
}

// mailHandler returns the handler of a job whose payload is a T and that send runs: a payload
// that does not decode fails the job, and send's error decides as queue.MailResult does.
func mailHandler[T any](send func(ctx context.Context, job T) error) queue.Handler {
	return func(ctx context.Context, j queue.Job) queue.Result {
		var job T
		if err := json.Unmarshal(j.Payload, &job); err != nil {
			return queue.Fail(fmt.Errorf("identity: the payload of %s %s: %w", j.Type, j.ID, err))
		}
		return queue.MailResult(send(ctx, job))
	}
}

// enqueueMail enqueues in tx the job jobType with payload, keyed by jobType and key.
func (s *Service) enqueueMail(ctx context.Context, tx uow.Tx, jobType, key string, payload any) error {
	if s.d.Outbox == nil {
		return errors.New("identity: sending email needs an Outbox")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: jobType, Payload: b, IdempotencyKey: jobType + ":" + key})
}

// verificationRequest returns the job of a request for the verification email of a user's
// address, or nil when there is no mailer to send it.
func (s *Service) verificationRequest(userID, emailID string, now time.Time) *verificationEmail {
	if s.d.Mailer == nil {
		return nil
	}
	return &verificationEmail{RequestID: s.d.IDs.NewID(), UserID: userID, EmailAddressID: emailID, RequestedAt: now}
}

func (s *Service) enqueueVerification(ctx context.Context, tx uow.Tx, job verificationEmail) error {
	return s.enqueueMail(ctx, tx, JobVerificationEmail, job.RequestID, job)
}

// enqueuePasswordNotice enqueues in tx the email that tells a user that the password credential
// credentialID was set or removed; it enqueues nothing without email delivery.
func (s *Service) enqueuePasswordNotice(ctx context.Context, tx uow.Tx, userID string, notice passwordNotice, credentialID string) error {
	if !s.PasswordResetByEmail() {
		return nil
	}
	job := passwordNoticeEmail{UserID: userID, Notice: notice, CredentialID: credentialID}
	return s.enqueueMail(ctx, tx, JobPasswordNoticeEmail, string(notice)+":"+credentialID, job)
}

// sendVerificationEmail runs JobVerificationEmail: while the address is still the user's
// primary address, unverified, and the request is not older than VerificationEmailDeadline, it
// issues a verification token and sends its link.
func (s *Service) sendVerificationEmail(ctx context.Context, job verificationEmail) error {
	now := s.d.Clock.Now()
	if now.Sub(job.RequestedAt) > VerificationEmailDeadline {
		return nil
	}
	email, verified, err := s.d.Repository.FindPrimaryEmail(ctx, job.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if verified || email.ID != job.EmailAddressID {
		return nil
	}
	prefs, err := s.d.Repository.FindPreferences(ctx, job.UserID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	m, err := s.mailContext(ctx, prefs.Locale)
	if err != nil {
		return err
	}
	secret, hash := NewToken(EmailVerificationTokenPrefix)
	t := identity.UserToken{
		ID: s.d.IDs.NewID(), UserID: job.UserID, Purpose: identity.PurposeEmailVerification, EmailAddressID: email.ID,
		TokenHash: hash, ExpiresAt: now.Add(EmailVerificationTTL), IssuedAt: now,
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertUserToken(ctx, tx, t) }); err != nil {
		return err
	}
	return s.send(ctx, JobVerificationEmail, job.UserID, m.composer, kernel.MailDraft{
		Category: kernel.MailTransactional, To: email.Address,
		Subject: m.l.ServerMailVerifyEmailSubject(m.name),
		Paragraphs: []string{
			m.l.ServerMailVerifyEmailBody(m.name),
			m.l.ServerMailVerifyEmailDetail(int(EmailVerificationTTL / time.Hour)),
		},
		Action: &kernel.MailAction{Label: m.l.ServerMailVerifyEmailAction(), URL: s.link(PathVerifyEmail, secret)},
		Reason: m.l.ServerMailVerifyEmailReason(m.host),
	})
}

// sendPasswordResetEmail runs JobPasswordResetEmail: while users can ask for a reset link by
// email, the user's local account can sign in, the user has a primary address, and the request
// is not older than PasswordResetEmailDeadline, it issues a reset token and sends its link to
// that address.
func (s *Service) sendPasswordResetEmail(ctx context.Context, job passwordResetEmail) error {
	if !s.PasswordResetByEmail() || s.d.Clock.Now().Sub(job.RequestedAt) > PasswordResetEmailDeadline {
		return nil
	}
	switch err := s.checkActive(ctx, job.UserID); {
	case errors.Is(err, ErrInvalidCredentials):
		return nil
	case err != nil:
		return err
	}
	email, _, err := s.d.Repository.FindPrimaryEmail(ctx, job.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	prefs, err := s.d.Repository.FindPreferences(ctx, job.UserID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	m, err := s.mailContext(ctx, prefs.Locale)
	if err != nil {
		return err
	}
	secret, err := s.issueResetToken(ctx, job.UserID, nil)
	if err != nil {
		return err
	}
	return s.send(ctx, JobPasswordResetEmail, job.UserID, m.composer, kernel.MailDraft{
		Category: kernel.MailTransactional, To: email.Address,
		Subject: m.l.ServerMailPasswordResetSubject(m.name),
		Paragraphs: []string{
			m.l.ServerMailPasswordResetBody(m.name),
			m.l.ServerMailPasswordResetDetail(int(PasswordResetTTL / time.Minute)),
		},
		Action: &kernel.MailAction{Label: m.l.ServerMailActionSetNewPassword(), URL: s.link(PathResetPassword, secret)},
		Reason: m.l.ServerMailPasswordResetReason(m.host),
	})
}

// sendPasswordNoticeEmail runs JobPasswordNoticeEmail: it tells the user that their password
// changed or was removed, with the link that asks for a reset in case they did not do it. It
// sends nothing without email delivery or a primary address.
func (s *Service) sendPasswordNoticeEmail(ctx context.Context, job passwordNoticeEmail) error {
	if !s.PasswordResetByEmail() {
		return nil
	}
	email, _, err := s.d.Repository.FindPrimaryEmail(ctx, job.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	prefs, err := s.d.Repository.FindPreferences(ctx, job.UserID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	m, err := s.mailContext(ctx, prefs.Locale)
	if err != nil {
		return err
	}
	d := kernel.MailDraft{
		Category: kernel.MailTransactional, To: email.Address,
		Action: &kernel.MailAction{Label: m.l.ServerMailActionSetNewPassword(), URL: s.link(PathForgotPassword, "")},
		Reason: m.l.ServerMailReasonAccount(m.name),
	}
	switch job.Notice {
	case passwordChanged:
		d.Subject = m.l.ServerMailPasswordChangedSubject(m.name)
		d.Paragraphs = []string{m.l.ServerMailPasswordChangedBody(m.name), m.l.ServerMailPasswordChangedDetail()}
	case passwordRemoved:
		d.Subject = m.l.ServerMailPasswordRemovedSubject(m.name)
		d.Paragraphs = []string{m.l.ServerMailPasswordRemovedBody(m.name), m.l.ServerMailPasswordRemovedDetail()}
	default:
		return nil
	}
	return s.send(ctx, JobPasswordNoticeEmail, job.UserID, m.composer, d)
}

// send composes d with c and sends it, the email of the job jobType to userID. Its error names
// the job's type and the user's ID with the composer's or the Mailer's error, which never
// contains the address or the body.
func (s *Service) send(ctx context.Context, jobType, userID string, c kernel.MailComposer, d kernel.MailDraft) error {
	if s.d.Mailer == nil {
		return nil
	}
	m, err := c.Compose(d)
	if err != nil {
		return fmt.Errorf("identity: compose the %s email of user %s: %w", jobType, userID, err)
	}
	if err := s.d.Mailer.Send(ctx, m); err != nil {
		return fmt.Errorf("identity: send the %s email of user %s: %w", jobType, userID, err)
	}
	return nil
}
