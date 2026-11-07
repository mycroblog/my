// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

var _ appidentity.AccountAdminRepository = (*Repository)(nil)

// ListUserIDsByEmailDomain implements identity.AccountAdminRepository.
func (r *Repository) ListUserIDsByEmailDomain(ctx context.Context, domain, before string, limit int) ([]string, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUserIDsByEmailDomain(ctx, identitygen.ListUserIDsByEmailDomainParams{
		Suffix: "@" + domain, Before: postgres.ToNullString(before), Lim: lim,
	})
	return ids, postgres.Wrap(err, "list users by email domain")
}

// ListUserIDsByIP implements identity.AccountAdminRepository.
func (r *Repository) ListUserIDsByIP(ctx context.Context, ip, before string, limit int) ([]string, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUserIDsByIP(ctx, identitygen.ListUserIDsByIPParams{Ip: ip, Before: postgres.ToNullString(before), Lim: lim})
	return ids, postgres.Wrap(err, "list users by IP address")
}

// ListUserIDsInRegistrationState implements identity.AccountAdminRepository.
func (r *Repository) ListUserIDsInRegistrationState(ctx context.Context, state identity.RegistrationState, before string, limit int) ([]string, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListUserIDsInRegistrationState(ctx, identitygen.ListUserIDsInRegistrationStateParams{
		State: string(state), Before: postgres.ToNullString(before), Lim: lim,
	})
	return ids, postgres.Wrap(err, "list users in a registration state")
}

// ListSessionOriginsOfUser implements identity.AccountAdminRepository.
func (r *Repository) ListSessionOriginsOfUser(ctx context.Context, userID string, limit int) ([]identity.SessionOrigin, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListSessionOriginsOfUser(ctx, identitygen.ListSessionOriginsOfUserParams{UserID: userID, Lim: lim})
	if err != nil {
		return nil, postgres.Wrap(err, "list session origins of a user")
	}
	out := make([]identity.SessionOrigin, len(rows))
	for i, row := range rows {
		out[i] = identity.SessionOrigin{
			SessionID: row.SessionID, UserID: row.UserID, IP: postgres.FromNullString(row.Ip), UserAgent: postgres.FromNullString(row.UserAgent),
			CreatedAt: postgres.UTC(row.CreatedAt),
		}
	}
	return out, nil
}
