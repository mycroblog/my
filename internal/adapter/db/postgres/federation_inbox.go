// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// This file holds the received activities and their documents (federation.md §3.1).

var _ appfederation.InboxRepository = (*FederationRepository)(nil)

// InsertReceivedActivity implements federation.InboxRepository.
func (r *FederationRepository) InsertReceivedActivity(ctx context.Context, tx uow.Tx, a federation.ReceivedActivity) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReceivedActivity(ctx, postgresgen.InsertReceivedActivityParams{
		ActivityUri: string(a.ActivityURI), ActorUri: string(a.ActorURI), Type: a.Type,
		SignatureScheme: a.Scheme, Trust: string(a.Trust), ReceivedAt: a.ReceivedAt,
	})
	return n == 1, wrap(err, "insert received activity")
}

// InsertReceivedDocument implements federation.InboxRepository.
func (r *FederationRepository) InsertReceivedDocument(ctx context.Context, tx uow.Tx, activityURI federation.IRI, document []byte) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertReceivedActivityDocument(ctx, postgresgen.InsertReceivedActivityDocumentParams{
		ActivityUri: string(activityURI), Document: document,
	})
	return wrap(err, "insert received activity document")
}

// FindReceivedActivity implements federation.InboxRepository.
func (r *FederationRepository) FindReceivedActivity(ctx context.Context, activityURI federation.IRI) (federation.ReceivedActivity, error) {
	row, err := r.db.reader().GetReceivedActivity(ctx, string(activityURI))
	if err != nil {
		return federation.ReceivedActivity{}, notFound(err, appfederation.ErrNotFound, "received activity "+string(activityURI))
	}
	return federation.ReceivedActivity{
		ActivityURI: federation.IRI(row.ActivityUri), ActorURI: federation.IRI(row.ActorUri), Type: row.Type,
		Scheme: row.SignatureScheme, Trust: federation.Trust(row.Trust), ReceivedAt: utc(row.ReceivedAt),
	}, nil
}

// FindReceivedDocument implements federation.InboxRepository.
func (r *FederationRepository) FindReceivedDocument(ctx context.Context, activityURI federation.IRI) ([]byte, error) {
	doc, err := r.db.reader().GetReceivedActivityDocument(ctx, string(activityURI))
	if err != nil {
		return nil, notFound(err, appfederation.ErrNotFound, "document of received activity "+string(activityURI))
	}
	return doc, nil
}

// ReleaseUnconfirmedActivity implements federation.InboxRepository.
func (r *FederationRepository) ReleaseUnconfirmedActivity(ctx context.Context, tx uow.Tx, activityURI federation.IRI) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnconfirmedReceivedActivity(ctx, string(activityURI))
	if err != nil || n == 0 {
		return false, wrap(err, "delete unconfirmed received activity")
	}
	return true, wrap(q.DeleteReceivedActivityDocument(ctx, string(activityURI)), "delete received activity document")
}

// InsertRemoteObjectTombstone implements federation.InboxRepository.
func (r *FederationRepository) InsertRemoteObjectTombstone(ctx context.Context, tx uow.Tx, t federation.RemoteObjectTombstone) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteObjectTombstone(ctx, postgresgen.InsertRemoteObjectTombstoneParams{
		Uri: string(t.URI), ActorUri: string(t.ActorURI), ActivityUri: string(t.ActivityURI), ReceivedAt: t.ReceivedAt,
	})
	return n == 1, wrap(err, "insert remote object tombstone")
}

// RemoteObjectTombstoned implements federation.InboxRepository.
func (r *FederationRepository) RemoteObjectTombstoned(ctx context.Context, uri, actorURI federation.IRI) (bool, error) {
	ok, err := r.db.reader().HasRemoteObjectTombstone(ctx, postgresgen.HasRemoteObjectTombstoneParams{Uri: string(uri), ActorUri: string(actorURI)})
	return ok, wrap(err, "read remote object tombstone")
}
