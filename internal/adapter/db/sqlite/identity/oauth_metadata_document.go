// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// UpsertOAuthClientMetadataDocument implements identity.Repository.
func (r *Repository) UpsertOAuthClientMetadataDocument(ctx context.Context, tx uow.Tx, d identity.ClientMetadataDocument) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertOAuthClientMetadataDocument(ctx, identitygen.UpsertOAuthClientMetadataDocumentParams{
		OauthClientID: d.ClientID, ClientName: d.Name, ClientUri: d.Website, LogoUri: d.LogoURI, Scope: string(d.Scope),
		FetchedAt: sqlite.ToMillis(d.FetchedAt), ExpiresAt: sqlite.ToMillis(d.ExpiresAt),
	})
	return sqlite.Wrap(err, "upsert oauth client metadata document")
}

// FindOAuthClientMetadataDocument implements identity.Repository.
func (r *Repository) FindOAuthClientMetadataDocument(ctx context.Context, clientID string) (identity.ClientMetadataDocument, error) {
	row, err := r.db.reader().GetOAuthClientMetadataDocument(ctx, clientID)
	if err != nil {
		return identity.ClientMetadataDocument{}, sqlite.NotFound(err, appidentity.ErrNotFound, "oauth client metadata document")
	}
	return identity.ClientMetadataDocument{
		ClientID: row.OauthClientID, Name: row.ClientName, Website: row.ClientUri, LogoURI: row.LogoUri, Scope: identity.Scope(row.Scope),
		FetchedAt: sqlite.FromMillis(row.FetchedAt), ExpiresAt: sqlite.FromMillis(row.ExpiresAt),
	}, nil
}

// InsertOAuthClientHostDenial implements identity.Repository.
func (r *Repository) InsertOAuthClientHostDenial(ctx context.Context, tx uow.Tx, d identity.OAuthClientHostDenial) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOAuthClientHostDenialIfAllowed(ctx, identitygen.InsertOAuthClientHostDenialIfAllowedParams{
		ID: d.ID, Host: d.Host, DeniedByUserID: sqlite.ToNullString(d.DeniedByUserID), DeniedAt: sqlite.ToMillis(d.DeniedAt),
	})
	return n == 1, sqlite.Wrap(err, "insert oauth client host denial")
}

// FindOAuthClientHostDenial implements identity.Repository.
func (r *Repository) FindOAuthClientHostDenial(ctx context.Context, tx uow.Tx, host string) (identity.OAuthClientHostDenial, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.OAuthClientHostDenial{}, err
	}
	row, err := q.FindOAuthClientHostDenial(ctx, host)
	if err != nil {
		return identity.OAuthClientHostDenial{}, sqlite.NotFound(err, appidentity.ErrNotFound, "oauth client host denial")
	}
	return identity.OAuthClientHostDenial{
		ID: row.ID, Host: row.Host, DeniedByUserID: row.DeniedByUserID.String, DeniedAt: sqlite.FromMillis(row.DeniedAt),
	}, nil
}

// LiftOAuthClientHostDenials implements identity.Repository.
func (r *Repository) LiftOAuthClientHostDenials(ctx context.Context, tx uow.Tx, host, byUserID string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.LiftOAuthClientHostDenials(ctx, identitygen.LiftOAuthClientHostDenialsParams{
		LiftedByUserID: sqlite.ToNullString(byUserID), LiftedAt: sqlite.ToMillis(at), Host: host,
	})
	return n > 0, sqlite.Wrap(err, "lift oauth client host denials")
}

// ListOAuthClientHostDenials implements identity.Repository.
func (r *Repository) ListOAuthClientHostDenials(ctx context.Context, before string, limit int) ([]identity.OAuthClientHostDenial, error) {
	rows, err := r.db.reader().ListOAuthClientHostDenials(ctx, identitygen.ListOAuthClientHostDenialsParams{Before: before, PageLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list oauth client host denials")
	}
	out := make([]identity.OAuthClientHostDenial, len(rows))
	for i, row := range rows {
		out[i] = identity.OAuthClientHostDenial{
			ID: row.ID, Host: row.Host, DeniedByUserID: row.DeniedByUserID.String, DeniedAt: sqlite.FromMillis(row.DeniedAt),
		}
	}
	return out, nil
}

// ListMetadataDocumentClientsOfHost implements identity.Repository.
func (r *Repository) ListMetadataDocumentClientsOfHost(ctx context.Context, tx uow.Tx, host string) ([]identity.OAuthClient, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListMetadataDocumentClientsOfHost(ctx, sqlite.ToNullString(host))
	if err != nil {
		return nil, sqlite.Wrap(err, "list oauth clients of a host")
	}
	out := make([]identity.OAuthClient, len(rows))
	for i, row := range rows {
		out[i] = identity.OAuthClient{ID: row.ID, ClientID: row.ClientID, Registration: identity.ClientRegisteredByMetadataDocument}
	}
	return out, nil
}

// OAuthClientHostDenied implements identity.Repository.
func (r *Repository) OAuthClientHostDenied(ctx context.Context, host string) (bool, error) {
	denied, err := r.db.reader().OAuthClientHostDenied(ctx, host)
	return denied, sqlite.Wrap(err, "check oauth client host denial")
}
