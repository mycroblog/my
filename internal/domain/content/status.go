// SPDX-License-Identifier: AGPL-3.0-only

package content

import "time"

// CountsAsReply reports whether a reply of visibility v counts toward its parent's replies
// count. Followers-only and direct replies do not, so that the count does not reveal them.
func (v Visibility) CountsAsReply() bool {
	return v == VisibilityPublic || v == VisibilityLocal || v == VisibilityUnlisted
}

// Status is a local or remote status: the immutable facts fixed when it is created. Its content
// lives in revisions.
type Status struct {
	ID              string
	URI             string
	URL             string // a remote status's HTML page, "" when it has none or the status is local (PageURL)
	AuthorAccountID string
	IsLocal         bool
	Visibility      Visibility
	InReplyToURI    string // "" unless the status is a reply
	PublishedAt     time.Time
}

// CountsAsLocalPost reports whether the status counts toward its author's local posts, the
// usage.localPosts of NodeInfo. As Mastodon and GoToSocial count them, these are the statuses
// of local accounts except the direct ones.
func (s Status) CountsAsLocalPost() bool {
	return s.IsLocal && s.Visibility != VisibilityDirect
}

// LocalStatus is the local subtype of a status: the client that posted it and the client API's
// Idempotency-Key, unique per author.
type LocalStatus struct {
	StatusID        string
	AuthorAccountID string
	ClientID        string // "" when no OAuth client is recorded
	IdempotencyKey  string // "" when the request had none
}

// SourceFormat is the markup of a local status's source text.
type SourceFormat string

// Source formats (status_revision_sources.source_format).
const SourceTextPlain SourceFormat = "text_plain"

// MaxAttachments is the largest number of media attachments of one revision.
const MaxAttachments = 4

// Revision is one version of a status's content (FR-109). Seq starts at 1 and grows by one per
// revision of the same status; the revision with the highest Seq is current.
type Revision struct {
	ID          string
	StatusID    string
	Seq         int
	SpoilerText string
	Sensitive   bool
	Language    string // BCP 47, "und" when unknown
	RevisedAt   time.Time
	// Bodies hold the content per language, ordered by Language. A status without contentMap
	// has one body in the revision's Language.
	Bodies []Body
	// Source is the text a local author typed, nil for remote statuses.
	Source *Source
	// Attachments are ordered by Position, 0 to MaxAttachments-1.
	Attachments []Attachment
	// MentionedAccountIDs, TagIDs, and EmojiIDs are sets, each ordered by ID.
	MentionedAccountIDs []string
	TagIDs              []string
	EmojiIDs            []string
	// Poll is the poll of the revision, nil for none (FR-112).
	Poll *Poll
	// Remote is the remote subtype of a remote status's revision, nil for local statuses.
	Remote *RemoteRevisionSource
}

// Body is the content of a revision in one language.
type Body struct {
	Language    string
	ContentHTML string
}

// Source is the input of a local revision.
type Source struct {
	Text   string
	Format SourceFormat
}

// Attachment is a media attachment of a revision, with its alt text.
type Attachment struct {
	Position    int
	MediaID     string
	Description string // "" when there is no alt text
}

// DeletionOrigin says who deleted a status.
type DeletionOrigin string

// Deletion origins (status_deletions.origin).
const (
	DeletedByAuthor          DeletionOrigin = "author"
	DeletedByModeration      DeletionOrigin = "moderation"
	DeletedByRemote          DeletionOrigin = "remote"
	DeletedByAccountDeletion DeletionOrigin = "account_deletion"
	// DeletedByAutoDelete is the deletion of an old local post by its author's automatic
	// deletion policy (FR-118). It federates as the author's own deletion does.
	DeletedByAutoDelete DeletionOrigin = "auto_delete"
	// DeletedByRetention is the purge of an old remote status (ADR-0006). Unlike the others, it
	// does not stop the same URI from being ingested again.
	DeletedByRetention DeletionOrigin = "retention"
)

// Tombstone reports whether d stops a later Create, Update, or fetch of its URI from bringing
// the status back (data-model §4.4): every deletion but the purge of an old remote status.
func (d Deletion) Tombstone() bool { return d.Origin != DeletedByRetention }

// Deletion is the deletion of a status. It keeps the URI after the status is purged, as the
// tombstone that stops a late Create or Update from bringing the status back.
type Deletion struct {
	StatusID           string
	URI                string
	AuthorAccountID    string
	Origin             DeletionOrigin
	ModerationActionID string // "" unless a moderation action deleted the status
	DeletedAt          time.Time
}

// Tag is a hashtag. NameNormalized is unique; Name is the spelling that first used it.
type Tag struct {
	ID             string
	Name           string
	NameNormalized string
	CreatedAt      time.Time
}
