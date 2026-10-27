// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// InsertEmailDeliveryFailure implements identity.Repository.
func (r *IdentityRepository) InsertEmailDeliveryFailure(ctx context.Context, tx uow.Tx, f identity.EmailDeliveryFailure) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertEmailDeliveryFailure(ctx, sqlitegen.InsertEmailDeliveryFailureParams{
		ID: f.ID, EmailAddressID: f.EmailAddressID, Kind: string(f.Kind), Source: string(f.Source), FeedbackKey: f.FeedbackKey,
		EnhancedStatus: toNullString(f.EnhancedStatus), OccurredAt: toMillis(f.OccurredAt),
	})
	return n > 0, wrap(err, "insert email delivery failure")
}

// InsertEmailSuppressionLift implements identity.Repository.
func (r *IdentityRepository) InsertEmailSuppressionLift(ctx context.Context, tx uow.Tx, l identity.EmailSuppressionLift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertEmailSuppressionLift(ctx, sqlitegen.InsertEmailSuppressionLiftParams{
		ID: l.ID, EmailAddressID: l.EmailAddressID, Reason: string(l.Reason), LiftedAt: toMillis(l.LiftedAt),
	})
	return wrap(err, "insert email suppression lift")
}

// RefreshEmailSuppression implements identity.Repository.
func (r *IdentityRepository) RefreshEmailSuppression(ctx context.Context, tx uow.Tx, emailAddressID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteEmailSuppression(ctx, emailAddressID); err != nil {
		return wrap(err, "delete email suppression")
	}
	return wrap(q.DeriveEmailSuppression(ctx, emailAddressID), "derive email suppression")
}

// FindEmailSuppression implements identity.Repository.
func (r *IdentityRepository) FindEmailSuppression(ctx context.Context, emailAddressID string) (identity.EmailSuppression, error) {
	row, err := r.db.reader().GetEmailSuppression(ctx, emailAddressID)
	if err != nil {
		return identity.EmailSuppression{}, notFound(err, appidentity.ErrNotFound, "email suppression of "+emailAddressID)
	}
	return identity.EmailSuppression{
		EmailAddressID: row.EmailAddressID, Kind: identity.EmailFailureKind(row.Kind), FailureID: row.FailureID, Since: fromMillis(row.OccurredAt),
	}, nil
}

// FindPrimaryEmailAddressID implements identity.Repository.
func (r *IdentityRepository) FindPrimaryEmailAddressID(ctx context.Context, normalized string) (string, error) {
	id, err := r.db.reader().GetPrimaryEmailAddressIDByNormalized(ctx, normalized)
	if err != nil {
		return "", notFound(err, appidentity.ErrNotFound, "primary email address")
	}
	return id, nil
}

// RebuildEmailSuppressions implements identity.Repository.
func (r *IdentityRepository) RebuildEmailSuppressions(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild email suppressions", (*sqlitegen.Queries).DeleteAllEmailSuppressions, (*sqlitegen.Queries).RebuildEmailSuppressions)
}
