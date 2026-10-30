-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for PostgreSQL (ADR-0023): the tables of the timeline module
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
  id uuid NOT NULL PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN ('post', 'boost')),
  status_id uuid NOT NULL,
  author_account_id uuid NOT NULL,
  author_domain text NOT NULL,
  original_author_account_id uuid NOT NULL,
  original_author_domain text NOT NULL,
  visibility text NOT NULL CHECK (visibility IN ('public', 'local', 'unlisted', 'private', 'direct')),
  is_local boolean NOT NULL,
  language text NOT NULL,
  revision_seq integer NOT NULL CHECK (revision_seq >= 1),
  is_reply boolean NOT NULL
);
CREATE INDEX feed_entries_author_account_id_id ON feed_entries (author_account_id, id);
CREATE INDEX feed_entries_status_id ON feed_entries (status_id);
CREATE INDEX feed_entries_local ON feed_entries (id) WHERE is_local AND kind = 'post' AND visibility IN ('public', 'local');
CREATE INDEX feed_entries_public ON feed_entries (id) WHERE kind = 'post' AND visibility = 'public';

-- Projection: the recipients (current mentions) of a direct post.
CREATE TABLE feed_direct_recipients (
  entry_id uuid NOT NULL,
  recipient_account_id uuid NOT NULL,
  PRIMARY KEY (recipient_account_id, entry_id)
);
CREATE INDEX feed_direct_recipients_entry_id ON feed_direct_recipients (entry_id);

-- Projection: the current hashtags of a post, with its immutable author and visibility and the
-- time of its current revision.
CREATE TABLE feed_tag_entries (
  tag_id uuid NOT NULL,
  entry_id uuid NOT NULL,
  author_account_id uuid NOT NULL,
  visibility text NOT NULL CHECK (visibility IN ('public', 'local', 'unlisted', 'private', 'direct')),
  is_local boolean NOT NULL,
  revised_at timestamptz NOT NULL,
  PRIMARY KEY (tag_id, entry_id)
);
CREATE INDEX feed_tag_entries_entry_id ON feed_tag_entries (entry_id);
-- tag_usage.last_used_at is the latest revised_at of the posts that tag_usage counts.
CREATE INDEX feed_tag_entries_usage ON feed_tag_entries (tag_id, revised_at) WHERE visibility IN ('public', 'local', 'unlisted');

-- Projection: how many posts of visibility public, local, or unlisted carry each hashtag now,
-- and the latest time one of them was revised with it, for the prefix search of hashtags
-- (ADR-0092 §1, FR-119, FR-306). The same projector updates it with feed_tag_entries; a tag that
-- no such post carries has no row, so that a tag of private and direct posts only never shows.
CREATE TABLE tag_usage (
  tag_id uuid NOT NULL PRIMARY KEY,
  statuses_count bigint NOT NULL CHECK (statuses_count >= 1),
  last_used_at timestamptz NOT NULL
);

-- Projection: the active follows, with the follower's show_boosts preference.
CREATE TABLE feed_follow_edges (
  follower_account_id uuid NOT NULL,
  followee_account_id uuid NOT NULL,
  show_boosts boolean NOT NULL,
  PRIMARY KEY (follower_account_id, followee_account_id)
);

-- Projection: the accounts whose entries a viewer does not see: those the viewer blocks, those
-- that block the viewer, and those the viewer mutes (until expires_at; NULL never expires).
CREATE TABLE feed_exclusions (
  viewer_account_id uuid NOT NULL,
  target_account_id uuid NOT NULL,
  reason text NOT NULL CHECK (reason IN ('block', 'blocked_by', 'mute')),
  expires_at timestamptz,
  PRIMARY KEY (viewer_account_id, target_account_id, reason)
);

-- Projection: the domains (and their subdomains) whose posts a viewer does not see on the
-- timelines other than home, nor boosted on home (FR-304).
CREATE TABLE feed_domain_mutes (
  viewer_account_id uuid NOT NULL,
  domain text NOT NULL,
  PRIMARY KEY (viewer_account_id, domain)
);

-- Projection: the moderation restrictions of accounts that bear on timelines, until
-- effective_until (NULL: until lifted).
CREATE TABLE feed_author_restrictions (
  account_id uuid NOT NULL,
  restriction text NOT NULL CHECK (restriction IN ('profile_view', 'visibility_ceiling_private', 'public_discovery')),
  effective_until timestamptz,
  PRIMARY KEY (account_id, restriction)
);

-- Projection: the moderation restrictions of domains (and their subdomains) that bear on
-- timelines.
CREATE TABLE feed_domain_restrictions (
  domain text NOT NULL,
  restriction text NOT NULL CHECK (restriction IN ('profile_view', 'visibility_ceiling_private', 'public_discovery')),
  effective_until timestamptz,
  PRIMARY KEY (domain, restriction)
);

-- ---------------------------------------------------------------------------------------------
-- timeline: lists (data-model §4.6, ADR-0122). Owned by internal/app/timeline: the owner's
-- commands write them, the projector of graph.follow_terminated takes the ended follows out of
-- list_members, and only the owner reads them.
-- ---------------------------------------------------------------------------------------------

-- Resource: a list of the accounts that its owner follows, which only the owner sees.
CREATE TABLE lists (
  id uuid NOT NULL PRIMARY KEY,
  owner_account_id uuid NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE INDEX lists_owner_account_id_id ON lists (owner_account_id, id);

-- Current value: the name of a list (1 to 50 graphemes); renaming it keeps no history.
CREATE TABLE list_names (
  list_id uuid NOT NULL PRIMARY KEY,
  name text NOT NULL
);

-- Current value: the members of a list, each an account that the owner actively follows
-- (feed_follow_edges). owner_account_id copies the list's immutable owner, so that the end of a
-- follow removes its followee from every list of the follower in one statement.
CREATE TABLE list_members (
  list_id uuid NOT NULL,
  member_account_id uuid NOT NULL,
  owner_account_id uuid NOT NULL,
  added_at timestamptz NOT NULL,
  PRIMARY KEY (list_id, member_account_id)
);
CREATE INDEX list_members_owner_member ON list_members (owner_account_id, member_account_id);
CREATE INDEX list_members_member_account_id ON list_members (member_account_id);

-- The erasure of deleted local accounts' data (account, #397, data-model §4.2, §8) looks up
-- the rows of an account.
CREATE INDEX feed_entries_original_author_account_id ON feed_entries (original_author_account_id);
CREATE INDEX feed_follow_edges_followee_account_id ON feed_follow_edges (followee_account_id);
CREATE INDEX feed_exclusions_target_account_id ON feed_exclusions (target_account_id);
