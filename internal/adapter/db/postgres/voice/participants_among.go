// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/app/uow"
)

// ParticipantsAmong implements voice.Repository.
func (r *Repository) ParticipantsAmong(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.q(tx)
	if err != nil {
		return nil, err
	}
	list := accountIDs
	ids, err := q.ListVoiceRoomParticipantsAmong(ctx, list)
	if err != nil {
		return nil, postgres.Wrap(err, "list participants among accounts")
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}
