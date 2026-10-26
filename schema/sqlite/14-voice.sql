-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the voice module
-- (docs/architecture/data-model/04-tables/14-voice.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- voice: the voice chat rooms, their settings, hosts, participants, invitations, and endings
-- (data-model §4.14, ADR-0100 §12). Owned by internal/app/voice.
-- ---------------------------------------------------------------------------------------------

-- Resource: a room. transport is the voice setting when it opened.
CREATE TABLE voice_rooms (
  id TEXT NOT NULL PRIMARY KEY,
  opened_by_account_id TEXT NOT NULL,
  opened_at INTEGER NOT NULL,
  transport TEXT NOT NULL CHECK (transport IN ('p2p', 'livekit', 'sora'))
) STRICT;

-- Event (revision): the settings of a room; the settings it opened with are the first, and the
-- newest row is current. The name is 1 to 100 grapheme clusters (internal/domain/voice); the
-- erasure of the account that wrote a revision empties its name and description.
CREATE TABLE voice_room_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  room_id TEXT NOT NULL,
  name TEXT NOT NULL CHECK (length(name) <= 1000),
  description TEXT NOT NULL CHECK (length(description) <= 500),
  visibility TEXT NOT NULL CHECK (visibility IN ('local', 'followers', 'mutuals', 'invited')),
  speakers TEXT NOT NULL CHECK (speakers IN ('everyone', 'followers', 'mutuals', 'manual')),
  revised_by_account_id TEXT NOT NULL,
  revised_at INTEGER NOT NULL
) STRICT;
CREATE INDEX voice_room_revisions_room_id_id ON voice_room_revisions (room_id, id DESC);
-- The revisions that an account wrote, whose names and descriptions its erasure empties.
CREATE INDEX voice_room_revisions_revised_by_account_id ON voice_room_revisions (revised_by_account_id, room_id);

-- Resource (child of a revision): the hashtags of a revision, as typed and normalized like the
-- hashtags of statuses, in the order the host gave them.
CREATE TABLE voice_room_revision_hashtags (
  revision_id TEXT NOT NULL,
  position INTEGER NOT NULL CHECK (position BETWEEN 0 AND 4),
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
  name_normalized TEXT NOT NULL CHECK (length(name_normalized) BETWEEN 1 AND 100),
  PRIMARY KEY (revision_id, position)
) STRICT;
CREATE UNIQUE INDEX voice_room_revision_hashtags_revision_tag ON voice_room_revision_hashtags (revision_id, name_normalized);
-- The rooms of a hashtag.
CREATE INDEX voice_room_revision_hashtags_name_normalized ON voice_room_revision_hashtags (name_normalized, revision_id);

-- Event: a room has a new host; the first row (opened) is the account that opened it.
CREATE TABLE voice_room_host_changes (
  id TEXT NOT NULL PRIMARY KEY,
  room_id TEXT NOT NULL,
  host_account_id TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (reason IN ('opened', 'handed_over', 'host_disconnected', 'host_ineligible', 'host_left')),
  changed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX voice_room_host_changes_room_id_id ON voice_room_host_changes (room_id, id DESC);
-- The rooms a user hosted (ADR-0100 §13).
CREATE INDEX voice_room_host_changes_host_account_id ON voice_room_host_changes (host_account_id, room_id);

-- Event: the host assigned or removed a co-host. The order of the nominations is the order of
-- the assigned rows.
CREATE TABLE voice_room_cohost_changes (
  id TEXT NOT NULL PRIMARY KEY,
  room_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  change TEXT NOT NULL CHECK (change IN ('assigned', 'removed')),
  changed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX voice_room_cohost_changes_room_account_id ON voice_room_cohost_changes (room_id, account_id, id DESC);

-- Resource: an account in a room, from joining to its end.
CREATE TABLE voice_room_participations (
  id TEXT NOT NULL PRIMARY KEY,
  room_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  joined_at INTEGER NOT NULL
) STRICT;
CREATE INDEX voice_room_participations_room_id_id ON voice_room_participations (room_id, id);
CREATE INDEX voice_room_participations_account_id_id ON voice_room_participations (account_id, id DESC);

-- Event: a participation ended, once.
CREATE TABLE voice_room_participation_ends (
  participation_id TEXT NOT NULL PRIMARY KEY,
  reason TEXT NOT NULL CHECK (reason IN ('left', 'moved', 'disconnected', 'removed', 'blocked', 'suspended', 'room_ended')),
  ended_at INTEGER NOT NULL
) STRICT;

-- Event: a participant became a speaker or a listener, by themselves or by the host or a
-- co-host; the newest row is current, and a participation without one listens.
CREATE TABLE voice_room_role_changes (
  id TEXT NOT NULL PRIMARY KEY,
  participation_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('speaker', 'listener')),
  changed_by_account_id TEXT NOT NULL,
  changed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX voice_room_role_changes_participation_id_id ON voice_room_role_changes (participation_id, id DESC);

-- Event: the host or a co-host granted or revoked an account's explicit permission to speak in
-- a room (an invitation as a speaker writes granted); the newest row of the account is current.
CREATE TABLE voice_room_speak_grants (
  id TEXT NOT NULL PRIMARY KEY,
  room_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  change TEXT NOT NULL CHECK (change IN ('granted', 'revoked')),
  changed_by_account_id TEXT NOT NULL,
  changed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX voice_room_speak_grants_room_account_id ON voice_room_speak_grants (room_id, account_id, id DESC);

-- Event: a participant raised or lowered a hand, or the host or a co-host granted or dismissed
-- it; the hand is up while the newest row is raised.
CREATE TABLE voice_room_hand_raises (
  id TEXT NOT NULL PRIMARY KEY,
  participation_id TEXT NOT NULL,
  change TEXT NOT NULL CHECK (change IN ('raised', 'lowered', 'granted', 'dismissed')),
  changed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX voice_room_hand_raises_participation_id_id ON voice_room_hand_raises (participation_id, id DESC);

-- Resource: an invitation of a local user to a room, as a listener or a speaker; one per room
-- and user.
CREATE TABLE voice_room_invitations (
  id TEXT NOT NULL PRIMARY KEY,
  room_id TEXT NOT NULL,
  invitee_account_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('speaker', 'listener')),
  invited_by_account_id TEXT NOT NULL,
  invited_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX voice_room_invitations_room_invitee ON voice_room_invitations (room_id, invitee_account_id);

-- Event: an invitation revoked, once.
CREATE TABLE voice_room_invitation_revocations (
  invitation_id TEXT NOT NULL PRIMARY KEY,
  revoked_by_account_id TEXT NOT NULL,
  revoked_at INTEGER NOT NULL
) STRICT;

-- Event: the host or a co-host removed an account from a room, which bans it from the room;
-- once per room and account.
CREATE TABLE voice_room_bans (
  id TEXT NOT NULL PRIMARY KEY,
  room_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  banned_by_account_id TEXT NOT NULL,
  banned_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX voice_room_bans_room_account ON voice_room_bans (room_id, account_id);

-- Event: a room ended, once. ended_by_account_id is NULL for an automatic end; note is the reason
-- that a moderator gave for ending it (moderator, ADR-0100 §11), '' for none.
CREATE TABLE voice_room_endings (
  room_id TEXT NOT NULL PRIMARY KEY,
  reason TEXT NOT NULL CHECK (reason IN ('host', 'empty', 'host_disconnected', 'host_ineligible', 'moderator', 'time_limit', 'disabled')),
  ended_by_account_id TEXT,
  ended_at INTEGER NOT NULL,
  note TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 500)
) STRICT;
-- The rooms whose participant records the purge erases after their retention (ADR-0100 §13).
CREATE INDEX voice_room_endings_ended_at ON voice_room_endings (ended_at, room_id);

-- Projection: the current revision, host, and state of each room, and how many participations
-- of an open room have not ended. The room list reads it.
CREATE TABLE voice_room_heads (
  room_id TEXT NOT NULL PRIMARY KEY,
  revision_id TEXT NOT NULL,
  host_account_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('open', 'ended')),
  participants_count INTEGER NOT NULL CHECK (participants_count >= 0)
) STRICT;
CREATE INDEX voice_room_heads_state_room_id ON voice_room_heads (state, room_id);

-- Projection: the participations that have not ended, one per account (one room per account,
-- ADR-0100 §5), with the role, the newest co-host assignment of the account in the room when it
-- is a co-host (NULL otherwise; its ID orders the nominations), and whether the hand is up.
CREATE TABLE voice_room_participants (
  account_id TEXT NOT NULL PRIMARY KEY,
  room_id TEXT NOT NULL,
  participation_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('speaker', 'listener')),
  cohost_change_id TEXT,
  hand_raised INTEGER NOT NULL CHECK (hand_raised IN (0, 1))
) STRICT;
CREATE UNIQUE INDEX voice_room_participants_participation_id ON voice_room_participants (participation_id);
CREATE INDEX voice_room_participants_room_id ON voice_room_participants (room_id, participation_id);

-- Current value: the grace of a disconnected host, while it lasts (ADR-0100 §6); ends_at is the
-- deadline the decision set.
CREATE TABLE voice_room_host_graces (
  room_id TEXT NOT NULL PRIMARY KEY,
  ends_at INTEGER NOT NULL
) STRICT;
CREATE INDEX voice_room_host_graces_ends_at ON voice_room_host_graces (ends_at);
