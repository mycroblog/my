// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The reads behind the moderator notifications and the counts of what awaits the moderators
// (FR-712).

// HasOtherOpenReportOf implements moderation.Repository.
func (r *ModerationRepository) HasOtherOpenReportOf(ctx context.Context, tx uow.Tx, targetAccountID, reportID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.HasOtherOpenReportOf(ctx, postgresgen.HasOtherOpenReportOfParams{TargetAccountID: targetAccountID, ReportID: reportID})
	return ok, wrap(err, "find other open reports")
}

// CountReports implements moderation.Repository.
func (r *ModerationRepository) CountReports(ctx context.Context, state moderation.ReportState) (int64, error) {
	n, err := r.db.reader().CountReportsInState(ctx, string(state))
	return n, wrap(err, "count reports")
}

// ListUserIDsWithPermission implements identity.Repository.
func (r *IdentityRepository) ListUserIDsWithPermission(ctx context.Context, tx uow.Tx, p identity.Permission) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListUserIDsWithPermission(ctx, string(p))
	return ids, wrap(err, "list users with a permission")
}

// CountRegistrations implements identity.RegistrationRepository.
func (r *IdentityRepository) CountRegistrations(ctx context.Context, state identity.RegistrationState) (int64, error) {
	n, err := r.db.reader().CountRegistrationsInState(ctx, string(state))
	return n, wrap(err, "count registrations")
}
