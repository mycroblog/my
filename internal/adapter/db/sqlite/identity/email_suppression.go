// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertEmailDeliveryFailure implements identity.Repository.
func (r *Repository) InsertEmailDeliveryFailure(ctx context.Context, tx uow.Tx, f identity.EmailDeliveryFailure) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertEmailDeliveryFailure(ctx, identitygen.InsertEmailDeliveryFailureParams{
		ID: f.ID, EmailAddressID: f.EmailAddressID, Kind: string(f.Kind), Source: string(f.Source), FeedbackKey: f.FeedbackKey,
		EnhancedStatus: sqlite.ToNullString(f.EnhancedStatus), OccurredAt: sqlite.ToMillis(f.OccurredAt),
	})
	return n > 0, sqlite.Wrap(err, "insert email delivery failure")
}

// InsertEmailSuppressionLift implements identity.Repository.
func (r *Repository) InsertEmailSuppressionLift(ctx context.Context, tx uow.Tx, l identity.EmailSuppressionLift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertEmailSuppressionLift(ctx, identitygen.InsertEmailSuppressionLiftParams{
		ID: l.ID, EmailAddressID: l.EmailAddressID, Reason: string(l.Reason), LiftedAt: sqlite.ToMillis(l.LiftedAt),
	})
	return sqlite.Wrap(err, "insert email suppression lift")
}

// RefreshEmailSuppression implements identity.Repository.
func (r *Repository) RefreshEmailSuppression(ctx context.Context, tx uow.Tx, emailAddressID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteEmailSuppression(ctx, emailAddressID); err != nil {
		return sqlite.Wrap(err, "delete email suppression")
	}
	return sqlite.Wrap(q.DeriveEmailSuppression(ctx, emailAddressID), "derive email suppression")
}

// FindEmailSuppression implements identity.Repository.
func (r *Repository) FindEmailSuppression(ctx context.Context, emailAddressID string) (identity.EmailSuppression, error) {
	row, err := r.db.reader().GetEmailSuppression(ctx, emailAddressID)
	if err != nil {
		return identity.EmailSuppression{}, sqlite.NotFound(err, appidentity.ErrNotFound, "email suppression of "+emailAddressID)
	}
	return identity.EmailSuppression{
		EmailAddressID: row.EmailAddressID, Kind: identity.EmailFailureKind(row.Kind), FailureID: row.FailureID, Since: sqlite.FromMillis(row.OccurredAt),
	}, nil
}

// FindPrimaryEmailAddressID implements identity.Repository.
func (r *Repository) FindPrimaryEmailAddressID(ctx context.Context, normalized string) (string, error) {
	id, err := r.db.reader().GetPrimaryEmailAddressIDByNormalized(ctx, normalized)
	if err != nil {
		return "", sqlite.NotFound(err, appidentity.ErrNotFound, "primary email address")
	}
	return id, nil
}

// RebuildEmailSuppressions implements identity.Repository.
func (r *Repository) RebuildEmailSuppressions(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild email suppressions", (*identitygen.Queries).DeleteAllEmailSuppressions, (*identitygen.Queries).RebuildEmailSuppressions)
}
