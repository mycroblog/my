// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertRegistrationApplication implements identity.Repository.
func (r *IdentityRepository) InsertRegistrationApplication(ctx context.Context, tx uow.Tx, a identity.RegistrationApplication) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationApplication(ctx, sqlitegen.InsertRegistrationApplicationParams{
		ID: a.ID, UserID: a.UserID, Reason: toNullString(a.Reason), InviteID: toNullString(a.InviteID), SubmittedAt: toMillis(a.SubmittedAt),
	})
	return wrap(err, "insert registration application")
}

// InsertRegistrationDecision implements identity.Repository.
func (r *IdentityRepository) InsertRegistrationDecision(ctx context.Context, tx uow.Tx, d identity.RegistrationDecision) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationDecision(ctx, sqlitegen.InsertRegistrationDecisionParams{
		ID: d.ID, ApplicationID: d.ApplicationID, Decision: string(d.Decision), DecidedBy: string(d.DecidedBy),
		DecidedByUserID: toNullString(d.DecidedByUserID), Note: toNullString(d.Note), DecidedAt: toMillis(d.DecidedAt),
	})
	return wrap(err, "insert registration decision")
}

// UpsertRegistrationState implements identity.Repository.
func (r *IdentityRepository) UpsertRegistrationState(ctx context.Context, tx uow.Tx, s identity.UserRegistrationState) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertRegistrationState(ctx, sqlitegen.UpsertRegistrationStateParams{
		UserID: s.UserID, State: string(s.State), Since: toMillis(s.Since),
	})
	return wrap(err, "upsert registration state")
}

// TransitionPendingRegistrationState implements identity.Repository.
func (r *IdentityRepository) TransitionPendingRegistrationState(ctx context.Context, tx uow.Tx, s identity.UserRegistrationState) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.TransitionPendingRegistrationState(ctx, sqlitegen.TransitionPendingRegistrationStateParams{
		State: string(s.State), Since: toMillis(s.Since), UserID: s.UserID,
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
		ID: row.ID, UserID: row.UserID, Reason: row.Reason.String, InviteID: row.InviteID.String, SubmittedAt: fromMillis(row.SubmittedAt),
	}, nil
}

// FindRegistrationState implements identity.Repository.
func (r *IdentityRepository) FindRegistrationState(ctx context.Context, userID string) (identity.UserRegistrationState, error) {
	row, err := r.db.reader().GetRegistrationState(ctx, userID)
	if err != nil {
		return identity.UserRegistrationState{}, notFound(err, appidentity.ErrNotFound, "registration state of "+userID)
	}
	return identity.UserRegistrationState{UserID: row.UserID, State: identity.RegistrationState(row.State), Since: fromMillis(row.Since)}, nil
}

// ListRegistrations implements identity.Repository.
func (r *IdentityRepository) ListRegistrations(ctx context.Context, state identity.RegistrationState, after appidentity.RegistrationPosition, limit int) ([]appidentity.RegistrationEntry, error) {
	rows, err := r.db.reader().ListRegistrationsInState(ctx, sqlitegen.ListRegistrationsInStateParams{
		State: string(state), AfterUserID: after.UserID, AfterSince: toMillis(after.Since), PageLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list registrations")
	}
	out := make([]appidentity.RegistrationEntry, len(rows))
	for i, row := range rows {
		out[i] = appidentity.RegistrationEntry{
			Application: identity.RegistrationApplication{
				ID: row.ApplicationID, UserID: row.UserID, Reason: row.Reason.String, InviteID: row.InviteID.String,
				SubmittedAt: fromMillis(row.SubmittedAt),
			},
			State:         identity.UserRegistrationState{UserID: row.UserID, State: state, Since: fromMillis(row.Since)},
			Email:         row.Email,
			EmailVerified: row.EmailVerified,
		}
	}
	return out, nil
}

// InsertInvite implements identity.Repository.
func (r *IdentityRepository) InsertInvite(ctx context.Context, tx uow.Tx, i identity.Invite) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	maxUses := sql.NullInt64{Int64: int64(i.MaxUses), Valid: i.MaxUses > 0}
	err = q.InsertInvite(ctx, sqlitegen.InsertInviteParams{
		ID: i.ID, Code: i.Code, CreatedByUserID: toNullString(i.CreatedByUserID), MaxUses: maxUses,
		ExpiresAt: toNullMillis(i.ExpiresAt), CreatedAt: toMillis(i.CreatedAt),
	})
	if isUniqueViolation(err, "invites.code") {
		return fmt.Errorf("insert invite: %w", appidentity.ErrInviteCodeTaken)
	}
	return wrap(err, "insert invite")
}

func inviteFrom(id, code string, createdBy sql.NullString, maxUses, expiresAt sql.NullInt64, createdAt int64) identity.Invite {
	return identity.Invite{
		ID: id, Code: code, CreatedByUserID: createdBy.String, MaxUses: int(maxUses.Int64),
		ExpiresAt: fromNullMillis(expiresAt), CreatedAt: fromMillis(createdAt),
	}
}

// FindInviteByCode implements identity.Repository.
func (r *IdentityRepository) FindInviteByCode(ctx context.Context, code string) (identity.Invite, error) {
	row, err := r.db.reader().GetInviteByCode(ctx, code)
	if err != nil {
		return identity.Invite{}, notFound(err, appidentity.ErrNotFound, "invite")
	}
	return inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.MaxUses, row.ExpiresAt, row.CreatedAt), nil
}

// FindInviteByID implements identity.Repository.
func (r *IdentityRepository) FindInviteByID(ctx context.Context, id string) (identity.Invite, error) {
	row, err := r.db.reader().GetInviteByID(ctx, id)
	if err != nil {
		return identity.Invite{}, notFound(err, appidentity.ErrNotFound, "invite")
	}
	return inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.MaxUses, row.ExpiresAt, row.CreatedAt), nil
}

// InviteUsage implements identity.Repository.
func (r *IdentityRepository) InviteUsage(ctx context.Context, tx uow.Tx, inviteID string) (int, bool, error) {
	q, err := queries(tx)
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
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInviteRedemption(ctx, sqlitegen.InsertInviteRedemptionParams{
		InviteID: rd.InviteID, Seq: int64(rd.Seq), UserID: rd.UserID, RedeemedAt: toMillis(rd.RedeemedAt),
	})
	if isUniqueViolation(err, "invite_redemptions.invite_id, invite_redemptions.seq") {
		return uow.MarkRetryable(wrap(err, "insert invite redemption"))
	}
	return wrap(err, "insert invite redemption")
}

// InsertInviteRevocation implements identity.Repository.
func (r *IdentityRepository) InsertInviteRevocation(ctx context.Context, tx uow.Tx, rv identity.InviteRevocation) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInviteRevocationIfAbsent(ctx, sqlitegen.InsertInviteRevocationIfAbsentParams{
		InviteID: rv.InviteID, RevokedByUserID: toNullString(rv.RevokedByUserID), RevokedAt: toMillis(rv.RevokedAt),
	})
	return wrap(err, "insert invite revocation")
}

// ListInvites implements identity.Repository.
func (r *IdentityRepository) ListInvites(ctx context.Context, before string, limit int) ([]appidentity.InviteStatus, error) {
	rows, err := r.db.reader().ListInvites(ctx, sqlitegen.ListInvitesParams{Before: before, PageLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list invites")
	}
	out := make([]appidentity.InviteStatus, len(rows))
	for i, row := range rows {
		out[i] = appidentity.InviteStatus{
			Invite: inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.MaxUses, row.ExpiresAt, row.CreatedAt),
			Uses:   int(row.Uses), Revoked: row.Revoked,
		}
	}
	return out, nil
}

// InsertUserToken implements identity.Repository.
func (r *IdentityRepository) InsertUserToken(ctx context.Context, tx uow.Tx, t identity.UserToken) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserToken(ctx, sqlitegen.InsertUserTokenParams{
		ID: t.ID, UserID: t.UserID, Purpose: string(t.Purpose), EmailAddressID: toNullString(t.EmailAddressID),
		TokenHash: t.TokenHash, ExpiresAt: toMillis(t.ExpiresAt), IssuedAt: toMillis(t.IssuedAt),
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
		ID: row.ID, UserID: row.UserID, Purpose: identity.TokenPurpose(row.Purpose), EmailAddressID: row.EmailAddressID.String,
		TokenHash: row.TokenHash, ExpiresAt: fromMillis(row.ExpiresAt), IssuedAt: fromMillis(row.IssuedAt),
		UnsubscribeCategory: identity.UnsubscribeCategory(row.UnsubscribeCategory.String),
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
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserTokenRedemption(ctx, sqlitegen.InsertUserTokenRedemptionParams{TokenID: rd.TokenID, RedeemedAt: toMillis(rd.RedeemedAt)})
	if isUniqueViolation(err, "user_token_redemptions.token_id") {
		return fmt.Errorf("redeem user token: %w", appidentity.ErrInvalidToken)
	}
	return wrap(err, "insert user token redemption")
}

// InsertEmailVerification implements identity.Repository.
func (r *IdentityRepository) InsertEmailVerification(ctx context.Context, tx uow.Tx, v identity.EmailVerification) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertEmailVerificationIfAbsent(ctx, sqlitegen.InsertEmailVerificationIfAbsentParams{
		EmailAddressID: v.EmailAddressID, VerifiedAt: toMillis(v.VerifiedAt),
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
		ID: row.ID, UserID: row.UserID, Address: row.Address, AddressNormalized: row.AddressNormalized, AddedAt: fromMillis(row.AddedAt),
	}, row.Verified, nil
}
