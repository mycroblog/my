// SPDX-License-Identifier: AGPL-3.0-only

package diagnostics

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/diagnostics"
)

// Domain events of the diagnostics module. The administrators' notifications and the instance
// webhooks subscribe to the first three (ADR-0103 §8): a new group notifies when it is created by
// a report with a session (GroupCreated.Group.Unauthenticated is false) or, when it was created
// without one, when it is confirmed.
const (
	// EventGroupCreated follows the first occurrence of a fingerprint, in the transaction that
	// inserts its group.
	EventGroupCreated = "diagnostics.error_group_created"
	// EventGroupRegressed follows the recurrence of a resolved group with another version, in
	// the transaction that records its regressed event.
	EventGroupRegressed = "diagnostics.error_group_regressed"
	// EventGroupConfirmed follows the first occurrence with a session of a group that a report
	// without one created (ADR-0103 §6), once per group.
	EventGroupConfirmed = "diagnostics.error_group_confirmed"
	// EventGroupStateChanged follows a resolve, a mute, or an unmute, with the new state.
	EventGroupStateChanged = "diagnostics.error_group_state_changed"
)

// GroupCreated carries the new group, whose state is open. Group.Unauthenticated tells that its
// first occurrence came without a session and notifies nobody (GroupConfirmed follows the first
// one with a session).
type GroupCreated struct{ Group Group }

// EventType implements uow.Event.
func (GroupCreated) EventType() string { return EventGroupCreated }

// GroupRegressed carries the group that regressed and its regressed event, whose Version is the
// version of the recurrence. Unauthenticated tells that the recurrence was a web report sent
// without a session.
type GroupRegressed struct {
	Group           Group
	Event           Event
	Unauthenticated bool
}

// EventType implements uow.Event.
func (GroupRegressed) EventType() string { return EventGroupRegressed }

// GroupConfirmed carries a group that a report without a session created, at the first
// occurrence with one.
type GroupConfirmed struct {
	Group       Group
	ConfirmedAt time.Time
}

// EventType implements uow.Event.
func (GroupConfirmed) EventType() string { return EventGroupConfirmed }

// GroupStateChanged carries the event of a resolve, a mute, or an unmute and the state it
// leads to.
type GroupStateChanged struct {
	Event Event
	Head  Head
}

// EventType implements uow.Event.
func (GroupStateChanged) EventType() string { return EventGroupStateChanged }

// RegisterProjectors registers the projector of the module's own projection, error_group_states:
// open for a new group and a regression, and the new state for a change.
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventGroupCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		g := e.(GroupCreated).Group
		return repo.UpsertHead(ctx, tx, Head{GroupID: g.ID, State: diagnostics.StateOpen, Since: g.FirstSeenAt})
	})
	r.Register(EventGroupRegressed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(GroupRegressed).Event
		return repo.UpsertHead(ctx, tx, Head{GroupID: ev.GroupID, State: diagnostics.StateOpen, Since: ev.OccurredAt})
	})
	r.Register(EventGroupStateChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertHead(ctx, tx, e.(GroupStateChanged).Head)
	})
}

// RebuildProjections derives error_group_states again from the groups and their events, in one
// transaction (data-model §9).
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	if err := u.Tx(ctx, func(tx uow.Tx) error { return repo.RebuildHeads(ctx, tx) }); err != nil {
		return fmt.Errorf("rebuild the diagnostics projections: %w", err)
	}
	return nil
}
