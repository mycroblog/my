-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for PostgreSQL (ADR-0023): the tables of the gathering module
-- (docs/architecture/data-model/04-tables/19-gathering.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- gathering: the events of the local users, their settings, places, dates, invitations,
-- answers, approvals, co-organizers, and cancellations (data-model §4.19, ADR-0161 §13). Owned by
-- internal/app/gathering.
-- ---------------------------------------------------------------------------------------------

-- Resource: an event. Its organizer never changes.
CREATE TABLE gatherings (
  id uuid NOT NULL PRIMARY KEY,
  organizer_account_id uuid NOT NULL,
  created_at timestamptz NOT NULL
);
-- The events that an account organizes.
CREATE INDEX gatherings_organizer_account_id ON gatherings (organizer_account_id, id);

-- Event (revision): the settings of an event; the settings it was created with are the first,
-- and the newest row is current. The title is 1 to 100 grapheme clusters
-- (internal/domain/gathering); time_zone is an IANA name. The erasure of the account that wrote a
-- revision empties its title and description and deletes its places.
CREATE TABLE gathering_revisions (
  id uuid NOT NULL PRIMARY KEY,
  gathering_id uuid NOT NULL,
  title text NOT NULL CHECK (length(title) <= 1000),
  description text NOT NULL CHECK (length(description) <= 3000),
  visibility text NOT NULL CHECK (visibility IN ('public', 'followers', 'following', 'mutuals', 'invited')),
  time_zone text NOT NULL CHECK (length(time_zone) BETWEEN 1 AND 100),
  place_disclosure text NOT NULL CHECK (place_disclosure IN ('approved', 'viewers')),
  capacity integer NOT NULL CHECK (capacity BETWEEN 0 AND 10000),
  revised_by_account_id uuid NOT NULL,
  revised_at timestamptz NOT NULL
);
CREATE INDEX gathering_revisions_gathering_id_id ON gathering_revisions (gathering_id, id DESC);
-- The revisions that an account wrote, which its erasure empties.
CREATE INDEX gathering_revisions_revised_by_account_id ON gathering_revisions (revised_by_account_id, id);

-- Resource (child of a revision): the places of a revision, at most one of each kind, in the order
-- the organizer gave them. name, address, and url are those of a physical place (url is its map
-- link, '' for none) and of a URL place, '' otherwise; speakers is that of a voice place, ''
-- otherwise. A URL is http or https and is never fetched.
CREATE TABLE gathering_revision_places (
  revision_id uuid NOT NULL,
  position integer NOT NULL CHECK (position BETWEEN 0 AND 2),
  kind text NOT NULL CHECK (kind IN ('voice', 'physical', 'url')),
  name text NOT NULL CHECK (length(name) <= 100),
  address text NOT NULL CHECK (length(address) <= 500),
  url text NOT NULL CHECK (length(url) <= 2000),
  speakers text NOT NULL CHECK (speakers IN ('', 'everyone', 'followers', 'mutuals', 'manual')),
  PRIMARY KEY (revision_id, position)
);
CREATE UNIQUE INDEX gathering_revision_places_revision_kind ON gathering_revision_places (revision_id, kind);

-- Event: the date of an event fixed; the newest row is the current date.
CREATE TABLE gathering_schedule_confirmations (
  id uuid NOT NULL PRIMARY KEY,
  gathering_id uuid NOT NULL,
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  confirmed_by_account_id uuid NOT NULL,
  confirmed_at timestamptz NOT NULL,
  CONSTRAINT gathering_schedule_confirmations_check CHECK (ends_at > starts_at)
);
CREATE INDEX gathering_schedule_confirmations_gathering_id_id ON gathering_schedule_confirmations (gathering_id, id DESC);

-- Resource: an invitation to an event. An account is invited to an event once.
CREATE TABLE gathering_invitations (
  id uuid NOT NULL PRIMARY KEY,
  gathering_id uuid NOT NULL,
  invitee_account_id uuid NOT NULL,
  invited_by_account_id uuid NOT NULL,
  invited_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX gathering_invitations_gathering_invitee ON gathering_invitations (gathering_id, invitee_account_id);
-- The invitations of an account.
CREATE INDEX gathering_invitations_invitee_account_id ON gathering_invitations (invitee_account_id, gathering_id);

-- Event: the revocation of an invitation, once.
CREATE TABLE gathering_invitation_revocations (
  invitation_id uuid NOT NULL PRIMARY KEY,
  revoked_by_account_id uuid NOT NULL,
  revoked_at timestamptz NOT NULL
);

-- Event: an answer of an account to an event; the newest row of the account is its answer.
CREATE TABLE gathering_rsvps (
  id uuid NOT NULL PRIMARY KEY,
  gathering_id uuid NOT NULL,
  account_id uuid NOT NULL,
  answer text NOT NULL CHECK (answer IN ('going', 'maybe', 'declined')),
  answered_at timestamptz NOT NULL
);
CREATE INDEX gathering_rsvps_gathering_account ON gathering_rsvps (gathering_id, account_id, id DESC);

-- Event: an organizer approved an attendee, or revoked the approval; the newest row of the account
-- counts until the account declines.
CREATE TABLE gathering_rsvp_approvals (
  id uuid NOT NULL PRIMARY KEY,
  gathering_id uuid NOT NULL,
  account_id uuid NOT NULL,
  change text NOT NULL CHECK (change IN ('approved', 'revoked')),
  changed_by_account_id uuid NOT NULL,
  changed_at timestamptz NOT NULL
);
CREATE INDEX gathering_rsvp_approvals_gathering_account ON gathering_rsvp_approvals (gathering_id, account_id, id DESC);

-- Event: the organizer assigned a co-organizer or removed one.
CREATE TABLE gathering_coorganizer_changes (
  id uuid NOT NULL PRIMARY KEY,
  gathering_id uuid NOT NULL,
  account_id uuid NOT NULL,
  change text NOT NULL CHECK (change IN ('assigned', 'removed')),
  changed_at timestamptz NOT NULL
);
CREATE INDEX gathering_coorganizer_changes_gathering_account ON gathering_coorganizer_changes (gathering_id, account_id, id DESC);

-- Event: the cancellation of an event, once. cancelled_by_account_id is NULL for an automatic
-- cancellation; note is the reason a moderator or the organizer gave, '' for none.
CREATE TABLE gathering_cancellations (
  gathering_id uuid NOT NULL PRIMARY KEY,
  reason text NOT NULL CHECK (reason IN ('organizer', 'moderator', 'organizer_suspended', 'organizer_deleted')),
  cancelled_by_account_id uuid,
  note text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
  cancelled_at timestamptz NOT NULL
);

-- Projection: the current revision and date of each event, whether it was cancelled, and how
-- many attendees answer going. The list reads it.
CREATE TABLE gathering_heads (
  gathering_id uuid NOT NULL PRIMARY KEY,
  organizer_account_id uuid NOT NULL,
  revision_id uuid NOT NULL,
  confirmation_id uuid NOT NULL,
  state text NOT NULL CHECK (state IN ('scheduled', 'cancelled')),
  starts_at timestamptz NOT NULL,
  ends_at timestamptz NOT NULL,
  going_count integer NOT NULL CHECK (going_count >= 0)
);
-- The upcoming events, nearest first.
CREATE INDEX gathering_heads_state_starts_at ON gathering_heads (state, starts_at, gathering_id);

-- Projection: the current answer of each account that answered an event (its newest RSVP), and
-- whether an organizer approved it.
CREATE TABLE gathering_attendees (
  gathering_id uuid NOT NULL,
  account_id uuid NOT NULL,
  rsvp_id uuid NOT NULL,
  answer text NOT NULL CHECK (answer IN ('going', 'maybe', 'declined')),
  approved boolean NOT NULL,
  PRIMARY KEY (gathering_id, account_id)
);
-- The attendee list, in the order of the answers.
CREATE INDEX gathering_attendees_gathering_rsvp ON gathering_attendees (gathering_id, rsvp_id);

-- Projection: the current co-organizers of each event; change_id, the assignment, orders the
-- nominations.
CREATE TABLE gathering_coorganizers (
  gathering_id uuid NOT NULL,
  account_id uuid NOT NULL,
  change_id uuid NOT NULL,
  PRIMARY KEY (gathering_id, account_id)
);
-- The events an account co-organizes.
CREATE INDEX gathering_coorganizers_account_id ON gathering_coorganizers (account_id, gathering_id);
