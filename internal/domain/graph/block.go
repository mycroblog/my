// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"errors"
	"time"
)

// Block is one block of an account by another from its creation on (blocks).
// It is immutable: lifting it is a separate Retraction event, and blocking again after a
// retraction creates a new Block, so earlier blocks stay on record for harassment reviews.
type Block struct {
	ID string
	// URI is the IRI of the Block activity (ADR-0018). Local blocks always have one; it is ""
	// only for a received block whose activity has no id.
	URI              string
	BlockerAccountID string
	BlockedAccountID string
	CreatedAt        time.Time
}

// BlockURI returns the IRI of the Block activity of a local block on host (ADR-0018: every
// activity other than Create lives at /ap/activities/{uuid}).
func BlockURI(host, blockID string) string {
	return "https://" + host + "/ap/activities/" + blockID
}

// ErrSelfBlock reports a block whose blocker and blocked account are the same.
var ErrSelfBlock = errors.New("graph: an account cannot block itself")

// Validate checks the invariants of a new block.
func (b Block) Validate() error {
	if b.BlockerAccountID == "" || b.BlockedAccountID == "" {
		return errors.New("graph: a block needs a blocker and a blocked account")
	}
	if b.BlockerAccountID == b.BlockedAccountID {
		return ErrSelfBlock
	}
	return nil
}

// BlockRetraction is the event that a block was lifted (block_retractions).
type BlockRetraction struct {
	BlockID     string
	RetractedAt time.Time
}

// BlockEdge is the block in force from one account to another (the block_edges projection): a
// block without a retraction. There is at most one edge per (blocker, blocked).
type BlockEdge struct {
	BlockerAccountID string
	BlockedAccountID string
	BlockID          string
}

// EdgeOf returns the edge of block b.
func EdgeOf(b Block) BlockEdge {
	return BlockEdge{BlockerAccountID: b.BlockerAccountID, BlockedAccountID: b.BlockedAccountID, BlockID: b.ID}
}

// BlockHistory is a block together with its retraction: the source of its edge.
type BlockHistory struct {
	Block      Block
	Retraction *BlockRetraction
}

// Edge returns the edge the block projects to, and false when it was retracted. Updating
// block_edges event by event and rebuilding it from histories with this function agree.
func (h BlockHistory) Edge() (BlockEdge, bool) {
	if h.Retraction != nil {
		return BlockEdge{}, false
	}
	return EdgeOf(h.Block), true
}

// Mute hides an account's posts, and optionally its notifications, from the muting account
// (mutes, a current value: it affects only the muting account and keeps no history). A mute
// with an expiry stops applying at ExpiresAt; readers check it with Active, so an expired mute
// needs no write to end.
type Mute struct {
	AccountID         string
	TargetAccountID   string
	HideNotifications bool
	ExpiresAt         *time.Time // nil: until unmuted
	CreatedAt         time.Time
}

var (
	// ErrSelfMute reports a mute whose account and target are the same.
	ErrSelfMute = errors.New("graph: an account cannot mute itself")
	// ErrMuteExpiry reports a mute whose expiry is not after its creation.
	ErrMuteExpiry = errors.New("graph: a mute must expire after it is created")
)

// Validate checks the invariants of a mute.
func (m Mute) Validate() error {
	if m.AccountID == "" || m.TargetAccountID == "" {
		return errors.New("graph: a mute needs an account and a target")
	}
	if m.AccountID == m.TargetAccountID {
		return ErrSelfMute
	}
	if m.ExpiresAt != nil && !m.ExpiresAt.After(m.CreatedAt) {
		return ErrMuteExpiry
	}
	return nil
}

// Active reports whether the mute applies at now: it has no expiry or expires after now.
func (m Mute) Active(now time.Time) bool {
	return m.ExpiresAt == nil || m.ExpiresAt.After(now)
}
