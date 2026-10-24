// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Clients whose client_id is the https URL of a Client ID Metadata Document (ADR-0090 §1,
// draft-ietf-oauth-client-id-metadata-document). An authorization request fetches the document
// when the cached copy is missing or stale; the token endpoint uses the client that the fetch
// stored and never fetches.

// Cache lifetimes of a metadata document when its response names none, and at most.
const (
	DefaultMetadataDocumentTTL = time.Hour
	MaxMetadataDocumentTTL     = 24 * time.Hour
)

// errMetadataDocumentClient reports a metadata document client that may not be used: the
// instance does not accept them, the host is denied, or the document is unusable. The
// authorization request answers it as an unknown client.
var errMetadataDocumentClient = errors.New("identity: metadata document client refused")

// metadataDocumentsEnabled reports whether the instance accepts metadata document clients: it
// can fetch them and the setting is on.
func (s *Service) metadataDocumentsEnabled(ctx context.Context) (bool, error) {
	if s.d.HTTP == nil {
		return false, nil
	}
	if s.d.MetadataDocumentClients == nil {
		return true, nil
	}
	return s.d.MetadataDocumentClients(ctx)
}

// usableMetadataDocumentHost checks that the instance accepts metadata document clients and
// that the host of the client_id URL is not denied; it returns the host.
func (s *Service) usableMetadataDocumentHost(ctx context.Context, clientID string) (string, error) {
	enabled, err := s.metadataDocumentsEnabled(ctx)
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", fmt.Errorf("%w: the instance does not accept them", errMetadataDocumentClient)
	}
	host, err := identity.ParseMetadataDocumentClientID(clientID)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errMetadataDocumentClient, err)
	}
	denied, err := s.d.Repository.OAuthClientHostDenied(ctx, host)
	if err != nil {
		return "", err
	}
	if denied {
		return "", fmt.Errorf("%w: the host %s is denied", errMetadataDocumentClient, host)
	}
	return host, nil
}

// metadataDocumentClient returns c, a client registered by its metadata document, with what
// the stored document declares. It is ErrNotFound when the instance does not accept such
// clients, the host is denied, or no document is stored.
func (s *Service) metadataDocumentClient(ctx context.Context, c identity.OAuthClient) (identity.OAuthClient, error) {
	if _, err := s.usableMetadataDocumentHost(ctx, c.ClientID); err != nil {
		if errors.Is(err, errMetadataDocumentClient) {
			return identity.OAuthClient{}, ErrNotFound
		}
		return identity.OAuthClient{}, err
	}
	doc, err := s.d.Repository.FindOAuthClientMetadataDocument(ctx, c.ID)
	if err != nil {
		return identity.OAuthClient{}, err
	}
	return doc.Apply(c), nil
}

// refreshMetadataDocument makes sure that a fresh copy of the metadata document at clientID is
// stored, fetching it when there is none or the stored one is stale. The fetch goes through the
// SSRF-guarded HTTPClient without following redirects, reads at most 5 KB, and happens before
// the transaction (ADR-0003); a failure stores nothing, so it is not cached. It fails with a *kernel.RateLimitedError when
// the fetch limits are used up (ADR-0090 §7), and with an
// error wrapping errMetadataDocumentClient when the client may not be used.
func (s *Service) refreshMetadataDocument(ctx context.Context, clientID string) error {
	host, err := s.usableMetadataDocumentHost(ctx, clientID)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	existing, _, err := s.d.Repository.FindOAuthClient(ctx, clientID)
	switch {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return err
	default:
		if existing.Registration != identity.ClientRegisteredByMetadataDocument {
			return fmt.Errorf("%w: client_id is registered by another route", errMetadataDocumentClient)
		}
		state, err := s.d.Repository.FindOAuthClientState(ctx, existing.ID)
		if err != nil {
			return err
		}
		if !state.Usable() {
			return fmt.Errorf("%w: the client is disabled or deleted", errMetadataDocumentClient)
		}
		doc, err := s.d.Repository.FindOAuthClientMetadataDocument(ctx, existing.ID)
		if err == nil && doc.Fresh(now) {
			return nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	if err := s.takeMetadataDocumentFetch(ctx, clientID, host); err != nil {
		return err
	}
	meta, ttl, err := s.fetchMetadataDocument(ctx, clientID, now)
	if err != nil {
		return fmt.Errorf("%w: %w", errMetadataDocumentClient, err)
	}
	c := identity.OAuthClient{
		ID: s.d.IDs.NewID(), ClientID: clientID, Name: host, Scope: identity.MetadataDocumentDefaultScope(),
		Registration: identity.ClientRegisteredByMetadataDocument, CreatedAt: now,
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		stored, err := s.d.Repository.EnsureOAuthClient(ctx, tx, c)
		if err != nil {
			return err
		}
		if stored.Registration != identity.ClientRegisteredByMetadataDocument {
			return fmt.Errorf("%w: client_id is registered by another route", errMetadataDocumentClient)
		}
		doc := identity.ClientMetadataDocument{
			ClientID: stored.ID, Name: meta.Name, Website: meta.Website, LogoURI: meta.LogoURI, Scope: meta.Scope,
			FetchedAt: now, ExpiresAt: now.Add(ttl),
		}
		if err := s.d.Repository.UpsertOAuthClientMetadataDocument(ctx, tx, doc); err != nil {
			return err
		}
		return s.d.Repository.ReplaceOAuthClientRedirectURIs(ctx, tx, stored.ID, meta.RedirectURIs)
	})
}

// fetchMetadataDocument fetches and checks the metadata document at clientID, and returns how
// long its response lets it be reused.
func (s *Service) fetchMetadataDocument(ctx context.Context, clientID string, now time.Time) (identity.ClientMetadata, time.Duration, error) {
	resp, err := s.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		URL: clientID, Purpose: kernel.HTTPPurposeClientMetadata, Header: http.Header{"Accept": {"application/json"}},
	})
	if err != nil {
		return identity.ClientMetadata{}, 0, fmt.Errorf("fetch the metadata document: %w", err)
	}
	body, err := resp.ReadAll()
	if err != nil {
		return identity.ClientMetadata{}, 0, fmt.Errorf("read the metadata document: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// A redirect is refused too: the draft forbids following it (ADR-0090 §1).
		return identity.ClientMetadata{}, 0, fmt.Errorf("the metadata document answered %d", resp.StatusCode)
	}
	meta, err := identity.ParseClientMetadataDocument(clientID, body)
	if err != nil {
		return identity.ClientMetadata{}, 0, err
	}
	return meta, metadataDocumentTTL(resp.Header, now), nil
}

// metadataDocumentTTL returns how long a response with header h may be reused from now
// (RFC 9111 §4.2.1): zero for no-store, no-cache, or a malformed directive; s-maxage, max-age, or
// Expires (against Date) when present; DefaultMetadataDocumentTTL otherwise. It is at most
// MaxMetadataDocumentTTL.
func metadataDocumentTTL(h http.Header, now time.Time) time.Duration {
	maxAge, sharedMaxAge := -1, -1
	for _, line := range h.Values("Cache-Control") {
		for _, directive := range strings.Split(line, ",") {
			name, value, _ := strings.Cut(strings.TrimSpace(directive), "=")
			value = strings.Trim(strings.TrimSpace(value), `"`)
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "no-store", "no-cache":
				return 0
			case "max-age":
				if maxAge = seconds(value); maxAge < 0 {
					return 0
				}
			case "s-maxage":
				if sharedMaxAge = seconds(value); sharedMaxAge < 0 {
					return 0
				}
			}
		}
	}
	var ttl time.Duration
	switch {
	case sharedMaxAge >= 0:
		ttl = time.Duration(sharedMaxAge) * time.Second
	case maxAge >= 0:
		ttl = time.Duration(maxAge) * time.Second
	case h.Get("Expires") != "":
		expires, err := http.ParseTime(h.Get("Expires"))
		if err != nil {
			return 0
		}
		base := now
		if date, err := http.ParseTime(h.Get("Date")); err == nil {
			base = date
		}
		ttl = expires.Sub(base)
	default:
		ttl = DefaultMetadataDocumentTTL
	}
	return min(max(ttl, 0), MaxMetadataDocumentTTL)
}

// seconds parses a delta-seconds value; -1 when it is not one. A value too large to hold is
// capped, as RFC 9111 §1.2.2 allows.
func seconds(v string) int {
	if v == "" || strings.Trim(v, "0123456789") != "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return int(MaxMetadataDocumentTTL / time.Second)
	}
	return n
}

// DenyOAuthClientHost records that no metadata document on host is fetched or used (ADR-0090
// §1); byUserID is the administrator, "" for the CLI. Denying a denied host records nothing new.
// The denial also revokes every token of the clients of the host with client_disabled, so that
// no access token outlives it; the call is safe to repeat.
func (s *Service) DenyOAuthClientHost(ctx context.Context, host, byUserID string) error {
	h, err := identity.ClientHost(host)
	if err != nil {
		return err
	}
	d := identity.OAuthClientHostDenial{ID: s.d.IDs.NewID(), Host: h, DeniedByUserID: byUserID, DeniedAt: s.d.Clock.Now()}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted, err := s.d.Repository.InsertOAuthClientHostDenial(ctx, tx, d)
		if err != nil {
			return err
		}
		clients, err := s.d.Repository.ListMetadataDocumentClientsOfHost(ctx, tx, h)
		if err != nil {
			return err
		}
		for _, c := range clients {
			if ch, err := identity.ParseMetadataDocumentClientID(c.ClientID); err != nil || ch != h {
				continue
			}
			if err := s.d.Repository.RevokeTokensOfClient(ctx, tx, c.ID, identity.TokenRevokedClientDisabled, d.DeniedAt); err != nil {
				return err
			}
		}
		if !inserted {
			return nil
		}
		return s.emitOAuthHostAudit(ctx, tx, appmoderation.AuditOAuthClientHostDenied, h, d.ID, byUserID, d.DeniedAt)
	})
}

// emitOAuthHostAudit emits the audit log entry of an administrator's operation on a host of
// metadata documents; the host is the target's ID and label.
func (s *Service) emitOAuthHostAudit(ctx context.Context, tx uow.Tx, action appmoderation.AuditAction, host, source, byUserID string, at time.Time) error {
	entry := appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), Action: action, TargetKind: appmoderation.AuditTargetOAuthClientHost, TargetID: host,
		TargetLabel: host, SourceEventID: source, OccurredAt: at,
	}
	entry.ActorKind, entry.ActorUserID = appmoderation.AuditActor(byUserID)
	return tx.Emit(ctx, appmoderation.AuditRecorded{Entry: entry})
}

// ListOAuthClientHostDenials returns the denied hosts, newest denial first, at most limit (1 to
// MaxListLimit) of them. A before other than "" is the ID of the last denial of the previous
// page.
func (s *Service) ListOAuthClientHostDenials(ctx context.Context, before string, limit int) ([]identity.OAuthClientHostDenial, error) {
	if limit < 1 || limit > MaxListLimit {
		return nil, fmt.Errorf("identity: limit must be 1 to %d", MaxListLimit)
	}
	return s.d.Repository.ListOAuthClientHostDenials(ctx, before, limit)
}

// AllowOAuthClientHost lifts the denials of host; byUserID is the administrator, "" for the
// CLI. Allowing a host that is not denied does nothing.
func (s *Service) AllowOAuthClientHost(ctx context.Context, host, byUserID string) error {
	h, err := identity.ClientHost(host)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		lifted, err := s.d.Repository.LiftOAuthClientHostDenials(ctx, tx, h, byUserID, now)
		if err != nil || !lifted {
			return err
		}
		return s.emitOAuthHostAudit(ctx, tx, appmoderation.AuditOAuthClientHostAllowed, h, "", byUserID, now)
	})
}

// MetadataDocumentClientsEnabled reports whether the instance accepts Client ID Metadata
// Document clients, for the authorization server metadata.
func (s *Service) MetadataDocumentClientsEnabled(ctx context.Context) (bool, error) {
	return s.metadataDocumentsEnabled(ctx)
}
