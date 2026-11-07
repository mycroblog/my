// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
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
		FetchedAt: d.FetchedAt, ExpiresAt: d.ExpiresAt,
	})
	return postgres.Wrap(err, "upsert oauth client metadata document")
}

// FindOAuthClientMetadataDocument implements identity.Repository.
func (r *Repository) FindOAuthClientMetadataDocument(ctx context.Context, clientID string) (identity.ClientMetadataDocument, error) {
	row, err := r.db.reader().GetOAuthClientMetadataDocument(ctx, clientID)
	if err != nil {
		return identity.ClientMetadataDocument{}, postgres.NotFound(err, appidentity.ErrNotFound, "oauth client metadata document")
	}
	return identity.ClientMetadataDocument{
		ClientID: row.OauthClientID, Name: row.ClientName, Website: row.ClientUri, LogoURI: row.LogoUri, Scope: identity.Scope(row.Scope),
		FetchedAt: row.FetchedAt, ExpiresAt: row.ExpiresAt,
	}, nil
}

// InsertOAuthClientHostDenial implements identity.Repository.
func (r *Repository) InsertOAuthClientHostDenial(ctx context.Context, tx uow.Tx, d identity.OAuthClientHostDenial) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertOAuthClientHostDenialIfAllowed(ctx, identitygen.InsertOAuthClientHostDenialIfAllowedParams{
		ID: d.ID, Host: d.Host, DeniedByUserID: postgres.ToNullString(d.DeniedByUserID), DeniedAt: d.DeniedAt,
	})
	return n == 1, postgres.Wrap(err, "insert oauth client host denial")
}

// FindOAuthClientHostDenial implements identity.Repository.
func (r *Repository) FindOAuthClientHostDenial(ctx context.Context, tx uow.Tx, host string) (identity.OAuthClientHostDenial, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.OAuthClientHostDenial{}, err
	}
	row, err := q.FindOAuthClientHostDenial(ctx, host)
	if err != nil {
		return identity.OAuthClientHostDenial{}, postgres.NotFound(err, appidentity.ErrNotFound, "oauth client host denial")
	}
	return identity.OAuthClientHostDenial{
		ID: row.ID, Host: row.Host, DeniedByUserID: postgres.FromNullString(row.DeniedByUserID), DeniedAt: postgres.UTC(row.DeniedAt),
	}, nil
}

// LiftOAuthClientHostDenials implements identity.Repository.
func (r *Repository) LiftOAuthClientHostDenials(ctx context.Context, tx uow.Tx, host, byUserID string, at time.Time) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.LiftOAuthClientHostDenials(ctx, identitygen.LiftOAuthClientHostDenialsParams{
		LiftedByUserID: postgres.ToNullString(byUserID), LiftedAt: at, Host: host,
	})
	return n > 0, postgres.Wrap(err, "lift oauth client host denials")
}

// ListOAuthClientHostDenials implements identity.Repository.
func (r *Repository) ListOAuthClientHostDenials(ctx context.Context, before string, limit int) ([]identity.OAuthClientHostDenial, error) {
	n, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListOAuthClientHostDenials(ctx, identitygen.ListOAuthClientHostDenialsParams{Before: postgres.ToNullString(before), PageLimit: n})
	if err != nil {
		return nil, postgres.Wrap(err, "list oauth client host denials")
	}
	out := make([]identity.OAuthClientHostDenial, len(rows))
	for i, row := range rows {
		out[i] = identity.OAuthClientHostDenial{
			ID: row.ID, Host: row.Host, DeniedByUserID: postgres.FromNullString(row.DeniedByUserID), DeniedAt: postgres.UTC(row.DeniedAt),
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
	rows, err := q.ListMetadataDocumentClientsOfHost(ctx, host)
	if err != nil {
		return nil, postgres.Wrap(err, "list oauth clients of a host")
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
	return denied, postgres.Wrap(err, "check oauth client host denial")
}
