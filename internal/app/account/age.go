// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
)

// CreatedAtTx returns, inside tx, when the account was created: the registration of a local
// account, and the time this server first knew a remote one. The notification policy's
// new_account condition reads it (ADR-0158 §6).
func (s *Service) CreatedAtTx(ctx context.Context, tx uow.Tx, id string) (time.Time, error) {
	a, err := s.d.Repository.FindAccountTx(ctx, tx, id)
	if err != nil {
		return time.Time{}, err
	}
	return a.CreatedAt, nil
}
