// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// RemoteActorTTL is how long a fetched remote actor stays fresh. After it, the actor is fetched
// again the next time it is used, and the periodic refresh fetches the ones that local accounts
// follow (federation.md §6.2, ADR-0019 §7).
const RemoteActorTTL = 24 * time.Hour

// RemoteAccount is the remote subtype of an account: an actor of another server.
type RemoteAccount struct {
	AccountID   string
	FirstSeenAt time.Time
}

// RemoteSnapshot is one fetch of a remote actor's document whose content differed from the
// previous fetch (the remote_account_snapshots event). Optional URLs are "" when the document
// has none; the totals are nil when it does not say.
type RemoteSnapshot struct {
	ID                string
	AccountID         string
	PreferredUsername string
	ProfileURL        string
	InboxURL          string
	SharedInboxURL    string
	OutboxURL         string
	FollowersURL      string
	FollowingURL      string
	FeaturedURL       string
	FeaturedTagsURL   string
	FollowersTotal    *int64
	FollowingTotal    *int64
	StatusesTotal     *int64
	// DocumentDigest is the SHA-256 of the fetched document; a fetch with the same digest as
	// the newest snapshot records no new one.
	DocumentDigest []byte
	FetchedAt      time.Time
}

// SameDocument reports whether s was made from the same document as the digest.
func (s RemoteSnapshot) SameDocument(digest []byte) bool {
	return s.ID != "" && bytes.Equal(s.DocumentDigest, digest)
}

// RemoteHead is the newest snapshot of a remote actor (the remote_account_heads projection):
// the endpoints delivery reads and the fetch state of the periodic refresh (data-model §4.2).
type RemoteHead struct {
	AccountID  string
	SnapshotID string
	InboxURL   string
	// SharedInboxURL is the server's shared inbox (endpoints.sharedInbox), "" when the actor
	// declares none.
	SharedInboxURL           string
	LastFetchedAt            time.Time
	ConsecutiveFetchFailures int
}

// Stale reports whether the actor was last fetched RemoteActorTTL or more before now.
func (h RemoteHead) Stale(now time.Time) bool {
	return !now.Before(h.LastFetchedAt.Add(RemoteActorTTL))
}

// HandleOutcome is the result of checking a remote actor's handle with WebFinger.
type HandleOutcome string

// Handle check outcomes (remote_account_handle_verifications.outcome).
const (
	// HandleVerified means that WebFinger of acct:{username}@{domain} names the actor as self.
	HandleVerified HandleOutcome = "verified"
	// HandleMismatch means that WebFinger answered without naming the actor: the account has
	// no handle until a later check verifies one.
	HandleMismatch HandleOutcome = "mismatch"
)

// HandleVerification is one WebFinger check of a remote actor's handle
// (the remote_account_handle_verifications event). Username and Domain are the handle that was
// checked; Domain is a canonical host.
type HandleVerification struct {
	ID        string
	AccountID string
	Username  string
	Domain    string
	Outcome   HandleOutcome
	CheckedAt time.Time
}

// SameResult reports whether v found what o found: the same outcome for the same handle.
func (v HandleVerification) SameResult(o HandleVerification) bool {
	return v.Outcome == o.Outcome && v.Username == o.Username && v.Domain == o.Domain
}

// Handle returns the handle that a verified check gives the account, and false for a mismatch.
func (v HandleVerification) Handle() (Handle, bool) {
	if v.Outcome != HandleVerified {
		return Handle{}, false
	}
	return Handle{
		AccountID: v.AccountID, Username: v.Username,
		UsernameNormalized: NormalizeUsername(v.Username), Domain: NormalizeDomain(v.Domain),
	}, true
}

// RemoteProfileSource is where a remote profile revision came from
// (the remote_profile_revisions subtype): the snapshot that was current when it was made and
// the version of the HTML policy that sanitized its NoteHTML and field values (ADR-0019 §2).
type RemoteProfileSource struct {
	SnapshotID        string
	HTMLPolicyVersion int
	// ProfileDigest is the Digest of the ReceivedProfile the revision was made from. A revision
	// that kept the profile before it keeps that revision's digest.
	ProfileDigest []byte
}

// ReceivedProfile is the part of a remote actor's profile that the decision hook inbound.actor
// reviews (ADR-0147 §2): the display name, the note, and the fields as sanitized, and the URLs of
// the avatar and the header as received. The server sends no bytes of the images.
type ReceivedProfile struct {
	DisplayName string
	NoteHTML    string
	Fields      []ReceivedProfileField
	AvatarURL   string
	HeaderURL   string
}

// ReceivedProfileField is a field of a ReceivedProfile.
type ReceivedProfileField struct {
	Name      string
	ValueHTML string
}

// receivedProfileVersion names the encoding of Digest; changing what the digest covers changes it.
const receivedProfileVersion = "inbound.actor/1"

// Digest returns the SHA-256 of the profile in an encoding where each value has its length, so
// that no two profiles share one. Two profiles with the same digest are the same review subject:
// a profile fetched again unchanged asks no plugin (ADR-0147 §6).
func (p ReceivedProfile) Digest() []byte {
	var b []byte
	add := func(s string) {
		b = binary.AppendUvarint(b, uint64(len(s)))
		b = append(b, s...)
	}
	add(receivedProfileVersion)
	add(p.DisplayName)
	add(p.NoteHTML)
	b = binary.AppendUvarint(b, uint64(len(p.Fields)))
	for _, f := range p.Fields {
		add(f.Name)
		add(f.ValueHTML)
	}
	add(p.AvatarURL)
	add(p.HeaderURL)
	sum := sha256.Sum256(b)
	return sum[:]
}

// KeyRevocation records that a key was replaced or withdrawn (the account_key_revocations
// event). Signatures never verify with a revoked key.
type KeyRevocation struct {
	AccountKeyID string
	RevokedAt    time.Time
}
