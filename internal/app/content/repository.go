// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

var (
	// ErrNotFound reports that no row matches a lookup.
	ErrNotFound = errors.New("content: not found")
	// ErrStatusExists reports a status whose URI another status already has.
	ErrStatusExists = errors.New("content: a status with this URI exists")
	// ErrIdempotencyKeyUsed reports a local status whose author already posted with the same
	// Idempotency-Key.
	ErrIdempotencyKeyUsed = errors.New("content: the idempotency key is used")
	// ErrAlreadyDeleted reports a second deletion of a status or of its URI.
	ErrAlreadyDeleted = errors.New("content: the status is already deleted")
)

// Repository is the persistence port of the content module's statuses (data-model §4.4). The
// database adapters implement it and pass testkit.StatusRepositoryContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	// Resources and events, written by commands.

	// InsertStatus inserts a status. A URI that another status has fails with ErrStatusExists.
	InsertStatus(ctx context.Context, tx uow.Tx, s content.Status) error
	// InsertRelayedStatus records a remote status that only a relay brought in (ADR-0061),
	// unless it is recorded.
	InsertRelayedStatus(ctx context.Context, tx uow.Tx, statusID, relayID string, receivedAt time.Time) error
	// InsertLocalStatus inserts the local subtype of a status. An Idempotency-Key that the
	// author already used fails with ErrIdempotencyKeyUsed.
	InsertLocalStatus(ctx context.Context, tx uow.Tx, ls content.LocalStatus) error
	// InsertRevision inserts a revision with its bodies, source, attachments, mentions, tags, and
	// emoji. On PostgreSQL, a Seq that a concurrent revision took fails with an error for which
	// errors.Is(err, uow.ErrRetryable) holds, so that a new attempt reads the new head.
	InsertRevision(ctx context.Context, tx uow.Tx, r content.Revision) error
	// InsertDeletion records the deletion of a status. A status or URI that is already deleted
	// fails with ErrAlreadyDeleted.
	InsertDeletion(ctx context.Context, tx uow.Tx, d content.Deletion) error
	// EnsureTag inserts t unless a tag with its NameNormalized exists, and returns the stored tag.
	EnsureTag(ctx context.Context, tx uow.Tx, t content.Tag) (content.Tag, error)

	// Projections, written only by this module's projectors and rebuild.

	// Head returns the head of a status.
	Head(ctx context.Context, tx uow.Tx, statusID string) (content.Head, error)
	UpsertHead(ctx context.Context, tx uow.Tx, h content.Head) error
	// ReplaceMentions makes accountIDs the mentions of a status in status_mention_index.
	ReplaceMentions(ctx context.Context, tx uow.Tx, statusID string, accountIDs []string) error
	// LockStatusKeys takes a lock on each of keys, in order, that the transaction holds until it
	// ends, so that the projections of the statuses that share a key are written one transaction
	// at a time under READ COMMITTED. The keys are the strings of statusLockKeys. On SQLite,
	// whose write transactions already run one at a time, it does nothing.
	LockStatusKeys(ctx context.Context, tx uow.Tx, keys []string) error
	// IsDeleted reports whether a status has a deletion.
	IsDeleted(ctx context.Context, tx uow.Tx, statusID string) (bool, error)
	// FindLiveStatusByURI returns the status with uri unless it is deleted.
	FindLiveStatusByURI(ctx context.Context, tx uow.Tx, uri string) (content.Status, error)
	// ListLiveRepliesTo returns the statuses that reply to uri and are not deleted, ordered by ID.
	ListLiveRepliesTo(ctx context.Context, tx uow.Tx, uri string) ([]content.Status, error)
	UpsertReplyLink(ctx context.Context, tx uow.Tx, l content.ReplyLink) error
	// ReplyLink returns the link from a reply to its parent.
	ReplyLink(ctx context.Context, tx uow.Tx, statusID string) (content.ReplyLink, error)
	// InsertCounters inserts the counters of a status unless it has them.
	InsertCounters(ctx context.Context, tx uow.Tx, c content.Counters) error
	// AddReplies adds delta to the replies count of a status that has counters.
	AddReplies(ctx context.Context, tx uow.Tx, statusID string, delta int64) error
	// AddPost counts one more status of an account, published at publishedAt.
	AddPost(ctx context.Context, tx uow.Tx, accountID string, publishedAt time.Time) error
	// RemovePost counts one status of an account less. It runs after the deletion is inserted
	// and takes LastStatusAt again from the account's statuses that are not deleted; the
	// counter goes away with the last of them.
	RemovePost(ctx context.Context, tx uow.Tx, accountID string) error
	// AddLocalPosts adds delta to the local posts of an account (local_post_counters); the
	// counter goes away when it reaches zero.
	AddLocalPosts(ctx context.Context, tx uow.Tx, accountID string, delta int64) error
	// DeleteStatusProjections removes a status from every status projection: its head, its
	// mentions, its counters, its link to its parent, and the links of its replies to it.
	DeleteStatusProjections(ctx context.Context, tx uow.Tx, statusID string) error
	// InsertSensitivityOverride records that a mark_statuses_sensitive action shows a status as
	// sensitive (status_sensitivity_overrides); recording it again does nothing.
	InsertSensitivityOverride(ctx context.Context, tx uow.Tx, statusID, actionID string) error
	// DeleteSensitivityOverridesOf deletes the overrides of a lifted action.
	DeleteSensitivityOverridesOf(ctx context.Context, tx uow.Tx, actionID string) error
	// DeleteAllSensitivityOverrides empties status_sensitivity_overrides, for its rebuild.
	DeleteAllSensitivityOverrides(ctx context.Context, tx uow.Tx) error

	// Rebuilds empty one projection and derive it again from the statuses, their revisions,
	// and their deletions, by the same rules as the projectors (data-model §9).

	RebuildHeads(ctx context.Context, tx uow.Tx) error
	RebuildMentions(ctx context.Context, tx uow.Tx) error
	RebuildReplyLinks(ctx context.Context, tx uow.Tx) error
	// RebuildCounters also counts the boosts that are not retracted and the favourites of
	// each status, by the rules of RegisterInteractionProjectors.
	RebuildCounters(ctx context.Context, tx uow.Tx) error
	RebuildPostCounters(ctx context.Context, tx uow.Tx) error
	// RebuildLocalPostCounters also counts the local boosts that are not retracted, of
	// statuses that are not deleted, by the rules of RegisterInteractionProjectors.
	RebuildLocalPostCounters(ctx context.Context, tx uow.Tx) error
	// RebuildPollTallies derives poll_tallies from the newest observation of each poll.
	RebuildPollTallies(ctx context.Context, tx uow.Tx) error

	// Projection contents, for verifying the projections: heads, reply links, counters, and post
	// counters ordered by their key; mentions ordered by account ID and status ID.

	ListHeads(ctx context.Context) ([]content.Head, error)
	ListMentions(ctx context.Context) ([]content.Mention, error)
	ListReplyLinks(ctx context.Context) ([]content.ReplyLink, error)
	ListCounters(ctx context.Context) ([]content.Counters, error)
	ListPostCounters(ctx context.Context) ([]content.PostCounter, error)
	ListLocalPostCounters(ctx context.Context) ([]content.LocalPostCounter, error)

	// Sources of other modules' projections (ReplayEvents).

	// ListCurrentPosts returns every status that is not deleted with its current revision and
	// the revision's children, ordered by status ID.
	ListCurrentPosts(ctx context.Context, tx uow.Tx) ([]Post, error)

	// Lookups.

	FindStatus(ctx context.Context, id string) (content.Status, error)
	FindStatusByURI(ctx context.Context, uri string) (content.Status, error)
	FindLocalStatus(ctx context.Context, statusID string) (content.LocalStatus, error)
	// FindLocalStatusByIdempotencyKey returns the local status that an author posted with key.
	FindLocalStatusByIdempotencyKey(ctx context.Context, authorAccountID, key string) (content.LocalStatus, error)
	// FindRevision returns a revision with its bodies, source, attachments, mentions, tags, and emoji.
	FindRevision(ctx context.Context, id string) (content.Revision, error)
	// FindCurrentRevision returns the revision that the head of a status points at; a deleted
	// status has none.
	FindCurrentRevision(ctx context.Context, statusID string) (content.Revision, error)
	// ListRevisions returns the revisions of a status, oldest first, without their children
	// (the edit history, FR-109).
	ListRevisions(ctx context.Context, statusID string) ([]content.Revision, error)
	FindDeletion(ctx context.Context, statusID string) (content.Deletion, error)
	// FindDeletionByURI returns the tombstone of a URI.
	FindDeletionByURI(ctx context.Context, uri string) (content.Deletion, error)
	FindTagByName(ctx context.Context, nameNormalized string) (content.Tag, error)
	FindTag(ctx context.Context, id string) (content.Tag, error)
	// ListTagsByPrefix returns up to limit tags whose NameNormalized is at least low and less
	// than high, compared byte by byte, in that order (the prefix search of hashtags, ADR-0092 §1).
	ListTagsByPrefix(ctx context.Context, low, high string, limit int) ([]content.Tag, error)
	// ListReplies returns the statuses that reply to uri and are not deleted, ordered by ID.
	ListReplies(ctx context.Context, uri string) ([]content.Status, error)
	// ListLiveRemoteStatusIDs returns up to limit remote statuses of an author that are not
	// deleted, newest first, by ID, whose ID is below before ("" for the first page).
	ListLiveRemoteStatusIDs(ctx context.Context, authorAccountID, before string, limit int) ([]string, error)
	FindCounters(ctx context.Context, statusID string) (content.Counters, error)
	// SensitivityOverridden reports whether an action shows a status as sensitive.
	SensitivityOverridden(ctx context.Context, statusID string) (bool, error)
	// FindPostCounter returns the post counter of an account; ErrNotFound means it has no
	// statuses.
	FindPostCounter(ctx context.Context, accountID string) (content.PostCounter, error)
	// LocalPostsTotal returns the sum of the local post counters.
	LocalPostsTotal(ctx context.Context) (int64, error)

	// Remote statuses: custom emoji, poll tallies, and the scope of ingestion (ADR-0034).
	// InsertRevision also writes a revision's Poll and Remote, and FindRevision reads them.

	// LocalStatusInConversation reports whether a local status that is not deleted belongs to
	// the conversation conversationURI.
	LocalStatusInConversation(ctx context.Context, conversationURI string) (bool, error)
	// EnsureCustomEmoji inserts e unless an emoji with its Shortcode and Domain exists, and
	// returns the stored emoji. A URI that another emoji has is not stored again.
	EnsureCustomEmoji(ctx context.Context, tx uow.Tx, e content.CustomEmoji) (content.CustomEmoji, error)
	// CurrentCustomEmojiImageURL returns the remote URL of the newest image of an emoji, "" when
	// that image is not a remote media file; ErrNotFound means the emoji has no image.
	CurrentCustomEmojiImageURL(ctx context.Context, tx uow.Tx, emojiID string) (string, error)
	InsertCustomEmojiImage(ctx context.Context, tx uow.Tx, img content.CustomEmojiImage) error
	// LatestPollObservation returns the newest tally observation of a poll, with its votes.
	LatestPollObservation(ctx context.Context, tx uow.Tx, pollID string) (content.PollTallyObservation, error)
	InsertPollObservation(ctx context.Context, tx uow.Tx, o content.PollTallyObservation) error
	// ReplacePollTallies makes tallies the poll_tallies of a poll; only projectors call it.
	ReplacePollTallies(ctx context.Context, tx uow.Tx, pollID string, tallies []content.PollTally) error

	FindCustomEmoji(ctx context.Context, id string) (content.CustomEmoji, error)
	// FindCustomEmojiByShortcode returns the emoji with a shortcode on a domain ("" for this
	// instance).
	FindCustomEmojiByShortcode(ctx context.Context, shortcode, domain string) (content.CustomEmoji, error)
	// FindCustomEmojiImage returns the current image of an emoji.
	FindCustomEmojiImage(ctx context.Context, emojiID string) (content.CustomEmojiImage, error)
	// FindPollTallies returns the tallies of a poll ordered by position; a poll without an
	// observation has none.
	FindPollTallies(ctx context.Context, pollID string) ([]content.PollTally, error)

	// Votes in polls (FR-112).

	// InsertPollVote inserts v and reports true, or reports false when the account voted for
	// its choice in the poll, or a vote has its URI, already.
	InsertPollVote(ctx context.Context, tx uow.Tx, v content.PollVote) (bool, error)
	// ListPollVotes returns the votes of accountID in pollID ordered by position.
	ListPollVotes(ctx context.Context, pollID, accountID string) ([]content.PollVote, error)
	// ListPollVotesTx is ListPollVotes in tx.
	ListPollVotesTx(ctx context.Context, tx uow.Tx, pollID, accountID string) ([]content.PollVote, error)
	// PollStatusID returns the ID of the status whose revision has the poll.
	PollStatusID(ctx context.Context, pollID string) (string, error)
	// LockPoll locks the poll, local or remote, until tx ends, so that the votes in it are
	// checked and counted one transaction at a time (a no-op where write transactions are
	// serialized already).
	LockPoll(ctx context.Context, tx uow.Tx, pollID string) error
	// CountPollVotes returns, in tx, the votes of each choice of a poll by position (choices
	// without votes are left out) and the number of accounts that voted.
	CountPollVotes(ctx context.Context, tx uow.Tx, pollID string) (map[int]int64, int64, error)
	// ListPollVoterIDs returns at most limit accounts that voted in a poll, ordered by ID, after
	// the ID after ("" for the first page).
	ListPollVoterIDs(ctx context.Context, pollID, after string, limit int) ([]string, error)
}
