-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the account module
-- (docs/architecture/data-model/04-tables/02-account.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- account (data-model §4.2) ------------------------------------------------------------------

-- Resource: a local or remote actor. domain is '' for local actors.
CREATE TABLE accounts (
  id TEXT NOT NULL PRIMARY KEY,
  uri TEXT NOT NULL,
  domain TEXT NOT NULL,
  actor_type TEXT NOT NULL CHECK (actor_type IN ('person', 'service', 'application', 'group', 'organization')),
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX accounts_uri ON accounts (uri);
CREATE INDEX accounts_domain_id ON accounts (domain, id);
-- The only local Application actor is the instance actor.
CREATE UNIQUE INDEX accounts_instance_actor ON accounts (domain, actor_type) WHERE domain = '' AND actor_type = 'application';

-- Resource (subtype): a local account, owned by one user.
CREATE TABLE local_accounts (
  account_id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  username TEXT NOT NULL,
  username_normalized TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX local_accounts_user_id ON local_accounts (user_id);
CREATE UNIQUE INDEX local_accounts_username_normalized ON local_accounts (username_normalized);

-- Projection: @username@domain resolution (WebFinger, FR-305, FR-401).
CREATE TABLE account_handles (
  account_id TEXT NOT NULL PRIMARY KEY,
  username TEXT NOT NULL,
  username_normalized TEXT NOT NULL,
  domain TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX account_handles_username_normalized_domain ON account_handles (username_normalized, domain);

-- Event (revision): one version of a profile.
CREATE TABLE account_profile_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  account_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  display_name TEXT NOT NULL,
  note_html TEXT NOT NULL,
  locked INTEGER NOT NULL CHECK (locked IN (0, 1)),
  bot INTEGER NOT NULL CHECK (bot IN (0, 1)),
  discoverable INTEGER NOT NULL CHECK (discoverable IN (0, 1)),
  indexable INTEGER NOT NULL CHECK (indexable IN (0, 1)),
  ai_opt_out INTEGER NOT NULL CHECK (ai_opt_out IN (0, 1)),
  avatar_media_id TEXT,
  header_media_id TEXT,
  revised_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX account_profile_revisions_account_id_seq ON account_profile_revisions (account_id, seq);

-- Event (child of a revision): the profile's metadata fields (FR-006).
CREATE TABLE account_profile_fields (
  revision_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  name TEXT NOT NULL,
  value_html TEXT NOT NULL,
  PRIMARY KEY (revision_id, position)
) STRICT;

-- Event (child of a revision): a custom emoji of the profile's display name, note, or fields
-- (ADR-0051). emoji_id is a custom_emojis row of the content module.
CREATE TABLE account_profile_revision_emojis (
  revision_id TEXT NOT NULL,
  emoji_id TEXT NOT NULL,
  PRIMARY KEY (revision_id, emoji_id)
) STRICT;
-- The purge of unreferenced remote custom emoji looks up the references to an emoji.
CREATE INDEX account_profile_revision_emojis_emoji_id ON account_profile_revision_emojis (emoji_id);

-- Projection: the current profile revision of each account.
CREATE TABLE account_profile_heads (
  account_id TEXT NOT NULL PRIMARY KEY,
  revision_id TEXT NOT NULL
) STRICT;

-- Resource: a public signing key of an actor.
CREATE TABLE account_keys (
  id TEXT NOT NULL PRIMARY KEY,
  account_id TEXT NOT NULL,
  key_id TEXT NOT NULL,
  algorithm TEXT NOT NULL CHECK (algorithm IN ('rsa_sha256', 'ed25519')),
  public_key_pem TEXT NOT NULL,
  fingerprint BLOB NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX account_keys_key_id_fingerprint ON account_keys (key_id, fingerprint);
CREATE INDEX account_keys_key_id_id ON account_keys (key_id, id DESC);
CREATE INDEX account_keys_account_id ON account_keys (account_id);

-- Resource (subtype): the sealed private key of a local actor's key (NFR-SEC-7).
CREATE TABLE local_account_private_keys (
  account_key_id TEXT NOT NULL PRIMARY KEY,
  private_key_enc BLOB NOT NULL
) STRICT;

-- Event: a lifecycle transition of an account (self and system axis, data-model §5.1).
CREATE TABLE account_lifecycle_events (
  id TEXT NOT NULL PRIMARY KEY,
  account_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN (
    'deactivated', 'reactivated', 'deletion_scheduled', 'deletion_cancelled', 'deleted', 'remote_deleted'
  )),
  initiated_by TEXT NOT NULL CHECK (initiated_by IN ('self', 'moderator', 'system', 'remote')),
  moderation_action_id TEXT,
  deletion_due_at INTEGER,
  occurred_at INTEGER NOT NULL,
  CHECK ((kind = 'deletion_scheduled') = (deletion_due_at IS NOT NULL))
) STRICT;
CREATE INDEX account_lifecycle_events_account_id_id ON account_lifecycle_events (account_id, id DESC);

-- Projection: the current lifecycle state. An account without a row is active.
CREATE TABLE account_lifecycle_states (
  account_id TEXT NOT NULL PRIMARY KEY,
  state TEXT NOT NULL CHECK (state IN ('active', 'deactivated', 'deletion_scheduled', 'deleted')),
  since INTEGER NOT NULL,
  deletion_due_at INTEGER,
  CHECK ((state = 'deletion_scheduled') = (deletion_due_at IS NOT NULL))
) STRICT;
CREATE INDEX account_lifecycle_states_state_deletion_due_at ON account_lifecycle_states (state, deletion_due_at);

-- Event: an account moved to another (ADR-0089, FR-414): a local account's own move (self) or a
-- Move that a remote account's server sent and that passed its checks (remote, activity_uri is
-- the Move). An account moves to one target once; the wait between the moves of one account is
-- read from its newest row.
CREATE TABLE account_moves (
  id TEXT NOT NULL PRIMARY KEY,
  account_id TEXT NOT NULL,
  target_account_id TEXT NOT NULL,
  initiated_by TEXT NOT NULL CHECK (initiated_by IN ('self', 'remote')),
  activity_uri TEXT,
  occurred_at INTEGER NOT NULL,
  CHECK (account_id <> target_account_id)
) STRICT;
CREATE UNIQUE INDEX account_moves_account_id_target_account_id ON account_moves (account_id, target_account_id);
CREATE INDEX account_moves_account_id_occurred_at ON account_moves (account_id, occurred_at DESC, id DESC);

-- Event: a local account cancelled its move (ADR-0089). A move is cancelled at most once.
CREATE TABLE account_move_cancellations (
  move_id TEXT NOT NULL PRIMARY KEY,
  cancelled_at INTEGER NOT NULL
) STRICT;

-- Projection of account_moves and account_move_cancellations: the move in force of an account,
-- its newest move unless that is cancelled. An account without a row has not moved. It is an
-- axis of its own, apart from account_lifecycle_states (ADR-0020 rule 2).
CREATE TABLE account_move_states (
  account_id TEXT NOT NULL PRIMARY KEY,
  move_id TEXT NOT NULL,
  target_account_id TEXT NOT NULL,
  since INTEGER NOT NULL
) STRICT;
CREATE INDEX account_move_states_target_account_id ON account_move_states (target_account_id);

-- Current value: the aliases of a local account (ADR-0089, FR-009), the actor ids of the accounts
-- it moves from, which its actor names in alsoKnownAs. At most 5 per account. Only local
-- accounts have rows; the alsoKnownAs of remote actors is fetched when a move is checked and not
-- stored.
CREATE TABLE account_aliases (
  account_id TEXT NOT NULL,
  alias_uri TEXT NOT NULL,
  added_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, alias_uri)
) STRICT;

-- Projection: the words that the prefix search of accounts matches (ADR-0092 §1, FR-119, FR-306),
-- from the handles, the profile heads, the lifecycle states, and the suspensions of the
-- moderation module. A username row is the normalized username, and for a remote account also
-- username@domain; a display_name row is the display name under NFKC and lowercase without its
-- :shortcode:s, whole and word by word. Accounts that are not active, suspended, without a
-- handle, or Application actors have no rows. term compares byte by byte (the default BINARY),
-- so that the range term >= q AND term < q || U+10FFFF reads the index in order.
CREATE TABLE account_search_terms (
  account_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('username', 'display_name')),
  term TEXT NOT NULL,
  is_local INTEGER NOT NULL CHECK (is_local IN (0, 1)),
  PRIMARY KEY (account_id, kind, term)
) STRICT;
-- The search reads the range of a prefix in index order, at most 100 rows.
CREATE INDEX account_search_terms_term_account_id ON account_search_terms (term, account_id);

-- The cleanup of unattached uploads (content, FR-803) looks up the references to a media file.
CREATE INDEX account_profile_revisions_avatar_media_id ON account_profile_revisions (avatar_media_id);
CREATE INDEX account_profile_revisions_header_media_id ON account_profile_revisions (header_media_id);

-- ---------------------------------------------------------------------------------------------
-- account: what the owner of a local profile typed (data-model §4.2). Owned by
-- internal/app/account; the client API returns it for editing the profile.
-- ---------------------------------------------------------------------------------------------

-- Event (subtype of account_profile_revisions): the profile text of a local revision as typed,
-- from which note_html is rendered.
CREATE TABLE account_profile_revision_sources (
  revision_id TEXT NOT NULL PRIMARY KEY,
  note_source TEXT NOT NULL
) STRICT;

-- Event (subtype of account_profile_fields): the value of a local field as typed, from which
-- value_html is rendered.
CREATE TABLE account_profile_field_sources (
  revision_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  value_source TEXT NOT NULL,
  PRIMARY KEY (revision_id, position)
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- account: remote actors and key rotation (data-model §4.2, docs/architecture/federation.md).
-- Owned by internal/app/account.
-- ---------------------------------------------------------------------------------------------

-- Resource (subtype): an actor of another server.
CREATE TABLE remote_accounts (
  account_id TEXT NOT NULL PRIMARY KEY,
  first_seen_at INTEGER NOT NULL
) STRICT;

-- Event: one fetch of a remote actor's document whose content differed from the previous one.
-- document_digest is the SHA-256 of the document.
CREATE TABLE remote_account_snapshots (
  id TEXT NOT NULL PRIMARY KEY,
  account_id TEXT NOT NULL,
  preferred_username TEXT NOT NULL,
  profile_url TEXT,
  inbox_url TEXT NOT NULL,
  shared_inbox_url TEXT,
  outbox_url TEXT,
  followers_url TEXT,
  following_url TEXT,
  featured_url TEXT,
  followers_total INTEGER,
  following_total INTEGER,
  statuses_total INTEGER,
  document_digest BLOB NOT NULL,
  fetched_at INTEGER NOT NULL
) STRICT;
CREATE INDEX remote_account_snapshots_account_id_id ON remote_account_snapshots (account_id, id DESC);

-- Projection: the newest snapshot of each remote actor, with the endpoints delivery reads and
-- the fetch state of the periodic refresh.
CREATE TABLE remote_account_heads (
  account_id TEXT NOT NULL PRIMARY KEY,
  snapshot_id TEXT NOT NULL,
  inbox_url TEXT NOT NULL,
  shared_inbox_url TEXT,
  last_fetched_at INTEGER NOT NULL,
  consecutive_fetch_failures INTEGER NOT NULL CHECK (consecutive_fetch_failures >= 0)
) STRICT;
CREATE INDEX remote_account_heads_last_fetched_at ON remote_account_heads (last_fetched_at);
CREATE INDEX remote_account_heads_shared_inbox_url ON remote_account_heads (shared_inbox_url);

-- Event: a WebFinger check of acct:{username}@{domain}. verified means that its self link points
-- at the actor's uri; mismatch means that it answered otherwise, and the account has no handle
-- until a later check verifies one.
CREATE TABLE remote_account_handle_verifications (
  id TEXT NOT NULL PRIMARY KEY,
  account_id TEXT NOT NULL,
  username TEXT NOT NULL,
  domain TEXT NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('verified', 'mismatch')),
  checked_at INTEGER NOT NULL
) STRICT;
CREATE INDEX remote_account_handle_verifications_account_id_id ON remote_account_handle_verifications (account_id, id DESC);

-- Event (subtype of account_profile_revisions): the snapshot a remote revision came from and the
-- version of the HTML policy that sanitized its note_html and value_html (ADR-0019 §2).
CREATE TABLE remote_profile_revisions (
  revision_id TEXT NOT NULL PRIMARY KEY,
  snapshot_id TEXT NOT NULL,
  html_policy_version INTEGER NOT NULL
) STRICT;

-- Event: a key was replaced or withdrawn. Verification uses the newest key of a key_id without
-- a revocation.
CREATE TABLE account_key_revocations (
  account_key_id TEXT NOT NULL PRIMARY KEY,
  revoked_at INTEGER NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- account: the erasure of deleted local accounts' data (#397, data-model §4.2, §8). Owned by
-- internal/app/account; the indexes serve the erasure's statements in the other modules.
-- ---------------------------------------------------------------------------------------------

-- Event: the erasure job removed the data of a deleted local account (data-model §8). An
-- account is erased at most once; one whose statuses the moderation records still keep has no
-- row until they are erased too.
CREATE TABLE account_erasures (
  account_id TEXT NOT NULL PRIMARY KEY,
  erased_at INTEGER NOT NULL
) STRICT;
