// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/instancegen"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

// InsertDomains implements appinstance.DomainsRepository.
func (r *InstanceRepository) InsertDomains(ctx context.Context, tx uow.Tx, d instance.Domains) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertInstanceDomains(ctx, instancegen.InsertInstanceDomainsParams{
		Host: d.Host, AccountDomain: d.AccountDomain, RecordedAt: d.RecordedAt,
	})
	return n > 0, wrap(err, "insert instance domains")
}

// FindDomains implements appinstance.DomainsRepository.
func (r *InstanceRepository) FindDomains(ctx context.Context) (instance.Domains, error) {
	row, err := r.db.reader().GetInstanceDomains(ctx)
	if err != nil {
		return instance.Domains{}, notFound(err, appinstance.ErrNotFound, "instance domains")
	}
	return instance.Domains{Host: row.Host, AccountDomain: row.AccountDomain, RecordedAt: row.RecordedAt.UTC()}, nil
}
