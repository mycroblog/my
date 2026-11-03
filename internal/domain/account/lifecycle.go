// SPDX-License-Identifier: AGPL-3.0-only

package account

import "time"

// LifecycleKind is the kind of a lifecycle event.
type LifecycleKind string

// Lifecycle event kinds (data-model §4.2, account_lifecycle_events.kind).
const (
	LifecycleDeactivated       LifecycleKind = "deactivated"
	LifecycleReactivated       LifecycleKind = "reactivated"
	LifecycleDeletionScheduled LifecycleKind = "deletion_scheduled"
	LifecycleDeletionCanceled  LifecycleKind = "deletion_canceled"
	LifecycleDeleted           LifecycleKind = "deleted"
	LifecycleRemoteDeleted     LifecycleKind = "remote_deleted"
)

// Initiator says who caused a lifecycle event.
type Initiator string

// Initiators (account_lifecycle_events.initiated_by).
const (
	InitiatedBySelf      Initiator = "self"
	InitiatedByModerator Initiator = "moderator"
	InitiatedBySystem    Initiator = "system"
	InitiatedByRemote    Initiator = "remote"
)

// LifecycleEvent is one transition on the lifecycle axis of an account (data-model §5.1).
type LifecycleEvent struct {
	ID                 string
	AccountID          string
	Kind               LifecycleKind
	InitiatedBy        Initiator
	ModerationActionID string     // "" unless a moderation action caused the event
	DeletionDueAt      *time.Time // set only for LifecycleDeletionScheduled
	OccurredAt         time.Time
}

// LifecycleStateName is a value of the lifecycle axis.
type LifecycleStateName string

// Lifecycle states (account_lifecycle_states.state).
const (
	StateActive            LifecycleStateName = "active"
	StateDeactivated       LifecycleStateName = "deactivated"
	StateDeletionScheduled LifecycleStateName = "deletion_scheduled"
	StateDeleted           LifecycleStateName = "deleted"
)

// LifecycleState is the current lifecycle state of an account (the account_lifecycle_states
// projection). An account without lifecycle events is active; its zero Since means
// "since it was created".
type LifecycleState struct {
	AccountID     string
	State         LifecycleStateName
	Since         time.Time
	DeletionDueAt *time.Time // set only in StateDeletionScheduled
}

// Active reports whether the account is active. Deactivated accounts and accounts that are
// scheduled for deletion or deleted are presented to no one.
func (s LifecycleState) Active() bool { return s.State == StateActive }

// DeletionScheduled reports whether the account is waiting for its scheduled deletion.
func (s LifecycleState) DeletionScheduled() bool { return s.State == StateDeletionScheduled }

// InitialLifecycleState is the state of an account before its first lifecycle event.
func InitialLifecycleState(accountID string) LifecycleState {
	return LifecycleState{AccountID: accountID, State: StateActive}
}

// Apply returns the state after event e. Deletion is final: once deleted, later events do not
// change the state. Folding an account's events in order with Apply from InitialLifecycleState
// yields its current state, which is how the projection is both updated and rebuilt.
func (s LifecycleState) Apply(e LifecycleEvent) LifecycleState {
	if s.State == StateDeleted {
		return s
	}
	next := LifecycleState{AccountID: s.AccountID, Since: e.OccurredAt}
	switch e.Kind {
	case LifecycleDeactivated:
		next.State = StateDeactivated
	case LifecycleReactivated, LifecycleDeletionCanceled:
		next.State = StateActive
	case LifecycleDeletionScheduled:
		next.State = StateDeletionScheduled
		if e.DeletionDueAt != nil {
			due := *e.DeletionDueAt
			next.DeletionDueAt = &due
		}
	case LifecycleDeleted, LifecycleRemoteDeleted:
		next.State = StateDeleted
	default:
		return s
	}
	return next
}
