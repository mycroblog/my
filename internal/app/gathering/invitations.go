// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// The errors of the invitations and the co-organizers (ADR-0161 §4, §9).
var (
	// ErrInvalidInvitee reports an account that may not be invited to an event: not a local,
	// active account, the organizer or a co-organizer, or in a block relation with the organizer
	// or a co-organizer.
	ErrInvalidInvitee = errors.New("gathering: the account may not be invited to the event")
	// ErrAlreadyInvited reports an account invited to the event already: an account is invited
	// to an event once, even after a revocation.
	ErrAlreadyInvited = errors.New("gathering: the account was invited to the event already")
	// ErrInvalidCoorganizer reports an account that may not co-organize an event: not a local,
	// active account, the organizer, or in a block relation with the organizer or a co-organizer.
	ErrInvalidCoorganizer = errors.New("gathering: the account may not co-organize the event")
	// ErrTooManyCoorganizers reports a co-organizer past gathering.MaxCoorganizers.
	ErrTooManyCoorganizers = errors.New("gathering: the event has as many co-organizers as it may")
)

// InvitationLimit bounds the invitations of each event (ADR-0161 §3).
var InvitationLimit = kernel.Limit{Name: "gathering.invitations", Max: 100, Window: time.Hour}

// joinable reports whether accountID is a local, active account that is not the organizer and
// is in no block relation with the organizer or a co-organizer of rec.
func (s *Service) joinable(ctx context.Context, rec Record, accountID string) (bool, error) {
	if accountID == rec.OrganizerAccountID {
		return false, nil
	}
	m, err := s.d.Members.Member(ctx, accountID)
	if err != nil || !m.Local || !m.Active {
		return false, err
	}
	blocked, err := s.d.Relations.BlockedAmong(ctx, accountID, rec.organizerIDs())
	if err != nil {
		return false, err
	}
	for _, id := range rec.organizerIDs() {
		if blocked[id] {
			return false, nil
		}
	}
	return true, nil
}

// Invite invites accountID to the event id, by byAccountID, the organizer or a co-organizer
// (ADR-0161 §9): the account sees the event from now on, whatever its visibility. It fails with
// ErrNotFound, ErrNotOrganizer, ErrClosed, ErrInvalidInvitee, ErrAlreadyInvited, and a
// *kernel.RateLimitedError past InvitationLimit.
func (s *Service) Invite(ctx context.Context, byAccountID, id, accountID string) (Invitation, error) {
	v, err := s.organizing(ctx, byAccountID, id)
	if err != nil {
		return Invitation{}, err
	}
	ok, err := s.joinable(ctx, v.Record, accountID)
	if err != nil {
		return Invitation{}, err
	}
	if !ok || v.Coorganizes(accountID) {
		return Invitation{}, ErrInvalidInvitee
	}
	if err := s.take(ctx, InvitationLimit, id); err != nil {
		return Invitation{}, err
	}
	inv := Invitation{ID: s.d.IDs.NewID(), GatheringID: id, InviteeAccountID: accountID, InvitedByAccountID: byAccountID, InvitedAt: s.now()}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		ok, err := s.d.Repository.InsertInvitation(ctx, tx, inv)
		if err != nil {
			return err
		}
		if !ok {
			return ErrAlreadyInvited
		}
		return tx.Emit(ctx, Invited{Invitation: inv})
	})
	if err != nil {
		return Invitation{}, err
	}
	return inv, nil
}

// RevokeInvitation revokes the invitation of accountID to the event id, by the organizer or a
// co-organizer: the account sees the event only as its visibility allows, or as long as it
// answered it. A revoked invitation stays revoked. It fails with ErrNotFound for an account that
// was not invited, ErrNotOrganizer, and ErrClosed.
func (s *Service) RevokeInvitation(ctx context.Context, byAccountID, id, accountID string) error {
	if _, err := s.organizing(ctx, byAccountID, id); err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inv, err := s.d.Repository.FindInvitation(ctx, tx, id, accountID)
		if err != nil {
			return err
		}
		r := InvitationRevocation{InvitationID: inv.ID, RevokedByAccountID: byAccountID, RevokedAt: s.now()}
		ok, err := s.d.Repository.InsertInvitationRevocation(ctx, tx, r)
		if err != nil || !ok {
			return err
		}
		return tx.Emit(ctx, InvitationRevoked{Invitation: inv.Invitation, Revocation: r})
	})
}

// Invitations returns at most limit invitations of the event id that were not revoked, oldest
// first, after the invitation afterID unless it is "", and whether more follow, for the organizer
// and the co-organizers. It fails with ErrNotFound and ErrNotOrganizer.
func (s *Service) Invitations(ctx context.Context, byAccountID, id, afterID string, limit int) ([]Invitation, bool, error) {
	v, err := s.Get(ctx, byAccountID, id)
	if err != nil {
		return nil, false, err
	}
	if !v.Organizes() {
		return nil, false, ErrNotOrganizer
	}
	invs, err := s.d.Repository.Invitations(ctx, nil, id, afterID, limit+1)
	if err != nil {
		return nil, false, err
	}
	if len(invs) > limit {
		return invs[:limit], true, nil
	}
	return invs, false, nil
}

// AssignCoorganizer names accountID a co-organizer of the event id, by its organizer (ADR-0161
// §4). A co-organizer stays one. It fails with ErrNotFound, ErrNotOrganizer, ErrOrganizerOnly,
// ErrClosed, ErrInvalidCoorganizer, and ErrTooManyCoorganizers.
func (s *Service) AssignCoorganizer(ctx context.Context, byAccountID, id, accountID string) (View, error) {
	return s.changeCoorganizer(ctx, byAccountID, id, accountID, gathering.CoorganizerAssigned)
}

// RemoveCoorganizer removes accountID from the co-organizers of the event id, by its organizer.
// An account that is not a co-organizer stays so. It fails with ErrNotFound, ErrNotOrganizer,
// ErrOrganizerOnly, and ErrClosed.
func (s *Service) RemoveCoorganizer(ctx context.Context, byAccountID, id, accountID string) (View, error) {
	return s.changeCoorganizer(ctx, byAccountID, id, accountID, gathering.CoorganizerRemoved)
}

func (s *Service) changeCoorganizer(ctx context.Context, byAccountID, id, accountID string, change gathering.CoorganizerChange) (View, error) {
	v, err := s.organizing(ctx, byAccountID, id)
	if err != nil {
		return View{}, err
	}
	if !v.Viewer.Organizer {
		return View{}, ErrOrganizerOnly
	}
	if change == gathering.CoorganizerAssigned {
		ok, err := s.joinable(ctx, v.Record, accountID)
		if err != nil {
			return View{}, err
		}
		if !ok {
			return View{}, ErrInvalidCoorganizer
		}
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.LockGathering(ctx, tx, id); err != nil {
			return err
		}
		rec, err := s.d.Repository.FindGathering(ctx, tx, id)
		if err != nil {
			return err
		}
		if rec.Cancellation != nil {
			return ErrClosed
		}
		is := rec.Coorganizes(accountID)
		switch {
		case is == (change == gathering.CoorganizerAssigned):
			return nil
		case change == gathering.CoorganizerAssigned && len(rec.Coorganizers) >= gathering.MaxCoorganizers:
			return ErrTooManyCoorganizers
		}
		c := CoorganizerChange{ID: s.d.IDs.NewID(), GatheringID: id, AccountID: accountID, Change: change, ChangedAt: s.now()}
		if err := s.d.Repository.InsertCoorganizerChange(ctx, tx, c); err != nil {
			return err
		}
		return tx.Emit(ctx, CoorganizerChanged{Change: c})
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, byAccountID, id)
}
