// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// External login providers and the external identities linked to users (data-model §4.1,
// ADR-0106).

// InsertLoginProvider implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) InsertLoginProvider(ctx context.Context, tx uow.Tx, id, createdByUserID string, createdAt time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertLoginProvider(ctx, postgresgen.InsertLoginProviderParams{
		ID: id, CreatedByUserID: toNullString(createdByUserID), CreatedAt: createdAt,
	}), "insert login provider")
}

// InsertLoginProviderRevision implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) InsertLoginProviderRevision(ctx context.Context, tx uow.Tx, rev identity.LoginProviderRevision) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	s := rev.Settings
	err = q.InsertLoginProviderRevision(ctx, postgresgen.InsertLoginProviderRevisionParams{
		ID: rev.ID, ProviderID: rev.ProviderID, Kind: string(s.Kind), DisplayName: s.DisplayName,
		Issuer: toNullString(s.Issuer), ClientID: s.ClientID, Scopes: toNullString(strings.Join(s.Scopes, " ")),
		AllowSignup: s.AllowSignup, AllowLink: s.AllowLink,
		RevisedByUserID: toNullString(rev.RevisedByUserID), RevisedAt: rev.RevisedAt,
	})
	if err != nil {
		return wrap(err, "insert login provider revision")
	}
	for _, c := range s.Claims {
		for _, v := range c.Values {
			err := q.InsertLoginProviderRevisionClaim(ctx, postgresgen.InsertLoginProviderRevisionClaimParams{
				RevisionID: rev.ID, Claim: c.Claim, Value: v,
			})
			if err != nil {
				return wrap(err, "insert login provider revision claim")
			}
		}
	}
	for _, o := range s.Organizations {
		err := q.InsertLoginProviderRevisionOrganization(ctx, postgresgen.InsertLoginProviderRevisionOrganizationParams{
			RevisionID: rev.ID, Organization: o,
		})
		if err != nil {
			return wrap(err, "insert login provider revision organization")
		}
	}
	return nil
}

// InsertLoginProviderSecret implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) InsertLoginProviderSecret(ctx context.Context, tx uow.Tx, s identity.LoginProviderSecret) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertLoginProviderSecret(ctx, postgresgen.InsertLoginProviderSecretParams{
		ID: s.ID, ProviderID: s.ProviderID, SealedSecret: s.Sealed, CreatedByUserID: toNullString(s.CreatedByUserID),
		CreatedAt: s.CreatedAt,
	}), "insert login provider secret")
}

// InsertLoginProviderStateChange implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) InsertLoginProviderStateChange(ctx context.Context, tx uow.Tx, c identity.LoginProviderStateChange) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertLoginProviderStateChange(ctx, postgresgen.InsertLoginProviderStateChangeParams{
		ID: c.ID, ProviderID: c.ProviderID, State: string(c.State), ChangedByUserID: toNullString(c.ChangedByUserID),
		ChangedAt: c.ChangedAt,
	}), "insert login provider state change")
}

// UpsertLoginProviderHead implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) UpsertLoginProviderHead(ctx context.Context, tx uow.Tx, h identity.LoginProviderHead) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertLoginProviderHead(ctx, postgresgen.UpsertLoginProviderHeadParams{
		ProviderID: h.ProviderID, RevisionID: h.RevisionID, State: string(h.State), SecretID: h.SecretID,
	}), "upsert login provider head")
}

// LoginProviderHeadForUpdate implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) LoginProviderHeadForUpdate(ctx context.Context, tx uow.Tx, providerID string) (identity.LoginProviderHead, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.LoginProviderHead{}, err
	}
	if providerID == "" {
		return identity.LoginProviderHead{}, appidentity.ErrNotFound
	}
	row, err := q.GetLoginProviderHeadForUpdate(ctx, providerID)
	if err != nil {
		return identity.LoginProviderHead{}, notFound(err, appidentity.ErrNotFound, "login provider "+providerID)
	}
	return identity.LoginProviderHead{
		ProviderID: row.ProviderID, RevisionID: row.RevisionID, State: identity.LoginProviderState(row.State), SecretID: row.SecretID,
	}, nil
}

// FindLoginProvider implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) FindLoginProvider(ctx context.Context, id string) (identity.LoginProvider, error) {
	if id == "" {
		return identity.LoginProvider{}, appidentity.ErrNotFound
	}
	out, err := r.loginProviders(ctx, &id)
	if err != nil {
		return identity.LoginProvider{}, err
	}
	if len(out) == 0 {
		return identity.LoginProvider{}, notFound(pgx.ErrNoRows, appidentity.ErrNotFound, "login provider "+id)
	}
	return out[0], nil
}

// ListLoginProviders implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) ListLoginProviders(ctx context.Context) ([]identity.LoginProvider, error) {
	return r.loginProviders(ctx, nil)
}

func (r *IdentityRepository) loginProviders(ctx context.Context, id *string) ([]identity.LoginProvider, error) {
	q := r.db.reader()
	rows, err := q.ListLoginProviders(ctx, id)
	if err != nil {
		return nil, wrap(err, "list login providers")
	}
	revisions := make([]string, len(rows))
	out := make([]identity.LoginProvider, len(rows))
	byRevision := make(map[string]int, len(rows))
	for i, row := range rows {
		revisions[i] = row.RevisionID
		byRevision[row.RevisionID] = i
		out[i] = identity.LoginProvider{
			ID: row.ID, CreatedByUserID: fromNullString(row.CreatedByUserID), CreatedAt: row.CreatedAt.UTC(),
			RevisionID: row.RevisionID, RevisedAt: row.RevisedAt.UTC(),
			Settings: identity.LoginProviderSettings{
				Kind: identity.LoginProviderKind(row.Kind), DisplayName: row.DisplayName, Issuer: fromNullString(row.Issuer),
				ClientID: row.ClientID, Scopes: scopeList(fromNullString(row.Scopes)),
				AllowSignup: row.AllowSignup, AllowLink: row.AllowLink,
			},
			State: identity.LoginProviderState(row.State), StateChangedAt: row.StateChangedAt.UTC(), SecretID: row.SecretID,
		}
	}
	if len(rows) == 0 {
		return out, nil
	}
	claims, err := q.ListLoginProviderRevisionClaims(ctx, revisions)
	if err != nil {
		return nil, wrap(err, "list login provider claims")
	}
	for _, c := range claims {
		s := &out[byRevision[c.RevisionID]].Settings
		if n := len(s.Claims); n == 0 || s.Claims[n-1].Claim != c.Claim {
			s.Claims = append(s.Claims, identity.ClaimCondition{Claim: c.Claim})
		}
		last := &s.Claims[len(s.Claims)-1]
		last.Values = append(last.Values, c.Value)
	}
	orgs, err := q.ListLoginProviderRevisionOrganizations(ctx, revisions)
	if err != nil {
		return nil, wrap(err, "list login provider organizations")
	}
	for _, o := range orgs {
		s := &out[byRevision[o.RevisionID]].Settings
		s.Organizations = append(s.Organizations, o.Organization)
	}
	return out, nil
}

// FindLoginProviderSecret implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) FindLoginProviderSecret(ctx context.Context, providerID string) (identity.LoginProviderSecret, error) {
	if providerID == "" {
		return identity.LoginProviderSecret{}, appidentity.ErrNotFound
	}
	row, err := r.db.reader().GetCurrentLoginProviderSecret(ctx, providerID)
	if err != nil {
		return identity.LoginProviderSecret{}, notFound(err, appidentity.ErrNotFound, "login provider secret of "+providerID)
	}
	return identity.LoginProviderSecret{
		ID: row.ID, ProviderID: row.ProviderID, Sealed: row.SealedSecret, CreatedByUserID: fromNullString(row.CreatedByUserID),
		CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

// EnsureExternalIdentity implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) EnsureExternalIdentity(ctx context.Context, tx uow.Tx, x identity.ExternalIdentity) (identity.ExternalIdentity, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return identity.ExternalIdentity{}, err
	}
	err = q.InsertExternalIdentity(ctx, postgresgen.InsertExternalIdentityParams{
		ID: x.ID, ProviderID: x.ProviderID, Subject: x.Subject, CreatedAt: x.CreatedAt,
	})
	if err != nil {
		return identity.ExternalIdentity{}, wrap(err, "insert external identity")
	}
	row, err := q.GetExternalIdentity(ctx, postgresgen.GetExternalIdentityParams{ProviderID: x.ProviderID, Subject: x.Subject})
	if err != nil {
		return identity.ExternalIdentity{}, wrap(err, "find external identity")
	}
	return identity.ExternalIdentity{ID: row.ID, ProviderID: row.ProviderID, Subject: row.Subject, CreatedAt: row.CreatedAt.UTC()}, nil
}

// InsertExternalIdentityLink implements appidentity.LoginProviderRepository. The current link
// goes first: its primary key and its unique (user_id, provider_id) refuse a second link.
func (r *IdentityRepository) InsertExternalIdentityLink(ctx context.Context, tx uow.Tx, x identity.ExternalIdentity, l identity.ExternalIdentityLink) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertUserExternalIdentity(ctx, postgresgen.InsertUserExternalIdentityParams{
		ExternalIdentityID: x.ID, LinkID: l.ID, UserID: l.UserID, ProviderID: x.ProviderID,
	})
	if err != nil {
		return wrap(err, "insert user external identity")
	}
	if n == 0 {
		if _, err := q.GetUserExternalIdentityByIdentity(ctx, x.ID); err == nil {
			return appidentity.ErrExternalIdentityLinked
		}
		return appidentity.ErrLoginProviderLinked
	}
	return wrap(q.InsertExternalIdentityLink(ctx, postgresgen.InsertExternalIdentityLinkParams{
		ID: l.ID, ExternalIdentityID: l.ExternalIdentityID, UserID: l.UserID, LinkedAt: l.LinkedAt,
	}), "insert external identity link")
}

// InsertExternalIdentityUnlink implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) InsertExternalIdentityUnlink(ctx context.Context, tx uow.Tx, u identity.ExternalIdentityUnlink) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertExternalIdentityUnlink(ctx, postgresgen.InsertExternalIdentityUnlinkParams{LinkID: u.LinkID, UnlinkedAt: u.UnlinkedAt})
	if err != nil || n == 0 {
		return wrap(err, "insert external identity unlink")
	}
	return wrap(q.DeleteUserExternalIdentityByLink(ctx, u.LinkID), "delete user external identity")
}

// FindLinkedExternalIdentity implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) FindLinkedExternalIdentity(ctx context.Context, providerID, subject string) (identity.LinkedExternalIdentity, error) {
	if providerID == "" {
		return identity.LinkedExternalIdentity{}, appidentity.ErrNotFound
	}
	row, err := r.db.reader().FindLinkedExternalIdentity(ctx, postgresgen.FindLinkedExternalIdentityParams{ProviderID: providerID, Subject: subject})
	if err != nil {
		return identity.LinkedExternalIdentity{}, notFound(err, appidentity.ErrNotFound, "linked external identity")
	}
	return identity.LinkedExternalIdentity{
		LinkID: row.LinkID, ExternalIdentityID: row.ExternalIdentityID, ProviderID: row.ProviderID, Subject: row.Subject,
		UserID: row.UserID, LinkedAt: row.LinkedAt.UTC(),
	}, nil
}

// ListLinkedExternalIdentities implements appidentity.LoginProviderRepository.
func (r *IdentityRepository) ListLinkedExternalIdentities(ctx context.Context, userID string) ([]identity.LinkedExternalIdentity, error) {
	rows, err := r.db.reader().ListLinkedExternalIdentities(ctx, userID)
	if err != nil {
		return nil, wrap(err, "list linked external identities")
	}
	out := make([]identity.LinkedExternalIdentity, len(rows))
	for i, row := range rows {
		out[i] = identity.LinkedExternalIdentity{
			LinkID: row.LinkID, ExternalIdentityID: row.ExternalIdentityID, ProviderID: row.ProviderID, Subject: row.Subject,
			UserID: row.UserID, LinkedAt: row.LinkedAt.UTC(),
		}
	}
	return out, nil
}

// RebuildLoginProviderHeads implements appidentity.ProjectionRepository.
func (r *IdentityRepository) RebuildLoginProviderHeads(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild login provider heads",
		(*postgresgen.Queries).DeleteAllLoginProviderHeads, (*postgresgen.Queries).RebuildLoginProviderHeads)
}

// RebuildUserExternalIdentities implements appidentity.ProjectionRepository.
func (r *IdentityRepository) RebuildUserExternalIdentities(ctx context.Context, tx uow.Tx) error {
	return r.db.execAll(ctx, tx, "rebuild user external identities",
		(*postgresgen.Queries).DeleteAllUserExternalIdentities, (*postgresgen.Queries).RebuildUserExternalIdentities)
}

// scopeList splits the space-separated scopes of a revision, nil for none.
func scopeList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}
