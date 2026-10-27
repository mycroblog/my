// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

var _ appfederation.RelayRepository = (*FederationRepository)(nil)

// relayRow is the shape of every query that reads a relay with its state; the state's columns
// are NULL for a relay without one.
type relayRow = sqlitegen.GetRelayRow

func relayRecord(row relayRow) appfederation.RelayRecord {
	rec := appfederation.RelayRecord{Relay: federation.Relay{
		ID: row.ID, Protocol: federation.RelayProtocol(row.Protocol), URL: federation.IRI(row.Url),
		InboxURL: federation.IRI(row.InboxUrl), Receive: row.Receive != 0, CreatedByUserID: row.CreatedByUserID.String,
		CreatedAt: fromMillis(row.CreatedAt),
	}}
	if row.State.Valid {
		rec.Status = federation.RelayStatus{
			RelayID: row.ID, State: federation.RelayState(row.State.String), FollowActivityURI: federation.IRI(row.FollowActivityUri.String),
			RelayActorURI: federation.IRI(row.RelayActorUri.String), Since: fromMillis(row.Since.Int64),
		}
	}
	return rec
}

// InsertRelay implements federation.RelayRepository.
func (r *FederationRepository) InsertRelay(ctx context.Context, tx uow.Tx, rl federation.Relay) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertRelay(ctx, sqlitegen.InsertRelayParams{
		ID: rl.ID, Protocol: string(rl.Protocol), Url: string(rl.URL), InboxUrl: string(rl.InboxURL), Receive: boolToInt(rl.Receive),
		CreatedByUserID: toNullString(rl.CreatedByUserID), CreatedAt: toMillis(rl.CreatedAt),
	})
	if isUniqueViolation(err, "relays.url") {
		return fmt.Errorf("%s: %w", rl.URL, appfederation.ErrRelayExists)
	}
	return wrap(err, "insert relay")
}

// LockRelay implements federation.RelayRepository.
func (r *FederationRepository) LockRelay(ctx context.Context, tx uow.Tx, id string) (appfederation.RelayRecord, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return appfederation.RelayRecord{}, err
	}
	row, err := q.LockRelay(ctx, id)
	if err != nil {
		return appfederation.RelayRecord{}, notFound(err, appfederation.ErrNotFound, "relay "+id)
	}
	return relayRecord(relayRow(row)), nil
}

// InsertRelayEvent implements federation.RelayRepository.
func (r *FederationRepository) InsertRelayEvent(ctx context.Context, tx uow.Tx, e federation.RelayEvent) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRelayEvent(ctx, sqlitegen.InsertRelayEventParams{
		ID: e.ID, RelayID: e.RelayID, Kind: string(e.Kind), FollowActivityUri: string(e.FollowActivityURI),
		RelayActorUri: toNullString(string(e.RelayActorURI)), ActorUserID: toNullString(e.ActorUserID), OccurredAt: toMillis(e.OccurredAt),
	})
	return n == 1, wrap(err, "insert relay event")
}

// UpsertRelayState implements federation.RelayRepository.
func (r *FederationRepository) UpsertRelayState(ctx context.Context, tx uow.Tx, s federation.RelayStatus) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertRelayState(ctx, sqlitegen.UpsertRelayStateParams{
		RelayID: s.RelayID, State: string(s.State), FollowActivityUri: string(s.FollowActivityURI),
		RelayActorUri: toNullString(string(s.RelayActorURI)), Since: toMillis(s.Since),
	})
	return wrap(err, "upsert relay state")
}

// DeleteRelay implements federation.RelayRepository.
func (r *FederationRepository) DeleteRelay(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteRelayState(ctx, id); err != nil {
		return wrap(err, "delete relay state")
	}
	if err := q.DeleteRelayEvents(ctx, id); err != nil {
		return wrap(err, "delete relay events")
	}
	return wrap(q.DeleteRelay(ctx, id), "delete relay")
}

// RebuildRelayStates implements federation.RelayRepository.
func (r *FederationRepository) RebuildRelayStates(ctx context.Context, tx uow.Tx) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	if err := q.DeleteAllRelayStates(ctx); err != nil {
		return wrap(err, "delete relay states")
	}
	return wrap(q.RebuildRelayStates(ctx), "rebuild relay states")
}

// FindRelay implements federation.RelayRepository.
func (r *FederationRepository) FindRelay(ctx context.Context, id string) (appfederation.RelayRecord, error) {
	row, err := r.db.reader().GetRelay(ctx, id)
	if err != nil {
		return appfederation.RelayRecord{}, notFound(err, appfederation.ErrNotFound, "relay "+id)
	}
	return relayRecord(row), nil
}

// ListRelays implements federation.RelayRepository.
func (r *FederationRepository) ListRelays(ctx context.Context) ([]appfederation.RelayRecord, error) {
	rows, err := r.db.reader().ListRelays(ctx)
	if err != nil {
		return nil, wrap(err, "list relays")
	}
	out := make([]appfederation.RelayRecord, len(rows))
	for i, row := range rows {
		out[i] = relayRecord(relayRow(row))
	}
	return out, nil
}

// FindRelayByFollow implements federation.RelayRepository.
func (r *FederationRepository) FindRelayByFollow(ctx context.Context, followURI federation.IRI) (appfederation.RelayRecord, error) {
	row, err := r.db.reader().GetRelayByFollow(ctx, string(followURI))
	if err != nil {
		return appfederation.RelayRecord{}, notFound(err, appfederation.ErrNotFound, "relay of "+string(followURI))
	}
	return relayRecord(relayRow(row)), nil
}

// FindAcceptedRelayByActor implements federation.RelayRepository.
func (r *FederationRepository) FindAcceptedRelayByActor(ctx context.Context, actorURI federation.IRI) (appfederation.RelayRecord, error) {
	row, err := r.db.reader().GetAcceptedRelayByActor(ctx, sql.NullString{String: string(actorURI), Valid: true})
	if err != nil {
		return appfederation.RelayRecord{}, notFound(err, appfederation.ErrNotFound, "accepted relay of "+string(actorURI))
	}
	return relayRecord(relayRow(row)), nil
}

// ListAcceptedRelayInboxes implements federation.RelayRepository.
func (r *FederationRepository) ListAcceptedRelayInboxes(ctx context.Context) ([]federation.IRI, error) {
	rows, err := r.db.reader().ListAcceptedRelayInboxes(ctx)
	if err != nil {
		return nil, wrap(err, "list accepted relay inboxes")
	}
	out := make([]federation.IRI, len(rows))
	for i, s := range rows {
		out[i] = federation.IRI(s)
	}
	return out, nil
}
