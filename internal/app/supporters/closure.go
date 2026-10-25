// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/supporters"
)

// The notice period of the service closure (ADR-0088 §2): from the announcement on, nobody
// starts a subscription, and the start of the notice period cancels every live subscription at
// the end of its period, as the retirement of a plan does (ADR-0072 §9), and tells each
// subscriber who had paid. The cancellation runs in JobClosureNotice, out of the transaction of
// the announcement, since its work grows with the subscriptions.

// ErrServiceClosing: the closure of the service is announced, which stops new subscriptions.
var ErrServiceClosing = errors.New("supporters: the closure of the service is announced")

// ServiceClosure reports whether the state of the service closure lets users subscribe:
// *instance.Service implements it.
type ServiceClosure interface {
	SubscribingAllowed(ctx context.Context) (bool, error)
}

// JobClosureNotice is the job that cancels every live subscription at the end of its period when
// the notice period of the service closure starts.
const JobClosureNotice = "supporters.closure_notice"

// closureNoticeBatch is how many subscriptions one transaction of JobClosureNotice handles.
const closureNoticeBatch = 100

// closureNoticePayload is the payload of JobClosureNotice.
type closureNoticePayload struct {
	// ClosureEventID is the announcement that started the notice period.
	ClosureEventID string `json:"closure_event_id"`
}

// EventClosureNoticeCancellations follows a batch of the cancellations of JobClosureNotice.
const EventClosureNoticeCancellations = "supporters.closure_notice_cancellations"

// ClosureNoticeCancellations are subscriptions that the announcement of the service closure
// cancels at the end of their periods, and that had been paid for (every live one but a pending
// one), with the name of each one's plan.
type ClosureNoticeCancellations struct {
	// ClosureEventID is the announcement that started the notice period.
	ClosureEventID string
	Subscriptions  []ClosureNoticeCancellation
	At             time.Time
}

// ClosureNoticeCancellation is a subscription that the announcement of the service closure
// cancels.
type ClosureNoticeCancellation struct {
	SubscriptionID string
	UserID         string
	// AccountID is the subscriber's local account; "" when it has none.
	AccountID string
	PlanName  string
}

// EventType implements uow.Event.
func (ClosureNoticeCancellations) EventType() string { return EventClosureNoticeCancellations }

// subscribingAllowed fails with ErrServiceClosing while the service closure stops new
// subscriptions; without a ServiceClosure it allows them.
func (s *Service) subscribingAllowed(ctx context.Context) error {
	if s.d.Closure == nil {
		return nil
	}
	ok, err := s.d.Closure.SubscribingAllowed(ctx)
	switch {
	case err != nil:
		return fmt.Errorf("supporters: the service closure: %w", err)
	case !ok:
		return ErrServiceClosing
	}
	return nil
}

// registerClosureProjectors subscribes the subscriptions to the service closure (ADR-0088 §2):
// the start of the notice period enqueues JobClosureNotice, and the closure of the server ends
// every live subscription at once (cancelForClosure).
func (s *Service) registerClosureProjectors(r *uow.Projectors) {
	r.Register(appinstance.EventClosureRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appinstance.ClosureRecorded)
		switch {
		case ev.Closes():
			return s.cancelForClosure(ctx, tx, ev.Event.ID)
		case ev.Starts():
			b, err := json.Marshal(closureNoticePayload{ClosureEventID: ev.Event.ID})
			if err != nil {
				return err
			}
			return s.d.Outbox.EnqueueTx(ctx, tx, queue.NewJob{Type: JobClosureNotice, Payload: b, IdempotencyKey: JobClosureNotice + ":" + ev.Event.ID})
		}
		return nil
	})
}

// CancelForClosureNotice is the work of JobClosureNotice: in batches, it cancels every live
// subscription as the retirement of a plan does, with the reason service_closed: a pending one
// is abandoned, a lapsed one is canceled now at the provider, and every other one is canceled
// at the end of its period, its role staying until then. Each batch emits
// ClosureNoticeCancellations, by which the subscribers are told. The commands' idempotency keys
// and the notifications' dedupe keys name the announcement, so a job run again changes nothing
// more.
func (s *Service) CancelForClosureNotice(ctx context.Context, closureEventID string) error {
	var ids []string
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		ids, err = s.d.Repository.LiveSubscriptionIDs(ctx, tx)
		return err
	})
	if err != nil {
		return fmt.Errorf("supporters: the live subscriptions: %w", err)
	}
	cause := "closure_notice:" + closureEventID
	for start := 0; start < len(ids); start += closureNoticeBatch {
		batch := ids[start:min(start+closureNoticeBatch, len(ids))]
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			now := s.now()
			out := ClosureNoticeCancellations{ClosureEventID: closureEventID, At: now}
			for _, id := range batch {
				rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
				if err != nil {
					return err
				}
				if !rec.Current.State.Live() {
					continue
				}
				switch rec.Current.State {
				case supporters.StatePending:
					if err := s.close(ctx, tx, rec, supporters.ReasonServiceClosed, now); err != nil {
						return err
					}
					continue
				case supporters.StateLapsed:
					err = s.enqueueCommand(ctx, tx, rec.ID, CommandCancelNow, supporters.ReasonServiceClosed, cause)
				default:
					err = s.enqueueCommand(ctx, tx, rec.ID, CommandCancelAtPeriodEnd, supporters.ReasonServiceClosed, cause)
				}
				if err != nil {
					return err
				}
				accountID, err := s.accountOf(ctx, tx, rec.UserID)
				if err != nil {
					return err
				}
				plan, err := s.d.Repository.FindPlan(ctx, tx, rec.Current.PlanID)
				if err != nil {
					return err
				}
				out.Subscriptions = append(out.Subscriptions, ClosureNoticeCancellation{
					SubscriptionID: rec.ID, UserID: rec.UserID, AccountID: accountID, PlanName: plan.Revision.Name,
				})
			}
			if len(out.Subscriptions) == 0 {
				return nil
			}
			return tx.Emit(ctx, out)
		})
		if err != nil {
			return fmt.Errorf("supporters: cancel the subscriptions for the service closure: %w", err)
		}
	}
	return nil
}

// registerClosureJobs registers the handler of JobClosureNotice.
func (s *Service) registerClosureJobs(h *queue.JobHandlerRegistry) {
	h.Register(JobClosureNotice, func(ctx context.Context, j queue.Job) queue.Result {
		var p closureNoticePayload
		if err := json.Unmarshal(j.Payload, &p); err != nil || p.ClosureEventID == "" {
			return queue.Fail(fmt.Errorf("supporters: the payload of %s: %w", j.ID, err))
		}
		if err := s.CancelForClosureNotice(ctx, p.ClosureEventID); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
}
