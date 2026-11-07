// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

var _ appidentity.AccountAdminRepository = (*Repository)(nil)

// ListUserIDsByEmailDomain implements identity.AccountAdminRepository.
func (r *Repository) ListUserIDsByEmailDomain(ctx context.Context, domain, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUserIDsByEmailDomain(ctx, identitygen.ListUserIDsByEmailDomainParams{
		Suffix: "@" + domain, Before: before, Lim: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list users by email domain")
}

// ListUserIDsByIP implements identity.AccountAdminRepository.
func (r *Repository) ListUserIDsByIP(ctx context.Context, ip, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUserIDsByIP(ctx, identitygen.ListUserIDsByIPParams{Ip: ip, Before: before, Lim: int64(limit)})
	return ids, sqlite.Wrap(err, "list users by IP address")
}

// ListUserIDsInRegistrationState implements identity.AccountAdminRepository.
func (r *Repository) ListUserIDsInRegistrationState(ctx context.Context, state identity.RegistrationState, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListUserIDsInRegistrationState(ctx, identitygen.ListUserIDsInRegistrationStateParams{
		State: string(state), Before: before, Lim: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list users in a registration state")
}

// ListSessionOriginsOfUser implements identity.AccountAdminRepository.
func (r *Repository) ListSessionOriginsOfUser(ctx context.Context, userID string, limit int) ([]identity.SessionOrigin, error) {
	rows, err := r.db.reader().ListSessionOriginsOfUser(ctx, identitygen.ListSessionOriginsOfUserParams{UserID: userID, Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list session origins of a user")
	}
	out := make([]identity.SessionOrigin, len(rows))
	for i, row := range rows {
		out[i] = identity.SessionOrigin{
			SessionID: row.SessionID, UserID: row.UserID, IP: row.Ip.String, UserAgent: row.UserAgent.String,
			CreatedAt: sqlite.FromMillis(row.CreatedAt),
		}
	}
	return out, nil
}
