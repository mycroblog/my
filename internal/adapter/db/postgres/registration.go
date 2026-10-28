// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertRegistrationApplication implements identity.Repository.
func (r *IdentityRepository) InsertRegistrationApplication(ctx context.Context, tx uow.Tx, a identity.RegistrationApplication) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationApplication(ctx, postgresgen.InsertRegistrationApplicationParams{
		ID: a.ID, UserID: a.UserID, Reason: toNullString(a.Reason), InviteID: toNullString(a.InviteID), SubmittedAt: a.SubmittedAt,
	})
	return wrap(err, "insert registration application")
}

// InsertRegistrationDecision implements identity.Repository.
func (r *IdentityRepository) InsertRegistrationDecision(ctx context.Context, tx uow.Tx, d identity.RegistrationDecision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationDecision(ctx, postgresgen.InsertRegistrationDecisionParams{
		ID: d.ID, ApplicationID: d.ApplicationID, Decision: string(d.Decision), DecidedBy: string(d.DecidedBy),
		DecidedByUserID: toNullString(d.DecidedByUserID), Note: toNullString(d.Note), DecidedAt: d.DecidedAt,
	})
	return wrap(err, "insert registration decision")
}

// UpsertRegistrationState implements identity.Repository.
func (r *IdentityRepository) UpsertRegistrationState(ctx context.Context, tx uow.Tx, s identity.UserRegistrationState) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertRegistrationState(ctx, postgresgen.UpsertRegistrationStateParams{UserID: s.UserID, State: string(s.State), Since: s.Since})
	return wrap(err, "upsert registration state")
}

// TransitionPendingRegistrationState implements identity.Repository.
func (r *IdentityRepository) TransitionPendingRegistrationState(ctx context.Context, tx uow.Tx, s identity.UserRegistrationState) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.TransitionPendingRegistrationState(ctx, postgresgen.TransitionPendingRegistrationStateParams{
		State: string(s.State), Since: s.Since, UserID: s.UserID,
	})
	if err != nil {
		return false, wrap(err, "transition pending registration state")
	}
	return n > 0, nil
}

// FindRegistrationApplication implements identity.Repository.
func (r *IdentityRepository) FindRegistrationApplication(ctx context.Context, userID string) (identity.RegistrationApplication, error) {
	row, err := r.db.reader().GetRegistrationApplicationByUser(ctx, userID)
	if err != nil {
		return identity.RegistrationApplication{}, notFound(err, appidentity.ErrNotFound, "registration application of "+userID)
	}
	return identity.RegistrationApplication{
		ID: row.ID, UserID: row.UserID, Reason: fromNullString(row.Reason), InviteID: fromNullString(row.InviteID),
		SubmittedAt: utc(row.SubmittedAt),
	}, nil
}

// FindRegistrationState implements identity.Repository.
func (r *IdentityRepository) FindRegistrationState(ctx context.Context, userID string) (identity.UserRegistrationState, error) {
	row, err := r.db.reader().GetRegistrationState(ctx, userID)
	if err != nil {
		return identity.UserRegistrationState{}, notFound(err, appidentity.ErrNotFound, "registration state of "+userID)
	}
	return identity.UserRegistrationState{UserID: row.UserID, State: identity.RegistrationState(row.State), Since: utc(row.Since)}, nil
}

// ListRegistrations implements identity.Repository.
func (r *IdentityRepository) ListRegistrations(ctx context.Context, state identity.RegistrationState, after appidentity.RegistrationPosition, limit int) ([]appidentity.RegistrationEntry, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListRegistrationsInState(ctx, postgresgen.ListRegistrationsInStateParams{
		State: string(state), AfterUserID: toNullString(after.UserID), AfterSince: after.Since, PageLimit: n})
	if err != nil {
		return nil, wrap(err, "list registrations")
	}
	out := make([]appidentity.RegistrationEntry, len(rows))
	for i, row := range rows {
		out[i] = appidentity.RegistrationEntry{
			Application: identity.RegistrationApplication{
				ID: row.ApplicationID, UserID: row.UserID, Reason: fromNullString(row.Reason), InviteID: fromNullString(row.InviteID),
				SubmittedAt: utc(row.SubmittedAt),
			},
			State:         identity.UserRegistrationState{UserID: row.UserID, State: state, Since: utc(row.Since)},
			Email:         row.Email,
			EmailVerified: row.EmailVerified,
		}
	}
	return out, nil
}

// InsertInvite implements identity.Repository.
func (r *IdentityRepository) InsertInvite(ctx context.Context, tx uow.Tx, i identity.Invite) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	var maxUses *int32
	if i.MaxUses > 0 {
		n, err := toInt32(i.MaxUses, "max uses")
		if err != nil {
			return err
		}
		maxUses = &n
	}
	err = q.InsertInvite(ctx, postgresgen.InsertInviteParams{
		ID: i.ID, Code: i.Code, CreatedByUserID: toNullString(i.CreatedByUserID), CreatorKind: string(inviteCreatorKind(i.CreatorKind)),
		MaxUses: maxUses, ExpiresAt: i.ExpiresAt, CreatedAt: i.CreatedAt,
	})
	if isUniqueViolation(err, "invites_code") {
		return fmt.Errorf("insert invite: %w", appidentity.ErrInviteCodeTaken)
	}
	return wrap(err, "insert invite")
}

// inviteCreatorKind returns the creator kind an invite is stored with: staff unless a member
// created it.
func inviteCreatorKind(k identity.InviteCreatorKind) identity.InviteCreatorKind {
	if k == identity.InviteByMember {
		return k
	}
	return identity.InviteByStaff
}

func inviteFrom(id, code string, createdBy *string, creatorKind string, maxUses *int32, expiresAt *time.Time, createdAt time.Time) identity.Invite {
	inv := identity.Invite{
		ID: id, Code: code, CreatedByUserID: fromNullString(createdBy), CreatorKind: identity.InviteCreatorKind(creatorKind),
		ExpiresAt: utcPtr(expiresAt), CreatedAt: utc(createdAt),
	}
	if maxUses != nil {
		inv.MaxUses = int(*maxUses)
	}
	return inv
}

// FindInviteByCode implements identity.Repository.
func (r *IdentityRepository) FindInviteByCode(ctx context.Context, code string) (identity.Invite, error) {
	row, err := r.db.reader().GetInviteByCode(ctx, code)
	if err != nil {
		return identity.Invite{}, notFound(err, appidentity.ErrNotFound, "invite")
	}
	return inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.CreatorKind, row.MaxUses, row.ExpiresAt, row.CreatedAt), nil
}

// FindInviteByID implements identity.Repository.
func (r *IdentityRepository) FindInviteByID(ctx context.Context, id string) (identity.Invite, error) {
	row, err := r.db.reader().GetInviteByID(ctx, id)
	if err != nil {
		return identity.Invite{}, notFound(err, appidentity.ErrNotFound, "invite")
	}
	return inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.CreatorKind, row.MaxUses, row.ExpiresAt, row.CreatedAt), nil
}

// InviteUsage implements identity.Repository.
func (r *IdentityRepository) InviteUsage(ctx context.Context, tx uow.Tx, inviteID string) (int, bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, false, err
	}
	row, err := q.GetInviteUsage(ctx, inviteID)
	if err != nil {
		return 0, false, wrap(err, "get invite usage")
	}
	return int(row.Uses), row.Revoked, nil
}

// InsertInviteRedemption implements identity.Repository.
func (r *IdentityRepository) InsertInviteRedemption(ctx context.Context, tx uow.Tx, rd identity.InviteRedemption) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	seq, err := toInt32(rd.Seq, "invite redemption seq")
	if err != nil {
		return err
	}
	err = q.InsertInviteRedemption(ctx, postgresgen.InsertInviteRedemptionParams{
		InviteID: rd.InviteID, Seq: seq, UserID: rd.UserID, RedeemedAt: rd.RedeemedAt,
	})
	if isUniqueViolation(err, "invite_redemptions_pkey") {
		return uow.MarkRetryable(wrap(err, "insert invite redemption"))
	}
	return wrap(err, "insert invite redemption")
}

// InsertInviteRevocation implements identity.Repository.
func (r *IdentityRepository) InsertInviteRevocation(ctx context.Context, tx uow.Tx, rv identity.InviteRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInviteRevocationIfAbsent(ctx, postgresgen.InsertInviteRevocationIfAbsentParams{
		InviteID: rv.InviteID, RevokedByUserID: toNullString(rv.RevokedByUserID), RevokedAt: rv.RevokedAt,
	})
	return wrap(err, "insert invite revocation")
}

// ListInvites implements identity.Repository.
func (r *IdentityRepository) ListInvites(ctx context.Context, before string, limit int) ([]appidentity.InviteStatus, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListInvites(ctx, postgresgen.ListInvitesParams{Before: toNullString(before), PageLimit: n})
	if err != nil {
		return nil, wrap(err, "list invites")
	}
	out := make([]appidentity.InviteStatus, len(rows))
	for i, row := range rows {
		out[i] = appidentity.InviteStatus{
			Invite: inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.CreatorKind, row.MaxUses, row.ExpiresAt, row.CreatedAt),
			Uses:   int(row.Uses), Revoked: row.Revoked,
		}
	}
	return out, nil
}

// ListMemberInvites implements identity.Repository.
func (r *IdentityRepository) ListMemberInvites(ctx context.Context, creatorUserID, before string, limit int) ([]appidentity.InviteStatus, error) {
	n, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListMemberInvitesByCreator(ctx, postgresgen.ListMemberInvitesByCreatorParams{
		Creator: &creatorUserID, Before: toNullString(before), PageLimit: n,
	})
	if err != nil {
		return nil, wrap(err, "list member invites")
	}
	out := make([]appidentity.InviteStatus, len(rows))
	for i, row := range rows {
		out[i] = appidentity.InviteStatus{
			Invite: inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.CreatorKind, row.MaxUses, row.ExpiresAt, row.CreatedAt),
			Uses:   int(row.Uses), Revoked: row.Revoked,
		}
	}
	return out, nil
}

// CountUsableMemberInvites implements identity.Repository.
func (r *IdentityRepository) CountUsableMemberInvites(ctx context.Context, tx uow.Tx, creatorUserID string, now time.Time) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountUsableMemberInvites(ctx, postgresgen.CountUsableMemberInvitesParams{Creator: &creatorUserID, Now: &now})
	if err != nil {
		return 0, wrap(err, "count usable member invites")
	}
	return int(n), nil
}

// InsertUserToken implements identity.Repository.
func (r *IdentityRepository) InsertUserToken(ctx context.Context, tx uow.Tx, t identity.UserToken) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserToken(ctx, postgresgen.InsertUserTokenParams{
		ID: t.ID, UserID: t.UserID, Purpose: string(t.Purpose), EmailAddressID: toNullString(t.EmailAddressID),
		TokenHash: t.TokenHash, ExpiresAt: t.ExpiresAt, IssuedAt: t.IssuedAt,
		UnsubscribeCategory: toNullString(string(t.UnsubscribeCategory)),
	})
	return wrap(err, "insert user token")
}

// FindUserTokenByHash implements identity.Repository.
func (r *IdentityRepository) FindUserTokenByHash(ctx context.Context, hash []byte) (identity.UserToken, error) {
	row, err := r.db.reader().GetUserTokenByHash(ctx, hash)
	if err != nil {
		return identity.UserToken{}, notFound(err, appidentity.ErrNotFound, "user token")
	}
	return identity.UserToken{
		ID: row.ID, UserID: row.UserID, Purpose: identity.TokenPurpose(row.Purpose), EmailAddressID: fromNullString(row.EmailAddressID),
		TokenHash: row.TokenHash, ExpiresAt: utc(row.ExpiresAt), IssuedAt: utc(row.IssuedAt),
		UnsubscribeCategory: identity.UnsubscribeCategory(fromNullString(row.UnsubscribeCategory)),
	}, nil
}

// UserTokenRedeemed implements identity.Repository.
func (r *IdentityRepository) UserTokenRedeemed(ctx context.Context, tokenID string) (bool, error) {
	v, err := r.db.reader().UserTokenRedeemed(ctx, tokenID)
	if err != nil {
		return false, wrap(err, "read the redemption of a user token")
	}
	return v, nil
}

// InsertUserTokenRedemption implements identity.Repository.
func (r *IdentityRepository) InsertUserTokenRedemption(ctx context.Context, tx uow.Tx, rd identity.UserTokenRedemption) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserTokenRedemption(ctx, postgresgen.InsertUserTokenRedemptionParams{TokenID: rd.TokenID, RedeemedAt: rd.RedeemedAt})
	if isUniqueViolation(err, "user_token_redemptions_pkey") {
		return fmt.Errorf("redeem user token: %w", appidentity.ErrInvalidToken)
	}
	return wrap(err, "insert user token redemption")
}

// InsertEmailVerification implements identity.Repository.
func (r *IdentityRepository) InsertEmailVerification(ctx context.Context, tx uow.Tx, v identity.EmailVerification) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertEmailVerificationIfAbsent(ctx, postgresgen.InsertEmailVerificationIfAbsentParams{
		EmailAddressID: v.EmailAddressID, VerifiedAt: v.VerifiedAt,
	})
	return wrap(err, "insert email verification")
}

// FindPrimaryEmail implements identity.Repository.
func (r *IdentityRepository) FindPrimaryEmail(ctx context.Context, userID string) (identity.EmailAddress, bool, error) {
	row, err := r.db.reader().GetPrimaryEmail(ctx, userID)
	if err != nil {
		return identity.EmailAddress{}, false, notFound(err, appidentity.ErrNotFound, "primary email of "+userID)
	}
	return identity.EmailAddress{
		ID: row.ID, UserID: row.UserID, Address: row.Address, AddressNormalized: row.AddressNormalized, AddedAt: utc(row.AddedAt),
	}, row.Verified, nil
}
