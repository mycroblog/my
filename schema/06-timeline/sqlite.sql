-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the timeline module
-- (docs/architecture/data-model/04-tables/06-timeline.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- timeline: feeds (data-model §4.6, §6, ADR-0013). Owned by internal/app/timeline, whose
-- synchronous projectors derive every table from the events of content and graph (and of
-- moderation for the restrictions); the timeline queries read only these tables.
-- ---------------------------------------------------------------------------------------------

-- Projection: one entry per status (kind post, id = status ID) and per live boost (kind boost,
-- id = boost ID) of a status that is not deleted. author_* is the poster or the booster,
-- original_author_* the author of the status; a domain is '' for a local account.
CREATE TABLE feed_entries (
  id TEXT NOT NULL PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('post', 'boost')),
  status_id TEXT NOT NULL,
  author_account_id TEXT NOT NULL,
  author_domain TEXT NOT NULL,
  original_author_account_id TEXT NOT NULL,
  original_author_domain TEXT NOT NULL,
  visibility TEXT NOT NULL CHECK (visibility IN ('public', 'local', 'unlisted', 'private', 'direct')),
  is_local INTEGER NOT NULL CHECK (is_local IN (0, 1)),
  language TEXT NOT NULL,
  revision_seq INTEGER NOT NULL CHECK (revision_seq >= 1),
  is_reply INTEGER NOT NULL CHECK (is_reply IN (0, 1))
) STRICT;
CREATE INDEX feed_entries_author_account_id_id ON feed_entries (author_account_id, id);
CREATE INDEX feed_entries_status_id ON feed_entries (status_id);
CREATE INDEX feed_entries_local ON feed_entries (id) WHERE is_local = 1 AND kind = 'post' AND visibility IN ('public', 'local');
CREATE INDEX feed_entries_public ON feed_entries (id) WHERE kind = 'post' AND visibility = 'public';

-- Projection: the recipients (current mentions) of a direct post.
CREATE TABLE feed_direct_recipients (
  entry_id TEXT NOT NULL,
  recipient_account_id TEXT NOT NULL,
  PRIMARY KEY (recipient_account_id, entry_id)
) STRICT;
CREATE INDEX feed_direct_recipients_entry_id ON feed_direct_recipients (entry_id);

-- Projection: the current hashtags of a post, with its immutable author and visibility and the
-- time of its current revision.
CREATE TABLE feed_tag_entries (
  tag_id TEXT NOT NULL,
  entry_id TEXT NOT NULL,
  author_account_id TEXT NOT NULL,
  visibility TEXT NOT NULL CHECK (visibility IN ('public', 'local', 'unlisted', 'private', 'direct')),
  is_local INTEGER NOT NULL CHECK (is_local IN (0, 1)),
  revised_at INTEGER NOT NULL,
  PRIMARY KEY (tag_id, entry_id)
) STRICT;
CREATE INDEX feed_tag_entries_entry_id ON feed_tag_entries (entry_id);
-- tag_usage.last_used_at is the latest revised_at of the posts that tag_usage counts.
CREATE INDEX feed_tag_entries_usage ON feed_tag_entries (tag_id, revised_at) WHERE visibility IN ('public', 'local', 'unlisted');
-- The counts of an account's featured hashtags (featured_tags) read its public and unlisted posts
-- of each tag.
CREATE INDEX feed_tag_entries_author ON feed_tag_entries (author_account_id, tag_id, revised_at) WHERE visibility IN ('public', 'unlisted');

-- Projection: how many posts of visibility public, local, or unlisted carry each hashtag now,
-- and the latest time one of them was revised with it, for the prefix search of hashtags
-- (ADR-0092 §1, FR-119, FR-306). The same projector updates it with feed_tag_entries; a tag that
-- no such post carries has no row, so that a tag of private and direct posts only never shows.
CREATE TABLE tag_usage (
  tag_id TEXT NOT NULL PRIMARY KEY,
  statuses_count INTEGER NOT NULL CHECK (statuses_count >= 1),
  last_used_at INTEGER NOT NULL
) STRICT;

-- Projection: the active follows, with the follower's show_boosts preference.
CREATE TABLE feed_follow_edges (
  follower_account_id TEXT NOT NULL,
  followee_account_id TEXT NOT NULL,
  show_boosts INTEGER NOT NULL CHECK (show_boosts IN (0, 1)),
  PRIMARY KEY (follower_account_id, followee_account_id)
) STRICT;

-- Projection: the accounts whose entries a viewer does not see: those the viewer blocks, those
-- that block the viewer, and those the viewer mutes (until expires_at; NULL never expires).
CREATE TABLE feed_exclusions (
  viewer_account_id TEXT NOT NULL,
  target_account_id TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (reason IN ('block', 'blocked_by', 'mute')),
  expires_at INTEGER,
  PRIMARY KEY (viewer_account_id, target_account_id, reason)
) STRICT;

-- Projection: the domains (and their subdomains) whose posts a viewer does not see on the
-- timelines other than home, nor boosted on home (FR-304).
CREATE TABLE feed_domain_mutes (
  viewer_account_id TEXT NOT NULL,
  domain TEXT NOT NULL,
  PRIMARY KEY (viewer_account_id, domain)
) STRICT;

-- Projection: the moderation restrictions of accounts that bear on timelines, until
-- effective_until (NULL: until lifted).
CREATE TABLE feed_author_restrictions (
  account_id TEXT NOT NULL,
  restriction TEXT NOT NULL CHECK (restriction IN ('profile_view', 'visibility_ceiling_private', 'public_discovery')),
  effective_until INTEGER,
  PRIMARY KEY (account_id, restriction)
) STRICT;

-- Projection: the moderation restrictions of domains (and their subdomains) that bear on
-- timelines.
CREATE TABLE feed_domain_restrictions (
  domain TEXT NOT NULL,
  restriction TEXT NOT NULL CHECK (restriction IN ('profile_view', 'public_discovery')),
  effective_until INTEGER,
  PRIMARY KEY (domain, restriction)
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- timeline: lists (data-model §4.6, ADR-0013). Owned by internal/app/timeline: the owner's
-- commands write them, the projector of graph.follow_terminated takes the ended follows out of
-- list_members, and only the owner reads them.
-- ---------------------------------------------------------------------------------------------

-- Resource: a list of the accounts that its owner follows, which only the owner sees.
CREATE TABLE lists (
  id TEXT NOT NULL PRIMARY KEY,
  owner_account_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX lists_owner_account_id_id ON lists (owner_account_id, id);

-- Current value: the name of a list (1 to 50 graphemes); renaming it keeps no history.
CREATE TABLE list_names (
  list_id TEXT NOT NULL PRIMARY KEY,
  name TEXT NOT NULL
) STRICT;

-- Current value: the members of a list, each an account that the owner actively follows
-- (feed_follow_edges). owner_account_id copies the list's immutable owner, so that the end of a
-- follow removes its followee from every list of the follower in one statement.
CREATE TABLE list_members (
  list_id TEXT NOT NULL,
  member_account_id TEXT NOT NULL,
  owner_account_id TEXT NOT NULL,
  added_at INTEGER NOT NULL,
  PRIMARY KEY (list_id, member_account_id)
) STRICT;
CREATE INDEX list_members_owner_member ON list_members (owner_account_id, member_account_id);
CREATE INDEX list_members_member_account_id ON list_members (member_account_id);

-- ---------------------------------------------------------------------------------------------
-- timeline: followed and featured hashtags (data-model §4.6, ADR-0013 §27, ADR-0006 §17). Owned
-- by internal/app/timeline: the account's commands write them, the home timeline reads the posts
-- of the followed tags from feed_tag_entries (data-model §6.1), and the profile counts the posts
-- of the featured tags there.
-- ---------------------------------------------------------------------------------------------

-- Current value: a hashtag (a tag of content) that an account follows, at most 100 per account;
-- unfollowing it deletes the row. A rebuild of the projections keeps them.
CREATE TABLE feed_tag_follows (
  account_id TEXT NOT NULL,
  tag_id TEXT NOT NULL,
  followed_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, tag_id)
) STRICT;
CREATE INDEX feed_tag_follows_tag_id ON feed_tag_follows (tag_id);

-- Current value: a hashtag (a tag of content) that an account features on its profile, at most
-- 10 per account in the order of ordinal (ADR-0006 §17). A local account's own command writes
-- them; those of a remote account are the copy of its featuredTags collection, which the fetch
-- replaces. The counts and the last use come from feed_tag_entries. A rebuild of the
-- projections keeps them.
CREATE TABLE featured_tags (
  account_id TEXT NOT NULL,
  tag_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL CHECK (ordinal BETWEEN 1 AND 10),
  featured_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, tag_id)
) STRICT;
CREATE UNIQUE INDEX featured_tags_account_id_ordinal ON featured_tags (account_id, ordinal);

-- ---------------------------------------------------------------------------------------------
-- timeline: keyword filters (data-model §4.6, ADR-0157). Owned by internal/app/timeline: the
-- account's commands write them, and the reads of the timelines, the threads, the notifications,
-- and the streams match the statuses they show against the active filters of the viewer.
-- ---------------------------------------------------------------------------------------------

-- Resource: a keyword filter of an account, at most 100 per account. A rebuild of the projections
-- keeps them.
CREATE TABLE keyword_filters (
  id TEXT NOT NULL PRIMARY KEY,
  account_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX keyword_filters_account_id_id ON keyword_filters (account_id, id);

-- Current value: the title (1 to 50 graphemes), action, and expiry of a filter. expires_at is the
-- time until which the filter applies (NULL for always), not a state: an expired filter is kept,
-- listed as expired, and applies again when its expiry moves later.
CREATE TABLE keyword_filter_settings (
  filter_id TEXT NOT NULL PRIMARY KEY,
  title TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('warn', 'hide')),
  expires_at INTEGER
) STRICT;

-- Current value: the contexts where a filter applies, at least one.
CREATE TABLE keyword_filter_contexts (
  filter_id TEXT NOT NULL,
  context TEXT NOT NULL CHECK (context IN ('home', 'notifications', 'public', 'thread', 'account')),
  PRIMARY KEY (filter_id, context)
) STRICT;

-- Current value: the words of a filter (1 to 100), in the order given (position): what the user
-- typed, its normalized form that statuses are matched by (ADR-0157 §4), and whether it matches
-- whole words only. A word is in a filter once by its normalized form.
CREATE TABLE keyword_filter_words (
  filter_id TEXT NOT NULL,
  normalized_word TEXT NOT NULL,
  word TEXT NOT NULL,
  whole_word INTEGER NOT NULL CHECK (whole_word IN (0, 1)),
  position INTEGER NOT NULL CHECK (position >= 0),
  PRIMARY KEY (filter_id, normalized_word)
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- timeline: direct messages by conversation (data-model §4.6, §6.4, ADR-0154 §6). Owned by
-- internal/app/timeline: its projectors derive feed_conversation_participants and
-- feed_conversations from the direct posts and follow content.conversation_merged; the owner's
-- commands write the read and hide boundaries, which the merges move too.
-- ---------------------------------------------------------------------------------------------

-- Projection: the parties of each direct post (its author and its recipients, as
-- feed_direct_recipients holds them), with the conversation the post belongs to now.
CREATE TABLE feed_conversation_participants (
  conversation_id TEXT NOT NULL,
  entry_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  PRIMARY KEY (account_id, conversation_id, entry_id)
) STRICT;
CREATE INDEX feed_conversation_participants_conversation ON feed_conversation_participants (conversation_id, entry_id);
CREATE INDEX feed_conversation_participants_entry_id ON feed_conversation_participants (entry_id);

-- Projection: for each account, every conversation with a direct post it sent or received, and
-- the newest such post, which orders the account's list of direct messages.
CREATE TABLE feed_conversations (
  account_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  last_entry_id TEXT NOT NULL,
  PRIMARY KEY (account_id, conversation_id)
) STRICT;
CREATE INDEX feed_conversations_account_last ON feed_conversations (account_id, last_entry_id);
CREATE INDEX feed_conversations_conversation_id ON feed_conversations (conversation_id);

-- Current value: the newest direct post of a conversation that an account has read; the newer
-- ones by others are unread.
CREATE TABLE feed_conversation_reads (
  account_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  entry_id TEXT NOT NULL,
  PRIMARY KEY (account_id, conversation_id)
) STRICT;
CREATE INDEX feed_conversation_reads_conversation_id ON feed_conversation_reads (conversation_id);

-- Current value: the newest direct post of a conversation when an account hid it; a newer one
-- brings the conversation back to the list.
CREATE TABLE feed_conversation_hides (
  account_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  entry_id TEXT NOT NULL,
  PRIMARY KEY (account_id, conversation_id)
) STRICT;
CREATE INDEX feed_conversation_hides_conversation_id ON feed_conversation_hides (conversation_id);

-- The erasure of deleted local accounts' data (account, #397, data-model §4.2, §8) looks up
-- the rows of an account.
CREATE INDEX feed_entries_original_author_account_id ON feed_entries (original_author_account_id);
CREATE INDEX feed_follow_edges_followee_account_id ON feed_follow_edges (followee_account_id);
CREATE INDEX feed_exclusions_target_account_id ON feed_exclusions (target_account_id);
