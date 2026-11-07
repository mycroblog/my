// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ appaccount.ProfilePruneRepository = (*Repository)(nil)

// ListRemoteAccountsWithPastProfiles implements account.ProfilePruneRepository.
func (r *Repository) ListRemoteAccountsWithPastProfiles(ctx context.Context, after string, limit int) ([]string, error) {
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListRemoteAccountsWithPastProfiles(ctx, accountgen.ListRemoteAccountsWithPastProfilesParams{
		After: postgres.ToNullString(after), BatchLimit: batch,
	})
	return ids, postgres.Wrap(err, "list remote accounts with past profiles")
}

// PrunePastProfiles implements account.ProfilePruneRepository.
func (r *Repository) PrunePastProfiles(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range []func(context.Context, string) error{
		q.PrunePastProfileFields, q.PrunePastProfileRevisionEmojis, q.PrunePastRemoteProfileRevisions, q.PrunePastProfileRevisions, q.PrunePastRemoteSnapshots,
	} {
		if err := step(ctx, accountID); err != nil {
			return postgres.Wrap(err, "prune past profiles")
		}
	}
	return nil
}

// DeleteHandleVerificationsBefore implements account.ProfilePruneRepository.
func (r *Repository) DeleteHandleVerificationsBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	batch, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteHandleVerificationsBefore(ctx, accountgen.DeleteHandleVerificationsBeforeParams{
		IDBefore: kernel.UUIDv7Floor(t.Add(time.Millisecond)), CheckedBefore: t, BatchLimit: batch,
	})
	return n, postgres.Wrap(err, "delete handle verifications")
}
