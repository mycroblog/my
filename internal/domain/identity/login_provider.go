// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// External login providers (FR-010, ADR-0106): identity providers that an administrator sets up
// for users to sign in, sign up, and confirm operations with, and the accounts at them that
// users link.

// LoginProviderKind is the kind of an external login provider
// (login_provider_revisions.kind, ADR-0106 §1).
type LoginProviderKind string

// Login provider kinds.
const (
	// LoginProviderOIDC is an OpenID Connect provider found by its issuer's discovery document.
	LoginProviderOIDC LoginProviderKind = "oidc"
	// LoginProviderGitHub is github.com, whose endpoints the server knows.
	LoginProviderGitHub LoginProviderKind = "github"
	// LoginProviderMastodon is one Mastodon server that the administrator names.
	LoginProviderMastodon LoginProviderKind = "mastodon"
	// LoginProviderDiscord is discord.com, whose endpoints the server knows.
	LoginProviderDiscord LoginProviderKind = "discord"
)

// LoginProviderKinds lists every kind.
func LoginProviderKinds() []LoginProviderKind {
	return []LoginProviderKind{LoginProviderOIDC, LoginProviderGitHub, LoginProviderMastodon, LoginProviderDiscord}
}

// Valid reports whether k is a known kind.
func (k LoginProviderKind) Valid() bool { return slices.Contains(LoginProviderKinds(), k) }

// LoginProviderState is the state of a login provider (login_provider_state_changes.state).
type LoginProviderState string

// Login provider states.
const (
	// LoginProviderEnabled: users sign in, sign up, and link with the provider.
	LoginProviderEnabled LoginProviderState = "enabled"
	// LoginProviderDisabled: the provider is kept but nobody uses it; the sign-in screen does
	// not show it.
	LoginProviderDisabled LoginProviderState = "disabled"
	// LoginProviderDeleted: the provider is gone for good.
	LoginProviderDeleted LoginProviderState = "deleted"
)

// Valid reports whether s is a known state.
func (s LoginProviderState) Valid() bool {
	return s == LoginProviderEnabled || s == LoginProviderDisabled || s == LoginProviderDeleted
}

// Limits of a login provider's settings.
const (
	// MaxLoginProviderDisplayName is the number of characters of a provider's display name.
	MaxLoginProviderDisplayName = 100
	// MaxLoginProviderURL is the number of bytes of an issuer or a server URL.
	MaxLoginProviderURL = 2048
	// MaxLoginProviderClientID is the number of bytes of a client_id.
	MaxLoginProviderClientID = 512
	// MaxLoginProviderScopes is the number of extra scopes.
	MaxLoginProviderScopes = 20
	// MaxLoginProviderScopesLength is the number of bytes of the extra scopes joined by spaces.
	MaxLoginProviderScopesLength = 1024
	// MaxLoginProviderClaims is the number of claims that the conditions of an OIDC provider name.
	MaxLoginProviderClaims = 10
	// MaxLoginProviderClaimName is the number of bytes of a claim's name.
	MaxLoginProviderClaimName = 100
	// MaxLoginProviderClaimValues is the number of values allowed for one claim.
	MaxLoginProviderClaimValues = 50
	// MaxLoginProviderClaimValue is the number of characters of an allowed value.
	MaxLoginProviderClaimValue = 256
	// MaxLoginProviderOrganizations is the number of GitHub Organizations allowed.
	MaxLoginProviderOrganizations = 50
	// MaxLoginProviderGuilds is the number of Discord servers (guilds) allowed.
	MaxLoginProviderGuilds = 50
	// MaxDiscordSnowflake is the number of digits of a Discord ID (a snowflake is a 64-bit
	// unsigned integer).
	MaxDiscordSnowflake = 20
	// MaxGitHubLogin is the number of characters of a GitHub login.
	MaxGitHubLogin = 39
	// MaxExternalSubject is the number of bytes of an identifier at a provider (the OIDC sub is
	// at most 255 ASCII characters, OpenID Connect Core 1.0 §2).
	MaxExternalSubject = 255
)

var (
	// ErrInvalidLoginProviderKind reports an unknown kind.
	ErrInvalidLoginProviderKind = errors.New("identity: invalid login provider kind")
	// ErrInvalidLoginProviderName reports a display name that is empty, too long, or has control
	// characters.
	ErrInvalidLoginProviderName = errors.New("identity: invalid login provider display name")
	// ErrInvalidLoginProviderURL reports an issuer or a server URL that the kind does not allow:
	// see LoginProviderSettings.Validate.
	ErrInvalidLoginProviderURL = errors.New("identity: invalid login provider URL")
	// ErrInvalidLoginProviderClientID reports a client_id that is empty, too long, or has
	// control characters.
	ErrInvalidLoginProviderClientID = errors.New("identity: invalid login provider client_id")
	// ErrInvalidLoginProviderScopes reports an extra scope that is not an RFC 6749 scope token,
	// too many scopes, or extra scopes for a kind that takes none.
	ErrInvalidLoginProviderScopes = errors.New("identity: invalid login provider scopes")
	// ErrInvalidLoginProviderConditions reports conditions that the kind does not take, or a
	// claim, a value, an Organization, or a guild ID that is not valid.
	ErrInvalidLoginProviderConditions = errors.New("identity: invalid login provider conditions")
	// ErrLoginProviderKindChanged reports a revision that changes the kind of a provider: the
	// identifiers of the linked accounts mean something else at another kind.
	ErrLoginProviderKindChanged = errors.New("identity: the kind of a login provider cannot change")
	// ErrInvalidExternalSubject reports an identifier at a provider that is empty, too long, or
	// has control characters.
	ErrInvalidExternalSubject = errors.New("identity: invalid external subject")
)

// ClaimCondition lets a user of an OIDC provider sign in only when the ID token's claim has one
// of Values (ADR-0106 §4), such as hd for a Google Workspace domain.
type ClaimCondition struct {
	Claim string
	// Values are the allowed values, sorted.
	Values []string
}

// LoginProviderSettings are what an administrator sets on a login provider: one revision of it
// (login_provider_revisions with its claims and organizations).
type LoginProviderSettings struct {
	Kind LoginProviderKind
	// DisplayName is the name that the sign-in screen shows on the provider's button for oidc,
	// as the administrator wrote it, and that the administration screens show for every kind.
	DisplayName string
	// Issuer is the OIDC issuer for oidc, the server's URL (https://<host>) for mastodon, and
	// "" for github and discord.
	Issuer   string
	ClientID string
	// Scopes are the scopes asked for besides the ones the kind needs, sorted.
	Scopes []string
	// AllowSignup lets new users sign up with the provider (false by default).
	AllowSignup bool
	// AllowLink lets users link their account to the provider (true by default).
	AllowLink bool
	// Claims are the conditions of an oidc provider, by claim: a user meets them when each claim
	// has one of its values. None for the other kinds.
	Claims []ClaimCondition
	// Organizations are the logins of the GitHub Organizations, in lower case and sorted, one of
	// which a user of a github provider must belong to. None lets every GitHub user in, and the
	// other kinds have none.
	Organizations []string
	// Guilds are the IDs of the Discord servers (guilds), sorted, one of which a user of a
	// discord provider must belong to. None lets every Discord user in, and the other kinds
	// have none.
	Guilds []string
}

// NewLoginProviderSettings returns the settings of a new provider of kind k with the default
// policy: no signup, linking allowed (ADR-0106 §4).
func NewLoginProviderSettings(k LoginProviderKind) LoginProviderSettings {
	return LoginProviderSettings{Kind: k, AllowLink: true}
}

// Normalize trims the strings of s; turns the server of a mastodon provider, given as a domain
// or a URL, into https://<host>; and sorts the scopes, the claims and their values, and the
// Organizations in lower case and the guild IDs, without duplicates. It leaves an OIDC issuer as it is, since the
// iss of the ID tokens must equal it.
func (s LoginProviderSettings) Normalize() LoginProviderSettings {
	s.DisplayName = strings.TrimSpace(s.DisplayName)
	s.Issuer = strings.TrimSpace(s.Issuer)
	s.ClientID = strings.TrimSpace(s.ClientID)
	if s.Kind == LoginProviderMastodon && s.Issuer != "" {
		s.Issuer = normalizeServerURL(s.Issuer)
	}
	s.Scopes = sortedUnique(s.Scopes, strings.TrimSpace)
	byClaim := map[string][]string{}
	for _, c := range s.Claims {
		name := strings.TrimSpace(c.Claim)
		byClaim[name] = append(byClaim[name], c.Values...)
	}
	s.Claims = nil
	for _, name := range slices.Sorted(maps.Keys(byClaim)) {
		s.Claims = append(s.Claims, ClaimCondition{Claim: name, Values: sortedUnique(byClaim[name], strings.TrimSpace)})
	}
	s.Organizations = sortedUnique(s.Organizations, func(o string) string { return strings.ToLower(strings.TrimSpace(o)) })
	s.Guilds = sortedUnique(s.Guilds, strings.TrimSpace)
	return s
}

// sortedUnique maps each of in with f, drops the empty results, and sorts the rest without
// duplicates. It returns nil for nothing.
func sortedUnique(in []string, f func(string) string) []string {
	var out []string
	for _, v := range in {
		if v = f(v); v != "" {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// normalizeServerURL turns a Mastodon server given as a domain (example.social) or a URL
// (https://Example.social/) into https://<host>, the host in lower case. What it cannot parse
// it returns as it is, for Validate to reject.
func normalizeServerURL(raw string) string {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	u.Host = strings.ToLower(u.Host)
	if u.Path == "/" {
		u.Path = ""
	}
	return u.String()
}

// Validate checks normalized settings:
//
//   - the kind is known and the display name has 1 to MaxLoginProviderDisplayName characters
//     without control characters; the client_id has 1 to MaxLoginProviderClientID bytes
//     without control characters;
//   - oidc: the issuer is an https URL with a host and without user info, a query, or a
//     fragment (OpenID Connect Discovery 1.0 §2); conditions only on claims;
//   - github: no issuer (its endpoints are fixed); conditions only on Organizations;
//   - discord: no issuer (its endpoints are fixed); conditions only on guild IDs (decimal
//     snowflakes);
//   - mastodon: the server is https://<host> with nothing after the host; no extra scopes, since
//     the server asks for profile and nothing wider; no conditions;
//   - each extra scope is an RFC 6749 scope token (§3.3), at most MaxLoginProviderScopes of them.
func (s LoginProviderSettings) Validate() error {
	if !s.Kind.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidLoginProviderKind, s.Kind)
	}
	if s.DisplayName == "" || !utf8.ValidString(s.DisplayName) ||
		utf8.RuneCountInString(s.DisplayName) > MaxLoginProviderDisplayName || strings.ContainsFunc(s.DisplayName, unicode.IsControl) {
		return ErrInvalidLoginProviderName
	}
	if s.ClientID == "" || len(s.ClientID) > MaxLoginProviderClientID || !utf8.ValidString(s.ClientID) ||
		strings.ContainsFunc(s.ClientID, unicode.IsControl) {
		return ErrInvalidLoginProviderClientID
	}
	if err := s.validateIssuer(); err != nil {
		return err
	}
	if err := s.validateScopes(); err != nil {
		return err
	}
	return s.validateConditions()
}

func (s LoginProviderSettings) validateIssuer() error {
	switch s.Kind {
	case LoginProviderGitHub, LoginProviderDiscord:
		if s.Issuer != "" {
			return fmt.Errorf("%w: %s takes no issuer", ErrInvalidLoginProviderURL, s.Kind)
		}
		return nil
	case LoginProviderOIDC:
		_, err := parseProviderURL(s.Issuer)
		return err
	default: // LoginProviderMastodon
		u, err := parseProviderURL(s.Issuer)
		if err != nil {
			return err
		}
		if u.Path != "" || u.RawPath != "" || strings.HasSuffix(s.Issuer, "/") {
			return fmt.Errorf("%w: a Mastodon server has no path", ErrInvalidLoginProviderURL)
		}
		return nil
	}
}

// parseProviderURL parses an https URL with a host, without user info, a query, or a fragment.
func parseProviderURL(raw string) (*url.URL, error) {
	if raw == "" || len(raw) > MaxLoginProviderURL || strings.ContainsFunc(raw, unicode.IsControl) {
		return nil, ErrInvalidLoginProviderURL
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return nil, ErrInvalidLoginProviderURL
	case u.Scheme != "https":
		return nil, fmt.Errorf("%w: scheme %q", ErrInvalidLoginProviderURL, u.Scheme)
	case u.Hostname() == "":
		return nil, fmt.Errorf("%w: no host", ErrInvalidLoginProviderURL)
	case u.User != nil:
		return nil, fmt.Errorf("%w: user info", ErrInvalidLoginProviderURL)
	case u.RawQuery != "" || u.ForceQuery:
		return nil, fmt.Errorf("%w: query", ErrInvalidLoginProviderURL)
	case u.Fragment != "" || strings.Contains(raw, "#"):
		return nil, fmt.Errorf("%w: fragment", ErrInvalidLoginProviderURL)
	}
	return u, nil
}

func (s LoginProviderSettings) validateScopes() error {
	if len(s.Scopes) == 0 {
		return nil
	}
	if s.Kind == LoginProviderMastodon {
		return fmt.Errorf("%w: mastodon takes no extra scopes", ErrInvalidLoginProviderScopes)
	}
	if len(s.Scopes) > MaxLoginProviderScopes || len(strings.Join(s.Scopes, " ")) > MaxLoginProviderScopesLength {
		return fmt.Errorf("%w: too many", ErrInvalidLoginProviderScopes)
	}
	for _, sc := range s.Scopes {
		if !isScopeToken(sc) {
			return fmt.Errorf("%w: %q", ErrInvalidLoginProviderScopes, sc)
		}
	}
	return nil
}

// isScopeToken reports whether s is a scope-token of RFC 6749 §3.3:
// 1*( %x21 / %x23-5B / %x5D-7E ).
func isScopeToken(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if c < 0x21 || c == 0x22 || c == 0x5c || c > 0x7e {
			return false
		}
	}
	return true
}

func (s LoginProviderSettings) validateConditions() error {
	if len(s.Claims) > 0 && s.Kind != LoginProviderOIDC {
		return fmt.Errorf("%w: only oidc takes claims", ErrInvalidLoginProviderConditions)
	}
	if len(s.Organizations) > 0 && s.Kind != LoginProviderGitHub {
		return fmt.Errorf("%w: only github takes Organizations", ErrInvalidLoginProviderConditions)
	}
	if len(s.Guilds) > 0 && s.Kind != LoginProviderDiscord {
		return fmt.Errorf("%w: only discord takes guilds", ErrInvalidLoginProviderConditions)
	}
	if len(s.Claims) > MaxLoginProviderClaims {
		return fmt.Errorf("%w: too many claims", ErrInvalidLoginProviderConditions)
	}
	for _, c := range s.Claims {
		if len(c.Claim) > MaxLoginProviderClaimName || !isScopeToken(c.Claim) {
			return fmt.Errorf("%w: claim %q", ErrInvalidLoginProviderConditions, c.Claim)
		}
		if len(c.Values) == 0 || len(c.Values) > MaxLoginProviderClaimValues {
			return fmt.Errorf("%w: claim %q needs 1 to %d values", ErrInvalidLoginProviderConditions, c.Claim, MaxLoginProviderClaimValues)
		}
		for _, v := range c.Values {
			if v == "" || !utf8.ValidString(v) || utf8.RuneCountInString(v) > MaxLoginProviderClaimValue || strings.ContainsFunc(v, unicode.IsControl) {
				return fmt.Errorf("%w: a value of claim %q", ErrInvalidLoginProviderConditions, c.Claim)
			}
		}
	}
	if len(s.Organizations) > MaxLoginProviderOrganizations {
		return fmt.Errorf("%w: too many Organizations", ErrInvalidLoginProviderConditions)
	}
	for _, o := range s.Organizations {
		if !IsGitHubLogin(o) {
			return fmt.Errorf("%w: Organization %q", ErrInvalidLoginProviderConditions, o)
		}
	}
	if len(s.Guilds) > MaxLoginProviderGuilds {
		return fmt.Errorf("%w: too many guilds", ErrInvalidLoginProviderConditions)
	}
	for _, g := range s.Guilds {
		if !IsDiscordSnowflake(g) {
			return fmt.Errorf("%w: guild %q", ErrInvalidLoginProviderConditions, g)
		}
	}
	return nil
}

// IsDiscordSnowflake reports whether s is a Discord ID: 1 to MaxDiscordSnowflake decimal
// digits, without a leading zero.
func IsDiscordSnowflake(s string) bool {
	if s == "" || len(s) > MaxDiscordSnowflake || s[0] == '0' {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IsGitHubLogin reports whether s is a GitHub login in lower case: 1 to MaxGitHubLogin letters,
// digits, and hyphens, without a hyphen at either end or two in a row.
func IsGitHubLogin(s string) bool {
	if s == "" || len(s) > MaxGitHubLogin || s[0] == '-' || s[len(s)-1] == '-' || strings.Contains(s, "--") {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// ValidateLoginProviderChange checks a new revision next of a provider whose current settings
// are prev: it keeps the kind.
func ValidateLoginProviderChange(prev, next LoginProviderSettings) error {
	if prev.Kind != next.Kind {
		return ErrLoginProviderKindChanged
	}
	return nil
}

// LoginProviderRevision is one revision of a provider's settings (login_provider_revisions).
type LoginProviderRevision struct {
	ID         string
	ProviderID string
	Settings   LoginProviderSettings
	// RevisedByUserID is the administrator who made the revision, or "" for the CLI.
	RevisedByUserID string
	RevisedAt       time.Time
}

// LoginProviderSecret is a client secret of a provider, sealed with kernel.Sealer
// (login_provider_secrets). A rotation adds one; the newest is current. The secret is never
// shown again after it is entered.
type LoginProviderSecret struct {
	ID         string
	ProviderID string
	Sealed     []byte
	// CreatedByUserID is the administrator who entered the secret, or "" for the CLI.
	CreatedByUserID string
	CreatedAt       time.Time
}

// LoginProviderStateChange is a change of a provider's state (login_provider_state_changes).
type LoginProviderStateChange struct {
	ID         string
	ProviderID string
	State      LoginProviderState
	// ChangedByUserID is the administrator who changed it, or "" for the CLI.
	ChangedByUserID string
	ChangedAt       time.Time
}

// LoginProviderHead is the current revision, state, and secret of a provider
// (login_provider_heads).
type LoginProviderHead struct {
	ProviderID string
	RevisionID string
	State      LoginProviderState
	SecretID   string
}

// LoginProvider is a provider with its current settings and state.
type LoginProvider struct {
	ID string
	// CreatedByUserID is the administrator who set the provider up, or "" for the CLI.
	CreatedByUserID string
	CreatedAt       time.Time
	RevisionID      string
	Settings        LoginProviderSettings
	RevisedAt       time.Time
	State           LoginProviderState
	StateChangedAt  time.Time
	// SecretID is the current secret's row.
	SecretID string
}

// ExternalIdentity is an account at a provider (external_identities): Subject is its
// identifier there, the OIDC sub, the GitHub user's numeric id, the Mastodon account's id, or
// the Discord user's snowflake id.
// The pair of ProviderID and Subject is unique.
type ExternalIdentity struct {
	ID         string
	ProviderID string
	Subject    string
	CreatedAt  time.Time
}

// ValidateExternalSubject checks an identifier at a provider: 1 to MaxExternalSubject bytes of
// valid UTF-8 without control characters.
func ValidateExternalSubject(subject string) error {
	if subject == "" || len(subject) > MaxExternalSubject || !utf8.ValidString(subject) ||
		strings.ContainsFunc(subject, unicode.IsControl) {
		return ErrInvalidExternalSubject
	}
	return nil
}

// ExternalIdentityLink links an external identity to a user, who then signs in with it
// (external_identity_links). A link without an unlink is current.
type ExternalIdentityLink struct {
	ID                 string
	ExternalIdentityID string
	UserID             string
	LinkedAt           time.Time
}

// ExternalIdentityUnlink ends a link (external_identity_unlinks): its user removed it.
type ExternalIdentityUnlink struct {
	LinkID     string
	UnlinkedAt time.Time
}

// LinkedExternalIdentity is a current link with its external identity.
type LinkedExternalIdentity struct {
	LinkID             string
	ExternalIdentityID string
	ProviderID         string
	Subject            string
	UserID             string
	LinkedAt           time.Time
}
