// SPDX-License-Identifier: AGPL-3.0-only

package federation

import "time"

// This file holds what the service closure (FR-809, ADR-0088 §2, §3) asks of federation: once
// the server is closed, it sends the Delete of every local actor and nothing else, whatever the
// federation mode, and records when those deliveries have ended.

// ClosureSends reports whether a closed server that delivers its Deletes sends the outbound
// activity a: only the Delete of a local actor, which goes to the actor's followers and to every
// known shared inbox whatever the federation mode (the exception to ADR-0076's off). Everything
// else that was queued is skipped when it runs. The domain blocks still apply
// (Deliverable).
func ClosureSends(a OutboundActivity) bool {
	return a.Type == ActivityDelete && a.ObjectKind == OutboundAccount
}

// RecordsFanOutEnd reports whether the end of the fan-out of a is recorded
// (outbound_fanout_completions): for the Delete of a local actor, which the closure waits for,
// and for the Move, which the closure's checks count while it is sent (ADR-0089).
func RecordsFanOutEnd(a OutboundActivity) bool {
	return ClosureSends(a) || a.ObjectKind == OutboundMove
}

// ClosureDelete is the Delete of a local actor that the service closure sends, one per account
// (closure_deletes).
type ClosureDelete struct {
	AccountID          string
	OutboundActivityID string
	RecordedAt         time.Time
}

// ClosureDeliveryState is where one delivery of the closure's Deletes is.
type ClosureDeliveryState string

// States of the deliveries of the closure's Deletes (ADR-0088 §3). Every state but
// ClosureDeliveryPending has ended.
const (
	// ClosureDeliveryPending has not been attempted yet, or failed and will be attempted again.
	ClosureDeliveryPending ClosureDeliveryState = "pending"
	// ClosureDeliverySucceeded was accepted by the inbox.
	ClosureDeliverySucceeded ClosureDeliveryState = "succeeded"
	// ClosureDeliveryPermanentFailure was refused for good.
	ClosureDeliveryPermanentFailure ClosureDeliveryState = "permanent_failure"
	// ClosureDeliverySkipped was not sent: the domain was suspended or the server unavailable.
	ClosureDeliverySkipped ClosureDeliveryState = "skipped"
	// ClosureDeliveryDead used up its attempts: its job is a dead job, which is not run again
	// unless an operator requeues it.
	ClosureDeliveryDead ClosureDeliveryState = "dead"
)

// ClosureDeliveryStates returns every state, in order.
func ClosureDeliveryStates() []ClosureDeliveryState {
	return []ClosureDeliveryState{
		ClosureDeliveryPending, ClosureDeliverySucceeded, ClosureDeliveryPermanentFailure, ClosureDeliverySkipped, ClosureDeliveryDead,
	}
}

// ClosureDeliveryStateOf returns the state of a delivery whose latest attempt had outcome ("" for
// none yet) and that has used up its attempts or not.
func ClosureDeliveryStateOf(outcome DeliveryOutcome, exhausted bool) ClosureDeliveryState {
	switch outcome {
	case DeliverySucceeded:
		return ClosureDeliverySucceeded
	case DeliveryPermanentFailure:
		return ClosureDeliveryPermanentFailure
	case DeliverySkipped:
		return ClosureDeliverySkipped
	case DeliveryRetryableFailure:
		if exhausted {
			return ClosureDeliveryDead
		}
	}
	return ClosureDeliveryPending
}

// ClosureProgress is how far the closure's Deletes have gone (ADR-0088 §3).
type ClosureProgress struct {
	// Accounts is the number of local accounts whose Delete is recorded.
	Accounts int
	// FanOuts is the number of those Deletes whose fan-out has not ended.
	FanOuts int
	// Deliveries counts their deliveries by state.
	Deliveries map[ClosureDeliveryState]int
}

// Pending is the number of deliveries that have not ended.
func (p ClosureProgress) Pending() int { return p.Deliveries[ClosureDeliveryPending] }

// Ended reports whether every recorded Delete has been fanned out and every delivery has ended.
func (p ClosureProgress) Ended() bool { return p.FanOuts == 0 && p.Pending() == 0 }
