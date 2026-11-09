// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertRegistrationApplication implements identity.Repository.
func (r *Repository) InsertRegistrationApplication(ctx context.Context, tx uow.Tx, a identity.RegistrationApplication) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationApplication(ctx, identitygen.InsertRegistrationApplicationParams{
		ID: a.ID, UserID: a.UserID, Reason: postgres.ToNullString(a.Reason), InviteID: postgres.ToNullString(a.InviteID), SubmittedAt: a.SubmittedAt,
	})
	return postgres.Wrap(err, "insert registration application")
}

// InsertRegistrationDecision implements identity.Repository.
func (r *Repository) InsertRegistrationDecision(ctx context.Context, tx uow.Tx, d identity.RegistrationDecision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRegistrationDecision(ctx, identitygen.InsertRegistrationDecisionParams{
		ID: d.ID, ApplicationID: d.ApplicationID, Decision: string(d.Decision), DecidedBy: string(d.DecidedBy),
		DecidedByUserID: postgres.ToNullString(d.DecidedByUserID), Note: postgres.ToNullString(d.Note), DecidedAt: d.DecidedAt,
	})
	return postgres.Wrap(err, "insert registration decision")
}

// UpsertRegistrationState implements identity.Repository.
func (r *Repository) UpsertRegistrationState(ctx context.Context, tx uow.Tx, s identity.UserRegistrationState) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertRegistrationState(ctx, identitygen.UpsertRegistrationStateParams{UserID: s.UserID, State: string(s.State), Since: s.Since})
	return postgres.Wrap(err, "upsert registration state")
}

// TransitionPendingRegistrationState implements identity.Repository.
func (r *Repository) TransitionPendingRegistrationState(ctx context.Context, tx uow.Tx, s identity.UserRegistrationState) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.TransitionPendingRegistrationState(ctx, identitygen.TransitionPendingRegistrationStateParams{
		State: string(s.State), Since: s.Since, UserID: s.UserID,
	})
	if err != nil {
		return false, postgres.Wrap(err, "transition pending registration state")
	}
	return n > 0, nil
}

// FindRegistrationApplication implements identity.Repository.
func (r *Repository) FindRegistrationApplication(ctx context.Context, userID string) (identity.RegistrationApplication, error) {
	row, err := r.db.reader().GetRegistrationApplicationByUser(ctx, userID)
	if err != nil {
		return identity.RegistrationApplication{}, postgres.NotFound(err, appidentity.ErrNotFound, "registration application of "+userID)
	}
	return identity.RegistrationApplication{
		ID: row.ID, UserID: row.UserID, Reason: postgres.FromNullString(row.Reason), InviteID: postgres.FromNullString(row.InviteID),
		SubmittedAt: postgres.UTC(row.SubmittedAt),
	}, nil
}

// FindRegistrationApplicationByID implements identity.Repository.
func (r *Repository) FindRegistrationApplicationByID(ctx context.Context, id string) (identity.RegistrationApplication, error) {
	row, err := r.db.reader().GetRegistrationApplication(ctx, id)
	if err != nil {
		return identity.RegistrationApplication{}, postgres.NotFound(err, appidentity.ErrNotFound, "registration application "+id)
	}
	return identity.RegistrationApplication{
		ID: row.ID, UserID: row.UserID, Reason: postgres.FromNullString(row.Reason), InviteID: postgres.FromNullString(row.InviteID),
		SubmittedAt: postgres.UTC(row.SubmittedAt),
	}, nil
}

// RegistrationDecisionNote implements identity.Repository.
func (r *Repository) RegistrationDecisionNote(ctx context.Context, applicationID string) (string, error) {
	note, err := r.db.reader().FindRegistrationDecisionNote(ctx, applicationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return postgres.FromNullString(note), postgres.Wrap(err, "the note of the decision on registration "+applicationID)
}

// FindRegistrationState implements identity.Repository.
func (r *Repository) FindRegistrationState(ctx context.Context, userID string) (identity.UserRegistrationState, error) {
	row, err := r.db.reader().GetRegistrationState(ctx, userID)
	if err != nil {
		return identity.UserRegistrationState{}, postgres.NotFound(err, appidentity.ErrNotFound, "registration state of "+userID)
	}
	return identity.UserRegistrationState{UserID: row.UserID, State: identity.RegistrationState(row.State), Since: postgres.UTC(row.Since)}, nil
}

// FindRegistrationStates implements identity.Repository.
func (r *Repository) FindRegistrationStates(ctx context.Context, userIDs []string) (map[string]identity.UserRegistrationState, error) {
	out := map[string]identity.UserRegistrationState{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListRegistrationStatesOf(ctx, userIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list registration states")
	}
	for _, row := range rows {
		out[row.UserID] = identity.UserRegistrationState{UserID: row.UserID, State: identity.RegistrationState(row.State), Since: postgres.UTC(row.Since)}
	}
	return out, nil
}

// ListRegistrations implements identity.Repository.
func (r *Repository) ListRegistrations(ctx context.Context, state identity.RegistrationState, after appidentity.RegistrationPosition, limit int) ([]appidentity.RegistrationEntry, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListRegistrationsInState(ctx, identitygen.ListRegistrationsInStateParams{
		State: string(state), AfterUserID: postgres.ToNullString(after.UserID), AfterSince: after.Since, PageLimit: n})
	if err != nil {
		return nil, postgres.Wrap(err, "list registrations")
	}
	out := make([]appidentity.RegistrationEntry, len(rows))
	for i, row := range rows {
		out[i] = appidentity.RegistrationEntry{
			Application: identity.RegistrationApplication{
				ID: row.ApplicationID, UserID: row.UserID, Reason: postgres.FromNullString(row.Reason), InviteID: postgres.FromNullString(row.InviteID),
				SubmittedAt: postgres.UTC(row.SubmittedAt),
			},
			State:         identity.UserRegistrationState{UserID: row.UserID, State: state, Since: postgres.UTC(row.Since)},
			Email:         row.Email,
			EmailVerified: row.EmailVerified,
		}
	}
	return out, nil
}

// InsertInvite implements identity.Repository.
func (r *Repository) InsertInvite(ctx context.Context, tx uow.Tx, i identity.Invite) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	var maxUses *int32
	if i.MaxUses > 0 {
		n, err := postgres.ToInt32(i.MaxUses, "max uses")
		if err != nil {
			return err
		}
		maxUses = &n
	}
	err = q.InsertInvite(ctx, identitygen.InsertInviteParams{
		ID: i.ID, Code: i.Code, CreatedByUserID: postgres.ToNullString(i.CreatedByUserID), CreatorKind: string(inviteCreatorKind(i.CreatorKind)),
		MaxUses: maxUses, ExpiresAt: i.ExpiresAt, CreatedAt: i.CreatedAt,
	})
	if postgres.IsUniqueViolation(err, "invites_code") {
		return fmt.Errorf("insert invite: %w", appidentity.ErrInviteCodeTaken)
	}
	return postgres.Wrap(err, "insert invite")
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
		ID: id, Code: code, CreatedByUserID: postgres.FromNullString(createdBy), CreatorKind: identity.InviteCreatorKind(creatorKind),
		ExpiresAt: postgres.UTCPtr(expiresAt), CreatedAt: postgres.UTC(createdAt),
	}
	if maxUses != nil {
		inv.MaxUses = int(*maxUses)
	}
	return inv
}

// FindInviteByCode implements identity.Repository.
func (r *Repository) FindInviteByCode(ctx context.Context, code string) (identity.Invite, error) {
	row, err := r.db.reader().GetInviteByCode(ctx, code)
	if err != nil {
		return identity.Invite{}, postgres.NotFound(err, appidentity.ErrNotFound, "invite")
	}
	return inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.CreatorKind, row.MaxUses, row.ExpiresAt, row.CreatedAt), nil
}

// FindInviteByID implements identity.Repository.
func (r *Repository) FindInviteByID(ctx context.Context, id string) (identity.Invite, error) {
	row, err := r.db.reader().GetInviteByID(ctx, id)
	if err != nil {
		return identity.Invite{}, postgres.NotFound(err, appidentity.ErrNotFound, "invite")
	}
	return inviteFrom(row.ID, row.Code, row.CreatedByUserID, row.CreatorKind, row.MaxUses, row.ExpiresAt, row.CreatedAt), nil
}

// InviteUsage implements identity.Repository.
func (r *Repository) InviteUsage(ctx context.Context, tx uow.Tx, inviteID string) (int, bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, false, err
	}
	row, err := q.GetInviteUsage(ctx, inviteID)
	if err != nil {
		return 0, false, postgres.Wrap(err, "get invite usage")
	}
	return int(row.Uses), row.Revoked, nil
}

// InsertInviteRedemption implements identity.Repository.
func (r *Repository) InsertInviteRedemption(ctx context.Context, tx uow.Tx, rd identity.InviteRedemption) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	seq, err := postgres.ToInt32(rd.Seq, "invite redemption seq")
	if err != nil {
		return err
	}
	err = q.InsertInviteRedemption(ctx, identitygen.InsertInviteRedemptionParams{
		InviteID: rd.InviteID, Seq: seq, UserID: rd.UserID, RedeemedAt: rd.RedeemedAt,
	})
	if postgres.IsUniqueViolation(err, "invite_redemptions_pkey") {
		return uow.MarkRetryable(postgres.Wrap(err, "insert invite redemption"))
	}
	return postgres.Wrap(err, "insert invite redemption")
}

// InsertInviteRevocation implements identity.Repository.
func (r *Repository) InsertInviteRevocation(ctx context.Context, tx uow.Tx, rv identity.InviteRevocation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInviteRevocationIfAbsent(ctx, identitygen.InsertInviteRevocationIfAbsentParams{
		InviteID: rv.InviteID, RevokedByUserID: postgres.ToNullString(rv.RevokedByUserID), RevokedAt: rv.RevokedAt,
	})
	return postgres.Wrap(err, "insert invite revocation")
}

// ListInvites implements identity.Repository.
func (r *Repository) ListInvites(ctx context.Context, before string, limit int) ([]appidentity.InviteStatus, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListInvites(ctx, identitygen.ListInvitesParams{Before: postgres.ToNullString(before), PageLimit: n})
	if err != nil {
		return nil, postgres.Wrap(err, "list invites")
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
func (r *Repository) ListMemberInvites(ctx context.Context, creatorUserID, before string, limit int) ([]appidentity.InviteStatus, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListMemberInvitesByCreator(ctx, identitygen.ListMemberInvitesByCreatorParams{
		Creator: &creatorUserID, Before: postgres.ToNullString(before), PageLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list member invites")
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
func (r *Repository) CountUsableMemberInvites(ctx context.Context, tx uow.Tx, creatorUserID string, now time.Time) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountUsableMemberInvites(ctx, identitygen.CountUsableMemberInvitesParams{Creator: &creatorUserID, Now: &now})
	if err != nil {
		return 0, postgres.Wrap(err, "count usable member invites")
	}
	return int(n), nil
}

// InsertUserToken implements identity.Repository.
func (r *Repository) InsertUserToken(ctx context.Context, tx uow.Tx, t identity.UserToken) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserToken(ctx, identitygen.InsertUserTokenParams{
		ID: t.ID, UserID: t.UserID, Purpose: string(t.Purpose), EmailAddressID: postgres.ToNullString(t.EmailAddressID),
		TokenHash: t.TokenHash, ExpiresAt: t.ExpiresAt, IssuedAt: t.IssuedAt,
		UnsubscribeCategory: postgres.ToNullString(string(t.UnsubscribeCategory)),
	})
	return postgres.Wrap(err, "insert user token")
}

// FindUserTokenByHash implements identity.Repository.
func (r *Repository) FindUserTokenByHash(ctx context.Context, hash []byte) (identity.UserToken, error) {
	row, err := r.db.reader().GetUserTokenByHash(ctx, hash)
	if err != nil {
		return identity.UserToken{}, postgres.NotFound(err, appidentity.ErrNotFound, "user token")
	}
	return identity.UserToken{
		ID: row.ID, UserID: row.UserID, Purpose: identity.TokenPurpose(row.Purpose), EmailAddressID: postgres.FromNullString(row.EmailAddressID),
		TokenHash: row.TokenHash, ExpiresAt: postgres.UTC(row.ExpiresAt), IssuedAt: postgres.UTC(row.IssuedAt),
		UnsubscribeCategory: identity.UnsubscribeCategory(postgres.FromNullString(row.UnsubscribeCategory)),
	}, nil
}

// UserTokenRedeemed implements identity.Repository.
func (r *Repository) UserTokenRedeemed(ctx context.Context, tokenID string) (bool, error) {
	v, err := r.db.reader().UserTokenRedeemed(ctx, tokenID)
	if err != nil {
		return false, postgres.Wrap(err, "read the redemption of a user token")
	}
	return v, nil
}

// InsertUserTokenRedemption implements identity.Repository.
func (r *Repository) InsertUserTokenRedemption(ctx context.Context, tx uow.Tx, rd identity.UserTokenRedemption) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertUserTokenRedemption(ctx, identitygen.InsertUserTokenRedemptionParams{TokenID: rd.TokenID, RedeemedAt: rd.RedeemedAt})
	if postgres.IsUniqueViolation(err, "user_token_redemptions_pkey") {
		return fmt.Errorf("redeem user token: %w", appidentity.ErrInvalidToken)
	}
	return postgres.Wrap(err, "insert user token redemption")
}

// InsertEmailVerification implements identity.Repository.
func (r *Repository) InsertEmailVerification(ctx context.Context, tx uow.Tx, v identity.EmailVerification) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertEmailVerificationIfAbsent(ctx, identitygen.InsertEmailVerificationIfAbsentParams{
		EmailAddressID: v.EmailAddressID, VerifiedAt: v.VerifiedAt,
	})
	return postgres.Wrap(err, "insert email verification")
}

// FindPrimaryEmail implements identity.Repository.
func (r *Repository) FindPrimaryEmail(ctx context.Context, userID string) (identity.EmailAddress, bool, error) {
	row, err := r.db.reader().GetPrimaryEmail(ctx, userID)
	if err != nil {
		return identity.EmailAddress{}, false, postgres.NotFound(err, appidentity.ErrNotFound, "primary email of "+userID)
	}
	return identity.EmailAddress{
		ID: row.ID, UserID: row.UserID, Address: row.Address, AddressNormalized: row.AddressNormalized, AddedAt: postgres.UTC(row.AddedAt),
	}, row.Verified, nil
}
