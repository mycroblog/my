// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Client ID Metadata Documents (draft-ietf-oauth-client-id-metadata-document, ADR-0090 §1): a
// client whose client_id is an https URL is not registered in advance; the server fetches the
// JSON document at the URL and treats it as the client's registration. Only public clients are
// accepted, and the document must name its own URL as client_id, exactly.

// ErrInvalidClientIDURL reports a client_id that is not a URL a metadata document may live at.
var ErrInvalidClientIDURL = errors.New("identity: invalid client_id URL")

// ErrInvalidClientMetadata reports a metadata document that the server does not accept.
var ErrInvalidClientMetadata = errors.New("identity: invalid client metadata document")

// Limits of a metadata document.
const (
	// MaxClientIDURLLength bounds the client_id URL.
	MaxClientIDURLLength = 2048
	// MaxClientNameLength bounds client_name, in characters.
	MaxClientNameLength = 200
	// MaxClientRedirectURIs bounds redirect_uris.
	MaxClientRedirectURIs = 20
)

// ClientMetadataDocument is the last fetch of the metadata document of a client whose
// registration is ClientRegisteredByMetadataDocument
// (oauth_client_metadata_documents). It is a cache: fetching the document again rebuilds it.
type ClientMetadataDocument struct {
	// ClientID is the ID of the OAuthClient (not its public client_id).
	ClientID string
	// Name, Website, and LogoURI are what the document declares: client_name (the host of the
	// client_id URL when it has none), client_uri, and logo_uri ("" for none).
	Name    string
	Website string
	LogoURI string
	// Scope is the most that the client may be granted: the document's scope without the
	// unknown and first-party-only scopes, or MetadataDocumentDefaultScope when it names none.
	Scope     Scope
	FetchedAt time.Time
	// ExpiresAt is when the HTTP cache directives of the response stop the document from being
	// reused; FetchedAt for no-store and no-cache.
	ExpiresAt time.Time
}

// Fresh reports whether the document may be used at now without fetching it again.
func (d ClientMetadataDocument) Fresh(now time.Time) bool { return now.Before(d.ExpiresAt) }

// Apply returns c with what the document declares: its name, website, and scope.
func (d ClientMetadataDocument) Apply(c OAuthClient) OAuthClient {
	c.Name, c.Website, c.Scope = d.Name, d.Website, d.Scope
	return c
}

// ClientMetadata is a metadata document that ParseClientMetadataDocument accepted.
type ClientMetadata struct {
	Name         string
	Website      string
	LogoURI      string
	Scope        Scope
	RedirectURIs []string
}

// IsMetadataDocumentClientID reports whether a client_id names a metadata document: it is an
// https URL. Other client_ids are those of registered clients.
func IsMetadataDocumentClientID(clientID string) bool {
	return strings.HasPrefix(clientID, "https://")
}

// ParseMetadataDocumentClientID checks a client_id URL by the rules of the draft (§3): https, a
// host, a path, no fragment, no userinfo, and no "." or ".." path segment. It returns the host
// that host denials match (ClientHost).
func ParseMetadataDocumentClientID(clientID string) (host string, err error) {
	bad := func(why string) error { return fmt.Errorf("%w: %s", ErrInvalidClientIDURL, why) }
	if len(clientID) > MaxClientIDURLLength || !IsMetadataDocumentClientID(clientID) {
		return "", bad("not an https URL of at most 2048 bytes")
	}
	if strings.ContainsAny(clientID, "#\\ ") {
		return "", bad("a fragment, a backslash, or a space")
	}
	u, err := url.Parse(clientID)
	if err != nil || u.User != nil || u.Opaque != "" || u.Hostname() == "" {
		return "", bad("not a URL with a host and without userinfo")
	}
	if u.Path == "" || u.Path == "/" {
		return "", bad("no path")
	}
	for _, seg := range strings.Split(u.EscapedPath(), "/") {
		if seg == "." || seg == ".." || strings.EqualFold(seg, "%2e") || strings.EqualFold(seg, "%2e%2e") {
			return "", bad("a dot path segment")
		}
	}
	return ClientHost(u.Hostname())
}

// ClientHost returns the normal form of a host that host denials are recorded and matched by:
// the host name in lowercase, without a port or the brackets of an IPv6 literal.
func ClientHost(host string) (string, error) {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
	if h == "" || strings.ContainsAny(h, "/@[] ") {
		return "", fmt.Errorf("%w: empty or malformed host %q", ErrInvalidClientIDURL, host)
	}
	if strings.Contains(h, ":") {
		a, err := netip.ParseAddr(h)
		if err != nil || a.Zone() != "" {
			return "", fmt.Errorf("%w: a host with a port %q", ErrInvalidClientIDURL, host)
		}
		return a.String(), nil
	}
	return h, nil
}

// MetadataDocumentDefaultScope is the scope of a metadata document client whose document names
// none: every scope a third-party client may hold.
func MetadataDocumentDefaultScope() Scope {
	tokens := slices.DeleteFunc(AllScopes().Tokens(), FirstPartyOnlyScope)
	return Scope(strings.Join(tokens, " "))
}

// clientMetadataFields are the members of a metadata document that the server reads.
type clientMetadataFields struct {
	ClientID                string   `json:"client_id"`
	ClientName              *string  `json:"client_name"`
	ClientURI               *string  `json:"client_uri"`
	LogoURI                 *string  `json:"logo_uri"`
	RedirectURIs            []string `json:"redirect_uris"`
	Scope                   *string  `json:"scope"`
	TokenEndpointAuthMethod *string  `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

// ParseClientMetadataDocument checks the metadata document fetched from clientID (draft §4,
// ADR-0090 §1-§3) and returns what the server uses of it. The document must be a JSON object
// whose client_id is clientID, compared as strings; it must not hold a client_secret or name a
// token_endpoint_auth_method other than none (only public clients are accepted); its
// redirect_uris must be one to MaxClientRedirectURIs URIs that ValidateRedirectURI accepts;
// grant_types and response_types, when present, must allow the authorization code grant.
// client_uri and logo_uri other than https URLs are ignored, and so are unknown and
// first-party-only scopes.
func ParseClientMetadataDocument(clientID string, body []byte) (ClientMetadata, error) {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidClientMetadata, fmt.Sprintf(format, args...))
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil || members == nil {
		return ClientMetadata{}, bad("not a JSON object")
	}
	for _, secret := range []string{"client_secret", "client_secret_expires_at"} {
		if _, ok := members[secret]; ok {
			return ClientMetadata{}, bad("%s is not allowed", secret)
		}
	}
	var f clientMetadataFields
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&f); err != nil {
		return ClientMetadata{}, bad("malformed member: %v", err)
	}
	if f.ClientID != clientID {
		return ClientMetadata{}, bad("client_id %q is not the URL of the document", f.ClientID)
	}
	if f.TokenEndpointAuthMethod != nil && *f.TokenEndpointAuthMethod != "none" {
		return ClientMetadata{}, bad("token_endpoint_auth_method must be none")
	}
	if f.GrantTypes != nil && !slices.Contains(f.GrantTypes, "authorization_code") {
		return ClientMetadata{}, bad("grant_types must include authorization_code")
	}
	if f.ResponseTypes != nil && !slices.Contains(f.ResponseTypes, "code") {
		return ClientMetadata{}, bad("response_types must include code")
	}
	if len(f.RedirectURIs) == 0 || len(f.RedirectURIs) > MaxClientRedirectURIs {
		return ClientMetadata{}, bad("redirect_uris must hold 1 to %d URIs", MaxClientRedirectURIs)
	}
	for _, u := range f.RedirectURIs {
		if err := ValidateRedirectURI(u); err != nil {
			return ClientMetadata{}, bad("redirect_uris: %v", err)
		}
	}
	host, err := ParseMetadataDocumentClientID(clientID)
	if err != nil {
		return ClientMetadata{}, err
	}
	m := ClientMetadata{Name: host, RedirectURIs: slices.Compact(slices.Sorted(slices.Values(f.RedirectURIs)))}
	if f.ClientName != nil {
		name := strings.TrimSpace(*f.ClientName)
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxClientNameLength {
			return ClientMetadata{}, bad("client_name is longer than %d characters", MaxClientNameLength)
		}
		if name != "" {
			m.Name = name
		}
	}
	m.Website = httpsURL(f.ClientURI)
	m.LogoURI = httpsURL(f.LogoURI)
	m.Scope = MetadataDocumentDefaultScope()
	if f.Scope != nil && *f.Scope != "" {
		m.Scope = thirdPartyScope(*f.Scope)
	}
	return m, nil
}

// httpsURL returns *s when it is an https URL with a host, and "" otherwise.
func httpsURL(s *string) string {
	if s == nil || len(*s) > MaxClientIDURLLength {
		return ""
	}
	u, err := url.Parse(*s)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return *s
}

// thirdPartyScope returns the known scopes of a space-separated list that a third-party client
// may request, in normal form: neither the first-party-only scopes nor the permission scopes.
func thirdPartyScope(s string) Scope {
	var tokens []string
	for _, t := range strings.Fields(s) {
		if slices.Contains(knownScopes, t) && !FirstPartyOnlyScope(t) && !SelfIssuedOnlyScope(t) {
			tokens = append(tokens, t)
		}
	}
	scope, err := ParseScope(strings.Join(tokens, " "))
	if err != nil {
		return ""
	}
	return scope
}

// OAuthClientHostDenial records that an administrator denied the metadata documents of a host
// (oauth_client_host_denials). DeniedByUserID is "" for the CLI.
type OAuthClientHostDenial struct {
	ID             string
	Host           string
	DeniedByUserID string
	DeniedAt       time.Time
}
