// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

var _ appfederation.InstanceRepository = (*FederationRepository)(nil)

// InsertRemoteInstance implements federation.InstanceRepository.
func (r *FederationRepository) InsertRemoteInstance(ctx context.Context, tx uow.Tx, domain string, firstSeenAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteInstance(ctx, postgresgen.InsertRemoteInstanceParams{Domain: domain, FirstSeenAt: firstSeenAt})
	return n > 0, wrap(err, "insert remote instance")
}

// LatestNodeInfoObservation implements federation.InstanceRepository.
func (r *FederationRepository) LatestNodeInfoObservation(ctx context.Context, tx uow.Tx, domain string) (federation.NodeInfoObservation, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return federation.NodeInfoObservation{}, err
	}
	row, err := q.GetLatestInstanceNodeinfoObservation(ctx, domain)
	if err != nil {
		return federation.NodeInfoObservation{}, notFound(err, appfederation.ErrNotFound, "NodeInfo observation of "+domain)
	}
	return federation.NodeInfoObservation{
		ID: row.ID, Domain: row.Domain, SoftwareName: row.SoftwareName, SoftwareVersion: row.SoftwareVersion,
		ObservedAt: utc(row.ObservedAt),
	}, nil
}

// InsertNodeInfoObservation implements federation.InstanceRepository.
func (r *FederationRepository) InsertNodeInfoObservation(ctx context.Context, tx uow.Tx, o federation.NodeInfoObservation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceNodeinfoObservation(ctx, postgresgen.InsertInstanceNodeinfoObservationParams{
		ID: o.ID, Domain: o.Domain, SoftwareName: o.SoftwareName, SoftwareVersion: o.SoftwareVersion,
		ObservedAt: o.ObservedAt,
	})
	return wrap(err, "insert NodeInfo observation")
}
