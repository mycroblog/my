// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InsertOutboundActivities implements federation.Repository.
func (r *Repository) InsertOutboundActivities(ctx context.Context, tx uow.Tx, as []federation.OutboundActivity) error {
	if len(as) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	type row struct {
		ID             string `json:"id"`
		ActivityURI    string `json:"activity_uri"`
		Type           string `json:"type"`
		ActorAccountID string `json:"actor_account_id"`
		ObjectType     string `json:"object_type"`
		ObjectID       string `json:"object_id"`
		CreatedAt      int64  `json:"created_at"`
	}
	rows := make([]row, len(as))
	for i, a := range as {
		rows[i] = row{ID: a.ID, ActivityURI: string(a.ActivityURI), Type: string(a.Type), ActorAccountID: a.ActorAccountID,
			ObjectType: string(a.ObjectKind), ObjectID: a.ObjectID, CreatedAt: sqlite.ToMillis(a.CreatedAt)}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertOutboundActivities(ctx, string(list)), "insert outbound activities")
}
