// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/federationgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

var _ appfederation.InstanceRepository = (*Repository)(nil)

// InsertRemoteInstance implements federation.InstanceRepository.
func (r *Repository) InsertRemoteInstance(ctx context.Context, tx uow.Tx, domain string, firstSeenAt time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteInstance(ctx, federationgen.InsertRemoteInstanceParams{Domain: domain, FirstSeenAt: sqlite.ToMillis(firstSeenAt)})
	return n > 0, sqlite.Wrap(err, "insert remote instance")
}

// LatestNodeInfoObservation implements federation.InstanceRepository.
func (r *Repository) LatestNodeInfoObservation(ctx context.Context, tx uow.Tx, domain string) (federation.NodeInfoObservation, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return federation.NodeInfoObservation{}, err
	}
	row, err := q.GetLatestInstanceNodeinfoObservation(ctx, domain)
	if err != nil {
		return federation.NodeInfoObservation{}, sqlite.NotFound(err, appfederation.ErrNotFound, "NodeInfo observation of "+domain)
	}
	return federation.NodeInfoObservation{
		ID: row.ID, Domain: row.Domain, SoftwareName: row.SoftwareName, SoftwareVersion: row.SoftwareVersion,
		ObservedAt: sqlite.FromMillis(row.ObservedAt),
	}, nil
}

// InsertNodeInfoObservation implements federation.InstanceRepository.
func (r *Repository) InsertNodeInfoObservation(ctx context.Context, tx uow.Tx, o federation.NodeInfoObservation) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertInstanceNodeinfoObservation(ctx, federationgen.InsertInstanceNodeinfoObservationParams{
		ID: o.ID, Domain: o.Domain, SoftwareName: o.SoftwareName, SoftwareVersion: o.SoftwareVersion,
		ObservedAt: sqlite.ToMillis(o.ObservedAt),
	})
	return sqlite.Wrap(err, "insert NodeInfo observation")
}
