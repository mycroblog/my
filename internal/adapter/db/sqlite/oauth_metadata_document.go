// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// UpsertOAuthClientMetadataDocument implements identity.Repository.
func (r *IdentityRepository) UpsertOAuthClientMetadataDocument(ctx context.Context, tx uow.Tx, d identity.ClientMetadataDocument) error {
	q, err := queries(tx)
	if err != nil {
		return err
	}
	err = q.UpsertOAuthClientMetadataDocument(ctx, sqlitegen.UpsertOAuthClientMetadataDocumentParams{
		ClientID: d.ClientID, ClientName: d.Name, ClientUri: d.Website, LogoUri: d.LogoURI, Scope: string(d.Scope),
		FetchedAt: toMillis(d.FetchedAt), ExpiresAt: toMillis(d.ExpiresAt),
	})
	return wrap(err, "upsert oauth client metadata document")
}

// FindOAuthClientMetadataDocument implements identity.Repository.
func (r *IdentityRepository) FindOAuthClientMetadataDocument(ctx context.Context, clientID string) (identity.ClientMetadataDocument, error) {
	row, err := r.db.reader().GetOAuthClientMetadataDocument(ctx, clientID)
	if err != nil {
		return identity.ClientMetadataDocument{}, notFound(err, appidentity.ErrNotFound, "oauth client metadata document")
	}
	return identity.ClientMetadataDocument{
		ClientID: row.ClientID, Name: row.ClientName, Website: row.ClientUri, LogoURI: row.LogoUri, Scope: identity.Scope(row.Scope),
		FetchedAt: fromMillis(row.FetchedAt), ExpiresAt: fromMillis(row.ExpiresAt),
	}, nil
}

// InsertOAuthClientHostDenial implements identity.Repository.
func (r *IdentityRepository) InsertOAuthClientHostDenial(ctx context.Context, tx uow.Tx, d identity.OAuthClientHostDenial) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOAuthClientHostDenialIfAllowed(ctx, sqlitegen.InsertOAuthClientHostDenialIfAllowedParams{
		ID: d.ID, Host: d.Host, DeniedByUserID: toNullString(d.DeniedByUserID), DeniedAt: toMillis(d.DeniedAt),
	})
	return n == 1, wrap(err, "insert oauth client host denial")
}

// LiftOAuthClientHostDenials implements identity.Repository.
func (r *IdentityRepository) LiftOAuthClientHostDenials(ctx context.Context, tx uow.Tx, host, byUserID string, at time.Time) (bool, error) {
	q, err := queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.LiftOAuthClientHostDenials(ctx, sqlitegen.LiftOAuthClientHostDenialsParams{
		LiftedByUserID: toNullString(byUserID), LiftedAt: toMillis(at), Host: host,
	})
	return n > 0, wrap(err, "lift oauth client host denials")
}

// ListOAuthClientHostDenials implements identity.Repository.
func (r *IdentityRepository) ListOAuthClientHostDenials(ctx context.Context, before string, limit int) ([]identity.OAuthClientHostDenial, error) {
	rows, err := r.db.reader().ListOAuthClientHostDenials(ctx, sqlitegen.ListOAuthClientHostDenialsParams{Before: before, PageLimit: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list oauth client host denials")
	}
	out := make([]identity.OAuthClientHostDenial, len(rows))
	for i, row := range rows {
		out[i] = identity.OAuthClientHostDenial{
			ID: row.ID, Host: row.Host, DeniedByUserID: row.DeniedByUserID.String, DeniedAt: fromMillis(row.DeniedAt),
		}
	}
	return out, nil
}

// ListMetadataDocumentClientsOfHost implements identity.Repository.
func (r *IdentityRepository) ListMetadataDocumentClientsOfHost(ctx context.Context, tx uow.Tx, host string) ([]identity.OAuthClient, error) {
	q, err := queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListMetadataDocumentClientsOfHost(ctx, toNullString(host))
	if err != nil {
		return nil, wrap(err, "list oauth clients of a host")
	}
	out := make([]identity.OAuthClient, len(rows))
	for i, row := range rows {
		out[i] = identity.OAuthClient{ID: row.ID, ClientID: row.ClientID, Registration: identity.ClientRegisteredByMetadataDocument}
	}
	return out, nil
}

// OAuthClientHostDenied implements identity.Repository.
func (r *IdentityRepository) OAuthClientHostDenied(ctx context.Context, host string) (bool, error) {
	denied, err := r.db.reader().OAuthClientHostDenied(ctx, host)
	return denied, wrap(err, "check oauth client host denial")
}
