// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The errors of the invitations (ADR-0100 §7).
var (
	// ErrInvalidInvitee reports an account that may not be invited to a room: not a local,
	// active account, the host or the one who invites, in a block relation with the host or a
	// co-host, or banned from the room.
	ErrInvalidInvitee = errors.New("voice: the account may not be invited to the room")
	// ErrAlreadyInvited reports an account invited to the room already: an account is invited
	// to a room once, even after a revocation.
	ErrAlreadyInvited = errors.New("voice: the account was invited to the room already")
	// ErrInvalidRole reports a role of an invitation that is neither speaker nor listener.
	ErrInvalidRole = errors.New("voice: invalid role")
)

// InvitationLimit bounds the invitations of each room (ADR-0100 §7).
var InvitationLimit = kernel.Limit{Name: "voice.invitations", Max: 100, Window: time.Hour}

// Invite invites accountID to a room as a listener or a speaker, by byAccountID, the host or a
// co-host (ADR-0100 §7): the account sees the room from now on, and an invitation as a speaker
// is an explicit permission to speak (§4). It fails with ErrInvalidInvitee and
// ErrAlreadyInvited, and with a *kernel.RateLimitedError past InvitationLimit.
func (s *Service) Invite(ctx context.Context, byAccountID, roomID, accountID string, role voice.Role) (Invitation, error) {
	if role == "" {
		role = voice.RoleListener
	}
	if !role.Valid() {
		return Invitation{}, ErrInvalidRole
	}
	v, err := s.moderating(ctx, byAccountID, roomID)
	if err != nil {
		return Invitation{}, err
	}
	if err := s.invitable(ctx, v, byAccountID, accountID); err != nil {
		return Invitation{}, err
	}
	switch _, err := s.d.Repository.FindInvitation(ctx, nil, roomID, accountID); {
	case err == nil:
		return Invitation{}, ErrAlreadyInvited
	case !errors.Is(err, ErrNotFound):
		return Invitation{}, err
	}
	d, err := s.d.RateLimiter.Take(ctx, InvitationLimit, roomID)
	if err != nil {
		return Invitation{}, err
	}
	if !d.Allowed {
		return Invitation{}, &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	inv := Invitation{
		ID: s.d.IDs.NewID(), RoomID: roomID, InviteeAccountID: accountID, Role: role, InvitedByAccountID: byAccountID, InvitedAt: s.now(),
	}
	err = s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, _ RoomRecord) error {
		if err := s.authorityTx(ctx, tx, roomID, byAccountID, true); err != nil {
			return err
		}
		did, err := s.d.Repository.InsertInvitation(ctx, tx, inv)
		if err != nil {
			return err
		}
		if !did {
			return ErrAlreadyInvited
		}
		if err := tx.Emit(ctx, Invited{Invitation: inv}); err != nil {
			return err
		}
		if role != voice.RoleSpeaker {
			return nil
		}
		return s.speakGrantTx(ctx, tx, roomID, accountID, voice.SpeakGranted, byAccountID, inv.InvitedAt)
	})
	if err != nil {
		return Invitation{}, err
	}
	return inv, nil
}

// invitable checks that byAccountID may invite accountID to the room of v; ErrInvalidInvitee
// otherwise.
func (s *Service) invitable(ctx context.Context, v RoomView, byAccountID, accountID string) error {
	host := v.Head.HostAccountID
	if accountID == "" || accountID == byAccountID || accountID == host {
		return ErrInvalidInvitee
	}
	m, err := s.d.Members.Member(ctx, accountID)
	if err != nil {
		return err
	}
	if !m.Local || !m.Active {
		return ErrInvalidInvitee
	}
	others := []string{host}
	for _, p := range v.Participants {
		if p.Cohost() && p.AccountID != host && p.AccountID != accountID {
			others = append(others, p.AccountID)
		}
	}
	for _, id := range others {
		blocked, err := s.d.Relations.Blocked(ctx, accountID, id)
		if err != nil {
			return err
		}
		if blocked {
			return ErrInvalidInvitee
		}
	}
	banned, err := s.d.Repository.Banned(ctx, nil, v.ID, accountID)
	if err != nil {
		return err
	}
	if banned {
		return ErrInvalidInvitee
	}
	return nil
}

// RevokeInvitation revokes the invitation of accountID to a room, by byAccountID, the host or a
// co-host (ADR-0100 §7): the account no longer sees the room unless it is in it, and the
// explicit permission to speak of an invitation as a speaker goes too; a participant keeps its
// role. ErrNotFound for an account without an invitation that was not revoked.
func (s *Service) RevokeInvitation(ctx context.Context, byAccountID, roomID, accountID string) error {
	if _, err := s.moderating(ctx, byAccountID, roomID); err != nil {
		return err
	}
	inv, err := s.d.Repository.FindInvitation(ctx, nil, roomID, accountID)
	if err != nil {
		return err
	}
	if inv.Revoked {
		return ErrNotFound
	}
	r := InvitationRevocation{InvitationID: inv.ID, RevokedByAccountID: byAccountID, RevokedAt: s.now()}
	return s.roomTx(ctx, []string{roomID}, func(tx uow.Tx, _ RoomRecord) error {
		if err := s.authorityTx(ctx, tx, roomID, byAccountID, true); err != nil {
			return err
		}
		did, err := s.d.Repository.InsertInvitationRevocation(ctx, tx, r)
		if err != nil || !did {
			return err
		}
		if err := tx.Emit(ctx, InvitationRevoked{Invitation: inv.Invitation, Revocation: r}); err != nil {
			return err
		}
		if inv.Role != voice.RoleSpeaker {
			return nil
		}
		return s.speakGrantTx(ctx, tx, roomID, accountID, voice.SpeakRevoked, byAccountID, r.RevokedAt)
	})
}

// Invitations returns a page of at most limit invitations of a room that were not revoked, in
// the order they were made, after the invitation afterID unless it is "", for byAccountID, the
// host or a co-host, and whether more follow.
func (s *Service) Invitations(ctx context.Context, byAccountID, roomID, afterID string, limit int) ([]Invitation, bool, error) {
	if _, err := s.moderating(ctx, byAccountID, roomID); err != nil {
		return nil, false, err
	}
	out, err := s.d.Repository.Invitations(ctx, nil, roomID, afterID, limit+1)
	if err != nil {
		return nil, false, err
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}
