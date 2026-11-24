// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/federationgen"
	"github.com/mycroblog/my/internal/domain/federation"
)

// This file holds the reads of the known remote servers for the admin API: their health, when
// they were first seen, and their latest NodeInfo observation.

// ListInstanceHealth implements federation.Repository.
func (r *Repository) ListInstanceHealth(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.KnownInstance, error) {
	rows, err := r.db.reader().ListInstanceHealth(ctx, federationgen.ListInstanceHealthParams{
		State: string(state), AfterDomain: after, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list instance health")
	}
	out := make([]federation.KnownInstance, len(rows))
	for i, row := range rows {
		out[i] = federation.KnownInstance{
			InstanceHealth: instanceHealth(federationgen.InstanceHealth{
				Domain: row.Domain, State: row.State, Since: row.Since, FailureDays: row.FailureDays, LastFailureDay: row.LastFailureDay,
			}),
			FirstSeenAt: sqlite.FromNullTime(row.FirstSeenAt),
			Software:    observationOf(row.Domain, row.ObservationID, row.SoftwareName, row.SoftwareVersion, row.ObservedAt),
		}
	}
	return out, nil
}

// ListKnownInstances implements federation.Repository.
func (r *Repository) ListKnownInstances(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.KnownInstance, error) {
	rows, err := r.db.reader().ListKnownInstances(ctx, federationgen.ListKnownInstancesParams{
		State: string(state), AfterDomain: after, BatchLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list known instances")
	}
	out := make([]federation.KnownInstance, len(rows))
	for i, row := range rows {
		health := federation.InstanceHealth{Domain: row.Domain, State: federation.InstanceAvailable}
		if row.State.Valid {
			health = instanceHealth(federationgen.InstanceHealth{
				Domain: row.Domain, State: row.State.String, Since: row.Since.Int64, FailureDays: row.FailureDays.Int64,
				LastFailureDay: row.LastFailureDay.String,
			})
		}
		out[i] = federation.KnownInstance{
			InstanceHealth: health,
			FirstSeenAt:    sqlite.FromNullTime(row.FirstSeenAt),
			Software:       observationOf(row.Domain, row.ObservationID, row.SoftwareName, row.SoftwareVersion, row.ObservedAt),
		}
	}
	return out, nil
}

// FindKnownInstance implements federation.Repository.
func (r *Repository) FindKnownInstance(ctx context.Context, domain string) (federation.KnownInstance, error) {
	out := federation.KnownInstance{InstanceHealth: federation.InstanceHealth{Domain: domain, State: federation.InstanceAvailable}}
	db := r.db.reader()
	health, err := db.GetInstanceHealth(ctx, domain)
	switch {
	case err == nil:
		out.InstanceHealth = instanceHealth(health)
	case !errors.Is(err, sql.ErrNoRows):
		return federation.KnownInstance{}, sqlite.Wrap(err, "instance health of "+domain)
	}
	seen, err := db.GetRemoteInstance(ctx, domain)
	switch {
	case err == nil:
		out.FirstSeenAt = sqlite.FromMillis(seen.FirstSeenAt)
	case !errors.Is(err, sql.ErrNoRows):
		return federation.KnownInstance{}, sqlite.Wrap(err, "remote instance "+domain)
	}
	o, err := db.GetLatestInstanceNodeinfoObservation(ctx, domain)
	switch {
	case err == nil:
		out.Software = federation.NodeInfoObservation{
			ID: o.ID, Domain: o.Domain, SoftwareName: o.SoftwareName, SoftwareVersion: o.SoftwareVersion,
			ObservedAt: sqlite.FromMillis(o.ObservedAt),
		}
	case !errors.Is(err, sql.ErrNoRows):
		return federation.KnownInstance{}, sqlite.Wrap(err, "NodeInfo observation of "+domain)
	}
	return out, nil
}

// observationOf reads the columns of a left-joined NodeInfo observation: the zero value when
// the server has none.
func observationOf(domain string, id, name, version sql.NullString, observedAt sql.NullInt64) federation.NodeInfoObservation {
	if !id.Valid {
		return federation.NodeInfoObservation{}
	}
	return federation.NodeInfoObservation{
		ID: id.String, Domain: domain, SoftwareName: name.String, SoftwareVersion: version.String,
		ObservedAt: sqlite.FromNullTime(observedAt),
	}
}
