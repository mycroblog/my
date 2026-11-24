// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	"github.com/mycroblog/my/internal/domain/federation"
)

// This file holds the reads of the known remote servers for the admin API: their health, when
// they were first seen, and their latest NodeInfo observation.

// ListInstanceHealth implements federation.Repository.
func (r *Repository) ListInstanceHealth(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.KnownInstance, error) {
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListInstanceHealth(ctx, federationgen.ListInstanceHealthParams{
		State: string(state), AfterDomain: after, BatchLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list instance health")
	}
	out := make([]federation.KnownInstance, len(rows))
	for i, row := range rows {
		out[i] = federation.KnownInstance{
			InstanceHealth: instanceHealth(federationgen.InstanceHealth{
				Domain: row.Domain, State: row.State, Since: row.Since, FailureDays: row.FailureDays, LastFailureDay: row.LastFailureDay,
			}),
			FirstSeenAt: postgres.FromNullTime(row.FirstSeenAt),
			Software:    observationOf(row.Domain, row.ObservationID, row.SoftwareName, row.SoftwareVersion, row.ObservedAt),
		}
	}
	return out, nil
}

// ListKnownInstances implements federation.Repository.
func (r *Repository) ListKnownInstances(ctx context.Context, state federation.InstanceState, after string, limit int) ([]federation.KnownInstance, error) {
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListKnownInstances(ctx, federationgen.ListKnownInstancesParams{
		State: string(state), AfterDomain: after, BatchLimit: n,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list known instances")
	}
	out := make([]federation.KnownInstance, len(rows))
	for i, row := range rows {
		health := federation.InstanceHealth{Domain: row.Domain, State: federation.InstanceAvailable}
		if row.State != nil && row.Since != nil && row.FailureDays != nil && row.LastFailureDay != nil {
			health = instanceHealth(federationgen.InstanceHealth{
				Domain: row.Domain, State: *row.State, Since: *row.Since, FailureDays: *row.FailureDays,
				LastFailureDay: *row.LastFailureDay,
			})
		}
		out[i] = federation.KnownInstance{
			InstanceHealth: health,
			FirstSeenAt:    postgres.FromNullTime(row.FirstSeenAt),
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
	case !errors.Is(err, pgx.ErrNoRows):
		return federation.KnownInstance{}, postgres.Wrap(err, "instance health of "+domain)
	}
	seen, err := db.GetRemoteInstance(ctx, domain)
	switch {
	case err == nil:
		out.FirstSeenAt = postgres.UTC(seen.FirstSeenAt)
	case !errors.Is(err, pgx.ErrNoRows):
		return federation.KnownInstance{}, postgres.Wrap(err, "remote instance "+domain)
	}
	o, err := db.GetLatestInstanceNodeinfoObservation(ctx, domain)
	switch {
	case err == nil:
		out.Software = federation.NodeInfoObservation{
			ID: o.ID, Domain: o.Domain, SoftwareName: o.SoftwareName, SoftwareVersion: o.SoftwareVersion,
			ObservedAt: postgres.UTC(o.ObservedAt),
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return federation.KnownInstance{}, postgres.Wrap(err, "NodeInfo observation of "+domain)
	}
	return out, nil
}

// observationOf reads the columns of a left-joined NodeInfo observation: the zero value when
// the server has none.
func observationOf(domain string, id, name, version *string, observedAt *time.Time) federation.NodeInfoObservation {
	if id == nil {
		return federation.NodeInfoObservation{}
	}
	return federation.NodeInfoObservation{
		ID: *id, Domain: domain, SoftwareName: postgres.FromNullString(name), SoftwareVersion: postgres.FromNullString(version),
		ObservedAt: postgres.FromNullTime(observedAt),
	}
}
