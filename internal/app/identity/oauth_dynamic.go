// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Dynamic client registration (RFC 7591, ADR-0090 §1): any client may register itself without
// authentication, as a public client only, and the clean-up deletes the ones that are not used.
// RFC 7592 (reading, updating, and deleting a registration) and software statements are not
// supported.

// Error codes of the registration endpoint (RFC 7591 §3.2.2).
const (
	OAuthInvalidRedirectURI       = "invalid_redirect_uri"
	OAuthInvalidClientMetadata    = "invalid_client_metadata"
	OAuthInvalidSoftwareStatement = "invalid_software_statement"
)

// Limits of the metadata of a dynamic registration, which nothing else checks.
const (
	// MaxDynamicRedirectURIs bounds redirect_uris.
	MaxDynamicRedirectURIs = 10
	// MaxDynamicClientNameLength bounds client_name, in characters.
	MaxDynamicClientNameLength = 100
	// maxDynamicURILength bounds a URI of the metadata, in bytes.
	maxDynamicURILength = 2048
)

// Defaults of the clean-up of unused dynamically registered clients (ADR-0090 §1).
const (
	// DefaultDynamicClientUnredeemedTTL is how long a registered client may wait for the
	// exchange of its first authorization code.
	DefaultDynamicClientUnredeemedTTL = 24 * time.Hour
	// DefaultDynamicClientIdleTTL is how long a client may go unused after its last token was
	// issued once it has no usable token.
	DefaultDynamicClientIdleTTL = 90 * 24 * time.Hour

	// JobDeleteUnusedClients deletes the unused dynamically registered clients.
	JobDeleteUnusedClients = "identity.delete_unused_oauth_clients"
	// UnusedClientsInterval is the period of the JobDeleteUnusedClients schedule.
	UnusedClientsInterval = time.Hour

	// unusedClientBatch bounds the clients one batch of the clean-up deletes, and
	// unusedClientBatches the batches of one run; a run that stops there leaves the rest to the
	// next one.
	unusedClientBatch   = 100
	unusedClientBatches = 20
)

// ErrDynamicRegistrationDisabled reports that the instance does not accept dynamic client
// registration (the dynamic_client_registration setting).
var ErrDynamicRegistrationDisabled = errors.New("identity: dynamic client registration is disabled")

// DynamicRegistrationRequest is the client metadata of a registration request (RFC 7591 §2).
// Metadata that the server does not know are ignored by the caller.
type DynamicRegistrationRequest struct {
	RedirectURIs            []string
	TokenEndpointAuthMethod string
	GrantTypes              []string
	ResponseTypes           []string
	ClientName              string
	ClientURI               string
	LogoURI                 string
	// Scope is the space-separated scopes the client asks for; "" asks for the scope of
	// ThirdPartyScopes.
	Scope             string
	SoftwareStatement string
}

// DynamicRegistration is the registered client with the metadata the server settled on.
type DynamicRegistration struct {
	Client                  identity.OAuthClient
	RedirectURIs            []string
	GrantTypes              []string
	ResponseTypes           []string
	TokenEndpointAuthMethod string
}

// dynamicGrantTypes and dynamicResponseTypes are what a registration may ask for.
var (
	dynamicGrantTypes    = []string{"authorization_code", "refresh_token"}
	dynamicResponseTypes = []string{"code"}
)

func invalidMetadata(description string) *OAuthError {
	return oauthError(OAuthInvalidClientMetadata, description)
}

// RegisterDynamicClient registers a public client by the metadata of req (RFC 7591 §3). It
// fails with ErrDynamicRegistrationDisabled when the instance turned registration off, and
// otherwise with an *OAuthError for metadata it refuses: only token_endpoint_auth_method none
// (the default here, whatever RFC 7591 §2 says) is accepted; scopes that only first-party
// clients may hold are left out of the client's scope, and unknown scopes with them.
func (s *Service) RegisterDynamicClient(ctx context.Context, req DynamicRegistrationRequest) (DynamicRegistration, error) {
	if s.d.DynamicClientRegistration == nil {
		return DynamicRegistration{}, ErrDynamicRegistrationDisabled
	}
	enabled, err := s.d.DynamicClientRegistration(ctx)
	if err != nil {
		return DynamicRegistration{}, err
	}
	if !enabled {
		return DynamicRegistration{}, ErrDynamicRegistrationDisabled
	}

	if req.SoftwareStatement != "" {
		return DynamicRegistration{}, oauthError(OAuthInvalidSoftwareStatement, "software statements are not supported")
	}
	if req.TokenEndpointAuthMethod != "" && req.TokenEndpointAuthMethod != "none" {
		return DynamicRegistration{}, invalidMetadata("token_endpoint_auth_method must be none: only public clients register themselves")
	}
	grantTypes, err := dynamicSet("grant_types", req.GrantTypes, dynamicGrantTypes, []string{"authorization_code"}, "authorization_code")
	if err != nil {
		return DynamicRegistration{}, err
	}
	responseTypes, err := dynamicSet("response_types", req.ResponseTypes, dynamicResponseTypes, []string{"code"}, "code")
	if err != nil {
		return DynamicRegistration{}, err
	}
	uris, err := dynamicRedirectURIs(req.RedirectURIs)
	if err != nil {
		return DynamicRegistration{}, err
	}
	name := strings.TrimSpace(req.ClientName)
	if utf8.RuneCountInString(name) > MaxDynamicClientNameLength || strings.ContainsFunc(name, unicode.IsControl) || !utf8.ValidString(name) {
		return DynamicRegistration{}, invalidMetadata("client_name is too long or has control characters")
	}
	website, err := dynamicHTTPSURL("client_uri", req.ClientURI)
	if err != nil {
		return DynamicRegistration{}, err
	}
	// The logo is checked but not kept: nothing shows it.
	if _, err := dynamicHTTPSURL("logo_uri", req.LogoURI); err != nil {
		return DynamicRegistration{}, err
	}

	scope := identity.ThirdPartyScopes()
	if strings.TrimSpace(req.Scope) != "" {
		var keep []string
		for _, t := range strings.Fields(req.Scope) {
			if slices.Contains(identity.KnownScopes(), t) && !identity.FirstPartyOnlyScope(t) {
				keep = append(keep, t)
			}
		}
		scope, _ = identity.ParseScope(strings.Join(keep, " ")) // every token is known
	}

	clientID := newClientID()
	if name == "" {
		name = clientID
	}
	c := identity.OAuthClient{
		ID: s.d.IDs.NewID(), ClientID: clientID, Name: name, Website: website, Scope: scope,
		Registration: identity.ClientRegisteredDynamically, CreatedAt: s.d.Clock.Now(),
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		stored, err := s.d.Repository.EnsureOAuthClient(ctx, tx, c)
		if err != nil {
			return err
		}
		return s.d.Repository.ReplaceOAuthClientRedirectURIs(ctx, tx, stored.ID, uris)
	})
	if err != nil {
		return DynamicRegistration{}, fmt.Errorf("identity: register a dynamic client: %w", err)
	}
	return DynamicRegistration{
		Client: c, RedirectURIs: uris, GrantTypes: grantTypes, ResponseTypes: responseTypes, TokenEndpointAuthMethod: "none",
	}, nil
}

// newClientID returns a new random client_id: 128 random bits in base64url.
func newClientID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}

// dynamicSet checks the values of a metadata field that lists what the client uses: each must be
// in allowed and the list must hold required. An empty list is def. The result is sorted
// without duplicates.
func dynamicSet(field string, values, allowed, def []string, required string) ([]string, error) {
	if len(values) == 0 {
		return slices.Clone(def), nil
	}
	for _, v := range values {
		if !slices.Contains(allowed, v) {
			return nil, invalidMetadata(field + " has a value the server does not support")
		}
	}
	if !slices.Contains(values, required) {
		return nil, invalidMetadata(field + " must include " + required)
	}
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out), nil
}

// dynamicRedirectURIs checks redirect_uris with the rules of redirect URIs (ADR-0090 §3).
func dynamicRedirectURIs(uris []string) ([]string, error) {
	if len(uris) == 0 {
		return nil, oauthError(OAuthInvalidRedirectURI, "redirect_uris is required")
	}
	if len(uris) > MaxDynamicRedirectURIs {
		return nil, oauthError(OAuthInvalidRedirectURI, fmt.Sprintf("redirect_uris holds at most %d URIs", MaxDynamicRedirectURIs))
	}
	for _, u := range uris {
		if len(u) > maxDynamicURILength {
			return nil, oauthError(OAuthInvalidRedirectURI, "a redirect URI is too long")
		}
		if err := identity.ValidateRedirectURI(u); err != nil {
			return nil, oauthError(OAuthInvalidRedirectURI, "a redirect URI is not allowed: "+err.Error())
		}
	}
	out := slices.Clone(uris)
	slices.Sort(out)
	return slices.Compact(out), nil
}

// dynamicHTTPSURL checks an optional URL of the metadata: an https URL without userinfo or
// fragment. "" is none.
func dynamicHTTPSURL(field, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxDynamicURILength || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || strings.ContainsFunc(raw, unicode.IsControl) {
		return "", invalidMetadata(field + " must be an https URL")
	}
	return raw, nil
}

// DeleteUnusedDynamicClients deletes the dynamically registered clients that are unused
// (ADR-0090 §1), for the reason unused, and returns how many it deleted: the ones where no
// authorization code was exchanged within the unredeemed TTL of the registration, and the ones
// with no usable token whose newest token was issued longer ago than the idle TTL. It is safe to
// repeat: a deleted client is not unused again, and a run that stops at its batch limit leaves
// the rest to the next one.
func (s *Service) DeleteUnusedDynamicClients(ctx context.Context) (int, error) {
	now := s.d.Clock.Now()
	unredeemed, idle := s.d.DynamicClientUnredeemedTTL, s.d.DynamicClientIdleTTL
	if unredeemed <= 0 {
		unredeemed = DefaultDynamicClientUnredeemedTTL
	}
	if idle <= 0 {
		idle = DefaultDynamicClientIdleTTL
	}
	deleted := 0
	for range unusedClientBatches {
		ids, err := s.d.Repository.ListUnusedDynamicOAuthClients(ctx, now.Add(-unredeemed), now.Add(-idle), now, unusedClientBatch)
		if err != nil {
			return deleted, err
		}
		for _, id := range ids {
			err := s.DeleteOAuthClient(ctx, id, identity.ClientDeletedUnused, "")
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return deleted, err
			}
			deleted++
		}
		if len(ids) < unusedClientBatch {
			break
		}
	}
	return deleted, nil
}

// RegisterClientJobs registers the hourly deletion of the unused dynamically registered clients.
func (s *Service) RegisterClientJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobDeleteUnusedClients, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.DeleteUnusedDynamicClients(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobDeleteUnusedClients, Every: UnusedClientsInterval, Job: queue.NewJob{Type: JobDeleteUnusedClients}})
}

// DynamicClientRegistrationEnabled reports whether the instance accepts dynamic client
// registration (the dynamic_client_registration setting), for the authorization server
// metadata.
func (s *Service) DynamicClientRegistrationEnabled(ctx context.Context) (bool, error) {
	if s.d.DynamicClientRegistration == nil {
		return false, nil
	}
	return s.d.DynamicClientRegistration(ctx)
}
