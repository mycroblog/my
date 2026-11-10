// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// PrivateMentionTx implements content.Repository.
func (r *StatusRepository) PrivateMentionTx(ctx context.Context, tx uow.Tx, statusID, recipientID, sourceID string) (direct, solicited bool, err error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, false, err
	}
	row, err := q.PrivateMention(ctx, contentgen.PrivateMentionParams{StatusID: statusID, RecipientID: recipientID, SourceID: sourceID})
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, sqlite.Wrap(err, "read private mention")
	}
	return row.Direct == 1, row.Solicited == 1, nil
}
