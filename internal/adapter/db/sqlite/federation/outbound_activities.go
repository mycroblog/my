// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/federationgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InsertOutboundDocuments implements federation.Repository.
func (r *Repository) InsertOutboundDocuments(ctx context.Context, tx uow.Tx, documents map[string][]byte, builtAt time.Time) error {
	if len(documents) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	type row struct {
		ID       string `json:"id"`
		Document string `json:"document"`
	}
	rows := make([]row, 0, len(documents))
	for _, id := range slices.Sorted(maps.Keys(documents)) {
		rows = append(rows, row{ID: id, Document: hex.EncodeToString(documents[id])})
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertOutboundActivityDocuments(ctx, federationgen.InsertOutboundActivityDocumentsParams{BuiltAt: sqlite.ToMillis(builtAt), Documents: string(list)}), "insert outbound activity documents")
}

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
