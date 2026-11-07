// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/federationgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InsertFanOutCompletion implements federation.Repository.
func (r *Repository) InsertFanOutCompletion(ctx context.Context, tx uow.Tx, outboundActivityID string, completedAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertOutboundFanOutCompletion(ctx, federationgen.InsertOutboundFanOutCompletionParams{
		OutboundActivityID: outboundActivityID, CompletedAt: sqlite.ToMillis(completedAt),
	})
	return sqlite.Wrap(err, "insert outbound fan-out completion")
}

// InsertClosureDelete implements federation.Repository.
func (r *Repository) InsertClosureDelete(ctx context.Context, tx uow.Tx, d federation.ClosureDelete) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertClosureDelete(ctx, federationgen.InsertClosureDeleteParams{
		AccountID: d.AccountID, OutboundActivityID: d.OutboundActivityID, RecordedAt: sqlite.ToMillis(d.RecordedAt),
	})
	return n == 1, sqlite.Wrap(err, "insert closure delete")
}

// ClosureDeleteAccounts implements federation.Repository.
func (r *Repository) ClosureDeleteAccounts(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := r.db.reader().ListClosureDeleteAccounts(ctx, ids)
	return out, sqlite.Wrap(err, "list closure delete accounts")
}

// ClosureProgress implements federation.Repository.
func (r *Repository) ClosureProgress(ctx context.Context, maxAttempts int) (federation.ClosureProgress, error) {
	q := r.db.reader()
	accounts, err := q.CountClosureDeletes(ctx)
	if err != nil {
		return federation.ClosureProgress{}, sqlite.Wrap(err, "count closure deletes")
	}
	fanOuts, err := q.CountUnfinishedClosureFanOuts(ctx)
	if err != nil {
		return federation.ClosureProgress{}, sqlite.Wrap(err, "count unfinished closure fan-outs")
	}
	rows, err := q.CountClosureDeliveries(ctx, int64(maxAttempts))
	if err != nil {
		return federation.ClosureProgress{}, sqlite.Wrap(err, "count closure deliveries")
	}
	out := federation.ClosureProgress{Accounts: int(accounts), FanOuts: int(fanOuts), Deliveries: map[federation.ClosureDeliveryState]int{}}
	for _, row := range rows {
		out.Deliveries[federation.ClosureDeliveryStateOf(federation.DeliveryOutcome(row.Outcome), row.Exhausted != 0)] += int(row.Deliveries)
	}
	return out, nil
}

// CountPendingMoves implements federation.Repository.
func (r *Repository) CountPendingMoves(ctx context.Context, since time.Time, maxAttempts int) (int, error) {
	n, err := r.db.reader().CountPendingMoveSends(ctx, federationgen.CountPendingMoveSendsParams{Since: sqlite.ToMillis(since), MaxAttempts: int64(maxAttempts)})
	return int(n), sqlite.Wrap(err, "count pending moves")
}
