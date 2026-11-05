// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// Event types of the account module, for uow.Projectors.
const (
	EventLocalAccountCreated    = "account.local_account_created"
	EventProfileRevised         = "account.profile_revised"
	EventLifecycleEventRecorded = "account.lifecycle_event_recorded"
)

// LocalAccountCreated is emitted when a local account and its subtype row are inserted.
type LocalAccountCreated struct {
	Account account.Account
	Local   account.LocalAccount
}

// EventType implements uow.Event.
func (LocalAccountCreated) EventType() string { return EventLocalAccountCreated }

// ProfileRevised is emitted when a profile revision is inserted.
type ProfileRevised struct {
	Revision account.ProfileRevision
	// IndexableChanged reports that this revision turned indexable on or off.
	IndexableChanged bool
}

// EventType implements uow.Event.
func (ProfileRevised) EventType() string { return EventProfileRevised }

// Federates reports whether the revision is sent to the account's remote followers as an
// Update (federation.md §7.1): a revision of a local profile after the first one, which the
// account is created with and no one follows yet. Remote revisions come from the actor's own
// server.
func (e ProfileRevised) Federates() bool { return e.Revision.Remote == nil && e.Revision.Seq > 1 }

// LifecycleEventRecorded is emitted when a lifecycle event is inserted.
type LifecycleEventRecorded struct {
	Event account.LifecycleEvent
}

// EventType implements uow.Event.
func (LifecycleEventRecorded) EventType() string { return EventLifecycleEventRecorded }

// Deletes reports whether the event deletes the account: a local account's deletion or a
// remote actor's deletion on its server. Deletion is final, but for a 410 that the actor's server
// takes back (RestoresRemoteActor, account.LifecycleState.Apply).
func (e LifecycleEventRecorded) Deletes() bool {
	return e.Event.Kind == account.LifecycleDeleted || e.Event.Kind == account.LifecycleRemoteDeleted
}

// DeletesLocalAccount reports whether the event is a local account's deletion, which is sent
// to the remote servers as a Delete of the actor (federation.md §7.1).
func (e LifecycleEventRecorded) DeletesLocalAccount() bool {
	return e.Event.Kind == account.LifecycleDeleted
}

// ClosesLocalAccount reports whether the event deletes a local account or schedules its
// deletion (a ban), which ends its subscription to a supporter plan (ADR-0072 §10).
func (e LifecycleEventRecorded) ClosesLocalAccount() bool {
	return e.Event.Kind == account.LifecycleDeleted || e.Event.Kind == account.LifecycleDeletionScheduled
}

// Deactivates reports whether the event hides a local account from everyone at its owner's wish
// (ADR-0089).
func (e LifecycleEventRecorded) Deactivates() bool {
	return e.Event.Kind == account.LifecycleDeactivated
}

// Reactivates reports whether the event brings back a local account that its owner deactivated
// (ADR-0089).
func (e LifecycleEventRecorded) Reactivates() bool {
	return e.Event.Kind == account.LifecycleReactivated && e.Event.InitiatedBy != account.InitiatedBySystem
}

// RestoresRemoteActor reports whether the event brings back a remote actor that a 410 marked
// deleted and whose server serves it again.
func (e LifecycleEventRecorded) RestoresRemoteActor() bool {
	return e.Event.Kind == account.LifecycleReactivated && e.Event.InitiatedBy == account.InitiatedBySystem
}

// RegisterProjectors registers the account module's synchronous projectors (data-model §9):
// account_handles from local accounts and the handle checks of remote actors,
// account_profile_heads from profile revisions, account_lifecycle_states from lifecycle events,
// remote_account_heads from the fetches of remote actors, and account_move_states from moves. Each projector applies the same rule as
// RebuildProjections, so updating incrementally and rebuilding agree.
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventLocalAccountCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(LocalAccountCreated)
		return repo.UpsertHandle(ctx, tx, localHandle(ev.Local))
	})
	r.Register(EventProfileRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		rev := e.(ProfileRevised).Revision
		head, err := repo.ProfileHead(ctx, tx, rev.AccountID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if !rev.Supersedes(head) {
			return nil
		}
		return repo.UpsertProfileHead(ctx, tx, account.ProfileHead{AccountID: rev.AccountID, RevisionID: rev.ID, Seq: rev.Seq})
	})
	r.Register(EventLifecycleEventRecorded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(LifecycleEventRecorded).Event
		state, err := repo.LifecycleState(ctx, tx, ev.AccountID)
		if errors.Is(err, ErrNotFound) {
			state, err = account.InitialLifecycleState(ev.AccountID), nil
		}
		if err != nil {
			return err
		}
		return repo.UpsertLifecycleState(ctx, tx, state.Apply(ev))
	})
	r.Register(EventRemoteActorFetched, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertRemoteHead(ctx, tx, e.(RemoteActorFetched).Head)
	})
	r.Register(EventHandleChecked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return applyHandleCheck(ctx, tx, repo, e.(HandleChecked).Verification)
	})
	registerMoveProjector(r, repo)
}

// applyHandleCheck makes account_handles follow the newest handle check of a remote actor: a
// verified handle is the account's (taken from another account that verified the same name
// before), and a mismatch leaves the account without one.
func applyHandleCheck(ctx context.Context, tx uow.Tx, repo Repository, v account.HandleVerification) error {
	h, ok := v.Handle()
	if !ok {
		return repo.DeleteHandle(ctx, tx, v.AccountID)
	}
	if err := repo.DeleteOtherHandle(ctx, tx, h); err != nil {
		return err
	}
	return repo.UpsertHandle(ctx, tx, h)
}

func localHandle(la account.LocalAccount) account.Handle {
	return account.Handle{
		AccountID:          la.AccountID,
		Username:           la.Username,
		UsernameNormalized: la.UsernameNormalized,
		Domain:             account.LocalDomain,
	}
}

// RebuildProjections empties the account module's projections and derives them again from
// their sources (data-model §9), in one transaction per projection. remote_account_heads
// points each remote actor at its newest snapshot; the fetch state of the periodic refresh
// (LastFetchedAt, ConsecutiveFetchFailures) has no events, so an actor's head keeps it, and an
// actor without a head gets the snapshot's fetch time and no failures.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	steps := []struct {
		name string
		fn   func(tx uow.Tx) error
	}{
		{"account_handles", func(tx uow.Tx) error { return rebuildHandles(ctx, tx, repo) }},
		{"account_profile_heads", func(tx uow.Tx) error { return rebuildProfileHeads(ctx, tx, repo) }},
		{"account_lifecycle_states", func(tx uow.Tx) error { return rebuildLifecycleStates(ctx, tx, repo) }},
		{"remote_account_heads", func(tx uow.Tx) error { return rebuildRemoteHeads(ctx, tx, repo) }},
		{"account_move_states", func(tx uow.Tx) error { return rebuildMoveStates(ctx, tx, repo) }},
	}
	for _, s := range steps {
		if err := u.Tx(ctx, s.fn); err != nil {
			return fmt.Errorf("rebuild %s: %w", s.name, err)
		}
	}
	return nil
}

func rebuildHandles(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllHandles(ctx, tx); err != nil {
		return err
	}
	locals, err := repo.ListLocalAccounts(ctx, tx)
	if err != nil {
		return err
	}
	for _, la := range locals {
		if err := repo.UpsertHandle(ctx, tx, localHandle(la)); err != nil {
			return err
		}
	}
	checks, err := repo.ListLatestHandleVerifications(ctx, tx)
	if err != nil {
		return err
	}
	for _, v := range checks {
		if err := applyHandleCheck(ctx, tx, repo, v); err != nil {
			return err
		}
	}
	return nil
}

func rebuildRemoteHeads(ctx context.Context, tx uow.Tx, repo Repository) error {
	current, err := repo.ListRemoteHeads(ctx, tx)
	if err != nil {
		return err
	}
	fetchState := make(map[string]account.RemoteHead, len(current))
	for _, h := range current {
		fetchState[h.AccountID] = h
	}
	heads, err := repo.ListLatestRemoteSnapshotHeads(ctx, tx)
	if err != nil {
		return err
	}
	if err := repo.DeleteAllRemoteHeads(ctx, tx); err != nil {
		return err
	}
	for _, h := range heads {
		if cur, ok := fetchState[h.AccountID]; ok {
			h.LastFetchedAt, h.ConsecutiveFetchFailures = cur.LastFetchedAt, cur.ConsecutiveFetchFailures
		}
		if err := repo.UpsertRemoteHead(ctx, tx, h); err != nil {
			return err
		}
	}
	return nil
}

func rebuildProfileHeads(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllProfileHeads(ctx, tx); err != nil {
		return err
	}
	revs, err := repo.ListProfileRevisionSeqs(ctx, tx)
	if err != nil {
		return err
	}
	heads := map[string]account.ProfileHead{}
	var order []string
	for _, r := range revs {
		head, seen := heads[r.AccountID]
		if !seen {
			order = append(order, r.AccountID)
		}
		if (account.ProfileRevision{ID: r.RevisionID, Seq: r.Seq}).Supersedes(head) {
			heads[r.AccountID] = r
		}
	}
	for _, id := range order {
		if err := repo.UpsertProfileHead(ctx, tx, heads[id]); err != nil {
			return err
		}
	}
	return nil
}

func rebuildLifecycleStates(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllLifecycleStates(ctx, tx); err != nil {
		return err
	}
	events, err := repo.ListLifecycleEvents(ctx, tx)
	if err != nil {
		return err
	}
	states := map[string]account.LifecycleState{}
	var order []string
	for _, e := range events {
		s, seen := states[e.AccountID]
		if !seen {
			order = append(order, e.AccountID)
			s = account.InitialLifecycleState(e.AccountID)
		}
		states[e.AccountID] = s.Apply(e)
	}
	for _, id := range order {
		if err := repo.UpsertLifecycleState(ctx, tx, states[id]); err != nil {
			return err
		}
	}
	return nil
}

// ReplayLifecycle emits inside tx LifecycleEventRecorded for each lifecycle event that deletes an
// account (LifecycleEventRecorded.Deletes) and for the deactivated event of each account that is
// deactivated now, in account and event order, for the rebuild of the projections that other
// modules derive from them (data-model §9). The events of an actor that was restored since are
// left out, so these are the accounts that are deleted or deactivated now.
func ReplayLifecycle(ctx context.Context, tx uow.Tx, repo Repository, emit func(uow.Event) error) error {
	events, err := repo.ListLifecycleEvents(ctx, tx)
	if err != nil {
		return err
	}
	states := map[string]account.LifecycleState{}
	last := map[string]string{}
	for _, e := range events {
		s, seen := states[e.AccountID]
		if !seen {
			s = account.InitialLifecycleState(e.AccountID)
		}
		states[e.AccountID] = s.Apply(e)
		if e.Kind == account.LifecycleDeactivated {
			last[e.AccountID] = e.ID
		}
	}
	for _, e := range events {
		ev := LifecycleEventRecorded{Event: e}
		inForce := last[e.AccountID] == e.ID && states[e.AccountID].Deactivated()
		deleted := ev.Deletes() && states[e.AccountID].State == account.StateDeleted
		if !deleted && !inForce {
			continue
		}
		if err := emit(ev); err != nil {
			return err
		}
	}
	return nil
}
