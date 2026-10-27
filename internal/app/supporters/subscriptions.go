// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/supporters"
)

// The subscriptions of the users (ADR-0072 §6, §8) and of the administrators (§10). A user
// subscribes to one plan at a time, from the web client; the changes, cancellations, and
// resumptions go to the provider as commands and come back as the snapshot that the use case
// re-reads at once, so the state follows the same rules as for the webhooks.

// The errors of the subscriptions.
var (
	// ErrNotEligible is what every IneligibleError matches.
	ErrNotEligible = errors.New("supporters: the user cannot subscribe")
	// ErrNoLiveSubscription: the user has no subscription that goes on.
	ErrNoLiveSubscription = errors.New("supporters: the user has no subscription")
	// ErrSubscriptionState: the subscription is not in a state that allows the operation (a
	// change of plan needs active; a cancellation active or past due; a resumption canceling).
	ErrSubscriptionState = errors.New("supporters: the subscription's state does not allow it")
	// ErrSameChoice is supporters.ErrSameChoice.
	ErrSameChoice = supporters.ErrSameChoice
)

// Eligibility is why a user cannot subscribe (ADR-0072 §6).
type Eligibility string

// The reasons.
const (
	// IneligibleNotLocal: not a local user.
	IneligibleNotLocal Eligibility = "not_local"
	// IneligibleEmail: the primary address is not verified.
	IneligibleEmail Eligibility = "email_unverified"
	// IneligibleRestricted: the account is suspended, deleted, or its deletion is scheduled.
	IneligibleRestricted Eligibility = "restricted"
)

// IneligibleError reports why a user cannot subscribe.
type IneligibleError struct{ Reason Eligibility }

func (e *IneligibleError) Error() string {
	return "supporters: the user cannot subscribe: " + string(e.Reason)
}

// Is makes the error match ErrNotEligible.
func (e *IneligibleError) Is(target error) bool { return target == ErrNotEligible }

// eligible returns the user if they may subscribe.
func (s *Service) eligible(ctx context.Context, userID string) (User, error) {
	u, err := s.d.Users.SupporterUser(ctx, userID)
	switch {
	case errors.Is(err, ErrNotFound):
		return User{}, &IneligibleError{Reason: IneligibleNotLocal}
	case err != nil:
		return User{}, err
	case u.Suspended || u.Closed:
		return User{}, &IneligibleError{Reason: IneligibleRestricted}
	case !u.EmailVerified || u.Email == "":
		return User{}, &IneligibleError{Reason: IneligibleEmail}
	}
	return u, nil
}

// SubscriptionView is a subscription with its plans.
type SubscriptionView struct {
	SubscriptionRecord
	// Plan is the plan that the subscription pays for now, and Price its price.
	Plan  PlanRecord
	Price Price
	// ScheduledPrice is the price a scheduled change switches to, and ScheduledPlan its plan;
	// both nil without one.
	ScheduledPrice *Price
	ScheduledPlan  *PlanRecord
}

// view returns a subscription with its plan and prices.
func (s *Service) view(ctx context.Context, rec SubscriptionRecord) (SubscriptionView, error) {
	v := SubscriptionView{SubscriptionRecord: rec}
	var err error
	if v.Plan, err = s.d.Repository.FindPlan(ctx, nil, rec.Current.PlanID); err != nil {
		return SubscriptionView{}, err
	}
	if v.Price, err = s.d.Repository.FindPrice(ctx, nil, rec.Current.PriceID); err != nil {
		return SubscriptionView{}, err
	}
	if sc := rec.Current.Scheduled; sc != nil {
		p, err := s.d.Repository.FindPrice(ctx, nil, sc.PriceID)
		if err != nil {
			return SubscriptionView{}, err
		}
		v.ScheduledPrice = &p
		plan, err := s.d.Repository.FindPlan(ctx, nil, p.PlanID)
		if err != nil {
			return SubscriptionView{}, err
		}
		v.ScheduledPlan = &plan
	}
	return v, nil
}

// Eligibility returns why a user may not subscribe (ADR-0072 §6), "" for a user who may; a
// subscription that goes on is not a reason here (see OwnSubscription).
func (s *Service) Eligibility(ctx context.Context, userID string) (Eligibility, error) {
	_, err := s.eligible(ctx, userID)
	var ie *IneligibleError
	if errors.As(err, &ie) {
		return ie.Reason, nil
	}
	return "", err
}

// Returned queues the re-read of a user's live subscription from the provider (JobSync), which
// the web client asks for when the provider's checkout or portal returns to it (ADR-0072 §6),
// and returns the subscription as it is now. It fails as OwnSubscription does.
func (s *Service) Returned(ctx context.Context, userID string) (SubscriptionView, error) {
	v, err := s.OwnSubscription(ctx, userID)
	if err != nil {
		return SubscriptionView{}, err
	}
	cause := "return:" + s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Outbox.EnqueueTx(ctx, tx, SyncJob(v.ID, cause)) })
	if err != nil {
		return SubscriptionView{}, err
	}
	return v, nil
}

// OwnSubscription returns the live subscription of a user; ErrNoLiveSubscription without one,
// and ErrDisabled while the program is disabled.
func (s *Service) OwnSubscription(ctx context.Context, userID string) (SubscriptionView, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil {
		return SubscriptionView{}, err
	}
	if !enabled {
		return SubscriptionView{}, ErrDisabled
	}
	return s.liveView(ctx, userID)
}

func (s *Service) liveView(ctx context.Context, userID string) (SubscriptionView, error) {
	id, err := s.d.Repository.LiveSubscriptionID(ctx, nil, userID)
	if errors.Is(err, ErrNotFound) {
		return SubscriptionView{}, ErrNoLiveSubscription
	}
	if err != nil {
		return SubscriptionView{}, err
	}
	rec, err := s.d.Repository.FindSubscription(ctx, nil, id)
	if err != nil {
		return SubscriptionView{}, err
	}
	return s.view(ctx, rec)
}

// CheckoutInput starts a subscription.
type CheckoutInput struct {
	UserID   string
	PlanID   string
	Interval supporters.Interval
	// SuccessURL and CancelURL are where the provider's checkout returns to, on our origin.
	SuccessURL, CancelURL string
}

// StartedCheckout is a checkout that the browser goes to.
type StartedCheckout struct {
	SubscriptionID string
	RedirectURL    string
}

// StartCheckout starts a subscription to a plan (ADR-0072 §6): it records the pending
// subscription and returns the provider's checkout for it; starting again while it is pending
// makes a new checkout of the same subscription. It fails with ErrDisabled or
// ErrProviderMismatch when the program takes no subscriptions, with an *IneligibleError, with
// ErrPlanUnavailable for a plan that takes no subscriptions (ErrNotFound for an unknown one),
// with ErrLiveSubscription when the user has a subscription that is not pending, and with
// ErrServiceClosing from the announcement of the service closure on (ADR-0088 §2).
func (s *Service) StartCheckout(ctx context.Context, in CheckoutInput) (StartedCheckout, error) {
	if err := s.accepting(ctx); err != nil {
		return StartedCheckout{}, err
	}
	if err := s.subscribingAllowed(ctx); err != nil {
		return StartedCheckout{}, err
	}
	if !in.Interval.Valid() {
		return StartedCheckout{}, &supporters.PlanError{Field: "interval"}
	}
	u, err := s.eligible(ctx, in.UserID)
	if err != nil {
		return StartedCheckout{}, err
	}
	plan, err := s.d.Repository.FindPlan(ctx, nil, in.PlanID)
	if err != nil {
		return StartedCheckout{}, err
	}
	if !plan.Offered() {
		return StartedCheckout{}, ErrPlanUnavailable
	}
	price := plan.Prices[in.Interval]
	prov, name, now := s.d.Provider, s.d.Provider.Name(), s.now()
	subID := s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		live, err := s.d.Repository.LiveSubscriptionID(ctx, tx, u.ID)
		switch {
		case err == nil:
			rec, err := s.d.Repository.LockSubscription(ctx, tx, live)
			if err != nil {
				return err
			}
			if rec.Current.State != supporters.StatePending || rec.Link != "" {
				return ErrLiveSubscription
			}
			subID = rec.ID
			return nil
		case !errors.Is(err, ErrNotFound):
			return err
		}
		sub := Subscription{ID: subID, UserID: u.ID, PlanID: plan.ID, PriceID: price.ID, Provider: name, CreatedAt: now}
		first := TransitionRecord{
			ID: s.d.IDs.NewID(), SubscriptionID: subID, Reason: supporters.ReasonStarted, OccurredAt: now,
			Transition: supporters.Transition{State: supporters.StatePending, PlanID: plan.ID, PriceID: price.ID},
		}
		kind, at, _ := supporters.DeadlineOf(first.Transition, now)
		return s.d.Repository.InsertSubscription(ctx, tx, sub, first, &Deadline{SubscriptionID: subID, Kind: kind, DueAt: at})
	})
	if err != nil {
		return StartedCheckout{}, err
	}
	req := CheckoutRequest{
		LocalID: subID, Price: price.Ref, Email: u.Email, Locale: u.Locale, SuccessURL: in.SuccessURL, CancelURL: in.CancelURL,
		Key: "checkout:" + subID + ":" + s.d.IDs.NewID(),
	}
	if c, err := s.d.Repository.FindCustomer(ctx, nil, name, u.ID); err == nil {
		req.Customer, req.Email = c.Ref, ""
	} else if !errors.Is(err, ErrNotFound) {
		return StartedCheckout{}, err
	}
	co, err := prov.StartCheckout(ctx, req)
	if err != nil {
		return StartedCheckout{}, fmt.Errorf("supporters: start the checkout of %s: %w", subID, err)
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.InsertCheckout(ctx, tx, CheckoutRecord{
			ID: s.d.IDs.NewID(), SubscriptionID: subID, Provider: name, Ref: co.Ref, ExpiresAt: kernelTime(co.ExpiresAt), CreatedAt: now,
		})
	})
	if err != nil {
		return StartedCheckout{}, err
	}
	return StartedCheckout{SubscriptionID: subID, RedirectURL: co.RedirectURL}, nil
}

// resync re-reads a user's linked subscription from the provider after a command and applies
// it, so that the answer shows the new state.
func (s *Service) resync(ctx context.Context, rec SubscriptionRecord) (SubscriptionView, error) {
	snap, err := s.d.Provider.GetSubscription(ctx, rec.Link)
	if err != nil {
		return SubscriptionView{}, err
	}
	if snap.LocalID == "" {
		snap.LocalID = rec.ID
	}
	if _, err := s.SyncSubscription(ctx, s.d.Provider.Name(), snap); err != nil {
		return SubscriptionView{}, err
	}
	fresh, err := s.d.Repository.FindSubscription(ctx, nil, rec.ID)
	if err != nil {
		return SubscriptionView{}, err
	}
	return s.view(ctx, fresh)
}

// linkedLive returns the live subscription of a user that the provider has, in one of states.
func (s *Service) linkedLive(ctx context.Context, userID string, states ...supporters.State) (SubscriptionView, error) {
	if err := s.accepting(ctx); err != nil {
		return SubscriptionView{}, err
	}
	v, err := s.liveView(ctx, userID)
	if err != nil {
		return SubscriptionView{}, err
	}
	if v.Link == "" || !slices.Contains(states, v.Current.State) {
		return SubscriptionView{}, ErrSubscriptionState
	}
	return v, nil
}

// ChangePlan changes the plan or the interval of a user's active subscription (ADR-0072 §8):
// to a higher plan or to the yearly price at once with proration, and to a lower plan or to the
// monthly price at the end of the period; the role follows the snapshot. It fails with
// ErrSubscriptionState unless the subscription is active, with ErrPlanUnavailable for a plan
// that takes no subscriptions or of another currency, and with ErrSameChoice for no change.
func (s *Service) ChangePlan(ctx context.Context, userID, planID string, interval supporters.Interval) (SubscriptionView, error) {
	v, err := s.linkedLive(ctx, userID, supporters.StateActive)
	if err != nil {
		return SubscriptionView{}, err
	}
	to, err := s.d.Repository.FindPlan(ctx, nil, planID)
	if err != nil {
		return SubscriptionView{}, err
	}
	if !to.Offered() || to.Currency != v.Plan.Currency || !interval.Valid() {
		return SubscriptionView{}, ErrPlanUnavailable
	}
	price := to.Prices[interval]
	if price.ID == v.Current.PriceID {
		return SubscriptionView{}, ErrSameChoice
	}
	timing, err := supporters.ChangeTiming(
		supporters.Choice{Rank: v.Plan.Revision.Rank, Interval: v.Price.Interval}, supporters.Choice{Rank: to.Revision.Rank, Interval: interval},
	)
	if errors.Is(err, ErrSameChoice) {
		// The same plan and interval at a new price: the change applies at the period's end.
		timing, err = supporters.TimingPeriodEnd, nil
	}
	if err != nil {
		return SubscriptionView{}, err
	}
	if err := s.d.Provider.ChangePrice(ctx, v.Link, price.Ref, ChangeTiming(timing), changeKey(v, price.ID)); err != nil {
		return SubscriptionView{}, err
	}
	return s.resync(ctx, v.SubscriptionRecord)
}

// changeKey is the idempotency key of a change of v to the price toPriceID: the same for every
// attempt of one change, so that a retry after a failure part of the way (the difference charged,
// the plan not switched yet) does not charge again. It names the subscription, the price it
// pays now, the new price, and the end of the current period: a subscription cannot change from
// one price to another at once twice in one period, since it moves at once only up.
func changeKey(v SubscriptionView, toPriceID string) string {
	return "change:" + v.ID + ":" + v.Current.PriceID + ":" + toPriceID + ":" + strconv.FormatInt(v.Current.PeriodEndsAt.Unix(), 10)
}

// CancelScheduledChange withdraws the change of a user's subscription that waits for the end
// of the period. It fails with ErrSubscriptionState without one.
func (s *Service) CancelScheduledChange(ctx context.Context, userID string) (SubscriptionView, error) {
	v, err := s.linkedLive(ctx, userID, supporters.StateActive, supporters.StatePastDue)
	if err != nil {
		return SubscriptionView{}, err
	}
	if v.Current.Scheduled == nil {
		return SubscriptionView{}, ErrSubscriptionState
	}
	if err := s.d.Provider.CancelScheduledChange(ctx, v.Link, "unschedule:"+v.ID+":"+s.d.IDs.NewID()); err != nil {
		return SubscriptionView{}, err
	}
	return s.resync(ctx, v.SubscriptionRecord)
}

// Cancel cancels a user's subscription at the end of its period: the role stays until then
// (ADR-0072 §7, FR-911). It fails with ErrSubscriptionState unless it is active or past due.
func (s *Service) Cancel(ctx context.Context, userID string) (SubscriptionView, error) {
	v, err := s.linkedLive(ctx, userID, supporters.StateActive, supporters.StatePastDue)
	if err != nil {
		return SubscriptionView{}, err
	}
	if err := s.d.Provider.CancelAtPeriodEnd(ctx, v.Link, "cancel:"+v.ID+":"+s.d.IDs.NewID()); err != nil {
		return SubscriptionView{}, err
	}
	return s.resync(ctx, v.SubscriptionRecord)
}

// Resume withdraws the cancellation of a user's subscription. It fails with
// ErrSubscriptionState unless it is canceling.
func (s *Service) Resume(ctx context.Context, userID string) (SubscriptionView, error) {
	v, err := s.linkedLive(ctx, userID, supporters.StateCanceling)
	if err != nil {
		return SubscriptionView{}, err
	}
	if err := s.d.Provider.ResumeCancellation(ctx, v.Link, "resume:"+v.ID+":"+s.d.IDs.NewID()); err != nil {
		return SubscriptionView{}, err
	}
	return s.resync(ctx, v.SubscriptionRecord)
}

// PortalURL returns the provider's portal of a user's payment methods and invoices; it fails
// with ErrNoLiveSubscription for a user whom the provider does not know.
func (s *Service) PortalURL(ctx context.Context, userID, returnURL string) (string, error) {
	if err := s.accepting(ctx); err != nil {
		return "", err
	}
	c, err := s.d.Repository.FindCustomer(ctx, nil, s.d.Provider.Name(), userID)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNoLiveSubscription
	}
	if err != nil {
		return "", err
	}
	return s.d.Provider.PortalURL(ctx, c.Ref, returnURL)
}

// Subscriptions returns a page of the subscriptions for the administrators, newest first.
func (s *Service) Subscriptions(ctx context.Context, f SubscriptionFilter) ([]SubscriptionView, error) {
	recs, err := s.d.Repository.ListSubscriptions(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]SubscriptionView, len(recs))
	for i, r := range recs {
		if out[i], err = s.view(ctx, r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Subscription returns one subscription for the administrators.
func (s *Service) Subscription(ctx context.Context, id string) (SubscriptionView, error) {
	rec, err := s.d.Repository.FindSubscription(ctx, nil, id)
	if err != nil {
		return SubscriptionView{}, err
	}
	return s.view(ctx, rec)
}

// OwnSubscriptionsByID returns the subscriptions of a user among ids, as they are now; the
// unknown ones and those of other users are left out. The supporter notifications present
// theirs with it (ADR-0072 §16).
func (s *Service) OwnSubscriptionsByID(ctx context.Context, userID string, ids []string) ([]SubscriptionView, error) {
	out := []SubscriptionView{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		rec, err := s.d.Repository.FindSubscription(ctx, nil, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if rec.UserID != userID {
			continue
		}
		v, err := s.view(ctx, rec)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// CancelByStaff ends a live subscription at once without a refund (ADR-0072 §10): it expires
// here with the reason staff, which takes the role away, and the provider's subscription is
// canceled at once after the commit. It fails with ErrNotFound for an unknown subscription and
// with ErrNoLiveSubscription for one that ended. Audited.
func (s *Service) CancelByStaff(ctx context.Context, id, byUserID string) error {
	rec, err := s.d.Repository.FindSubscription(ctx, nil, id)
	if err != nil {
		return err
	}
	label := rec.UserID
	if u, err := s.d.Users.SupporterUser(ctx, rec.UserID); err == nil {
		label = u.Handle
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	plan, err := s.d.Repository.FindPlan(ctx, nil, rec.Current.PlanID)
	if err != nil {
		return err
	}
	now, auditID := s.now(), s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
		if err != nil {
			return err
		}
		if rec.Current.State.Final() {
			return ErrNoLiveSubscription
		}
		if err := s.close(ctx, tx, rec, supporters.ReasonStaff, now); err != nil {
			return err
		}
		if rec.Link != "" {
			if err := s.enqueueCommand(ctx, tx, rec.ID, CommandCancelNow, "", "staff"); err != nil {
				return err
			}
		}
		return tx.Emit(ctx, audit(auditID, byUserID, appmoderation.AuditSupporterSubscriptionCanceled,
			appmoderation.AuditTargetSupporterSubscription, rec.ID, plan.Revision.Name+" "+label, rec.ID, now))
	})
}

// RegisterProjectors subscribes the subscriptions to the accounts' lifecycle and suspensions
// (ADR-0072 §10) and to the closure of the service (ADR-0088 §2): the deletion of a local
// account, done or scheduled by a ban, ends its live subscription here at once (the role and the
// listing go) and cancels it at the provider without a refund; a suspension cancels it at the end
// of the period (S1), its role staying until then; the start of the notice period of the closure
// enqueues JobClosureNotice, and the closure of the server ends every live subscription as a
// deletion does, with the reason service_closed. Each runs in the transaction of its event; the
// provider's command runs after it.
func (s *Service) RegisterProjectors(r *uow.Projectors) {
	s.registerClosureProjectors(r)
	r.Register(account.EventLifecycleEventRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(account.LifecycleEventRecorded)
		if !ev.ClosesLocalAccount() {
			return nil
		}
		rec, ok, err := s.liveOfAccount(ctx, tx, ev.Event.AccountID)
		if err != nil || !ok {
			return err
		}
		if err := s.close(ctx, tx, rec, supporters.ReasonAccountClosed, s.now()); err != nil {
			return err
		}
		if rec.Link == "" {
			return nil
		}
		return s.enqueueCommand(ctx, tx, rec.ID, CommandCancelNow, "", "account_closed")
	})
	r.Register(appmoderation.EventAccountSuspensionChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appmoderation.AccountSuspensionChanged)
		if !ev.Suspended {
			return nil
		}
		rec, ok, err := s.liveOfAccount(ctx, tx, ev.AccountID)
		if err != nil || !ok {
			return err
		}
		switch rec.Current.State {
		case supporters.StateActive, supporters.StatePastDue, supporters.StatePending:
			return s.enqueueCommand(ctx, tx, rec.ID, CommandCancelAtPeriodEnd, supporters.ReasonAccountSuspended, "suspended:"+ev.ActionID)
		}
		return nil
	})
}

// cancelForClosure ends inside tx every live subscription at once, with the reason
// service_closed, and queues CancelNow at the provider for those linked to it (ADR-0088 §2). The
// command's idempotency key names the closure event, so a retried transaction queues it once.
func (s *Service) cancelForClosure(ctx context.Context, tx uow.Tx, closureEventID string) error {
	ids, err := s.d.Repository.LiveSubscriptionIDs(ctx, tx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, id := range ids {
		rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.close(ctx, tx, rec, supporters.ReasonServiceClosed, now); err != nil {
			return err
		}
		if rec.Link == "" {
			continue
		}
		if err := s.enqueueCommand(ctx, tx, rec.ID, CommandCancelNow, supporters.ReasonServiceClosed, "service_closed:"+closureEventID); err != nil {
			return err
		}
	}
	return nil
}

// LiveSubscriptionCount counts the subscriptions that go on, for the checks before the closure
// of the service (ADR-0088 §1).
func (s *Service) LiveSubscriptionCount(ctx context.Context) (int, error) {
	n, err := s.d.Repository.CountLiveSubscriptions(ctx, nil)
	return int(n), err
}

// liveOfAccount returns inside tx the live subscription of a local account's user.
func (s *Service) liveOfAccount(ctx context.Context, tx uow.Tx, accountID string) (SubscriptionRecord, bool, error) {
	if s.d.Accounts == nil {
		return SubscriptionRecord{}, false, nil
	}
	a, err := s.d.Accounts.FindByIDTx(ctx, tx, accountID)
	switch {
	case errors.Is(err, account.ErrNotFound):
		return SubscriptionRecord{}, false, nil
	case err != nil:
		return SubscriptionRecord{}, false, err
	case a.Local == nil || a.Local.UserID == "":
		return SubscriptionRecord{}, false, nil
	}
	id, err := s.d.Repository.LiveSubscriptionID(ctx, tx, a.Local.UserID)
	if errors.Is(err, ErrNotFound) {
		return SubscriptionRecord{}, false, nil
	}
	if err != nil {
		return SubscriptionRecord{}, false, err
	}
	rec, err := s.d.Repository.LockSubscription(ctx, tx, id)
	if err != nil {
		return SubscriptionRecord{}, false, err
	}
	return rec, true, nil
}

// EraseAccountData implements account.DataEraser: it deletes the listing and the provider's
// customers of an erased account's user (ADR-0072 §10); the subscriptions, their transitions,
// and their reversals stay for the accounting and the reconciliation with the provider.
func (s *Service) EraseAccountData(ctx context.Context, t account.ErasureTarget, _ string, _ int) (account.ErasureStep, error) {
	if t.UserID == "" {
		return account.ErasureStep{Done: true}, nil
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.EraseUser(ctx, tx, t.UserID) })
	return account.ErasureStep{Done: err == nil}, err
}

// RebuildProjections derives the projections of the module again, in one transaction (data-model
// §9).
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error { return repo.RebuildProjections(ctx, tx) })
	if err != nil {
		return fmt.Errorf("rebuild the supporters projections: %w", err)
	}
	return nil
}

// Subscriber returns the subscriber of a subscription for the administrators, and false for an
// erased account (ADR-0072 §10: "削除されたアカウント").
func (s *Service) Subscriber(ctx context.Context, userID string) (User, bool, error) {
	u, err := s.d.Users.SupporterUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return User{}, false, nil
	}
	return u, err == nil, err
}
