// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The service closure emails (ADR-0088 §2, FR-808, FR-809) go to every user whose primary
// address is verified, as transactional emails (ADR-0086 §3, §4: missing one loses the user's
// data, so no preference or unsubscribe link stops them and a complaint does not suppress
// them; a hard bounce does, as it does every email but the verification). The transaction of a
// closure event (the announcement, a change of its dates, the withdrawal, the closure) enqueues
// JobClosureEmailFanout for the event's email; the hourly JobClosureEmailReminders enqueues it
// for a reminder whose time has come (identity.ServiceClosureView.DueReminders). The fan-out
// records a service_closure_emails row per user and enqueues the user's JobClosureEmail in the
// same transaction, and only for a row it inserted, so that a user gets each email once however
// often the fan-out runs. JobClosureEmail checks that the email is still due (EmailDue) before
// it sends.

// Jobs of the service closure emails.
const (
	// JobClosureEmailFanout queues one email of the service closure for every user with a
	// verified primary address.
	JobClosureEmailFanout = "identity.closure_email_fanout"
	// JobClosureEmail sends one email of the service closure to one user.
	JobClosureEmail = "identity.closure_email"
	// JobClosureEmailReminders queues the reminders of the service closure whose time has come.
	JobClosureEmailReminders = "identity.closure_email_reminders"
)

// ClosureEmailReminderInterval is how often JobClosureEmailReminders runs. It is well under the
// day between the last reminder of the closing date and the date itself.
const ClosureEmailReminderInterval = time.Hour

// closureEmailBatch is how many users one transaction of JobClosureEmailFanout queues.
const closureEmailBatch = 100

// closureMailLayout is how a closure email writes a date: in the user's time zone, which the
// text names.
const closureMailLayout = "2006-01-02 15:04"

// pathExportData is the page of the web client that the closure emails link to.
const pathExportData = "/settings/import-export"

// ClosureEmailRepository is the part of Repository about the service closure emails.
type ClosureEmailRepository interface {
	// ListVerifiedEmailUsers returns a page of the users whose primary address is verified, in
	// ID order after the user after ("" for the first page), up to limit.
	ListVerifiedEmailUsers(ctx context.Context, after string, limit int) ([]string, error)
	// InsertServiceClosureEmail records e and reports whether it did: a row with the same event,
	// email, and user already exists otherwise.
	InsertServiceClosureEmail(ctx context.Context, tx uow.Tx, e identity.ServiceClosureEmailRecord) (bool, error)
	// ServiceClosureEmailQueued reports whether the email e of the event closureEventID was
	// queued for any user.
	ServiceClosureEmailQueued(ctx context.Context, closureEventID string, e identity.ClosureEmail) (bool, error)
}

// closureEmailFanout is the payload of JobClosureEmailFanout.
type closureEmailFanout struct {
	ClosureEventID string                `json:"closure_event_id"`
	Email          identity.ClosureEmail `json:"email"`
}

// closureEmail is the payload of JobClosureEmail.
type closureEmail struct {
	ClosureEventID string                `json:"closure_event_id"`
	Email          identity.ClosureEmail `json:"email"`
	UserID         string                `json:"user_id"`
}

// sendsClosureEmails reports whether the closure emails are sent: with a Mailer that delivers
// email, an Outbox, and the state of the closure.
func (s *Service) sendsClosureEmails() bool {
	return s.d.Mailer != nil && s.d.EmailDelivery && s.d.Outbox != nil && s.d.ClosureView != nil
}

// closureEventEmail returns the email that a closure event sends at once, false for none (the
// completed delivery).
func closureEventEmail(ev appinstance.ClosureRecorded) (identity.ClosureEmail, bool) {
	switch {
	case ev.Starts():
		return identity.ClosureEmailAnnounced, true
	case ev.Reschedules():
		return identity.ClosureEmailRescheduled, true
	case ev.Withdraws():
		return identity.ClosureEmailWithdrawn, true
	case ev.Closes():
		return identity.ClosureEmailClosed, true
	}
	return "", false
}

// RegisterClosureEmails registers the service closure emails: the projector that enqueues the
// email of a closure event in its transaction, the handlers of their jobs, and the hourly
// schedule of the reminders.
func (s *Service) RegisterClosureEmails(r *uow.Projectors, reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	r.Register(appinstance.EventClosureRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appinstance.ClosureRecorded)
		email, ok := closureEventEmail(ev)
		if !ok || !s.sendsClosureEmails() {
			return nil
		}
		return s.enqueueClosureFanout(ctx, tx, closureEmailFanout{ClosureEventID: ev.Event.ID, Email: email})
	})
	reg.Register(JobClosureEmailFanout, mailHandler(func(ctx context.Context, _ queue.Job, p closureEmailFanout) error {
		if p.ClosureEventID == "" || !p.Email.Valid() {
			return fmt.Errorf("identity: invalid %s payload: %w", JobClosureEmailFanout, kernel.ErrInvalidMail)
		}
		return s.fanOutClosureEmail(ctx, p)
	}))
	reg.Register(JobClosureEmail, mailHandler(s.sendClosureEmail))
	reg.Register(JobClosureEmailReminders, func(ctx context.Context, _ queue.Job) queue.Result {
		if err := s.QueueClosureReminders(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	if sched != nil {
		sched.Add(queue.Schedule{Name: JobClosureEmailReminders, Every: ClosureEmailReminderInterval, Job: queue.NewJob{Type: JobClosureEmailReminders}})
	}
}

// enqueueClosureFanout enqueues JobClosureEmailFanout for p in tx.
func (s *Service) enqueueClosureFanout(ctx context.Context, tx uow.Tx, p closureEmailFanout) error {
	return s.enqueueMail(ctx, tx, JobClosureEmailFanout, string(p.Email)+":"+p.ClosureEventID, p)
}

// QueueClosureReminders is the work of JobClosureEmailReminders: it enqueues the fan-out of every
// reminder whose time has come for the closure in effect, unless that reminder was already
// queued for some user. Running it again queues nothing twice: the fan-out queues each user's
// email once.
func (s *Service) QueueClosureReminders(ctx context.Context) error {
	if !s.sendsClosureEmails() {
		return nil
	}
	view, err := s.d.ClosureView(ctx)
	if err != nil {
		return fmt.Errorf("identity: the service closure: %w", err)
	}
	for _, email := range view.DueReminders(s.d.Clock.Now()) {
		queued, err := s.d.Repository.ServiceClosureEmailQueued(ctx, view.AnnouncementID, email)
		if err != nil {
			return err
		}
		if queued {
			continue
		}
		p := closureEmailFanout{ClosureEventID: view.AnnouncementID, Email: email}
		if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.enqueueClosureFanout(ctx, tx, p) }); err != nil {
			return fmt.Errorf("identity: queue the %s reminder of the service closure: %w", email, err)
		}
	}
	return nil
}

// fanOutClosureEmail queues the email of p for every user with a verified primary address, page
// by page: each page records the users' rows and enqueues their jobs in one transaction, for the
// rows it inserted only.
func (s *Service) fanOutClosureEmail(ctx context.Context, p closureEmailFanout) error {
	after := ""
	for {
		ids, err := s.d.Repository.ListVerifiedEmailUsers(ctx, after, closureEmailBatch)
		if err != nil {
			return fmt.Errorf("identity: the recipients of the %s closure email: %w", p.Email, err)
		}
		if len(ids) == 0 {
			return nil
		}
		now := s.d.Clock.Now()
		err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			for _, id := range ids {
				inserted, err := s.d.Repository.InsertServiceClosureEmail(ctx, tx, identity.ServiceClosureEmailRecord{
					ID: s.d.IDs.NewID(), ClosureEventID: p.ClosureEventID, Email: p.Email, UserID: id, QueuedAt: now,
				})
				if err != nil {
					return err
				}
				if !inserted {
					continue
				}
				job := closureEmail{ClosureEventID: p.ClosureEventID, Email: p.Email, UserID: id}
				if err := s.enqueueMail(ctx, tx, JobClosureEmail, string(p.Email)+":"+p.ClosureEventID+":"+id, job); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("identity: queue the %s closure emails: %w", p.Email, err)
		}
		if len(ids) < closureEmailBatch {
			return nil
		}
		after = ids[len(ids)-1]
	}
}

// sendClosureEmail runs JobClosureEmail: while the email is still due (EmailDue), the user can
// sign in, and their primary address is verified and not suppressed by a hard bounce, it sends
// the email in the user's locale with the dates in their time zone.
func (s *Service) sendClosureEmail(ctx context.Context, run queue.Job, job closureEmail) error {
	if !s.sendsClosureEmails() {
		return nil
	}
	view, err := s.d.ClosureView(ctx)
	if err != nil {
		return fmt.Errorf("identity: the service closure: %w", err)
	}
	if !view.EmailDue(job.Email, job.ClosureEventID, s.d.Clock.Now()) {
		return nil
	}
	switch err := s.checkActive(ctx, job.UserID); {
	case errors.Is(err, ErrInvalidCredentials):
		return nil
	case err != nil:
		return err
	}
	email, verified, err := s.d.Repository.FindPrimaryEmail(ctx, job.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !verified {
		return nil
	}
	if stopped, err := s.mailStopped(ctx, email.ID, identity.MailTransactional); err != nil || stopped {
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
	d, ok := ClosureEmailDraft(m.wording(), email.Address, job.Email, view, prefs.TimeZone, s.webLink(pathExportData))
	if !ok {
		return nil
	}
	return s.send(ctx, run, JobClosureEmail, job.UserID, email.ID, m.composer, d)
}

// webLink returns the URL of path in the web client: on WebOrigin, or BaseURL without one.
func (s *Service) webLink(path string) string {
	origin := s.d.WebOrigin
	if origin == "" {
		origin = s.d.BaseURL
	}
	return strings.TrimSuffix(origin, "/") + path
}
