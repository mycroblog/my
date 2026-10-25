// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"time"
)

// The state of a subscription (ADR-0072 §7, T1, G1) follows from the provider's snapshot,
// re-read on every signal, and from our own facts: a full refund or a chargeback, and the
// deadlines that our Scheduler keeps. Derive is a function of what it is given, so the order,
// duplicates, and losses of the provider's events change nothing; a transition is recorded only
// when the state or what the role depends on changes (Transition.Same).

// State is the state of a subscription (supporter_subscription_transitions.state).
type State string

// The states.
const (
	// StatePending: started; the provider has no subscription yet or its first payment is not
	// made. No role.
	StatePending State = "pending"
	// StateActive: paid (a trial included), without a cancellation. The role.
	StateActive State = "active"
	// StateCanceling: paid, canceled at the end of the period. The role until then.
	StateCanceling State = "canceling"
	// StatePastDue: a payment failed less than GracePeriod ago. The role.
	StatePastDue State = "past_due"
	// StateLapsed: a payment failed more than GracePeriod ago while the provider still retries,
	// or the subscription is paused. No role.
	StateLapsed State = "lapsed"
	// StateExpired: the provider's subscription ended. No role; final.
	StateExpired State = "expired"
	// StateRevoked: a full refund or a chargeback. No role; final, even when the dispute is won.
	StateRevoked State = "revoked"
	// StateAbandoned: ended before its first payment. No role; final.
	StateAbandoned State = "abandoned"
)

// States returns every state, in the order of the schema's CHECK.
func States() []State {
	return []State{StatePending, StateActive, StateCanceling, StatePastDue, StateLapsed, StateExpired, StateRevoked, StateAbandoned}
}

// Valid reports whether s is one of the states.
func (s State) Valid() bool {
	switch s {
	case StatePending, StateActive, StateCanceling, StatePastDue, StateLapsed, StateExpired, StateRevoked, StateAbandoned:
		return true
	}
	return false
}

// Final reports whether nothing changes a subscription in s any more.
func (s State) Final() bool { return s == StateExpired || s == StateRevoked || s == StateAbandoned }

// Live reports whether a subscription in s goes on (ADR-0072 §2 "続いている加入"): a user has at
// most one, and the program is not disabled while one exists.
func (s State) Live() bool { return s.Valid() && !s.Final() }

// Member reports whether a subscription in s holds the plan's role (ADR-0072 §12 "加入中"): the
// member lists, the counts by plan, and the supporter directory count these.
func (s State) Member() bool { return s == StateActive || s == StateCanceling || s == StatePastDue }

// HasPeriod reports whether a transition to s has the end of a billing period.
func (s State) HasPeriod() bool { return s != StatePending && s != StateAbandoned }

// Reason is why a transition was recorded (supporter_subscription_transitions.reason).
type Reason string

// The reasons.
const (
	// ReasonStarted: the user started the subscription.
	ReasonStarted Reason = "started"
	// ReasonSynced: the provider's snapshot changed it, or a deadline passed.
	ReasonSynced Reason = "synced"
	// ReasonGraceExpired: GracePeriod passed after a failed payment.
	ReasonGraceExpired Reason = "grace_expired"
	// ReasonReversed: a full refund or a chargeback.
	ReasonReversed Reason = "reversed"
	// ReasonPlanRetired: the plan was retired.
	ReasonPlanRetired Reason = "plan_retired"
	// ReasonAccountClosed: the user's account was deleted, or its deletion scheduled by a ban.
	ReasonAccountClosed Reason = "account_closed"
	// ReasonAccountSuspended: the user's account was suspended.
	ReasonAccountSuspended Reason = "account_suspended"
	// ReasonStaff: a staff member canceled the subscription.
	ReasonStaff Reason = "staff"
	// ReasonServiceClosed: the service closure (ADR-0088 §2): its announcement cancels the
	// subscriptions at the end of their periods, and the closure of the server ends them at once.
	ReasonServiceClosed Reason = "service_closed"
)

// Reasons returns every reason, in the order of the schema's CHECK.
func Reasons() []Reason {
	return []Reason{
		ReasonStarted, ReasonSynced, ReasonGraceExpired, ReasonReversed, ReasonPlanRetired, ReasonAccountClosed,
		ReasonAccountSuspended, ReasonStaff, ReasonServiceClosed,
	}
}

// EndReason is why a subscription lost the plan's role, as the subscriber is told
// (supporter_subscription_ended, ADR-0072 §16).
type EndReason string

// The reasons of an end.
const (
	// EndLapsed: the payments stopped: a payment failed more than GracePeriod ago, or the
	// subscription is paused.
	EndLapsed EndReason = "lapsed"
	// EndReversed: a payment was refunded in full or charged back.
	EndReversed EndReason = "reversed"
	// EndPlanRetired: the plan was retired.
	EndPlanRetired EndReason = "plan_retired"
	// EndAccountSuspended: the account was suspended.
	EndAccountSuspended EndReason = "account_suspended"
	// EndAccountClosed: the account was deleted.
	EndAccountClosed EndReason = "account_closed"
	// EndStaff: an administrator canceled it.
	EndStaff EndReason = "staff"
	// EndCanceled: it was canceled, or it ended at the provider.
	EndCanceled EndReason = "canceled"
)

// EndReasonOf returns why a transition to state for reason ended the subscription; "" when state
// is not lapsed, expired, or revoked.
func EndReasonOf(state State, reason Reason) EndReason {
	switch state {
	case StateLapsed:
		return EndLapsed
	case StateRevoked:
		return EndReversed
	case StateExpired:
		switch reason {
		case ReasonPlanRetired:
			return EndPlanRetired
		case ReasonAccountSuspended:
			return EndAccountSuspended
		case ReasonAccountClosed:
			return EndAccountClosed
		case ReasonStaff:
			return EndStaff
		case ReasonReversed:
			return EndReversed
		}
		return EndCanceled
	}
	return ""
}

// GracePeriod is how long after the first failed payment a past-due subscription keeps its role
// (ADR-0072 §7, G1). It does not depend on the provider's retries.
const GracePeriod = 7 * 24 * time.Hour

// CheckoutTimeout is how long a subscription may stay pending before the Scheduler re-reads it
// and closes it when the provider has nothing.
const CheckoutTimeout = 24 * time.Hour

// ProviderStatus is the provider's state of a subscription, normalized by the port (ADR-0074
// §1); the names are those of supporters.SubscriptionStatus in the application layer.
type ProviderStatus string

// The provider's states.
const (
	ProviderIncomplete        ProviderStatus = "incomplete"
	ProviderIncompleteExpired ProviderStatus = "incomplete_expired"
	ProviderActive            ProviderStatus = "active"
	ProviderPastDue           ProviderStatus = "past_due"
	ProviderUnpaid            ProviderStatus = "unpaid"
	ProviderPaused            ProviderStatus = "paused"
	ProviderCanceled          ProviderStatus = "canceled"
)

// Snapshot is the provider's state of a subscription in our terms: its prices resolved to ours.
type Snapshot struct {
	Status ProviderStatus
	// PlanID and PriceID are our plan and price of the provider's price; "" when the price is not
	// one of ours, which keeps the plan and the price of the previous transition.
	PlanID, PriceID string
	PeriodEnd       time.Time
	// CancelAtPeriodEnd reports a cancellation at PeriodEnd.
	CancelAtPeriodEnd bool
	// ScheduledPriceID is our price that a change waiting for PeriodEnd switches to; "" without
	// one, or when the price is not ours.
	ScheduledPriceID string
	// PaymentFailedAt is when the payment that is still due failed first; zero without one.
	PaymentFailedAt time.Time
}

// Grace is the grace period of a past-due subscription (supporter_grace_periods).
type Grace struct {
	FailedAt time.Time
	EndsAt   time.Time
}

// ScheduledChange is a change to a lower plan or to the monthly price that waits for the end of
// the period (supporter_scheduled_changes).
type ScheduledChange struct {
	PriceID     string
	EffectiveAt time.Time
}

// Transition is the state of a subscription after a change
// (supporter_subscription_transitions and its child rows).
type Transition struct {
	State State
	// PlanID and PriceID are the plan and the price that the subscription pays for.
	PlanID, PriceID string
	// PeriodEndsAt is the end of the current period; zero for pending and abandoned.
	PeriodEndsAt time.Time
	// Grace is set for a past-due subscription.
	Grace *Grace
	// Scheduled is set while a change waits for the end of the period.
	Scheduled *ScheduledChange
}

// Same reports whether t and u have the same state and the same plan, price, period end, grace,
// and scheduled change: a new snapshot that changes none of them records nothing.
func (t Transition) Same(u Transition) bool {
	if t.State != u.State || t.PlanID != u.PlanID || t.PriceID != u.PriceID || !t.PeriodEndsAt.Equal(u.PeriodEndsAt) {
		return false
	}
	if (t.Grace == nil) != (u.Grace == nil) || (t.Grace != nil && !t.Grace.FailedAt.Equal(u.Grace.FailedAt)) {
		return false
	}
	if (t.Scheduled == nil) != (u.Scheduled == nil) {
		return false
	}
	return t.Scheduled == nil || (t.Scheduled.PriceID == u.Scheduled.PriceID && t.Scheduled.EffectiveAt.Equal(u.Scheduled.EffectiveAt))
}

// HoldsRole reports whether the subscription holds the role of its plan.
func (t Transition) HoldsRole() bool { return t.State.Member() }

// Facts are our own facts that the state follows besides the snapshot.
type Facts struct {
	Now time.Time
	// StartedAt is when the subscription was started.
	StartedAt time.Time
	// Reversed reports a recorded full refund or chargeback of one of its payments.
	Reversed bool
}

// Derive returns the state that follows from prev, the provider's snapshot (nil when the
// provider has no subscription of ours yet), and the facts (ADR-0072 §7), with the reason of the
// transition when it differs from prev:
//
//   - a final state never changes;
//   - a full refund or a chargeback revokes the subscription;
//   - without a snapshot, a pending subscription is abandoned CheckoutTimeout after its start;
//   - incomplete is pending, incomplete_expired abandoned, canceled expired, and paused lapsed;
//   - active is canceling when a cancellation waits for the end of the period;
//   - past_due and unpaid are past due until GracePeriod after the first failed payment (the
//     snapshot's, or the one of prev, or now when neither has one), and lapsed after it; a
//     lapsed subscription stays lapsed until a payment succeeds;
//   - a state the port does not know is lapsed.
func Derive(prev Transition, snap *Snapshot, f Facts) (Transition, Reason) {
	if prev.State.Final() {
		return prev, ""
	}
	next := prev
	next.Grace, next.Scheduled = nil, nil
	if snap != nil {
		if snap.PriceID != "" {
			next.PlanID, next.PriceID = snap.PlanID, snap.PriceID
		}
		next.PeriodEndsAt = snap.PeriodEnd
	}
	reason := ReasonSynced
	switch {
	case f.Reversed:
		next.State, reason = StateRevoked, ReasonReversed
	case snap == nil:
		if prev.State == StatePending && !f.Now.Before(f.StartedAt.Add(CheckoutTimeout)) {
			next.State = StateAbandoned
		}
	default:
		switch snap.Status {
		case ProviderIncomplete:
			next.State = StatePending
		case ProviderIncompleteExpired:
			next.State = StateAbandoned
		case ProviderActive:
			next.State = StateActive
			if snap.CancelAtPeriodEnd {
				next.State = StateCanceling
			}
		case ProviderPastDue, ProviderUnpaid:
			next.State, reason = pastDue(prev, snap, f.Now)
			if next.State == StatePastDue {
				failed := failedAt(prev, snap, f.Now)
				next.Grace = &Grace{FailedAt: failed, EndsAt: failed.Add(GracePeriod)}
			}
		case ProviderCanceled:
			next.State = StateExpired
		default: // paused, and a state that the port does not know
			next.State = StateLapsed
		}
		if snap.ScheduledPriceID != "" && snap.ScheduledPriceID != next.PriceID && next.State.Live() && next.State != StatePending {
			next.Scheduled = &ScheduledChange{PriceID: snap.ScheduledPriceID, EffectiveAt: snap.PeriodEnd}
		}
	}
	if !next.State.HasPeriod() {
		next.PeriodEndsAt = time.Time{}
	} else if next.PeriodEndsAt.IsZero() {
		// A refund of a subscription that never had a snapshot; it ends now.
		next.PeriodEndsAt = f.Now
	}
	if next.Same(prev) {
		return prev, ""
	}
	return next, reason
}

// failedAt returns when the payment that is due failed first: the snapshot's time, else the one
// of prev's grace, else now, when we see the failure first.
func failedAt(prev Transition, snap *Snapshot, now time.Time) time.Time {
	switch {
	case !snap.PaymentFailedAt.IsZero():
		return snap.PaymentFailedAt
	case prev.Grace != nil:
		return prev.Grace.FailedAt
	default:
		return now
	}
}

// pastDue returns the state of a subscription whose payment is due.
func pastDue(prev Transition, snap *Snapshot, now time.Time) (State, Reason) {
	if prev.State == StateLapsed && snap.PaymentFailedAt.IsZero() {
		return StateLapsed, ReasonSynced
	}
	if now.Before(failedAt(prev, snap, now).Add(GracePeriod)) {
		return StatePastDue, ReasonSynced
	}
	if prev.State == StatePastDue {
		return StateLapsed, ReasonGraceExpired
	}
	return StateLapsed, ReasonSynced
}

// Close returns the transition that ends prev for one of our own reasons (an account closed, a
// staff member's cancellation, a retired plan's pending subscription): expired, or abandoned when
// it never started. It reports false when prev has ended already.
func Close(prev Transition, now time.Time) (Transition, bool) {
	if prev.State.Final() {
		return prev, false
	}
	next := Transition{State: StateExpired, PlanID: prev.PlanID, PriceID: prev.PriceID, PeriodEndsAt: prev.PeriodEndsAt}
	if prev.State == StatePending {
		next = Transition{State: StateAbandoned, PlanID: prev.PlanID, PriceID: prev.PriceID}
	} else if next.PeriodEndsAt.IsZero() || next.PeriodEndsAt.After(now) {
		next.PeriodEndsAt = now
	}
	return next, true
}

// Deadline is when the Scheduler re-reads a subscription (supporter_subscription_deadlines).
type Deadline string

// The deadlines.
const (
	// DeadlineCheckout: CheckoutTimeout after a pending subscription started.
	DeadlineCheckout Deadline = "checkout"
	// DeadlinePeriodEnd: the end of the period of an active or canceling subscription, to catch a
	// renewal or the end that a lost event did not bring.
	DeadlinePeriodEnd Deadline = "period_end"
	// DeadlineGrace: the end of the grace period of a past-due subscription.
	DeadlineGrace Deadline = "grace"
)

// DeadlineOf returns the deadline of a subscription in t, started at startedAt, and false when t
// has none (lapsed and final states, which the periodic reconciliation re-reads).
func DeadlineOf(t Transition, startedAt time.Time) (Deadline, time.Time, bool) {
	switch t.State {
	case StatePending:
		return DeadlineCheckout, startedAt.Add(CheckoutTimeout), true
	case StateActive, StateCanceling:
		return DeadlinePeriodEnd, t.PeriodEndsAt, true
	case StatePastDue:
		if t.Grace != nil {
			return DeadlineGrace, t.Grace.EndsAt, true
		}
	}
	return "", time.Time{}, false
}

// RoleChange is what a transition does to the roles of the subscriber.
type RoleChange struct {
	// Revoke and Grant are plans whose roles to revoke and grant; both set is a change of plan,
	// whose badge choice moves to the new role (ADR-0071 §2).
	Revoke, Grant string
}

// RoleChangeOf returns what moving from prev to next does to the roles: the role of the plan of
// a member state is held, the others are not.
func RoleChangeOf(prev, next Transition) RoleChange {
	var c RoleChange
	before, after := "", ""
	if prev.HoldsRole() {
		before = prev.PlanID
	}
	if next.HoldsRole() {
		after = next.PlanID
	}
	if before != after {
		c.Revoke, c.Grant = before, after
	}
	return c
}
