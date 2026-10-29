// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertSecret implements appinstance.SecretRepository.
func (r *InstanceRepository) InsertSecret(ctx context.Context, tx uow.Tx, s instance.Secret) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertInstanceSecret(ctx, sqlitegen.InsertInstanceSecretParams{
		ID: s.ID, Name: string(s.Name), SealedSecret: s.Sealed, CreatedAt: toMillis(s.CreatedAt),
	})
	return n > 0, wrap(err, "insert instance secret")
}

// FindSecret implements appinstance.SecretRepository.
func (r *InstanceRepository) FindSecret(ctx context.Context, name instance.SecretName) (instance.Secret, error) {
	row, err := r.db.reader().GetInstanceSecret(ctx, string(name))
	if err != nil {
		return instance.Secret{}, notFound(err, appinstance.ErrNotFound, "instance secret "+string(name))
	}
	return instance.Secret{ID: row.ID, Name: instance.SecretName(row.Name), Sealed: row.SealedSecret, CreatedAt: fromMillis(row.CreatedAt)}, nil
}
