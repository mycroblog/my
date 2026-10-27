// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// This file holds the received activities and their documents (federation.md §3.1).

var _ appfederation.InboxRepository = (*FederationRepository)(nil)

// InsertReceivedActivity implements federation.InboxRepository.
func (r *FederationRepository) InsertReceivedActivity(ctx context.Context, tx uow.Tx, a federation.ReceivedActivity) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReceivedActivity(ctx, sqlitegen.InsertReceivedActivityParams{
		ActivityUri: string(a.ActivityURI), ActorUri: string(a.ActorURI), Type: a.Type,
		SignatureScheme: a.Scheme, Trust: string(a.Trust), ReceivedAt: toMillis(a.ReceivedAt),
	})
	return n == 1, wrap(err, "insert received activity")
}

// InsertReceivedDocument implements federation.InboxRepository.
func (r *FederationRepository) InsertReceivedDocument(ctx context.Context, tx uow.Tx, activityURI federation.IRI, document []byte) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertReceivedActivityDocument(ctx, sqlitegen.InsertReceivedActivityDocumentParams{
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
		Scheme: row.SignatureScheme, Trust: federation.Trust(row.Trust), ReceivedAt: fromMillis(row.ReceivedAt),
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

// InsertRemoteObjectTombstone implements federation.InboxRepository.
func (r *FederationRepository) InsertRemoteObjectTombstone(ctx context.Context, tx uow.Tx, t federation.RemoteObjectTombstone) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteObjectTombstone(ctx, sqlitegen.InsertRemoteObjectTombstoneParams{
		Uri: string(t.URI), ActorUri: string(t.ActorURI), ActivityUri: string(t.ActivityURI), ReceivedAt: toMillis(t.ReceivedAt),
	})
	return n == 1, wrap(err, "insert remote object tombstone")
}

// RemoteObjectTombstoned implements federation.InboxRepository.
func (r *FederationRepository) RemoteObjectTombstoned(ctx context.Context, uri federation.IRI) (bool, error) {
	ok, err := r.db.reader().HasRemoteObjectTombstone(ctx, string(uri))
	return ok, wrap(err, "read remote object tombstone")
}
