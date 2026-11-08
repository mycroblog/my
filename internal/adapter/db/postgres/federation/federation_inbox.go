// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/federationgen"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// This file holds the received activities and their documents (federation.md §3.1).

var _ appfederation.InboxRepository = (*Repository)(nil)

// InsertReceivedActivity implements federation.InboxRepository.
func (r *Repository) InsertReceivedActivity(ctx context.Context, tx uow.Tx, a federation.ReceivedActivity) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReceivedActivity(ctx, federationgen.InsertReceivedActivityParams{
		ActivityUri: string(a.ActivityURI), ActorUri: string(a.ActorURI), Type: a.Type,
		SignatureScheme: a.Scheme, Trust: string(a.Trust), ReceivedAt: a.ReceivedAt,
	})
	return n == 1, postgres.Wrap(err, "insert received activity")
}

// InsertReceivedDocument implements federation.InboxRepository.
func (r *Repository) InsertReceivedDocument(ctx context.Context, tx uow.Tx, activityURI federation.IRI, document []byte) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertReceivedActivityDocument(ctx, federationgen.InsertReceivedActivityDocumentParams{
		ActivityUri: string(activityURI), Document: document,
	})
	return postgres.Wrap(err, "insert received activity document")
}

// FindReceivedActivity implements federation.InboxRepository.
func (r *Repository) FindReceivedActivity(ctx context.Context, activityURI federation.IRI) (federation.ReceivedActivity, error) {
	row, err := r.db.reader().GetReceivedActivity(ctx, string(activityURI))
	if err != nil {
		return federation.ReceivedActivity{}, postgres.NotFound(err, appfederation.ErrNotFound, "received activity "+string(activityURI))
	}
	return federation.ReceivedActivity{
		ActivityURI: federation.IRI(row.ActivityUri), ActorURI: federation.IRI(row.ActorUri), Type: row.Type,
		Scheme: row.SignatureScheme, Trust: federation.Trust(row.Trust), ReceivedAt: postgres.UTC(row.ReceivedAt),
	}, nil
}

// FindReceivedDocument implements federation.InboxRepository.
func (r *Repository) FindReceivedDocument(ctx context.Context, activityURI federation.IRI) ([]byte, error) {
	doc, err := r.db.reader().GetReceivedActivityDocument(ctx, string(activityURI))
	if err != nil {
		return nil, postgres.NotFound(err, appfederation.ErrNotFound, "document of received activity "+string(activityURI))
	}
	return doc, nil
}

// KeptReceivedActivityTx implements federation.InboxRepository.
func (r *Repository) KeptReceivedActivityTx(ctx context.Context, tx uow.Tx, activityURI federation.IRI) (federation.ReceivedActivity, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return federation.ReceivedActivity{}, err
	}
	if _, err := q.GetReceivedActivityDocument(ctx, string(activityURI)); err != nil {
		return federation.ReceivedActivity{}, postgres.NotFound(err, appfederation.ErrNotFound, "document of received activity "+string(activityURI))
	}
	row, err := q.GetReceivedActivity(ctx, string(activityURI))
	if err != nil {
		return federation.ReceivedActivity{}, postgres.NotFound(err, appfederation.ErrNotFound, "received activity "+string(activityURI))
	}
	return federation.ReceivedActivity{
		ActivityURI: federation.IRI(row.ActivityUri), ActorURI: federation.IRI(row.ActorUri), Type: row.Type,
		Scheme: row.SignatureScheme, Trust: federation.Trust(row.Trust), ReceivedAt: postgres.UTC(row.ReceivedAt),
	}, nil
}

// ReleaseUnconfirmedActivity implements federation.InboxRepository.
func (r *Repository) ReleaseUnconfirmedActivity(ctx context.Context, tx uow.Tx, activityURI federation.IRI) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteUnconfirmedReceivedActivity(ctx, string(activityURI))
	if err != nil || n == 0 {
		return false, postgres.Wrap(err, "delete unconfirmed received activity")
	}
	return true, postgres.Wrap(q.DeleteReceivedActivityDocument(ctx, string(activityURI)), "delete received activity document")
}

// InsertRemoteObjectTombstone implements federation.InboxRepository.
func (r *Repository) InsertRemoteObjectTombstone(ctx context.Context, tx uow.Tx, t federation.RemoteObjectTombstone) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertRemoteObjectTombstone(ctx, federationgen.InsertRemoteObjectTombstoneParams{
		Uri: string(t.URI), ActorUri: string(t.ActorURI), ActivityUri: string(t.ActivityURI), ReceivedAt: t.ReceivedAt,
	})
	return n == 1, postgres.Wrap(err, "insert remote object tombstone")
}

// RemoteObjectTombstoned implements federation.InboxRepository.
func (r *Repository) RemoteObjectTombstoned(ctx context.Context, uri, actorURI federation.IRI) (bool, error) {
	ok, err := r.db.reader().HasRemoteObjectTombstone(ctx, federationgen.HasRemoteObjectTombstoneParams{Uri: string(uri), ActorUri: string(actorURI)})
	return ok, postgres.Wrap(err, "read remote object tombstone")
}
