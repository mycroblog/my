-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for PostgreSQL (ADR-0023): the tables of the content module
-- (docs/architecture/data-model/04-tables/04-content.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- content: statuses (data-model §4.4) --------------------------------------------------------

-- Resource: a status. Visibility, author, reply target, and publication time never change.
CREATE TABLE statuses (
  id uuid NOT NULL PRIMARY KEY,
  uri text NOT NULL,
  -- the HTML page of a remote status; a local status's page is derived on read (ADR-0053).
  url text,
  author_account_id uuid NOT NULL,
  is_local boolean NOT NULL,
  visibility text NOT NULL CHECK (visibility IN ('public', 'local', 'unlisted', 'private', 'direct')),
  in_reply_to_uri text,
  conversation_uri text,
  published_at timestamptz NOT NULL,
  -- local statuses are never federated, so only this instance has them (ADR-0017).
  CONSTRAINT statuses_local_check CHECK (visibility <> 'local' OR is_local)
);
CREATE UNIQUE INDEX statuses_uri ON statuses (uri);
CREATE INDEX statuses_author_account_id_id ON statuses (author_account_id, id);
CREATE INDEX statuses_in_reply_to_uri ON statuses (in_reply_to_uri);
CREATE INDEX statuses_conversation_uri_id ON statuses (conversation_uri, id);

-- Resource (subtype): a status posted on this instance, with the client API's Idempotency-Key.
CREATE TABLE local_statuses (
  status_id uuid NOT NULL PRIMARY KEY,
  author_account_id uuid NOT NULL,
  client_id text COLLATE "C",
  idempotency_key text
);
CREATE UNIQUE INDEX local_statuses_author_account_id_idempotency_key ON local_statuses (author_account_id, idempotency_key);

-- Event (revision): one version of a status's content (FR-109).
CREATE TABLE status_revisions (
  id uuid NOT NULL PRIMARY KEY,
  status_id uuid NOT NULL,
  seq integer NOT NULL CHECK (seq >= 1),
  spoiler_text text NOT NULL,
  sensitive boolean NOT NULL,
  language text NOT NULL,
  revised_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX status_revisions_status_id_seq ON status_revisions (status_id, seq);

-- Event (child of a revision): the content per language, normalized from contentMap (NFR-I18N-5).
CREATE TABLE status_revision_bodies (
  revision_id uuid NOT NULL,
  language text NOT NULL,
  content_html text NOT NULL,
  PRIMARY KEY (revision_id, language)
);

-- Event (subtype of a revision): the source text a local author typed.
CREATE TABLE status_revision_sources (
  revision_id uuid NOT NULL PRIMARY KEY,
  source_text text NOT NULL,
  source_format text NOT NULL CHECK (source_format IN ('text_plain'))
);

-- Event (child of a revision): the attached media, with per-revision alt text (FR-105).
CREATE TABLE status_revision_attachments (
  revision_id uuid NOT NULL,
  position integer NOT NULL CHECK (position BETWEEN 0 AND 3),
  media_id uuid NOT NULL,
  description text,
  PRIMARY KEY (revision_id, position)
);

-- Event (child of a revision): the mentioned accounts.
CREATE TABLE status_revision_mentions (
  revision_id uuid NOT NULL,
  account_id uuid NOT NULL,
  PRIMARY KEY (revision_id, account_id)
);

-- Event (child of a revision): the hashtags.
CREATE TABLE status_revision_tags (
  revision_id uuid NOT NULL,
  tag_id uuid NOT NULL,
  PRIMARY KEY (revision_id, tag_id)
);

-- Event (child of a revision): the custom emoji.
CREATE TABLE status_revision_emojis (
  revision_id uuid NOT NULL,
  emoji_id uuid NOT NULL,
  PRIMARY KEY (revision_id, emoji_id)
);
-- The purge of unreferenced remote custom emoji looks up the references to an emoji.
CREATE INDEX status_revision_emojis_emoji_id ON status_revision_emojis (emoji_id);

-- Event: the deletion of a status. It outlives the purge as the tombstone of the URI.
CREATE TABLE status_deletions (
  status_id uuid NOT NULL PRIMARY KEY,
  uri text NOT NULL,
  author_account_id uuid NOT NULL,
  origin text NOT NULL CHECK (origin IN ('author', 'moderation', 'remote', 'account_deletion', 'retention', 'auto_delete')),
  moderation_action_id uuid,
  deleted_at timestamptz NOT NULL
);
CREATE INDEX status_deletions_uri ON status_deletions (uri);
-- A deletion other than by retention keeps its URI from coming back, so there is at most one.
-- A remote status deleted by retention may be fetched again and deleted again (ADR-0034).
CREATE UNIQUE INDEX status_deletions_tombstone_uri ON status_deletions (uri) WHERE origin <> 'retention';

-- Resource: a hashtag, unique by its normalized name. name_normalized is byte-ordered
-- (COLLATE "C"), so that the prefix search of hashtags (ADR-0092 §1) reads the range
-- name_normalized >= q AND name_normalized < q || U+10FFFF in index order.
CREATE TABLE tags (
  id uuid NOT NULL PRIMARY KEY,
  name text NOT NULL,
  name_normalized text COLLATE "C" NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX tags_name_normalized ON tags (name_normalized);

-- Projection: the current revision of each status that is not deleted.
CREATE TABLE status_heads (
  status_id uuid NOT NULL PRIMARY KEY,
  revision_id uuid NOT NULL,
  seq integer NOT NULL,
  last_revised_at timestamptz NOT NULL
);

-- Projection: the known parent of each reply, neither of them deleted (FR-106).
CREATE TABLE status_reply_links (
  status_id uuid NOT NULL PRIMARY KEY,
  parent_status_id uuid NOT NULL,
  parent_author_account_id uuid NOT NULL
);
CREATE INDEX status_reply_links_parent_status_id_status_id ON status_reply_links (parent_status_id, status_id);

-- Projection: the accounts that the current revision of each status mentions.
CREATE TABLE status_mention_index (
  account_id uuid NOT NULL,
  status_id uuid NOT NULL,
  PRIMARY KEY (account_id, status_id)
);
CREATE INDEX status_mention_index_status_id ON status_mention_index (status_id);

-- Projection: the counters of each status that is not deleted (data-model §2.3).
CREATE TABLE status_counters (
  status_id uuid NOT NULL PRIMARY KEY,
  replies_count bigint NOT NULL CHECK (replies_count >= 0),
  boosts_count bigint NOT NULL CHECK (boosts_count >= 0),
  favourites_count bigint NOT NULL CHECK (favourites_count >= 0)
);

-- Projection: the statuses of each account that are not deleted. No row means none.
CREATE TABLE account_post_counters (
  account_id uuid NOT NULL PRIMARY KEY,
  statuses_count bigint NOT NULL CHECK (statuses_count > 0),
  last_status_at timestamptz NOT NULL
);

-- ---------------------------------------------------------------------------------------------
-- content: media files (data-model §4.4, ADR-0005, ADR-0019 §8). Owned by internal/app/content.
-- ---------------------------------------------------------------------------------------------

-- Resource: a media file, uploaded by a local account or fetched from a remote server.
CREATE TABLE media_files (
  id uuid NOT NULL PRIMARY KEY,
  owner_account_id uuid,
  origin text NOT NULL CHECK (origin IN ('upload', 'remote')),
  created_at timestamptz NOT NULL
);
CREATE INDEX media_files_owner_account_id_id ON media_files (owner_account_id, id);

-- Resource (subtype): an upload as received. content_type is detected from the content. The
-- blob under blob_key is private and is deleted once processing has a result.
CREATE TABLE uploaded_media (
  media_id uuid NOT NULL PRIMARY KEY,
  blob_key text NOT NULL,
  content_type text NOT NULL,
  file_size bigint NOT NULL CHECK (file_size >= 0)
);

-- Event: one processing of a media file for a purpose, which picks the sizes of its variants
-- (ADR-0054). The newest row of a file is its current state.
CREATE TABLE media_processing_results (
  id uuid NOT NULL PRIMARY KEY,
  media_id uuid NOT NULL,
  purpose text NOT NULL CHECK (purpose IN ('attachment', 'avatar', 'header', 'emoji')),
  outcome text NOT NULL CHECK (outcome IN ('ready', 'failed')),
  media_type text NOT NULL CHECK (media_type IN ('image', 'gifv', 'video', 'audio', 'unknown')),
  width integer,
  height integer,
  duration_ms integer,
  blurhash text,
  processed_at timestamptz NOT NULL
);
CREATE INDEX media_processing_results_media_id_id ON media_processing_results (media_id, id DESC);

-- Event (child of a result): a processed rendition in blob storage, under the public prefix:
-- small, medium, and large still ones, and for an animation also animated ones (ADR-0054).
CREATE TABLE media_variants (
  result_id uuid NOT NULL,
  size text NOT NULL CHECK (size IN ('small', 'medium', 'large')),
  motion text NOT NULL CHECK (motion IN ('still', 'animated')),
  blob_key text NOT NULL,
  content_type text NOT NULL,
  file_size bigint NOT NULL CHECK (file_size >= 0),
  width integer NOT NULL,
  height integer NOT NULL,
  PRIMARY KEY (result_id, size, motion)
);

-- Current value: the focal point of a media file. No row means the center.
CREATE TABLE media_focus (
  media_id uuid NOT NULL PRIMARY KEY,
  focus_x double precision NOT NULL CHECK (focus_x BETWEEN -1.0 AND 1.0),
  focus_y double precision NOT NULL CHECK (focus_y BETWEEN -1.0 AND 1.0)
);

-- Current value: the description that the uploader set on an upload, which a post copies into
-- its attachment (client API #112). No row means none.
CREATE TABLE media_descriptions (
  media_id uuid NOT NULL PRIMARY KEY,
  description text NOT NULL CHECK (description <> '')
);

-- The cleanup of unattached uploads (FR-803) looks up the references to a media file.
CREATE INDEX status_revision_attachments_media_id ON status_revision_attachments (media_id);

-- ---------------------------------------------------------------------------------------------
-- content: boosts, favourites, bookmarks, and pins (data-model §4.4, FR-107, FR-113).
-- Owned by internal/app/content.
-- ---------------------------------------------------------------------------------------------

-- Resource: a boost (Announce) of a status. visibility copies the status's and never changes;
-- a boost of a local status stays on this instance (ADR-0017).
CREATE TABLE boosts (
  id uuid NOT NULL PRIMARY KEY,
  uri text NOT NULL,
  booster_account_id uuid NOT NULL,
  status_id uuid NOT NULL,
  visibility text NOT NULL CHECK (visibility IN ('public', 'local', 'unlisted', 'private', 'direct')),
  is_local boolean NOT NULL,
  boosted_at timestamptz NOT NULL,
  -- only local accounts boost local statuses, and such boosts never federate (ADR-0017).
  CONSTRAINT boosts_local_check CHECK (visibility <> 'local' OR is_local)
);
CREATE UNIQUE INDEX boosts_uri ON boosts (uri);
CREATE INDEX boosts_status_id ON boosts (status_id);

-- Event: a boost was retracted (Undo Announce). A boost is retracted at most once.
CREATE TABLE boost_retractions (
  boost_id uuid NOT NULL PRIMARY KEY,
  origin text NOT NULL CHECK (origin IN ('booster', 'status_deleted', 'moderation', 'remote')),
  retracted_at timestamptz NOT NULL
);

-- Projection: the live boost of each pair, not retracted, of a status that is not deleted.
CREATE TABLE boost_edges (
  booster_account_id uuid NOT NULL,
  status_id uuid NOT NULL,
  boost_id uuid NOT NULL,
  PRIMARY KEY (booster_account_id, status_id)
);
CREATE INDEX boost_edges_status_id ON boost_edges (status_id);

-- Current value: an account's favourite (Like) of a status.
CREATE TABLE favourites (
  account_id uuid NOT NULL,
  status_id uuid NOT NULL,
  uri text NOT NULL,
  created_at timestamptz NOT NULL,
  id uuid NOT NULL,
  PRIMARY KEY (account_id, status_id)
);
CREATE UNIQUE INDEX favourites_uri ON favourites (uri);
CREATE INDEX favourites_account_id_id ON favourites (account_id, id);
CREATE INDEX favourites_status_id_id ON favourites (status_id, id);

-- Resource: an account's emoji reaction to a status (FR-116, ADR-0052), deleted when it is taken
-- back (content.unreacted). emoji_key is the Unicode emoji, fully qualified, or the
-- custom_emojis.id of custom_emoji_id, NULL for a Unicode emoji. uri is the IRI of the Like or
-- EmojiReact.
CREATE TABLE reactions (
  id uuid NOT NULL PRIMARY KEY,
  account_id uuid NOT NULL,
  status_id uuid NOT NULL,
  emoji_key text COLLATE "C" NOT NULL,
  custom_emoji_id uuid,
  uri text NOT NULL,
  created_at timestamptz NOT NULL,
  CONSTRAINT reactions_custom_emoji_check CHECK (custom_emoji_id IS NULL OR custom_emoji_id::text = emoji_key)
);
CREATE UNIQUE INDEX reactions_account_id_status_id_emoji_key ON reactions (account_id, status_id, emoji_key);
CREATE UNIQUE INDEX reactions_uri ON reactions (uri);
CREATE INDEX reactions_status_id_emoji_key_id ON reactions (status_id, emoji_key, id);
CREATE INDEX reactions_status_id_id ON reactions (status_id, id);
-- The purge of unreferenced remote custom emoji looks up the reactions with an emoji.
CREATE INDEX reactions_custom_emoji_id ON reactions (custom_emoji_id);

-- Projection: the reactions to each status that is not deleted, per emoji (ADR-0052 §1). A row
-- goes when its count reaches zero.
CREATE TABLE status_reaction_counts (
  status_id uuid NOT NULL,
  emoji_key text COLLATE "C" NOT NULL,
  custom_emoji_id uuid,
  count bigint NOT NULL CHECK (count > 0),
  PRIMARY KEY (status_id, emoji_key),
  CONSTRAINT status_reaction_counts_custom_emoji_check CHECK (custom_emoji_id IS NULL OR custom_emoji_id::text = emoji_key)
);
CREATE INDEX status_reaction_counts_status_id_count ON status_reaction_counts (status_id, count DESC, emoji_key);

-- Current value: an account's private bookmark of a status.
CREATE TABLE bookmarks (
  account_id uuid NOT NULL,
  status_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  id uuid NOT NULL,
  PRIMARY KEY (account_id, status_id)
);
CREATE INDEX bookmarks_account_id_id ON bookmarks (account_id, id);
-- The retention purge keeps a bookmarked remote status (ADR-0034).
CREATE INDEX bookmarks_status_id ON bookmarks (status_id);

-- Current value: a status its author pinned to the profile (the Featured collection): a local
-- account's own pins, or a remote account's pins mirrored from its featured collection
-- (ADR-0044). Each pin holds one of the account's slots 1..5 (content.MaxPins), so the unique
-- slot index caps the pins of an account even under concurrent inserts.
CREATE TABLE status_pins (
  account_id uuid NOT NULL,
  status_id uuid NOT NULL,
  slot integer NOT NULL CHECK (slot BETWEEN 1 AND 5),
  created_at timestamptz NOT NULL,
  id uuid NOT NULL,
  PRIMARY KEY (account_id, status_id)
);
CREATE UNIQUE INDEX status_pins_account_id_slot ON status_pins (account_id, slot);
CREATE INDEX status_pins_account_id_id ON status_pins (account_id, id);

-- Current value: a local account's automatic deletion of its own old posts (FR-118). No row
-- means the default policy, which is disabled. Each keep_* flag is an exception that keeps a
-- post; keep_min_boosts and keep_min_favourites keep a post with at least that many, 0 for none.
CREATE TABLE status_auto_delete_policies (
  account_id uuid NOT NULL PRIMARY KEY,
  enabled boolean NOT NULL,
  min_age_days integer NOT NULL CHECK (min_age_days BETWEEN 1 AND 3650),
  keep_pinned boolean NOT NULL,
  keep_bookmarked boolean NOT NULL,
  keep_favourited boolean NOT NULL,
  keep_media boolean NOT NULL,
  keep_polls boolean NOT NULL,
  keep_min_boosts integer NOT NULL CHECK (keep_min_boosts BETWEEN 0 AND 1000000),
  keep_min_favourites integer NOT NULL CHECK (keep_min_favourites BETWEEN 0 AND 1000000),
  updated_at timestamptz NOT NULL
);
-- The hourly job walks the enabled policies in account order.
CREATE INDEX status_auto_delete_policies_enabled ON status_auto_delete_policies (account_id) WHERE enabled;

-- Current value: the visibilities whose posts an automatic deletion policy keeps.
CREATE TABLE status_auto_delete_kept_visibilities (
  account_id uuid NOT NULL,
  visibility text NOT NULL CHECK (visibility IN ('public', 'local', 'unlisted', 'private', 'direct')),
  PRIMARY KEY (account_id, visibility)
);

-- ---------------------------------------------------------------------------------------------
-- content: remote statuses and media (data-model §4.4, docs/architecture/federation.md §9).
-- Owned by internal/app/content.
-- ---------------------------------------------------------------------------------------------

-- Event (subtype of status_revisions): the activity that carried a remote revision (NULL for a
-- revision made from a fetch), the version of the HTML policy that sanitized its bodies, and the
-- number of media attachments of the received post that were left out after the first four.
CREATE TABLE remote_status_revisions (
  revision_id uuid NOT NULL PRIMARY KEY,
  activity_uri text,
  html_policy_version integer NOT NULL,
  omitted_attachment_count integer NOT NULL DEFAULT 0 CHECK (omitted_attachment_count >= 0)
);

-- Resource (subtype of media_files): a remote file as the received document declared it, and
-- what it was recorded for (purpose), which sizes the media proxy's small variant (ADR-0054).
-- The file itself is not fetched. owner_account_id copies media_files.owner_account_id.
CREATE TABLE remote_media (
  media_id uuid NOT NULL PRIMARY KEY,
  owner_account_id uuid NOT NULL,
  remote_url text NOT NULL,
  declared_content_type text,
  declared_media_type text NOT NULL CHECK (declared_media_type IN ('image', 'gifv', 'video', 'audio', 'unknown')),
  purpose text NOT NULL CHECK (purpose IN ('attachment', 'avatar', 'header', 'emoji')),
  width integer,
  height integer,
  blurhash text,
  focus_x double precision CHECK (focus_x BETWEEN -1.0 AND 1.0),
  focus_y double precision CHECK (focus_y BETWEEN -1.0 AND 1.0),
  description text
);
CREATE UNIQUE INDEX remote_media_owner_account_id_remote_url ON remote_media (owner_account_id, remote_url);

-- Current value (cache): a remote file copied into blob storage under remote/ (FR-409,
-- NFR-COST-4). The cleanup removes rows past expires_at and the least recently accessed ones.
CREATE TABLE remote_media_cache (
  url text NOT NULL PRIMARY KEY,
  blob_key text NOT NULL,
  content_type text NOT NULL,
  file_size bigint NOT NULL,
  fetched_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  last_accessed_at timestamptz NOT NULL
);
CREATE INDEX remote_media_cache_expires_at ON remote_media_cache (expires_at);
CREATE INDEX remote_media_cache_last_accessed_at ON remote_media_cache (last_accessed_at);

-- Current value (work list): a blob of the media files or of the remote media cache that is to be
-- deleted at or after delete_after, so that no blob is left that nothing refers to. A writer
-- records a key before it writes the blob, due after a grace period, and removes the row in the
-- transaction that records the blob; a deletion records the keys of the rows it deletes in the
-- same transaction, due at once. The cleanup jobs delete each due blob, then its row.
CREATE TABLE media_blob_deletions (
  blob_key text NOT NULL PRIMARY KEY,
  delete_after timestamptz NOT NULL
);
CREATE INDEX media_blob_deletions_delete_after ON media_blob_deletions (delete_after);

-- Current value (cache): the link card of a URL (ADR-0062), read from the page's head and shared
-- by every status that links the URL. url is the requested URL, normalized; card_url is where the
-- card links (the final URL, or its og:url of the same origin). A fetch upserts the row;
-- content.cleanup_link_cards deletes the rows that no status refers to 14 days after their fetch.
-- author_name is empty when the page names no author.
CREATE TABLE link_cards (
  url text NOT NULL PRIMARY KEY,
  card_url text NOT NULL,
  title text NOT NULL,
  description text NOT NULL,
  site_name text NOT NULL,
  author_name text NOT NULL,
  published_at timestamptz,
  image_media_id uuid,
  creator_account_id uuid,
  language text,
  fetched_at timestamptz NOT NULL
);
CREATE INDEX link_cards_fetched_at ON link_cards (fetched_at);
CREATE INDEX link_cards_image_media_id ON link_cards (image_media_id);

-- Resource (subtype of media_files): the image of link cards (ADR-0062 §3), fetched once to
-- check its format and read its size and blurhash; the file itself is not kept. Its owner is the
-- card's URL, not an account: media_files.owner_account_id is NULL. The media proxy serves it.
-- description is the og:image:alt, empty for none.
CREATE TABLE link_card_media (
  media_id uuid NOT NULL PRIMARY KEY,
  remote_url text NOT NULL,
  content_type text NOT NULL,
  width integer NOT NULL,
  height integer NOT NULL,
  blurhash text NOT NULL,
  description text NOT NULL
);
CREATE UNIQUE INDEX link_card_media_remote_url ON link_card_media (remote_url);

-- Projection: the URL whose card each status shows (ADR-0062 §1), chosen from the current
-- revision of a status that is not deleted. The status shows no card while link_cards has no
-- row of the URL.
CREATE TABLE status_link_cards (
  status_id uuid NOT NULL PRIMARY KEY,
  url text NOT NULL
);
CREATE INDEX status_link_cards_url ON status_link_cards (url);

-- Resource: the status that a status quotes (FR-114, ADR-0069 §1), fixed when the status is
-- created. quoted_uri may name a status that is not stored yet. form is fep_044f for
-- FEP-044f's quote, which its author approves, and legacy for the other spellings.
CREATE TABLE status_quotes (
  status_id uuid NOT NULL PRIMARY KEY,
  quoted_uri text NOT NULL,
  form text NOT NULL CHECK (form IN ('fep_044f', 'legacy'))
);
CREATE INDEX status_quotes_quoted_uri ON status_quotes (quoted_uri);

-- Event: a change of the approval of a quote (ADR-0069 §1). authorization_uri is the
-- QuoteAuthorization that an accepted event verified or a revoked event deleted.
CREATE TABLE status_quote_events (
  id uuid NOT NULL PRIMARY KEY,
  status_id uuid NOT NULL,
  kind text NOT NULL CHECK (kind IN ('accepted', 'rejected', 'revoked', 'detached')),
  authorization_uri text,
  occurred_at timestamptz NOT NULL,
  CONSTRAINT status_quote_events_authorization_uri_check CHECK (authorization_uri IS NULL OR kind IN ('accepted', 'revoked'))
);
CREATE INDEX status_quote_events_status_id_id ON status_quote_events (status_id, id);

-- Projection: the current approval of each quote, derived from its form and events
-- (content.QuoteStateOf). authorization_uri is the QuoteAuthorization of an accepted quote.
CREATE TABLE status_quote_states (
  status_id uuid NOT NULL PRIMARY KEY,
  state text NOT NULL CHECK (state IN ('pending', 'accepted', 'rejected', 'revoked', 'legacy', 'detached')),
  authorization_uri text,
  CONSTRAINT status_quote_states_authorization_uri_check CHECK (authorization_uri IS NULL OR state = 'accepted')
);
CREATE INDEX status_quote_states_authorization_uri ON status_quote_states (authorization_uri);

-- Current value: who may quote a status (ADR-0069 §1): what the author of a local status chose,
-- or what a remote status declares in interactionPolicy.canQuote. A remote status without a row
-- declares none; every local status has one.
CREATE TABLE status_quote_policies (
  status_id uuid NOT NULL PRIMARY KEY,
  policy text NOT NULL CHECK (policy IN ('public', 'followers', 'nobody')),
  updated_at timestamptz NOT NULL
);

-- Resource: this server's approval of a quote of a local status (FEP-044f's QuoteAuthorization,
-- ADR-0069 §1). quoting_uri is the quoting status, local or remote; its id is the IRI's last
-- segment. A quoting status has one per quoted status.
CREATE TABLE quote_authorizations (
  id uuid NOT NULL PRIMARY KEY,
  quoting_uri text NOT NULL,
  quoting_account_id uuid NOT NULL,
  quoted_status_id uuid NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX quote_authorizations_quoting_uri_quoted_status_id ON quote_authorizations (quoting_uri, quoted_status_id);
CREATE INDEX quote_authorizations_quoted_status_id_id ON quote_authorizations (quoted_status_id, id);

-- Event: the quoted author revoked a QuoteAuthorization; its IRI answers 410 from then on.
CREATE TABLE quote_authorization_revocations (
  authorization_id uuid NOT NULL PRIMARY KEY,
  revoked_at timestamptz NOT NULL
);

-- ---------------------------------------------------------------------------------------------
-- content: custom emoji and polls of statuses (data-model §4.4). Owned by internal/app/content.
-- ---------------------------------------------------------------------------------------------

-- Resource: a custom emoji, unique by its shortcode on a domain ('' for this instance). uri is
-- the id of a remote emoji's Emoji object. created_by_user_id is the user who made a local emoji
-- (ADR-0070 §7): the requester of an approved request, or the administrator who registered or
-- copied it; NULL for one made with the CLI and for a remote one. It never changes.
CREATE TABLE custom_emojis (
  id uuid NOT NULL PRIMARY KEY,
  shortcode text NOT NULL,
  domain text NOT NULL,
  uri text,
  created_by_user_id uuid,
  created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX custom_emojis_shortcode_domain ON custom_emojis (shortcode, domain);
CREATE UNIQUE INDEX custom_emojis_uri ON custom_emojis (uri);
-- The creators' own emoji (/custom_emojis/mine).
CREATE INDEX custom_emojis_created_by_user_id_id ON custom_emojis (created_by_user_id, id);

-- Event (revision): the image of a custom emoji. The newest row is the current image.
CREATE TABLE custom_emoji_images (
  id uuid NOT NULL PRIMARY KEY,
  emoji_id uuid NOT NULL,
  media_id uuid NOT NULL,
  revised_at timestamptz NOT NULL
);
CREATE INDEX custom_emoji_images_emoji_id_id ON custom_emoji_images (emoji_id, id DESC);
-- The remote media cache looks up whether a media file is a custom emoji image (avatars policy).
CREATE INDEX custom_emoji_images_media_id ON custom_emoji_images (media_id);

-- Current value: the settings of a custom emoji (ADR-0051 §1). No row means enabled, shown in the
-- picker, and without a category. A remote emoji uses enabled only.
CREATE TABLE custom_emoji_settings (
  emoji_id uuid NOT NULL PRIMARY KEY,
  enabled boolean NOT NULL,
  visible_in_picker boolean NOT NULL,
  category text CHECK (category IS NULL OR length(category) BETWEEN 1 AND 64)
);

-- Current value: the other names of a local custom emoji that the picker finds it by (ADR-0051
-- §1), of the characters of a shortcode.
CREATE TABLE custom_emoji_aliases (
  emoji_id uuid NOT NULL,
  alias text NOT NULL,
  PRIMARY KEY (emoji_id, alias)
);

-- Resource: a request of a holder of create_custom_emojis (ADR-0070 §2, §3): to create a local
-- custom emoji (kind create, emoji_id NULL) or to replace the image of one the requester made
-- (kind image, emoji_id the emoji, shortcode a copy of its own). media_id is an upload of the
-- requester's local account. Kept after it ends, for the requester's history and the audit log.
CREATE TABLE custom_emoji_requests (
  id uuid NOT NULL PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN ('create', 'image')),
  requester_user_id uuid NOT NULL,
  emoji_id uuid,
  shortcode text NOT NULL,
  media_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  CONSTRAINT custom_emoji_requests_emoji_check CHECK ((kind = 'create') = (emoji_id IS NULL))
);
CREATE INDEX custom_emoji_requests_requester_user_id_id ON custom_emoji_requests (requester_user_id, id);
-- The media cleanup keeps the upload of a pending request.
CREATE INDEX custom_emoji_requests_media_id ON custom_emoji_requests (media_id);

-- Event: the end of a custom emoji request, at most one per request; a request without one is
-- pending. decided_by_user_id is NULL for the CLI and for lapsed (a request to replace the image
-- of an emoji that was deleted); reason is the optional reason of a rejection; emoji_id is the
-- emoji that an approval made or changed.
CREATE TABLE custom_emoji_request_outcomes (
  id uuid NOT NULL PRIMARY KEY,
  request_id uuid NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('approved', 'rejected', 'withdrawn', 'lapsed')),
  decided_by_user_id uuid,
  reason text CHECK (reason IS NULL OR char_length(reason) BETWEEN 1 AND 500),
  emoji_id uuid,
  decided_at timestamptz NOT NULL,
  CONSTRAINT custom_emoji_request_outcomes_rejected_check CHECK (reason IS NULL OR outcome = 'rejected'),
  CONSTRAINT custom_emoji_request_outcomes_approved_check CHECK ((outcome = 'approved') = (emoji_id IS NOT NULL))
);
CREATE UNIQUE INDEX custom_emoji_request_outcomes_request_id ON custom_emoji_request_outcomes (request_id);

-- Projection: the pending custom emoji requests, written with the request and deleted with its
-- outcome in the same transaction; rebuilt from custom_emoji_requests without an outcome.
-- reserved_shortcode is the shortcode that a request to create reserves, which no other request
-- and no new local emoji may take; emoji_id is the emoji of a request to replace an image, at
-- most one pending per emoji.
CREATE TABLE custom_emoji_pending_requests (
  request_id uuid NOT NULL PRIMARY KEY,
  requester_user_id uuid NOT NULL,
  reserved_shortcode text,
  emoji_id uuid,
  CONSTRAINT custom_emoji_pending_requests_kind_check CHECK ((reserved_shortcode IS NULL) <> (emoji_id IS NULL))
);
CREATE UNIQUE INDEX custom_emoji_pending_requests_reserved_shortcode ON custom_emoji_pending_requests (reserved_shortcode);
CREATE UNIQUE INDEX custom_emoji_pending_requests_emoji_id ON custom_emoji_pending_requests (emoji_id);
CREATE INDEX custom_emoji_pending_requests_requester_user_id ON custom_emoji_pending_requests (requester_user_id);

-- Event (child of a revision): the poll of a status revision (FR-112). closes_at is NULL for a
-- poll without an end; hide_totals hides the counts of a local poll until it ends.
CREATE TABLE polls (
  id uuid NOT NULL PRIMARY KEY,
  revision_id uuid NOT NULL,
  multiple boolean NOT NULL,
  hide_totals boolean NOT NULL,
  closes_at timestamptz
);
CREATE UNIQUE INDEX polls_revision_id ON polls (revision_id);

-- Event: a remote status taken in only because a relay sent it (ADR-0061), kept for
-- relay_status_retention instead of remote_status_retention. relay_id is the relay it came from,
-- kept as an ID after the relay is deleted. Purged with the status.
CREATE TABLE relayed_statuses (
  status_id uuid NOT NULL PRIMARY KEY,
  relay_id uuid NOT NULL,
  received_at timestamptz NOT NULL
);

-- Event (child of a poll): one choice of a poll.
CREATE TABLE poll_options (
  poll_id uuid NOT NULL,
  position integer NOT NULL CHECK (position >= 0),
  title text NOT NULL,
  PRIMARY KEY (poll_id, position)
);

-- Event: one choice an account voted for in a poll (FR-112): a vote with several choices is one
-- row per choice. uri is the id of the Note that carries the choice to a remote poll's server,
-- or that a remote voter sent; votes are never taken back.
CREATE TABLE poll_votes (
  id uuid NOT NULL PRIMARY KEY,
  poll_id uuid NOT NULL,
  account_id uuid NOT NULL,
  position integer NOT NULL CHECK (position >= 0),
  uri text,
  voted_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX poll_votes_poll_id_account_id_position ON poll_votes (poll_id, account_id, position);
CREATE UNIQUE INDEX poll_votes_uri ON poll_votes (uri);
CREATE INDEX poll_votes_account_id ON poll_votes (account_id);

-- Event: the counts of a poll: of a remote poll as its server reported them (in a Create or an
-- Update), of a local poll as its votes counted when it was posted and after each vote.
CREATE TABLE poll_tally_observations (
  id uuid NOT NULL PRIMARY KEY,
  poll_id uuid NOT NULL,
  voters_count bigint CHECK (voters_count >= 0),
  observed_at timestamptz NOT NULL
);
CREATE INDEX poll_tally_observations_poll_id_id ON poll_tally_observations (poll_id, id DESC);

-- Event (child of an observation): the votes of one choice.
CREATE TABLE poll_tally_observation_counts (
  observation_id uuid NOT NULL,
  position integer NOT NULL CHECK (position >= 0),
  votes bigint NOT NULL CHECK (votes >= 0),
  PRIMARY KEY (observation_id, position)
);

-- Projection: the votes of each choice of a poll, from its newest observation.
-- voters_count is NULL when the server did not report it.
CREATE TABLE poll_tallies (
  poll_id uuid NOT NULL,
  position integer NOT NULL CHECK (position >= 0),
  votes bigint NOT NULL CHECK (votes >= 0),
  voters_count bigint CHECK (voters_count >= 0),
  PRIMARY KEY (poll_id, position)
);

-- moderation and content: actions on statuses (#364, data-model §4.3, §4.4) -------------------

-- Projection (content): a status that a mark_statuses_sensitive action shows as sensitive, until
-- the action is lifted.
CREATE TABLE status_sensitivity_overrides (
  status_id uuid NOT NULL,
  source_action_id uuid NOT NULL,
  PRIMARY KEY (status_id, source_action_id)
);
CREATE INDEX status_sensitivity_overrides_source_action_id ON status_sensitivity_overrides (source_action_id);

-- The erasure of deleted local accounts' data (account, #397, data-model §4.2, §8) looks up
-- the rows of an account.
CREATE INDEX boosts_booster_account_id ON boosts (booster_account_id);

-- ---------------------------------------------------------------------------------------------
-- content: the local posts that NodeInfo reports (#518, data-model §4.4). Owned by
-- internal/app/content.
-- ---------------------------------------------------------------------------------------------

-- Projection: the posts of each local account as Mastodon counts them for NodeInfo's
-- usage.localPosts: the statuses that are not deleted and not direct, and the boosts that are not
-- retracted, of statuses that are not deleted. No row means none.
CREATE TABLE local_post_counters (
  account_id uuid NOT NULL PRIMARY KEY,
  posts_count bigint NOT NULL CHECK (posts_count > 0)
);
