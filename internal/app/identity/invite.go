// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InviteStatus is an invite with how it has been used.
type InviteStatus struct {
	identity.Invite
	Uses    int
	Revoked bool
	// State is what the invite allowed when the Service read it (StateAt, and
	// InviteCreatorIneligible for the invite of a member who may no longer create invites);
	// the repository leaves it empty.
	State InviteState
}

// Usable reports whether the invite still lets someone sign up at now, before its creator is
// checked (ADR-0108 §3).
func (s InviteStatus) Usable(now time.Time) bool {
	return !s.Revoked && !s.Expired(now) && !s.Exhausted(s.Uses)
}

// CreateInviteInput describes a new invite.
type CreateInviteInput struct {
	// CreatedByUserID is the user creating the invite, or "" for the CLI.
	CreatedByUserID string
	// MaxUses is how many sign-ups the invite allows; 0 means no limit.
	MaxUses int
	// ExpiresIn is how long the invite works; 0 means forever.
	ExpiresIn time.Duration
}

// MaxInviteUses bounds CreateInviteInput.MaxUses (the integer column of invites.max_uses).
const MaxInviteUses = 1_000_000

// Errors of the invites that members create (ADR-0108).
var (
	// ErrInvitesNotAllowed reports a user who may not create invites of their own now: the
	// instance setting invite_creators and the user's roles do not let them, moderation
	// restricts the account, the registration is not approved, or the registration is closed.
	ErrInvitesNotAllowed = errors.New("identity: the user may not create invites")
	// ErrTooManyInvites reports a member who already holds as many usable invites as
	// member_invite_max_active allows.
	ErrTooManyInvites = errors.New("identity: too many usable invites")
)

// newInviteCode returns InviteCodeLength characters of base32 from 80 random bits.
func newInviteCode() string {
	b := make([]byte, identity.InviteCodeLength*5/8)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

// CreateInvite creates an invite of the staff (a holder of manage_invites, or the CLI) with a
// new random code; no cap of the members' invites applies to it. It fails with
// ErrRegistrationClosed while the registration mode is closed, which takes no invite
// (ADR-0108 §4).
func (s *Service) CreateInvite(ctx context.Context, in CreateInviteInput) (identity.Invite, error) {
	if in.MaxUses < 0 || in.MaxUses > MaxInviteUses {
		return identity.Invite{}, fmt.Errorf("identity: max uses must be 0 (no limit) to %d", MaxInviteUses)
	}
	if in.ExpiresIn < 0 {
		return identity.Invite{}, errors.New("identity: the expiry must not be negative")
	}
	mode, err := s.registrationMode(ctx)
	if err != nil {
		return identity.Invite{}, err
	}
	if mode == identity.RegistrationClosed {
		return identity.Invite{}, ErrRegistrationClosed
	}
	inv := s.newInvite(in.CreatedByUserID, identity.InviteByStaff, in.MaxUses, in.ExpiresIn)
	auditID := s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertInvite(ctx, tx, inv); err != nil {
			return err
		}
		return tx.Emit(ctx, inviteAudit(auditID, appmoderation.AuditInviteCreated, inv, in.CreatedByUserID, inv.CreatedAt))
	})
	if err != nil {
		return identity.Invite{}, err
	}
	return inv, nil
}

// newInvite returns a new invite with a random code; maxUses 0 allows any number of sign-ups
// and expiresIn 0 never expires.
func (s *Service) newInvite(createdBy string, kind identity.InviteCreatorKind, maxUses int, expiresIn time.Duration) identity.Invite {
	now := s.d.Clock.Now()
	inv := identity.Invite{
		ID: s.d.IDs.NewID(), Code: newInviteCode(), CreatedByUserID: createdBy, CreatorKind: kind, MaxUses: maxUses, CreatedAt: now,
	}
	if expiresIn > 0 {
		at := now.Add(expiresIn)
		inv.ExpiresAt = &at
	}
	return inv
}

// inviteAuditCodeChars is how many characters of an invite's code its audit log label keeps.
const inviteAuditCodeChars = 4

// inviteAudit returns the audit log entry of an operation on an invite (FR-704), performed by
// byUserID ("" for the CLI): a staff member, or a member on their own invite (ADR-0108 §6). The
// label is the first characters of the code followed by an ellipsis: enough to tell the invite
// in the list of invites, while the whole code, which lets anyone sign up, stays with those who
// manage the invites. The source is the invite, whose ID is also the key of its revocation.
func inviteAudit(id string, action appmoderation.AuditAction, inv identity.Invite, byUserID string, at time.Time) appmoderation.AuditRecorded {
	actor, user := appmoderation.AuditActor(byUserID)
	label := inv.Code
	if len(label) > inviteAuditCodeChars {
		label = label[:inviteAuditCodeChars] + "…"
	}
	return appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: id, ActorKind: actor, ActorUserID: user, Action: action, TargetKind: appmoderation.AuditTargetInvite,
		TargetID: inv.ID, TargetLabel: label, SourceEventID: inv.ID, OccurredAt: at,
	}}
}

// ListInvites returns invites with their usage, newest first, at most limit (1 to
// MaxListLimit) of them. A before other than "" is the ID of the last invite of the previous
// page: the list starts after it. The invite of a member who may no longer create invites is
// InviteCreatorIneligible (ADR-0108 §3).
func (s *Service) ListInvites(ctx context.Context, before string, limit int) ([]InviteStatus, error) {
	if limit < 1 || limit > MaxListLimit {
		return nil, fmt.Errorf("identity: limit must be 1 to %d", MaxListLimit)
	}
	list, err := s.d.Repository.ListInvites(ctx, before, limit)
	if err != nil {
		return nil, err
	}
	return list, s.setInviteStates(ctx, list)
}

// setInviteStates sets the State of each invite at the current time, checking the creator of
// each usable invite of a member once per creator.
func (s *Service) setInviteStates(ctx context.Context, list []InviteStatus) error {
	now := s.d.Clock.Now()
	var settings *MemberInviteSettings
	eligible := map[string]bool{}
	for i := range list {
		list[i].State = list[i].StateAt(now)
		if list[i].State != InviteUsable || list[i].CreatorKind != identity.InviteByMember {
			continue
		}
		if settings == nil {
			st, err := s.memberInviteSettings(ctx)
			if err != nil {
				return err
			}
			settings = &st
		}
		creator := list[i].CreatedByUserID
		ok, seen := eligible[creator]
		if !seen {
			var err error
			if ok, err = s.creatorEligible(ctx, list[i].Invite, *settings); err != nil {
				return err
			}
			eligible[creator] = ok
		}
		if !ok {
			list[i].State = InviteCreatorIneligible
		}
	}
	return nil
}

// InviteState is what an invite allows now.
type InviteState string

// Invite states, in the order StateAt checks them; InviteCreatorIneligible comes after them.
const (
	InviteUsable    InviteState = "usable"
	InviteRevoked   InviteState = "revoked"
	InviteExpired   InviteState = "expired"
	InviteExhausted InviteState = "exhausted" // used as many times as it allows
	// InviteCreatorIneligible is the otherwise usable invite of a member who may no longer
	// create invites (ADR-0108 §3); it works again when the member may.
	InviteCreatorIneligible InviteState = "creator_ineligible"
)

// StateAt returns what the invite allows at now: a revoked invite is revoked even after it
// expired, and an expired one expired even when it is used up. It does not check the creator.
func (s InviteStatus) StateAt(now time.Time) InviteState {
	switch {
	case s.Revoked:
		return InviteRevoked
	case s.Expired(now):
		return InviteExpired
	case s.Exhausted(s.Uses):
		return InviteExhausted
	}
	return InviteUsable
}

// RevokeInviteByID stops the invite with the ID from working, as RevokeInvite does, and returns
// it with its usage. It fails with ErrNotFound when no invite has the ID.
func (s *Service) RevokeInviteByID(ctx context.Context, id, revokedByUserID string) (InviteStatus, error) {
	inv, err := s.d.Repository.FindInviteByID(ctx, id)
	if err != nil {
		return InviteStatus{}, err
	}
	return s.revokeAndRead(ctx, inv, revokedByUserID)
}

// revokeAndRead revokes inv as revokedByUserID and returns it with its usage and state.
func (s *Service) revokeAndRead(ctx context.Context, inv identity.Invite, revokedByUserID string) (InviteStatus, error) {
	out := InviteStatus{Invite: inv}
	rev := identity.InviteRevocation{InviteID: inv.ID, RevokedByUserID: revokedByUserID, RevokedAt: s.d.Clock.Now()}
	auditID := s.d.IDs.NewID()
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.revokeInvite(ctx, tx, inv, rev, auditID); err != nil {
			return err
		}
		var err error
		out.Uses, out.Revoked, err = s.d.Repository.InviteUsage(ctx, tx, inv.ID)
		return err
	})
	if err != nil {
		return InviteStatus{}, err
	}
	out.State = out.StateAt(rev.RevokedAt)
	return out, nil
}

// revokeInvite records the revocation of inv in tx with its audit log entry, unless the invite
// is already revoked: revoking it again changes nothing, so it is not audited again.
func (s *Service) revokeInvite(ctx context.Context, tx uow.Tx, inv identity.Invite, rev identity.InviteRevocation, auditID string) error {
	_, revoked, err := s.d.Repository.InviteUsage(ctx, tx, inv.ID)
	if err != nil || revoked {
		return err
	}
	if err := s.d.Repository.InsertInviteRevocation(ctx, tx, rev); err != nil {
		return err
	}
	return tx.Emit(ctx, inviteAudit(auditID, appmoderation.AuditInviteRevoked, inv, rev.RevokedByUserID, rev.RevokedAt))
}

// RevokeInvite stops an invite from working. Revoking it again keeps the first revocation. It
// fails with ErrNotFound when no invite has the code.
func (s *Service) RevokeInvite(ctx context.Context, code, revokedByUserID string) (identity.Invite, error) {
	inv, err := s.d.Repository.FindInviteByCode(ctx, identity.NormalizeInviteCode(code))
	if err != nil {
		return identity.Invite{}, err
	}
	rev := identity.InviteRevocation{InviteID: inv.ID, RevokedByUserID: revokedByUserID, RevokedAt: s.d.Clock.Now()}
	auditID := s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.revokeInvite(ctx, tx, inv, rev, auditID) })
	if err != nil {
		return identity.Invite{}, err
	}
	return inv, nil
}

// MemberInviteSettings are the instance settings of the invites that members create
// (ADR-0108 §1, §2).
type MemberInviteSettings struct {
	// Everyone reports that invite_creators is everyone: every signed-in local user may create
	// invites, not only the holders of create_invites.
	Everyone bool
	Caps     identity.MemberInviteCaps
}

// DefaultMemberInviteSettings are the defaults of the instance settings: only the holders of
// create_invites, up to 5 usable invites of at most 10 uses and 30 days each.
func DefaultMemberInviteSettings() MemberInviteSettings {
	return MemberInviteSettings{Caps: identity.MemberInviteCaps{MaxActive: 5, MaxUses: 10, MaxExpiresIn: 30 * 24 * time.Hour}}
}

func (s *Service) memberInviteSettings(ctx context.Context) (MemberInviteSettings, error) {
	if s.d.MemberInvites == nil {
		return DefaultMemberInviteSettings(), nil
	}
	st, err := s.d.MemberInvites(ctx)
	if err != nil {
		return MemberInviteSettings{}, fmt.Errorf("member invite settings: %w", err)
	}
	return st, nil
}

// inviterFacts returns what decides whether the user userID may create invites of their own
// in the registration mode mode (identity.MayCreateInvites).
func (s *Service) inviterFacts(ctx context.Context, userID string, mode identity.RegistrationMode, settings MemberInviteSettings) (identity.InviterFacts, error) {
	f := identity.InviterFacts{Mode: mode, EveryoneInvites: settings.Everyone}
	var err error
	if f.HasPermission, err = s.HasPermission(ctx, userID, identity.PermCreateInvites); err != nil {
		return identity.InviterFacts{}, err
	}
	if !f.EveryoneInvites && !f.HasPermission {
		return f, nil
	}
	// A user without a registration state was created with the CLI and is approved.
	st, err := s.d.Repository.FindRegistrationState(ctx, userID)
	switch {
	case err == nil:
		f.Approved = st.State == identity.RegistrationApproved
	case errors.Is(err, ErrNotFound):
		f.Approved = true
	default:
		return identity.InviterFacts{}, err
	}
	if s.d.Accounts == nil {
		f.Active = true
		return f, nil
	}
	life, err := s.d.Accounts.UserLifecycle(ctx, userID)
	switch {
	case errors.Is(err, account.ErrNotFound):
		return f, nil
	case err != nil:
		return identity.InviterFacts{}, err
	}
	f.Active = life.Active()
	if f.Restricted, err = s.loginRestricted(ctx, life.AccountID); err != nil || f.Restricted {
		return f, err
	}
	if s.d.InviterRestricted != nil {
		if f.Restricted, err = s.d.InviterRestricted(ctx, life.AccountID); err != nil {
			return identity.InviterFacts{}, err
		}
	}
	return f, nil
}

// creatorEligible reports whether the creator of inv still lets it work (ADR-0108 §3): always
// for an invite of the staff, and while its creator may create invites for that of a member.
// The registration mode is the sign-up's to check, so it is not part of the answer.
func (s *Service) creatorEligible(ctx context.Context, inv identity.Invite, settings MemberInviteSettings) (bool, error) {
	if inv.CreatorKind != identity.InviteByMember {
		return true, nil
	}
	if inv.CreatedByUserID == "" {
		return false, nil
	}
	f, err := s.inviterFacts(ctx, inv.CreatedByUserID, identity.RegistrationOpen, settings)
	if err != nil {
		return false, err
	}
	return identity.MayCreateInvites(f), nil
}

// OwnInvites are a member's own invites (ADR-0108 §5) with what they may do.
type OwnInvites struct {
	// Invites are a page of the invites the member created as a member, newest first.
	Invites []InviteStatus
	// MayCreate reports whether the member may create invites now (identity.MayCreateInvites),
	// before the cap of usable invites.
	MayCreate bool
	// Caps are the caps of the members' invites now.
	Caps identity.MemberInviteCaps
	// Usable is how many usable invites the member holds, which member_invite_max_active
	// bounds.
	Usable int
}

// ListOwnInvites returns the invites that the user userID created as a member, newest first, at
// most limit (1 to MaxListLimit) of them after the invite before ("" for the first page), with
// whether the user may create one and the caps. A user who may no longer create invites still
// lists and revokes the ones they made; those that are otherwise usable are
// InviteCreatorIneligible.
func (s *Service) ListOwnInvites(ctx context.Context, userID, before string, limit int) (OwnInvites, error) {
	if limit < 1 || limit > MaxListLimit {
		return OwnInvites{}, fmt.Errorf("identity: limit must be 1 to %d", MaxListLimit)
	}
	settings, err := s.memberInviteSettings(ctx)
	if err != nil {
		return OwnInvites{}, err
	}
	mode, err := s.registrationMode(ctx)
	if err != nil {
		return OwnInvites{}, err
	}
	f, err := s.inviterFacts(ctx, userID, mode, settings)
	if err != nil {
		return OwnInvites{}, err
	}
	out := OwnInvites{MayCreate: identity.MayCreateInvites(f), Caps: settings.Caps}
	if out.Invites, err = s.d.Repository.ListMemberInvites(ctx, userID, before, limit); err != nil {
		return OwnInvites{}, err
	}
	now := s.d.Clock.Now()
	f.Mode = identity.RegistrationOpen // the creator's own standing, whatever the mode
	eligible := identity.MayCreateInvites(f)
	for i := range out.Invites {
		out.Invites[i].State = out.Invites[i].StateAt(now)
		if out.Invites[i].State == InviteUsable && !eligible {
			out.Invites[i].State = InviteCreatorIneligible
		}
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		out.Usable, err = s.d.Repository.CountUsableMemberInvites(ctx, tx, userID, now)
		return err
	})
	if err != nil {
		return OwnInvites{}, err
	}
	return out, nil
}

// CreateOwnInviteInput describes a member's new invite (ADR-0108 §2).
type CreateOwnInviteInput struct {
	UserID string
	// MaxUses is how many sign-ups the invite allows, 1 to the cap; 0 means the default.
	MaxUses int
	// ExpiresIn is how long the invite works, up to the cap; 0 means the default.
	ExpiresIn time.Duration
}

// CreateOwnInvite creates an invite of a member (ADR-0108 §1, §2), audited with the member as
// the actor (§6). It fails with ErrInvitesNotAllowed when the member may not create invites
// now, with identity.ErrInviteUsesOverCap or identity.ErrInviteExpiryOverCap for a value
// outside the caps, and with ErrTooManyInvites when the member already holds
// member_invite_max_active usable invites. Two creations at the same moment may both pass the
// last free place: the cap is a limit of abuse, not an invariant of the data.
func (s *Service) CreateOwnInvite(ctx context.Context, in CreateOwnInviteInput) (InviteStatus, error) {
	settings, err := s.memberInviteSettings(ctx)
	if err != nil {
		return InviteStatus{}, err
	}
	mode, err := s.registrationMode(ctx)
	if err != nil {
		return InviteStatus{}, err
	}
	f, err := s.inviterFacts(ctx, in.UserID, mode, settings)
	if err != nil {
		return InviteStatus{}, err
	}
	if !identity.MayCreateInvites(f) {
		return InviteStatus{}, ErrInvitesNotAllowed
	}
	uses, expiresIn, err := settings.Caps.Resolve(in.MaxUses, in.ExpiresIn)
	if err != nil {
		return InviteStatus{}, err
	}
	inv := s.newInvite(in.UserID, identity.InviteByMember, uses, expiresIn)
	auditID := s.d.IDs.NewID()
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		n, err := s.d.Repository.CountUsableMemberInvites(ctx, tx, in.UserID, inv.CreatedAt)
		if err != nil {
			return err
		}
		if n >= settings.Caps.MaxActive {
			return ErrTooManyInvites
		}
		if err := s.d.Repository.InsertInvite(ctx, tx, inv); err != nil {
			return err
		}
		return tx.Emit(ctx, inviteAudit(auditID, appmoderation.AuditInviteCreated, inv, in.UserID, inv.CreatedAt))
	})
	if err != nil {
		return InviteStatus{}, err
	}
	return InviteStatus{Invite: inv, State: InviteUsable}, nil
}

// RevokeOwnInvite stops an invite that the user userID created as a member from working, as
// RevokeInviteByID does, audited with the member as the actor (ADR-0108 §6). A member revokes
// their own invites even when they may no longer create invites. It fails with ErrNotFound when
// the user created no member invite with the ID.
func (s *Service) RevokeOwnInvite(ctx context.Context, userID, inviteID string) (InviteStatus, error) {
	inv, err := s.d.Repository.FindInviteByID(ctx, inviteID)
	if err != nil {
		return InviteStatus{}, err
	}
	if inv.CreatorKind != identity.InviteByMember || inv.CreatedByUserID != userID {
		return InviteStatus{}, ErrNotFound
	}
	return s.revokeAndRead(ctx, inv, userID)
}
