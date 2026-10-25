// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The notice period of the service closure (ADR-0088 §2): from the announcement on, the
// registration mode reads as closed (RegistrationMode, which the composition root gives the
// effective value), so nobody signs up and no invite works; no pending application is approved;
// and the start of the notice period rejects the pending applications, in JobClosureRejections
// out of the transaction of the announcement, since its work grows with the applications.

// ErrServiceClosing reports the approval of a registration while the closure of the service is
// announced.
var ErrServiceClosing = errors.New("identity: the closure of the service is announced")

// ServiceClosure reports whether the state of the service closure lets users register:
// *instance.Service implements it.
type ServiceClosure interface {
	RegistrationAllowed(ctx context.Context) (bool, error)
}

// JobClosureRejections is the job that rejects the pending applications when the notice period
// of the service closure starts.
const JobClosureRejections = "identity.closure_rejections"

// closureRejectionBatch is how many applications one page of JobClosureRejections rejects.
const closureRejectionBatch = 100

// closureRejectionsPayload is the payload of JobClosureRejections.
type closureRejectionsPayload struct {
	// ClosureEventID is the announcement that started the notice period.
	ClosureEventID string `json:"closure_event_id"`
}

// registrationAllowed fails with ErrServiceClosing while the service closure stops
// registrations; without a ServiceClosure it allows them.
func (s *Service) registrationAllowed(ctx context.Context) error {
	if s.d.Closure == nil {
		return nil
	}
	ok, err := s.d.Closure.RegistrationAllowed(ctx)
	switch {
	case err != nil:
		return fmt.Errorf("identity: the service closure: %w", err)
	case !ok:
		return ErrServiceClosing
	}
	return nil
}

// RegisterClosureProjectors subscribes the registrations to the service closure: the start of
// its notice period enqueues JobClosureRejections in the transaction of the announcement.
func (s *Service) RegisterClosureProjectors(r *uow.Projectors) {
	r.Register(appinstance.EventClosureRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appinstance.ClosureRecorded)
		if !ev.Starts() || s.d.Outbox == nil {
			return nil
		}
		b, err := json.Marshal(closureRejectionsPayload{ClosureEventID: ev.Event.ID})
		if err != nil {
			return err
		}
		return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobClosureRejections, Payload: b, IdempotencyKey: JobClosureRejections + ":" + ev.Event.ID})
	})
}

// RegisterClosureJobs registers the handler of JobClosureRejections on reg.
func (s *Service) RegisterClosureJobs(reg *queue.JobHandlerRegistry) {
	reg.Register(JobClosureRejections, func(ctx context.Context, j queue.Job) queue.Result {
		var p closureRejectionsPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.ClosureEventID == "" {
			return queue.Fail(fmt.Errorf("identity: the payload of %s: %w", j.ID, err))
		}
		if _, err := s.RejectPendingForClosure(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
}

// RejectPendingForClosure is the work of JobClosureRejections: page by page, it rejects every
// pending application, decided by service_closure, and returns how many it rejected. It stops
// when the service closure lets users register again (the announcement was withdrawn). A
// registration that is no longer pending is left as it is, so running it again rejects nothing
// more.
func (s *Service) RejectPendingForClosure(ctx context.Context) (int, error) {
	rejected := 0
	after := RegistrationPosition{}
	for {
		if err := s.registrationAllowed(ctx); !errors.Is(err, ErrServiceClosing) {
			return rejected, err
		}
		page, err := s.d.Repository.ListRegistrations(ctx, identity.RegistrationPending, after, closureRejectionBatch)
		if err != nil {
			return rejected, fmt.Errorf("identity: the pending registrations: %w", err)
		}
		for _, e := range page {
			now := s.d.Clock.Now()
			d := identity.RegistrationDecision{
				ID: s.d.IDs.NewID(), ApplicationID: e.Application.ID, Decision: identity.DecisionRejected,
				DecidedBy: identity.DecidedByServiceClosure, DecidedAt: now,
			}
			moved := false
			err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				var err error
				moved, err = s.d.Repository.TransitionPendingRegistrationState(ctx, tx, identity.UserRegistrationState{
					UserID: e.Application.UserID, State: identity.RegistrationRejected, Since: now,
				})
				if err != nil || !moved {
					return err
				}
				return s.d.Repository.InsertRegistrationDecision(ctx, tx, d)
			})
			if err != nil {
				return rejected, fmt.Errorf("identity: reject the registration of %s: %w", e.Application.UserID, err)
			}
			if moved {
				rejected++
			}
		}
		if len(page) < closureRejectionBatch {
			return rejected, nil
		}
		after = page[len(page)-1].Position()
	}
}
