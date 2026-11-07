// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/instancegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertSecret implements appinstance.SecretRepository.
func (r *Repository) InsertSecret(ctx context.Context, tx uow.Tx, s instance.Secret) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertInstanceSecret(ctx, instancegen.InsertInstanceSecretParams{
		ID: s.ID, Name: string(s.Name), SealedSecret: s.Sealed, CreatedAt: sqlite.ToMillis(s.CreatedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert instance secret")
}

// FindSecret implements appinstance.SecretRepository.
func (r *Repository) FindSecret(ctx context.Context, name instance.SecretName) (instance.Secret, error) {
	row, err := r.db.reader().GetInstanceSecret(ctx, string(name))
	if err != nil {
		return instance.Secret{}, sqlite.NotFound(err, appinstance.ErrNotFound, "instance secret "+string(name))
	}
	return instance.Secret{ID: row.ID, Name: instance.SecretName(row.Name), Sealed: row.SealedSecret, CreatedAt: sqlite.FromMillis(row.CreatedAt)}, nil
}
