// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"strings"
	"time"
)

// Conversation is a thread of statuses (ADR-0154 §1): a resource with an ID and an IRI. The IRI
// is the one a remote post declared (context, else conversation), or one this server made
// (ConversationURI), which IsLocal tells. Every stored status belongs to exactly one
// conversation, and a conversation's row stays when its statuses go, so that its IRI never
// names another one.
type Conversation struct {
	ID      string
	URI     string
	IsLocal bool
}

// HTTPS reports whether the conversation's IRI is an https IRI, which a Note may carry as its
// context; another one (Mastodon's tag: URIs) goes in conversation alone (ADR-0154 §5).
func (c Conversation) HTTPS() bool { return strings.HasPrefix(c.URI, "https://") }

// ConversationMerge records that a conversation was merged into another (ADR-0154 §3), once a
// reply showed both to be one thread: the status whose storing showed it, and when. The child's
// side is merged into the parent's, and a conversation is merged once.
type ConversationMerge struct {
	MergedConversationID string
	IntoConversationID   string
	StatusID             string
	MergedAt             time.Time
}

// MaxConversationMergeHops is how many merges a lookup of a merged conversation follows to the
// conversation it ended in (ADR-0154 §3); a longer chain is an inconsistency
// (ErrConversationMergeChain).
const MaxConversationMergeHops = 32

// ErrConversationMergeChain reports a chain of merges longer than MaxConversationMergeHops.
var ErrConversationMergeChain = errors.New("content: the chain of conversation merges is too long")

// conversationsPath is the path of the IRIs of this server's conversations before the ID
// (ADR-0018, ADR-0154 §4).
const conversationsPath = "/ap/contexts/"

// ConversationURI returns the IRI of a conversation that this server made on host (ADR-0154
// §4); the form never changes once it has been sent.
func ConversationURI(host, conversationID string) string {
	return "https://" + host + conversationsPath + conversationID
}

// IsLocalConversationURI reports whether uri has the form of the IRI of a conversation of this
// server on host, whether or not the conversation exists. A remote post that declares such an
// IRI does not join the conversation by it (ADR-0154 §2).
func IsLocalConversationURI(host, uri string) bool {
	prefix := "https://" + host + conversationsPath
	return len(uri) >= len(prefix) && strings.EqualFold(uri[:len(prefix)], prefix)
}
