-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the graph module
-- (docs/architecture/data-model/04-tables/05-graph.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- graph: follows (data-model §4.5) ------------------------------------------------------------

-- Resource: one follow relationship. Following again after a rejection or termination inserts a
-- new row, so earlier relationships stay on record.
CREATE TABLE follows (
  id TEXT NOT NULL PRIMARY KEY,
  uri TEXT NOT NULL,
  follower_account_id TEXT NOT NULL,
  followee_account_id TEXT NOT NULL,
  requested_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX follows_uri ON follows (uri);

-- Event: a follow became active.
CREATE TABLE follow_acceptances (
  follow_id TEXT NOT NULL PRIMARY KEY,
  accepted_by TEXT NOT NULL CHECK (accepted_by IN ('auto', 'followee', 'remote')),
  activity_uri TEXT,
  accepted_at INTEGER NOT NULL
) STRICT;

-- Event: a pending follow was declined.
CREATE TABLE follow_rejections (
  follow_id TEXT NOT NULL PRIMARY KEY,
  rejected_by TEXT NOT NULL CHECK (rejected_by IN ('followee', 'remote', 'system')),
  activity_uri TEXT,
  rejected_at INTEGER NOT NULL
) STRICT;

-- Event: a pending or active follow ended.
CREATE TABLE follow_terminations (
  follow_id TEXT NOT NULL PRIMARY KEY,
  reason TEXT NOT NULL CHECK (reason IN ('unfollowed', 'removed_by_followee', 'blocked', 'account_deleted', 'moderation', 'moved')),
  terminated_at INTEGER NOT NULL
) STRICT;

-- Projection: the live follow of each pair (neither rejected nor terminated).
CREATE TABLE follow_edges (
  follower_account_id TEXT NOT NULL,
  followee_account_id TEXT NOT NULL,
  follow_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('pending', 'active')),
  since INTEGER NOT NULL,
  PRIMARY KEY (follower_account_id, followee_account_id)
) STRICT;
CREATE INDEX follow_edges_followee_state_follow_id ON follow_edges (followee_account_id, state, follow_id);
CREATE INDEX follow_edges_follower_state_follow_id ON follow_edges (follower_account_id, state, follow_id);

-- Current value: the follower's settings of a follow.
CREATE TABLE follow_preferences (
  follow_id TEXT NOT NULL PRIMARY KEY,
  show_boosts INTEGER NOT NULL CHECK (show_boosts IN (0, 1)),
  notify INTEGER NOT NULL CHECK (notify IN (0, 1))
) STRICT;

-- Projection: the numbers of active follows into and out of an account. No row means zero.
CREATE TABLE follow_counters (
  account_id TEXT NOT NULL PRIMARY KEY,
  followers_count INTEGER NOT NULL CHECK (followers_count >= 0),
  following_count INTEGER NOT NULL CHECK (following_count >= 0)
) STRICT;

-- graph: blocks and mutes (data-model §4.5) -------------------------------------------------

-- Resource: one block. Blocking again after a retraction inserts a new row. uri is NULL only for
-- a received Block without an id.
CREATE TABLE blocks (
  id TEXT NOT NULL PRIMARY KEY,
  uri TEXT,
  blocker_account_id TEXT NOT NULL,
  blocked_account_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX blocks_uri ON blocks (uri);

-- Event: a block was lifted.
CREATE TABLE block_retractions (
  block_id TEXT NOT NULL PRIMARY KEY,
  retracted_at INTEGER NOT NULL
) STRICT;

-- Projection: the block in force of each pair (without a retraction).
CREATE TABLE block_edges (
  blocker_account_id TEXT NOT NULL,
  blocked_account_id TEXT NOT NULL,
  block_id TEXT NOT NULL,
  PRIMARY KEY (blocker_account_id, blocked_account_id)
) STRICT;
CREATE INDEX block_edges_blocked_account_id ON block_edges (blocked_account_id);
CREATE INDEX block_edges_blocker_block_id ON block_edges (blocker_account_id, block_id);

-- Current value: an account's mute of another. A mute whose expires_at has passed no longer
-- applies; readers filter it out, and the hourly cleanup deletes it.
CREATE TABLE mutes (
  account_id TEXT NOT NULL,
  target_account_id TEXT NOT NULL,
  hide_notifications INTEGER NOT NULL CHECK (hide_notifications IN (0, 1)),
  expires_at INTEGER,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, target_account_id),
  CHECK (expires_at IS NULL OR expires_at > created_at)
) STRICT;
CREATE INDEX mutes_account_created_at_target ON mutes (account_id, created_at, target_account_id);
-- The periodic cleanup deletes the expired mutes (FR-803).
CREATE INDEX mutes_expires_at ON mutes (expires_at);

-- Current value: an account's mute of a remote domain and of its subdomains (FR-304): their
-- posts and notifications are hidden from the account. It keeps no history and federates
-- nothing.
CREATE TABLE account_domain_mutes (
  account_id TEXT NOT NULL,
  domain TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, domain)
) STRICT;

-- graph and federation: re-sent Follows and early Undo Follows (data-model §4.5, §4.8) ----------

-- Event: a Follow received again, under another URI, for a follow that is already active. The
-- follow keeps its row; its Accept is sent again for the new URI.
CREATE TABLE follow_resends (
  uri TEXT NOT NULL PRIMARY KEY,
  follow_id TEXT NOT NULL,
  received_at INTEGER NOT NULL
) STRICT;

-- The erasure of deleted local accounts' data (account, #397, data-model §4.2, §8) looks up
-- the rows of an account.
CREATE INDEX follows_follower_account_id ON follows (follower_account_id);
CREATE INDEX follows_followee_account_id ON follows (followee_account_id);
CREATE INDEX blocks_blocker_account_id ON blocks (blocker_account_id);
CREATE INDEX blocks_blocked_account_id ON blocks (blocked_account_id);

CREATE INDEX mutes_target_account_id ON mutes (target_account_id);
