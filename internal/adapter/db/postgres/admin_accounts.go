// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The reads of the moderators' account management (FR-714) in the account, identity, and
// moderation repositories.

var (
	_ appaccount.SearchRepository          = (*AccountRepository)(nil)
	_ appidentity.AccountAdminRepository   = (*IdentityRepository)(nil)
	_ appmoderation.AccountAdminRepository = (*ModerationRepository)(nil)
)

// SearchAccountIDs implements account.SearchRepository.
func (r *AccountRepository) SearchAccountIDs(ctx context.Context, q appaccount.SearchQuery, limit int) ([]string, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().SearchAccountIDs(ctx, accountgen.SearchAccountIDsParams{
		Domain: q.Domain, Origin: q.Origin, UsernamePrefix: q.UsernamePrefix, Before: toNullString(q.Before), Lim: lim,
	})
	return ids, wrap(err, "search accounts")
}

// ListAccountIDsInLifecycleState implements account.SearchRepository.
func (r *AccountRepository) ListAccountIDsInLifecycleState(ctx context.Context, state account.LifecycleStateName, before string, limit int) ([]string, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListAccountIDsInLifecycleState(ctx, accountgen.ListAccountIDsInLifecycleStateParams{
		State: string(state), Before: toNullString(before), Lim: lim,
	})
	return ids, wrap(err, "list accounts in a lifecycle state")
}

// ListUserIDsByEmailDomain implements identity.AccountAdminRepository.
func (r *IdentityRepository) ListUserIDsByEmailDomain(ctx context.Context, domain, before string, limit int) ([]string, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUserIDsByEmailDomain(ctx, identitygen.ListUserIDsByEmailDomainParams{
		Suffix: "@" + domain, Before: toNullString(before), Lim: lim,
	})
	return ids, wrap(err, "list users by email domain")
}

// ListUserIDsByIP implements identity.AccountAdminRepository.
func (r *IdentityRepository) ListUserIDsByIP(ctx context.Context, ip, before string, limit int) ([]string, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUserIDsByIP(ctx, identitygen.ListUserIDsByIPParams{Ip: ip, Before: toNullString(before), Lim: lim})
	return ids, wrap(err, "list users by IP address")
}

// ListUserIDsInRegistrationState implements identity.AccountAdminRepository.
func (r *IdentityRepository) ListUserIDsInRegistrationState(ctx context.Context, state identity.RegistrationState, before string, limit int) ([]string, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUserIDsInRegistrationState(ctx, identitygen.ListUserIDsInRegistrationStateParams{
		State: string(state), Before: toNullString(before), Lim: lim,
	})
	return ids, wrap(err, "list users in a registration state")
}

// ListSessionOriginsOfUser implements identity.AccountAdminRepository.
func (r *IdentityRepository) ListSessionOriginsOfUser(ctx context.Context, userID string, limit int) ([]identity.SessionOrigin, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListSessionOriginsOfUser(ctx, identitygen.ListSessionOriginsOfUserParams{UserID: userID, Lim: lim})
	if err != nil {
		return nil, wrap(err, "list session origins of a user")
	}
	out := make([]identity.SessionOrigin, len(rows))
	for i, row := range rows {
		out[i] = identity.SessionOrigin{
			SessionID: row.SessionID, UserID: row.UserID, IP: fromNullString(row.Ip), UserAgent: fromNullString(row.UserAgent),
			CreatedAt: utc(row.CreatedAt),
		}
	}
	return out, nil
}

// ListRestrictedAccountIDs implements moderation.AccountAdminRepository.
func (r *ModerationRepository) ListRestrictedAccountIDs(ctx context.Context, restriction moderation.Restriction, now time.Time, before string, limit int) ([]string, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListRestrictedAccountIDs(ctx, moderationgen.ListRestrictedAccountIDsParams{
		Restriction: string(restriction), Now: now, Before: toNullString(before), Lim: lim,
	})
	return ids, wrap(err, "list restricted accounts")
}
