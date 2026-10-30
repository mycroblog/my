// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"errors"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/timeline"
)

// ErrNotFound reports that no row matches a lookup.
var ErrNotFound = errors.New("timeline: not found")

// Entry is one row of a timeline (feed_entries): a status as posted (KindPost, ID = status ID)
// or a live boost of one (KindBoost, ID = boost ID).
type Entry struct {
	ID       string
	Kind     timeline.Kind
	StatusID string
	// AuthorAccountID is the poster of a post or the booster of a boost, and AuthorDomain that
	// account's domain ("" when local).
	AuthorAccountID string
	AuthorDomain    string
	// OriginalAuthorAccountID is the author of the status; for a post, AuthorAccountID.
	OriginalAuthorAccountID string
	OriginalAuthorDomain    string
	// Visibility is the status's stored visibility; a boost copies it.
	Visibility appcontent.Visibility
	// IsLocal is true when the poster or booster is a local account.
	IsLocal bool
	// Language and RevisionSeq are those of the status's current revision.
	Language    string
	RevisionSeq int
	// IsReply is true for a post that replies to a status; a boost never is one.
	IsReply bool
}

// DirectRecipient is an account that a direct post is addressed to (feed_direct_recipients).
type DirectRecipient struct {
	EntryID            string
	RecipientAccountID string
}

// TagEntry is a hashtag of a post's current revision (feed_tag_entries); RevisedAt is the time
// of that revision.
type TagEntry struct {
	TagID           string
	EntryID         string
	AuthorAccountID string
	Visibility      appcontent.Visibility
	IsLocal         bool
	RevisedAt       time.Time
}

// TagUsage is how many posts that CountsTagUsage carry a hashtag now, and the latest RevisedAt
// of their tag entries (tag_usage, ADR-0092 §1). A tag that no such post carries has none.
type TagUsage struct {
	TagID         string
	StatusesCount int64
	LastUsedAt    time.Time
}

// CountsTagUsage reports whether the tags of a post of visibility v count in tag_usage: those of
// public, local, and unlisted posts do, while a tag of private or direct posts only would
// reveal that such a post exists (ADR-0092 §1).
func CountsTagUsage(v appcontent.Visibility) bool {
	switch v {
	case appcontent.VisibilityPublic, appcontent.VisibilityLocal, appcontent.VisibilityUnlisted:
		return true
	}
	return false
}

// FollowEdge is an active follow (feed_follow_edges).
type FollowEdge struct {
	FollowerAccountID string
	FolloweeAccountID string
	ShowBoosts        bool
}

// Exclusion is an account whose entries a viewer does not see (feed_exclusions); ExpiresAt is
// nil for an exclusion that lasts until it is removed.
type Exclusion struct {
	ViewerAccountID string
	TargetAccountID string
	Reason          timeline.ExclusionReason
	ExpiresAt       *time.Time
}

// DomainMute is a domain, with its subdomains, whose posts a viewer does not see outside the
// home timeline (feed_domain_mutes).
type DomainMute struct {
	ViewerAccountID string
	Domain          string
}

// AuthorRestriction is a restriction of an account (feed_author_restrictions); EffectiveUntil
// is nil for one that lasts until it is lifted.
type AuthorRestriction struct {
	AccountID      string
	Restriction    timeline.Restriction
	EffectiveUntil *time.Time
}

// DomainRestriction is a restriction of a domain and its subdomains (feed_domain_restrictions).
type DomainRestriction struct {
	Domain         string
	Restriction    timeline.Restriction
	EffectiveUntil *time.Time
}

// Projection is the content of the timeline projections that the projectors derive from the
// content and graph events, each ordered by its key, for verifying them.
type Projection struct {
	Entries          []Entry
	DirectRecipients []DirectRecipient
	TagEntries       []TagEntry
	TagUsage         []TagUsage
	FollowEdges      []FollowEdge
	Exclusions       []Exclusion
	DomainMutes      []DomainMute
}

// Window selects a page of a timeline, newest first.
type Window struct {
	// ViewerAccountID is the logged-in viewer, "" for a visitor.
	ViewerAccountID string
	// Before is the ID of the last entry of the previous page; "" starts at the newest.
	Before string
	// Limit is the largest number of entries to return.
	Limit int
	// Languages keeps only the entries in these languages and in UndeterminedLanguage; nil or
	// empty keeps every language (FR-208).
	Languages []string
	// Now decides which mutes and restrictions are in force.
	Now time.Time
}

// HomeQuery selects a page of the home timeline of Window.ViewerAccountID.
type HomeQuery struct {
	Window
	// Floor bounds the read window: only entries with an ID at or above it; "" for no bound.
	Floor string
}

// LocalQuery selects a page of the local timeline.
type LocalQuery struct {
	Window
	// IncludeLocal also lists the posts of visibility local (for logged-in viewers).
	IncludeLocal bool
}

// PublicQuery selects a page of the federated timeline.
type PublicQuery struct {
	Window
}

// TagQuery selects a page of the timeline of a hashtag.
type TagQuery struct {
	Window
	TagID        string
	IncludeLocal bool
}

// ProfileQuery selects a page of the profile timeline of an author.
type ProfileQuery struct {
	Window
	AuthorAccountID string
	// Visibilities are the visibilities listed, except direct.
	Visibilities []appcontent.Visibility
	// IncludeDirect also lists the direct posts addressed to the viewer.
	IncludeDirect bool
	// ExcludeReplies leaves out the posts that reply to a status; ExcludeBoosts the boosts.
	ExcludeReplies bool
	ExcludeBoosts  bool
}

// FactsQuery asks for what holds between a viewer and the accounts, domains, and entries of a
// page, for re-applying CanView.
type FactsQuery struct {
	ViewerAccountID string
	AccountIDs      []string
	Domains         []string
	EntryIDs        []string
	Now             time.Time
}

// Facts answer a FactsQuery. Expired mutes and restrictions are left out.
type Facts struct {
	// Followed holds the accounts that the viewer actively follows.
	Followed map[string]bool
	// Exclusions holds the reasons the viewer excludes each account.
	Exclusions map[string][]timeline.ExclusionReason
	// AccountRestrictions and DomainRestrictions hold the restrictions in force.
	AccountRestrictions map[string][]timeline.Restriction
	DomainRestrictions  map[string][]timeline.Restriction
	// Direct holds the direct entries addressed to the viewer.
	Direct map[string]bool
}

// Repository is the persistence port of the timeline module (data-model §4.6). It reads and
// writes only the feed_* tables, tag_usage, and the tables of lists. The database adapters implement it and pass
// testkit.TimelineContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	// Projections, written only by this module's projectors and rebuild.

	// InsertEntry inserts e and reports false, writing nothing, when an entry with its ID exists.
	InsertEntry(ctx context.Context, tx uow.Tx, e Entry) (bool, error)
	// Entry returns an entry by ID.
	Entry(ctx context.Context, tx uow.Tx, id string) (Entry, error)
	// UpdateRevision gives the entries of a status (its post and its boosts) the language and
	// seq of a revision, unless they already have that seq or a later one.
	UpdateRevision(ctx context.Context, tx uow.Tx, statusID, language string, seq int) error
	// DeleteEntry deletes an entry with its direct recipients and tags, taking the tags out of
	// tag_usage.
	DeleteEntry(ctx context.Context, tx uow.Tx, id string) error
	// DeleteStatusEntries deletes the entries of a status (its post and its boosts) with their
	// direct recipients and tags, taking the tags out of tag_usage.
	DeleteStatusEntries(ctx context.Context, tx uow.Tx, statusID string) error
	// ReplaceDirectRecipients makes accountIDs the direct recipients of an entry.
	ReplaceDirectRecipients(ctx context.Context, tx uow.Tx, entryID string, accountIDs []string) error
	// ReplaceTags makes tagIDs the tags of entry e, whose current revision is from revisedAt,
	// and, when CountsTagUsage(e.Visibility), adds the new tags to tag_usage, takes the removed
	// ones out (deleting a row that drops to zero), and reads LastUsedAt of every tag of both
	// again from the tag entries.
	ReplaceTags(ctx context.Context, tx uow.Tx, e Entry, tagIDs []string, revisedAt time.Time) error
	// UpsertFollowEdge stores e, replacing the pair's edge.
	UpsertFollowEdge(ctx context.Context, tx uow.Tx, e FollowEdge) error
	DeleteFollowEdge(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) error
	// UpsertExclusion stores x, replacing the expiry of the same exclusion.
	UpsertExclusion(ctx context.Context, tx uow.Tx, x Exclusion) error
	DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason timeline.ExclusionReason) error
	// InsertDomainMute stores m; storing it again changes nothing.
	InsertDomainMute(ctx context.Context, tx uow.Tx, m DomainMute) error
	DeleteDomainMute(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error
	// UpsertAuthorRestriction and UpsertDomainRestriction store a restriction, replacing its end.
	UpsertAuthorRestriction(ctx context.Context, tx uow.Tx, r AuthorRestriction) error
	DeleteAuthorRestriction(ctx context.Context, tx uow.Tx, accountID string, r timeline.Restriction) error
	UpsertDomainRestriction(ctx context.Context, tx uow.Tx, r DomainRestriction) error
	DeleteDomainRestriction(ctx context.Context, tx uow.Tx, domain string, r timeline.Restriction) error
	// DeleteProjection empties the projections that the content and graph events derive (every
	// table of Projection), for a rebuild.
	DeleteProjection(ctx context.Context, tx uow.Tx) error
	// ListRestrictions returns every row of feed_author_restrictions and
	// feed_domain_restrictions, expired ones included, for a rebuild.
	ListRestrictions(ctx context.Context, tx uow.Tx) ([]AuthorRestriction, []DomainRestriction, error)
	// DeleteRestrictions empties feed_author_restrictions and feed_domain_restrictions, for a
	// rebuild.
	DeleteRestrictions(ctx context.Context, tx uow.Tx) error

	// Lists (ADR-0122), written by the owner's commands and the projector of
	// graph.follow_terminated.

	// InsertList stores a new list with its name.
	InsertList(ctx context.Context, tx uow.Tx, l List) error
	// RenameList replaces the name of the list id.
	RenameList(ctx context.Context, tx uow.Tx, id, name string) error
	// DeleteList deletes the list id with its name and members.
	DeleteList(ctx context.Context, tx uow.Tx, id string) error
	// FindList returns the list id inside tx.
	FindList(ctx context.Context, tx uow.Tx, id string) (List, error)
	// CountLists returns how many lists ownerAccountID has.
	CountLists(ctx context.Context, tx uow.Tx, ownerAccountID string) (int, error)
	// Following reports whether feed_follow_edges holds the active follow of followee by follower.
	Following(ctx context.Context, tx uow.Tx, followerAccountID, followeeAccountID string) (bool, error)
	// InsertListMember stores m and reports false, writing nothing, when the list has the member.
	InsertListMember(ctx context.Context, tx uow.Tx, m ListMember) (bool, error)
	// DeleteListMember takes an account out of a list.
	DeleteListMember(ctx context.Context, tx uow.Tx, listID, memberAccountID string) error
	// CountListMembers returns how many members the list listID has.
	CountListMembers(ctx context.Context, tx uow.Tx, listID string) (int, error)
	// DeleteListMembersOfFollow takes memberAccountID out of every list of ownerAccountID.
	DeleteListMembersOfFollow(ctx context.Context, tx uow.Tx, ownerAccountID, memberAccountID string) error
	// DeleteListMembersWithoutFollow takes out of every list each member that its owner does not
	// actively follow (feed_follow_edges), for a rebuild.
	DeleteListMembersWithoutFollow(ctx context.Context, tx uow.Tx) error
	// List returns the list id.
	List(ctx context.Context, id string) (List, error)
	// ListsOf returns the lists of ownerAccountID, ordered by ID.
	ListsOf(ctx context.Context, ownerAccountID string) ([]List, error)
	// ListMembers returns the account IDs of up to limit members of a list after the member after
	// ("" starts at the first), ordered by ID.
	ListMembers(ctx context.Context, listID, after string, limit int) ([]string, error)
	// ListIDsWithMember returns the IDs of the lists of ownerAccountID that have memberAccountID,
	// ordered by ID.
	ListIDsWithMember(ctx context.Context, ownerAccountID, memberAccountID string) ([]string, error)
	// AllListMembers returns every list member, ordered by list and member, for verifying them.
	AllListMembers(ctx context.Context) ([]ListMember, error)

	// Timelines (data-model §6), newest first, at most Limit entries each.

	ListHome(ctx context.Context, q HomeQuery) ([]Entry, error)
	ListLocal(ctx context.Context, q LocalQuery) ([]Entry, error)
	ListPublic(ctx context.Context, q PublicQuery) ([]Entry, error)
	ListTag(ctx context.Context, q TagQuery) ([]Entry, error)
	ListProfile(ctx context.Context, q ProfileQuery) ([]Entry, error)
	// ListListEntries is the timeline of a list (ADR-0122 §4).
	ListListEntries(ctx context.Context, q ListTimelineQuery) ([]Entry, error)
	// Facts returns what holds between a viewer and the accounts, domains, and entries of q.
	Facts(ctx context.Context, q FactsQuery) (Facts, error)
	// TagUsageOf returns the usage of the tags among tagIDs that have one, ordered by tag ID.
	TagUsageOf(ctx context.Context, tagIDs []string) ([]TagUsage, error)

	// Snapshot returns the content of the projections, for verifying them.
	Snapshot(ctx context.Context) (Projection, error)
}
