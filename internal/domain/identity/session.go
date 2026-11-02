// SPDX-License-Identifier: AGPL-3.0-only

package identity

import "time"

// Session is a sign-in to the server-rendered screens (login, registration, consent), carried
// by a cookie (ADR-0019 §4). Only the SHA-256 digest of the cookie's token is stored.
type Session struct {
	ID        string
	UserID    string
	TokenHash []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Expired reports whether the session has expired at now.
func (s Session) Expired(now time.Time) bool { return !now.Before(s.ExpiresAt) }

// SessionOrigin is the client that started a session: its IP address and User-Agent, "" when
// unknown (data-model §4.1, session_origins). It is kept apart from the session so that it is
// erased after its retention (ADR-0038) while the session stays.
type SessionOrigin struct {
	SessionID string
	UserID    string
	IP        string
	UserAgent string
	CreatedAt time.Time
}

// Empty reports whether the origin records nothing; no row is written for it.
func (o SessionOrigin) Empty() bool { return o.IP == "" && o.UserAgent == "" }

// RegistrationOrigin is the IP address a user registered from on the registration page
// (data-model §4.1, registration_origins). It is erased after its retention (ADR-0038).
type RegistrationOrigin struct {
	UserID     string
	IP         string
	RecordedAt time.Time
}

// MaxUserAgentLength bounds the stored User-Agent in bytes; longer values are cut.
const MaxUserAgentLength = 512

// RevocationReason says why a session ended before it expired.
type RevocationReason string

// Revocation reasons (data-model §4.1, session_revocations.reason).
const (
	// RevokedLogout: the user signed out.
	RevokedLogout RevocationReason = "logout"
	// RevokedReplaced: the user signed in again from the same browser, which got a new session.
	RevokedReplaced RevocationReason = "replaced"
	// RevokedPasswordChanged: the user's password changed.
	RevokedPasswordChanged RevocationReason = "password_changed"
	// RevokedModeration: a moderator suspended the user's login.
	RevokedModeration RevocationReason = "moderation"
	// RevokedExpiredCleanup: the cleanup job ended an expired session.
	RevokedExpiredCleanup RevocationReason = "expired_cleanup"
	// RevokedAccountDeleted: the user's local account was deleted.
	RevokedAccountDeleted RevocationReason = "account_deleted"
)

// BrowserSession is a session that is a signed-in browser at some moment (ADR-0136 §1), with
// where it signed in from. IP and UserAgent are "" when unknown or erased after their retention.
type BrowserSession struct {
	ID         string
	IP         string
	UserAgent  string
	CreatedAt  time.Time
	LastSeenAt *time.Time
}

// BrowserSessionFamily is a working token family of the first-party client with the session it
// started from.
type BrowserSessionFamily struct {
	SessionID  string
	FamilyID   string
	LastUsedAt *time.Time
}

// SessionRevocation records that a session ended (data-model §4.1, session_revocations).
type SessionRevocation struct {
	SessionID string
	Reason    RevocationReason
	RevokedAt time.Time
}

// SessionActivity is when a session was last used (data-model §4.1, session_activity).
type SessionActivity struct {
	SessionID  string
	LastSeenAt time.Time
}
