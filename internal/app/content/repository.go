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
// database adapters implement it and pass contenttest.StatusRepositoryContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	// Resources and events, written by commands.

	// InsertStatus inserts a status. A URI that another status has fails with ErrStatusExists.
	InsertStatus(ctx context.Context, tx uow.Tx, s content.Status) error
	// InsertRelayedStatus records a remote status that only a relay brought in (ADR-0006),
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
	// InsertDeletions is InsertDeletion of ds, which share their origin, moderation action, and
	// time, with one statement.
	InsertDeletions(ctx context.Context, tx uow.Tx, ds []content.Deletion) error
	// EnsureTag inserts t unless a tag with its NameNormalized exists, and returns the stored tag.
	EnsureTag(ctx context.Context, tx uow.Tx, t content.Tag) (content.Tag, error)
	// EnsureTags is EnsureTag of each of tags, which share their CreatedAt, with one insert and one
	// read; it returns the stored tags by NameNormalized.
	EnsureTags(ctx context.Context, tx uow.Tx, tags []content.Tag) (map[string]content.Tag, error)

	// Conversations (ADR-0154): the resource, the conversation of each status (a current value
	// written with the status), and the merges.

	// EnsureConversation inserts c unless a conversation has its URI, and returns the stored one.
	EnsureConversation(ctx context.Context, tx uow.Tx, c content.Conversation) (content.Conversation, error)
	// FindConversationTx returns a conversation by ID inside tx.
	FindConversationTx(ctx context.Context, tx uow.Tx, id string) (content.Conversation, error)
	// ConversationMergeTargetTx returns, inside tx, the conversation that a merged conversation
	// was merged into; ErrNotFound means that it is not merged.
	ConversationMergeTargetTx(ctx context.Context, tx uow.Tx, id string) (string, error)
	// StatusConversationsTx is StatusConversations inside tx.
	StatusConversationsTx(ctx context.Context, tx uow.Tx, statusIDs []string) (map[string]content.Conversation, error)
	// InsertStatusConversation records the conversation that a status belongs to.
	InsertStatusConversation(ctx context.Context, tx uow.Tx, statusID, conversationID string) error
	// MergeConversation records m and moves every status of the merged conversation to the one
	// it is merged into. A conversation that is merged already fails.
	MergeConversation(ctx context.Context, tx uow.Tx, m content.ConversationMerge) error
	FindConversation(ctx context.Context, id string) (content.Conversation, error)
	FindConversationByURI(ctx context.Context, uri string) (content.Conversation, error)
	// FindConversationMerge returns the merge of a merged conversation.
	FindConversationMerge(ctx context.Context, id string) (content.ConversationMerge, error)
	// StatusConversations returns the conversation that each of statusIDs belongs to, by status,
	// with one query; a status without one is left out.
	StatusConversations(ctx context.Context, statusIDs []string) (map[string]content.Conversation, error)
	// ListLiveConversationStatuses returns up to limit statuses of a conversation that are neither
	// deleted nor held (held_replies), in ID order after the ID after ("" for the first page).
	ListLiveConversationStatuses(ctx context.Context, conversationID, after string, limit int) ([]content.Status, error)

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
	// ListLiveRepliesTo returns the statuses that reply to uri and are neither deleted nor held
	// (held_replies), ordered by ID.
	ListLiveRepliesTo(ctx context.Context, tx uow.Tx, uri string) ([]content.Status, error)
	// InsertHeldReply records that a reply is held out of the thread of its parent (ADR-0155 §6);
	// recording it again does nothing. IsHeldReply reports whether a status is held.
	InsertHeldReply(ctx context.Context, tx uow.Tx, h content.HeldReply) error
	IsHeldReply(ctx context.Context, tx uow.Tx, statusID string) (bool, error)
	UpsertReplyLink(ctx context.Context, tx uow.Tx, l content.ReplyLink) error
	// ReplyLink returns the link from a reply to its parent.
	ReplyLink(ctx context.Context, tx uow.Tx, statusID string) (content.ReplyLink, error)
	// ReplyLinksOf is ReplyLink of each of statusIDs that has one, by status.
	ReplyLinksOf(ctx context.Context, tx uow.Tx, statusIDs []string) (map[string]content.ReplyLink, error)
	// InsertCounters inserts the counters of a status unless it has them.
	InsertCounters(ctx context.Context, tx uow.Tx, c content.Counters) error
	// AddReplies adds delta to the replies count of a status that has counters.
	AddReplies(ctx context.Context, tx uow.Tx, statusID string, delta int64) error
	// AddManyReplies is AddReplies of each status of deltas, with one statement.
	AddManyReplies(ctx context.Context, tx uow.Tx, deltas map[string]int64) error
	// AddPost counts one more status of an account, published at publishedAt.
	AddPost(ctx context.Context, tx uow.Tx, accountID string, publishedAt time.Time) error
	// RemovePost counts one status of an account less. It runs after the deletion is inserted
	// and takes LastStatusAt again from the account's statuses that are not deleted; the
	// counter goes away with the last of them.
	RemovePost(ctx context.Context, tx uow.Tx, accountID string) error
	// RemovePosts is RemovePost n times.
	RemovePosts(ctx context.Context, tx uow.Tx, accountID string, n int64) error
	// SubtractLocalPostsOf is AddLocalPosts of -removed[id] for each account id.
	SubtractLocalPostsOf(ctx context.Context, tx uow.Tx, removed map[string]int64) error
	// AddLocalPosts adds delta to the local posts of an account (local_post_counters); the
	// counter goes away when it reaches zero.
	AddLocalPosts(ctx context.Context, tx uow.Tx, accountID string, delta int64) error
	// DeleteStatusProjections removes a status from every status projection: its head, its
	// mentions, its counters, its link to its parent, and the links of its replies to it.
	DeleteStatusProjections(ctx context.Context, tx uow.Tx, statusID string) error
	// DeleteStatusProjectionsOf is DeleteStatusProjections of each of statusIDs, with one
	// statement per projection.
	DeleteStatusProjectionsOf(ctx context.Context, tx uow.Tx, statusIDs []string) error
	// InsertSensitivityOverride records that a mark_statuses_sensitive action shows a status as
	// sensitive (status_sensitivity_overrides); recording it again does nothing. A status can have
	// rows of several sources, and each source ends only its own.
	InsertSensitivityOverride(ctx context.Context, tx uow.Tx, statusID, actionID string) error
	// InsertSensitivityOverrides is InsertSensitivityOverride of each of statusIDs.
	InsertSensitivityOverrides(ctx context.Context, tx uow.Tx, statusIDs []string, actionID string) error
	// DeleteSensitivityOverridesOf deletes the overrides of a lifted action.
	DeleteSensitivityOverridesOf(ctx context.Context, tx uow.Tx, actionID string) error
	// InsertSensitivityOverrideOfHookDecision records that a mark_sensitive decision of a hook shows a
	// status as sensitive; recording it again does nothing.
	InsertSensitivityOverrideOfHookDecision(ctx context.Context, tx uow.Tx, statusID, decisionID string) error
	// DeleteSensitivityOverridesOfHookDecision deletes the overrides of a cancelled decision.
	DeleteSensitivityOverridesOfHookDecision(ctx context.Context, tx uow.Tx, decisionID string) error
	// DeleteAllSensitivityOverrides empties status_sensitivity_overrides, for its rebuild.
	DeleteAllSensitivityOverrides(ctx context.Context, tx uow.Tx) error
	// DeleteSensitivityOverridesWithoutStatus drops the overrides of statuses with no row, for its rebuild.
	DeleteSensitivityOverridesWithoutStatus(ctx context.Context, tx uow.Tx) error

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
	// FindStatuses returns the statuses of ids that exist, by ID, as FindStatus returns one, with
	// one query whatever the number of ids.
	FindStatuses(ctx context.Context, ids []string) (map[string]content.Status, error)
	// FindStatusesByURIs returns the statuses of uris that exist, by URI, as FindStatusByURI
	// returns one, with one query whatever the number of URIs.
	FindStatusesByURIs(ctx context.Context, uris []string) (map[string]content.Status, error)
	// FindCurrentRevisions returns the current revisions of the statuses of ids that have one,
	// by status ID, as FindCurrentRevision returns one, with one query per part of a revision
	// whatever the number of ids.
	FindCurrentRevisions(ctx context.Context, statusIDs []string) (map[string]content.Revision, error)
	// ListSearchableStatusIDs returns up to limit IDs of the public and local statuses of the server
	// in ID order after the ID after ("" for the first page), deleted ones included.
	ListSearchableStatusIDs(ctx context.Context, after string, limit int) ([]string, error)
	// ListAuthorSearchableStatusIDs returns up to limit IDs of the public and local statuses of an
	// author in ID order, after the ID after ("" for the first page), deleted ones included.
	ListAuthorSearchableStatusIDs(ctx context.Context, authorAccountID, after string, limit int) ([]string, error)
	FindStatusByURI(ctx context.Context, uri string) (content.Status, error)
	// FindStatusByURITx is FindStatusByURI inside tx.
	FindStatusByURITx(ctx context.Context, tx uow.Tx, uri string) (content.Status, error)
	FindLocalStatus(ctx context.Context, statusID string) (content.LocalStatus, error)
	// FindLocalStatusByIdempotencyKey returns the local status that an author posted with key.
	FindLocalStatusByIdempotencyKey(ctx context.Context, authorAccountID, key string) (content.LocalStatus, error)
	// FindRevision returns a revision with its bodies, source, attachments, mentions, tags, and emoji.
	FindRevision(ctx context.Context, id string) (content.Revision, error)
	// FindRevisionTx is FindRevision inside tx.
	FindRevisionTx(ctx context.Context, tx uow.Tx, id string) (content.Revision, error)
	// FindCurrentRevision returns the revision that the head of a status points at; a deleted
	// status has none.
	FindCurrentRevision(ctx context.Context, statusID string) (content.Revision, error)
	// ListRevisions returns the revisions of a status, oldest first, without their children
	// (the edit history, FR-109).
	ListRevisions(ctx context.Context, statusID string) ([]content.Revision, error)
	FindDeletion(ctx context.Context, statusID string) (content.Deletion, error)
	// FindDeletions returns the deletions of the statuses of ids that are deleted, by status ID,
	// as FindDeletion returns one, with one query whatever the number of ids.
	FindDeletions(ctx context.Context, statusIDs []string) (map[string]content.Deletion, error)
	// FindDeletionByURI returns the tombstone of a URI.
	FindDeletionByURI(ctx context.Context, uri string) (content.Deletion, error)
	// FindDeletionByURITx is FindDeletionByURI inside tx.
	FindDeletionByURITx(ctx context.Context, tx uow.Tx, uri string) (content.Deletion, error)
	FindTagByName(ctx context.Context, nameNormalized string) (content.Tag, error)
	FindTag(ctx context.Context, id string) (content.Tag, error)
	// FindTags returns the tags of ids that exist, by ID, as FindTag returns one, with one query.
	FindTags(ctx context.Context, ids []string) (map[string]content.Tag, error)
	// ListTagsByPrefix returns up to limit tags whose NameNormalized is at least low and less
	// than high, compared byte by byte, in that order (the prefix search of hashtags, ADR-0092 §1).
	ListTagsByPrefix(ctx context.Context, low, high string, limit int) ([]content.Tag, error)
	// ListReplies returns the statuses that reply to uri and are neither deleted nor held, ordered
	// by ID.
	ListReplies(ctx context.Context, uri string) ([]content.Status, error)
	// ListLiveRemoteStatusIDs returns up to limit remote statuses of an author that are not
	// deleted, newest first, by ID, whose ID is below before ("" for the first page).
	ListLiveRemoteStatusIDs(ctx context.Context, authorAccountID, before string, limit int) ([]string, error)
	FindCounters(ctx context.Context, statusID string) (content.Counters, error)
	// FindCountersOf returns the counters of the statuses of ids that have them, by status ID, as
	// FindCounters returns one, with one query whatever the number of ids.
	FindCountersOf(ctx context.Context, statusIDs []string) (map[string]content.Counters, error)
	// SensitivityOverridden reports whether an action shows a status as sensitive.
	SensitivityOverridden(ctx context.Context, statusID string) (bool, error)
	// SensitivitiesOverridden returns the statuses among statusIDs that moderation marked
	// sensitive, as SensitivityOverridden tells of one, with one query.
	SensitivitiesOverridden(ctx context.Context, statusIDs []string) (map[string]bool, error)
	// FindPostCounter returns the post counter of an account; ErrNotFound means it has no
	// statuses.
	FindPostCounter(ctx context.Context, accountID string) (content.PostCounter, error)
	// LocalPostsTotal returns the sum of the local post counters.
	LocalPostsTotal(ctx context.Context) (int64, error)
	// DailyLocalPublicPosts counts the public local statuses that are not deleted in each of the
	// days periods of 24 hours before to, oldest first, by their publication time.
	DailyLocalPublicPosts(ctx context.Context, to time.Time, days int) ([]int, error)
	// ListRecentLocalPosters returns up to limit local accounts with a public or local status
	// that is not deleted, published at since or later, the one that posted last first.
	ListRecentLocalPosters(ctx context.Context, since time.Time, limit int) ([]string, error)

	// Remote statuses: custom emoji, poll tallies, and the scope of ingestion (ADR-0006).
	// InsertRevision also writes a revision's Poll and Remote, and FindRevision reads them.

	// LocalStatusInConversationOf reports whether a local status that is not deleted belongs to
	// the conversation of the status statusID.
	LocalStatusInConversationOf(ctx context.Context, statusID string) (bool, error)
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
	// FindCustomEmojis returns the custom emoji of ids that exist, by ID, as FindCustomEmoji returns
	// one, with one query.
	FindCustomEmojis(ctx context.Context, ids []string) (map[string]content.CustomEmoji, error)
	// FindCustomEmojiByShortcode returns the emoji with a shortcode on a domain ("" for this
	// instance).
	FindCustomEmojiByShortcode(ctx context.Context, shortcode, domain string) (content.CustomEmoji, error)
	// FindCustomEmojiByShortcodeTx is FindCustomEmojiByShortcode inside tx.
	FindCustomEmojiByShortcodeTx(ctx context.Context, tx uow.Tx, shortcode, domain string) (content.CustomEmoji, error)
	// FindCustomEmojiImage returns the current image of an emoji.
	FindCustomEmojiImage(ctx context.Context, emojiID string) (content.CustomEmojiImage, error)
	// FindPollTallies returns the tallies of a poll ordered by position; a poll without an
	// observation has none.
	FindPollTallies(ctx context.Context, pollID string) ([]content.PollTally, error)
	// FindPollTalliesOf returns the tallies of each of pollIDs that has some, by poll ID, as
	// FindPollTallies returns those of one, with one query.
	FindPollTalliesOf(ctx context.Context, pollIDs []string) (map[string][]content.PollTally, error)

	// Votes in polls (FR-112).

	// InsertPollVote inserts v and reports true, or reports false when the account voted for
	// its choice in the poll, or a vote has its URI, already.
	InsertPollVote(ctx context.Context, tx uow.Tx, v content.PollVote) (bool, error)
	// ListPollVotes returns the votes of accountID in pollID ordered by position.
	ListPollVotes(ctx context.Context, pollID, accountID string) ([]content.PollVote, error)
	// ListPollVotesIn returns accountID's votes in each of pollIDs where it voted, by poll ID, as
	// ListPollVotes returns those of one, with one query.
	ListPollVotesIn(ctx context.Context, pollIDs []string, accountID string) (map[string][]content.PollVote, error)
	// ListPollVotesOfAccountsIn is ListPollVotesIn of each of accountIDs, by account, with one
	// query.
	ListPollVotesOfAccountsIn(ctx context.Context, pollIDs, accountIDs []string) (map[string]map[string][]content.PollVote, error)
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
