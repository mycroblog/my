// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// LocalDomain is the domain of local accounts. It is the empty string rather than the
// configured host, so that a unique key over (username, domain) never sees NULL and stays valid
// whatever the host is.
const LocalDomain = ""

// ActorType is the ActivityStreams actor type of an account.
type ActorType string

// Actor types (accounts.actor_type).
const (
	ActorPerson       ActorType = "person"
	ActorService      ActorType = "service"
	ActorApplication  ActorType = "application"
	ActorGroup        ActorType = "group"
	ActorOrganization ActorType = "organization"
)

// Account is a local or remote actor: the immutable facts fixed when it is created.
type Account struct {
	ID        string
	URI       string
	Domain    string // LocalDomain for local accounts
	ActorType ActorType
	CreatedAt time.Time
}

// IsLocal reports whether the account belongs to this instance.
func (a Account) IsLocal() bool { return a.Domain == LocalDomain }

// LocalAccount is the local subtype of an account: the user that owns it and its username,
// which never changes.
type LocalAccount struct {
	AccountID          string
	UserID             string
	Username           string
	UsernameNormalized string
}

// Handle resolves @username@domain to an account (the account_handles projection).
type Handle struct {
	AccountID          string
	Username           string
	UsernameNormalized string
	Domain             string
}

// MaxUsernameLength is the longest local username, in bytes (the same limit as Mastodon).
const MaxUsernameLength = 30

// ErrInvalidUsername reports a username that ValidateUsername rejects.
var ErrInvalidUsername = errors.New("invalid username")

// ValidateUsername checks a local username: 1 to MaxUsernameLength ASCII letters, digits, and
// underscores. Usernames compare case-insensitively through NormalizeUsername.
func ValidateUsername(username string) error {
	if username == "" || len(username) > MaxUsernameLength {
		return fmt.Errorf("%w: must be 1 to %d characters", ErrInvalidUsername, MaxUsernameLength)
	}
	for i := range len(username) {
		c := username[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return fmt.Errorf("%w: only letters, digits, and underscores are allowed", ErrInvalidUsername)
		}
	}
	return nil
}

// NormalizeUsername returns the form under which usernames are unique: lowercase.
func NormalizeUsername(username string) string {
	return strings.ToLower(username)
}

// NormalizeDomain returns the stored form of a domain: lowercase. Domains arrive in punycode.
func NormalizeDomain(domain string) string {
	return strings.ToLower(domain)
}

// ActorURI returns the ActivityPub id of a local actor on host (ADR-0018).
func ActorURI(host, accountID string) string {
	return "https://" + host + "/ap/actors/" + accountID
}
