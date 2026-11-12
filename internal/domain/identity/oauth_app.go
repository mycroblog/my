// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The OAuth clients that users register for themselves (ADR-0090 §1, §2): what the user who
// registered a client chooses for it, and the events that change it.

// Limits of the clients users register (ADR-0090 §1).
const (
	// MaxOwnedOAuthClients is how many clients one user may have registered and not deleted.
	MaxOwnedOAuthClients = 25
	// MaxOAuthClientNameLength is the most characters of a client's name.
	MaxOAuthClientNameLength = 100
	// MaxOAuthClientWebsiteLength is the most bytes of a client's website URL.
	MaxOAuthClientWebsiteLength = 2048
	// MaxOAuthClientRedirectURIs is the most redirect URIs of a client.
	MaxOAuthClientRedirectURIs = 10
	// maxRedirectURILength is the most bytes of one redirect URI.
	maxRedirectURILength = 2048
)

// Errors of ValidateOAuthClientSettings, one per setting.
var (
	ErrInvalidOAuthClientName         = errors.New("identity: invalid OAuth client name")
	ErrInvalidOAuthClientWebsite      = errors.New("identity: invalid OAuth client website")
	ErrInvalidOAuthClientRedirectURIs = errors.New("identity: invalid OAuth client redirect URIs")
	ErrInvalidOAuthClientScope        = errors.New("identity: invalid OAuth client scope")
)

// OAuthClientSettings is what the user who registers a client chooses for it and may change
// later: its name, website, redirect URIs, and the most scope it may request.
type OAuthClientSettings struct {
	Name string
	// Website is the client's home page; "" when it has none.
	Website string
	// RedirectURIs are the URIs the client may redirect to, sorted and without duplicates.
	RedirectURIs []string
	Scope        Scope
}

// ValidateOAuthClientSettings checks and normalizes what a user enters for a client: a name of
// 1 to MaxOAuthClientNameLength characters on one line (trimmed), an optional absolute https or
// http website URL, 1 to MaxOAuthClientRedirectURIs redirect URIs that ValidateRedirectURI
// accepts, and a non-empty scope without the first-party-only scopes (ADR-0090 §4). It fails
// with the error of the first setting that it rejects.
func ValidateOAuthClientSettings(name, website string, redirectURIs []string, scope string) (OAuthClientSettings, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxOAuthClientNameLength ||
		strings.ContainsFunc(name, unicode.IsControl) {
		return OAuthClientSettings{}, ErrInvalidOAuthClientName
	}
	if website != "" && !validWebsite(website) {
		return OAuthClientSettings{}, ErrInvalidOAuthClientWebsite
	}
	uris := slices.Clone(redirectURIs)
	slices.Sort(uris)
	uris = slices.Compact(uris)
	if len(uris) == 0 || len(uris) > MaxOAuthClientRedirectURIs {
		return OAuthClientSettings{}, ErrInvalidOAuthClientRedirectURIs
	}
	for _, u := range uris {
		if len(u) > maxRedirectURILength || !utf8.ValidString(u) || strings.ContainsRune(u, 0) {
			return OAuthClientSettings{}, ErrInvalidOAuthClientRedirectURIs
		}
		if err := ValidateRedirectURI(u); err != nil {
			return OAuthClientSettings{}, fmt.Errorf("%w: %w", ErrInvalidOAuthClientRedirectURIs, err)
		}
	}
	s, err := ParseScope(scope)
	if err != nil || s == "" || s.HasFirstPartyOnly() {
		return OAuthClientSettings{}, ErrInvalidOAuthClientScope
	}
	return OAuthClientSettings{Name: name, Website: website, RedirectURIs: uris, Scope: s}, nil
}

// validWebsite reports whether w is an absolute https or http URL with a host, without user
// info or a fragment, of at most MaxOAuthClientWebsiteLength bytes.
func validWebsite(w string) bool {
	if len(w) > MaxOAuthClientWebsiteLength || !utf8.ValidString(w) || strings.ContainsFunc(w, unicode.IsControl) ||
		strings.TrimSpace(w) != w {
		return false
	}
	u, err := url.Parse(w)
	if err != nil || u.User != nil || u.Fragment != "" || u.Hostname() == "" {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http"
}

// OAuthClientRevision records that the user who registered a client changed its name,
// website, or scope (oauth_client_revisions). The newest revision is what the
// client is now.
type OAuthClientRevision struct {
	ID       string
	ClientID string
	Name     string
	// Website is "" when the client has none.
	Website   string
	Scope     Scope
	RevisedAt time.Time
}

// OAuthClientSecretRotation records that a confidential client got a new secret (data-model
// §4.1, oauth_client_secret_rotations). Only the newest secret works.
type OAuthClientSecretRotation struct {
	ID         string
	ClientID   string
	SecretHash []byte
	RotatedAt  time.Time
}

// Revised returns the client as the revision r makes it.
func (c OAuthClient) Revised(r OAuthClientRevision) OAuthClient {
	c.Name, c.Website, c.Scope = r.Name, r.Website, r.Scope
	return c
}

// WithSecret returns the client with the secret of the rotation r.
func (c OAuthClient) WithSecret(r OAuthClientSecretRotation) OAuthClient {
	c.SecretHash = r.SecretHash
	return c
}

// OwnedBy reports whether the user registered the client.
func (c OAuthClient) OwnedBy(userID string) bool {
	return c.Registration == ClientRegisteredByUser && userID != "" && c.OwnerUserID == userID
}
