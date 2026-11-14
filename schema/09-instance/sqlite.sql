-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the instance module
-- (their data model: the package comment of internal/domain/instance). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- instance: settings and texts. Owned by internal/app/instance.
-- Keys are defined in internal/domain/instance; add a key there and to its CHECK here together.
-- ---------------------------------------------------------------------------------------------

-- Event (revision): one change of a setting. value is the canonical string form of the key's
-- type: a scalar, or one of the structured values no query filters by item (ADR-0020 rule 4): the
-- ordered list of usernames of featured_accounts, the ordered list of providers of
-- translation_providers, the provider=visibility|visibility pairs of
-- translation_provider_visibilities, and the client_id=requests pairs of oauth_client_rate_limits.
-- revised_by_user_id is NULL for a change from the CLI.
CREATE TABLE instance_setting_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  key TEXT NOT NULL CHECK (key IN (
    'registration_mode', 'max_status_length', 'default_locale', 'local_timeline_visibility', 'federated_timeline_visibility',
    'preview_image', 'contact_account', 'contact_email', 'thumbnail', 'remote_media_policy', 'remote_media_cache_ttl_seconds', 'authorized_fetch',
    'domain_actions_visibility', 'domain_actions_rationale_visibility', 'appeal_window_days',
    'session_origins_retention_days', 'registration_origins_retention_days', 'reports_retention_days',
    'moderation_actions_retention_days', 'audit_log_retention_days',
    'search_indexing', 'ai_usage', 'indexable_default', 'security_contact', 'require_media_description', 'link_cards', 'link_cards_ignore_robots_txt',
    'finance_visibility', 'finance_viewer_item_names', 'finance_viewer_item_memos', 'finance_viewer_income',
    'supporter_list_visibility', 'income_visibility', 'federation_mode', 'federation_scope', 'oauth_metadata_document_clients', 'dynamic_client_registration',
    'user_app_registration', 'voice_room_starters', 'voice_public_summary', 'voice_participants_retention_days',
    'invite_creators', 'member_invite_max_active', 'member_invite_max_uses', 'member_invite_max_lifetime_seconds',
    'public_counts', 'featured_accounts', 'login_providers_only',
    'remote_media_cache_max_bytes', 'remote_status_retention_days', 'remote_status_max_retention_days',
    'relay_status_retention_days', 'oauth_client_rate_limits', 'search_visibilities',
    'translation_providers', 'translation_provider_visibilities', 'translation_browser',
    'translation_user_monthly_characters', 'translation_server_monthly_characters', 'translation_retention_days',
    'translation_reports_to_server',
    'auto_delete_default_enabled', 'auto_delete_default_min_age_days', 'auto_delete_default_keep_pinned',
    'auto_delete_default_keep_bookmarked', 'auto_delete_default_keep_favourited', 'auto_delete_default_keep_media',
    'auto_delete_default_keep_polls', 'auto_delete_default_keep_public', 'auto_delete_default_keep_local',
    'auto_delete_default_keep_unlisted', 'auto_delete_default_keep_private', 'auto_delete_default_keep_direct',
    'auto_delete_default_keep_min_boosts', 'auto_delete_default_keep_min_favourites',
    'auto_delete_max_age_days', 'auto_delete_max_allow_pinned', 'auto_delete_max_kept_pins',
    'auto_delete_max_allow_bookmarked', 'auto_delete_max_allow_favourited', 'auto_delete_max_allow_media',
    'auto_delete_max_allow_polls', 'auto_delete_max_allow_visibilities', 'auto_delete_max_allow_thresholds',
    'auto_delete_grace_days',
    'hook_inbound_status_mode', 'hook_inbound_follow_mode', 'hook_signup_check_mode', 'hook_media_check_mode', 'hook_local_status_mode', 'hook_inbound_actor_mode', 'hook_local_profile_mode',
    'hook_inbound_status_on_failure', 'hook_inbound_follow_on_failure', 'hook_signup_check_on_failure', 'hook_media_check_on_failure', 'hook_inbound_actor_on_failure',
    'hook_signup_check_reject', 'hook_local_status_account_age_days', 'hook_local_profile_account_age_days',
    'hook_inbound_status_unfollowed_direct',
    'hook_decisions_retention_days',
    'single_user',
    'max_drafts', 'max_scheduled_posts', 'max_scheduled_posts_per_day', 'scheduled_post_min_lead_minutes', 'scheduled_post_max_lead_days',
    'article_authoring', 'max_article_bytes', 'max_article_images', 'auto_delete_default_keep_articles', 'auto_delete_max_allow_articles', 'event_creators',
    'event_records_retention_days',
    'funding_meter_visibility', 'funding_meter_target_amount', 'funding_meter_target_currency', 'funding_meter_rail',
    'ux_measurement'
  )),
  value TEXT NOT NULL,
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL
) STRICT;
CREATE INDEX instance_setting_revisions_key_id ON instance_setting_revisions (key, id DESC);

-- Projection: the current value of each setting that has been set. Unset keys use the default
-- defined in code.
CREATE TABLE instance_settings (
  key TEXT NOT NULL PRIMARY KEY,
  revision_id TEXT NOT NULL,
  value TEXT NOT NULL
) STRICT;

-- Event (revision): one version of an instance text, one value shown in every locale. An empty
-- body clears the text. revised_by_user_id is NULL for a change from the CLI.
CREATE TABLE instance_text_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  key TEXT NOT NULL CHECK (key IN (
    'name', 'short_description', 'description', 'terms', 'privacy_policy', 'preview_image_alt', 'thumbnail_alt',
    'commerce_disclosure'
  )),
  body TEXT NOT NULL,
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL
) STRICT;
CREATE INDEX instance_text_revisions_key_id ON instance_text_revisions (key, id DESC);

-- Projection: the current revision of each text.
CREATE TABLE instance_texts (
  key TEXT NOT NULL PRIMARY KEY,
  revision_id TEXT NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- instance: theme (ADR-0059, FR-605). Owned by internal/app/instance.
-- ---------------------------------------------------------------------------------------------

-- Event: one version of the theme as the releases before the token set stored it, with its token
-- overrides row by row in instance_theme_tokens. Nothing writes it: the data upgrade step
-- theme.token_sets copies each revision into a set and its publication, and a later release
-- drops both tables.
CREATE TABLE instance_theme_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  custom_css TEXT NOT NULL,
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL
) STRICT;

-- Event (child of a revision): the value of one overridden token in one color scheme (light and
-- dark for a color token, all for the others). Read only by theme.token_sets.
CREATE TABLE instance_theme_tokens (
  revision_id TEXT NOT NULL,
  token TEXT NOT NULL,
  scheme TEXT NOT NULL CHECK (scheme IN ('light', 'dark', 'all')),
  value TEXT NOT NULL,
  PRIMARY KEY (revision_id, token, scheme)
) STRICT;

-- Event: one saved token set of the theme (ADR-0059 §15), as a versioned JSON document of the
-- overrides with their light and dark values, replaced as a whole by the next set. The token names
-- are checked by the domain against the list generated from @my/design-tokens
-- (internal/domain/instance/theme_tokens.gen.go), never by a constraint, so that a renamed or
-- retired token needs no schema change. saved_by_user_id is NULL for the CLI and the data upgrade.
CREATE TABLE instance_theme_sets (
  id TEXT NOT NULL PRIMARY KEY,
  document BLOB NOT NULL,
  saved_by_user_id TEXT,
  saved_at INTEGER NOT NULL
) STRICT;

-- Event: one publication of a set, which makes it the theme in effect. Its ID is the theme's
-- revision, which names the stylesheet. published_by_user_id is NULL for the CLI and the data
-- upgrade.
CREATE TABLE instance_theme_publications (
  id TEXT NOT NULL PRIMARY KEY,
  set_id TEXT NOT NULL,
  published_by_user_id TEXT,
  published_at INTEGER NOT NULL
) STRICT;

-- Projection: the theme in effect, the publication with the greatest ID, one row once a set has
-- been published.
CREATE TABLE instance_theme (
  id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
  revision_id TEXT NOT NULL
) STRICT;

-- Projection: the draft, the set with the greatest ID, one row once a set has been saved.
CREATE TABLE instance_theme_draft (
  id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
  set_id TEXT NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- instance: brand images (ADR-0059). Owned by internal/app/instance.
-- The files live in the BlobStore under keys the server makes from the revision ID.
-- ---------------------------------------------------------------------------------------------

-- Event (revision): one change of the icon, the logo, or the dark logo. A set revision has its
-- files in instance_brand_files; a clear revision has none and brings back the built-in images.
-- background_color is the #rrggbb color behind the opaque renditions of a set icon, NULL
-- otherwise. revised_by_user_id is NULL for a change from the CLI.
CREATE TABLE instance_brand_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('icon', 'logo', 'logo_dark')),
  action TEXT NOT NULL CHECK (action IN ('set', 'clear')),
  background_color TEXT CHECK (background_color IS NULL OR background_color GLOB '#[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]'),
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL,
  CHECK ((kind = 'icon' AND action = 'set') = (background_color IS NOT NULL))
) STRICT;

-- Event (child of a set revision): one stored file, the sanitized SVG or one of its PNG
-- renditions. width and height are pixels for a PNG and the viewBox, rounded, for the SVG;
-- sha256 is the hex SHA-256 of the content.
CREATE TABLE instance_brand_files (
  revision_id TEXT NOT NULL,
  rendition TEXT NOT NULL CHECK (rendition IN (
    'svg', 'favicon-32', 'favicon-48', 'apple-touch-180', 'pwa-192', 'pwa-512', 'maskable-512',
    'logo-mail', 'logo-dark-mail', 'preview-1200x630'
  )),
  blob_key TEXT NOT NULL,
  content_type TEXT NOT NULL CHECK (content_type IN ('image/svg+xml', 'image/png')),
  width INTEGER NOT NULL CHECK (width > 0),
  height INTEGER NOT NULL CHECK (height > 0),
  file_size INTEGER NOT NULL CHECK (file_size > 0),
  sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
  PRIMARY KEY (revision_id, rendition)
) STRICT;

-- Projection: the current set revision of each kind; a clear revision deletes the row.
CREATE TABLE instance_brands (
  kind TEXT NOT NULL PRIMARY KEY CHECK (kind IN ('icon', 'logo', 'logo_dark')),
  revision_id TEXT NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- instance: rules (FR-705). Owned by internal/app/instance.
-- A rule keeps its ID across revisions, so reports cite it by rule_id; retired rules stay.
-- ---------------------------------------------------------------------------------------------

-- Resource: a rule of the instance.
CREATE TABLE instance_rules (
  id TEXT NOT NULL PRIMARY KEY,
  created_at INTEGER NOT NULL
) STRICT;

-- Event (revision): one version of a rule, with its position, its text, and its hint (an optional
-- explanation, empty for none), one value shown in every locale. revised_by_user_id is NULL for a
-- change from the CLI.
CREATE TABLE instance_rule_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  rule_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  text TEXT NOT NULL,
  hint TEXT NOT NULL,
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL
) STRICT;
CREATE INDEX instance_rule_revisions_rule_id_id ON instance_rule_revisions (rule_id, id DESC);

-- Event: the retirement of a rule. retired_by_user_id is NULL for the CLI.
CREATE TABLE instance_rule_retirements (
  rule_id TEXT NOT NULL PRIMARY KEY,
  retired_by_user_id TEXT,
  retired_at INTEGER NOT NULL
) STRICT;

-- Projection: the current revision and position of each rule in effect (not retired).
CREATE TABLE instance_rule_heads (
  rule_id TEXT NOT NULL PRIMARY KEY,
  revision_id TEXT NOT NULL,
  position INTEGER NOT NULL
) STRICT;
CREATE INDEX instance_rule_heads_position ON instance_rule_heads (position);

-- ---------------------------------------------------------------------------------------------
-- instance: the rules of the link card crawler (FR-117). Owned by
-- internal/app/instance.
-- ---------------------------------------------------------------------------------------------

-- Current value: the rule of the link card crawler for a domain and its subdomains, keyed by the
-- canonical host: fetch nothing (no_fetch) or fetch no images (no_images). note is what the
-- administrators keep about the rule, '' for none. The audit log keeps the history of the changes.
CREATE TABLE link_card_domain_rules (
  domain TEXT NOT NULL PRIMARY KEY,
  mode TEXT NOT NULL CHECK (mode IN ('no_fetch', 'no_images')),
  note TEXT NOT NULL,
  updated_at INTEGER NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- instance: server-wide announcements (FR-726). Owned by internal/app/instance.
-- ---------------------------------------------------------------------------------------------

-- Resource: an announcement to every local user.
CREATE TABLE announcements (
  id TEXT NOT NULL PRIMARY KEY,
  created_at INTEGER NOT NULL
) STRICT;

-- Event (revision): one version of an announcement: its title (plain text of 1 to 100 grapheme
-- clusters, checked by the application; the default '' reads as no title), its body in Markdown,
-- and the window in which it shows once published. A NULL starts_at shows it from its
-- publication, a NULL ends_at until it is withdrawn. revised_by_user_id is NULL for a change from
-- the CLI.
CREATE TABLE announcement_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  announcement_id TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  starts_at INTEGER,
  ends_at INTEGER,
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL,
  CHECK (starts_at IS NULL OR ends_at IS NULL OR ends_at > starts_at)
) STRICT;

-- Event: an announcement was published or withdrawn; a withdrawal is for good.
-- changed_by_user_id is NULL for the CLI.
CREATE TABLE announcement_state_changes (
  id TEXT NOT NULL PRIMARY KEY,
  announcement_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('published', 'withdrawn')),
  changed_by_user_id TEXT,
  changed_at INTEGER NOT NULL
) STRICT;

-- Projection: the current revision and state of each announcement. published_at is the time of
-- its latest publication, set exactly when it has been published.
CREATE TABLE announcement_heads (
  announcement_id TEXT NOT NULL PRIMARY KEY,
  revision_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('draft', 'published', 'withdrawn')),
  published_at INTEGER,
  CHECK ((state = 'draft') = (published_at IS NULL))
) STRICT;
CREATE INDEX announcement_heads_state ON announcement_heads (state);

-- Current value: the announcements a user has read (ADR-0020 rule 5: the history means nothing).
CREATE TABLE announcement_reads (
  user_id TEXT NOT NULL,
  announcement_id TEXT NOT NULL,
  read_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, announcement_id)
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- instance: the service closure (FR-808, ADR-0088). Owned by
-- internal/app/instance. The transitions are checked in internal/domain/instance (closure.go).
-- ---------------------------------------------------------------------------------------------

-- Event: one change of the service closure: an announcement (or a change of its dates), its
-- withdrawal, the closure, and the end of the closure's deliveries. by_user_id is NULL for the
-- CLI and for the server itself.
CREATE TABLE service_closure_events (
  id TEXT NOT NULL PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('announced', 'withdrawn', 'closed', 'delivery_completed')),
  changed_by_user_id TEXT,
  occurred_at INTEGER NOT NULL
) STRICT;

-- Event (subtype of an announced event): the schedule that the announcement states: the closing
-- date, until when the users may export their data, and the operator's message ('' for none).
CREATE TABLE service_closure_schedules (
  event_id TEXT NOT NULL PRIMARY KEY,
  closes_at INTEGER NOT NULL,
  export_until INTEGER NOT NULL,
  message TEXT NOT NULL,
  CHECK (export_until >= closes_at)
) STRICT;

-- Projection: the current state of the service closure, one row while a closure is announced or
-- has happened; no row is a server without a closure. announcement_event_id is the announcement
-- whose schedule is in effect, event_id the latest event.
CREATE TABLE service_closure_heads (
  id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
  state TEXT NOT NULL CHECK (state IN ('announced', 'closed', 'delivery_completed')),
  event_id TEXT NOT NULL,
  announcement_event_id TEXT NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- instance: secrets of the instance (ADR-0019 §12). Owned by internal/app/instance.
-- Names are defined in internal/domain/instance.
-- ---------------------------------------------------------------------------------------------

-- Resource: a secret that the server generates once, such as the signing key of the media proxy
-- URLs, sealed with the Sealer bound to its id. `my kek rewrap` re-wraps sealed_secret.
CREATE TABLE instance_secrets (
  id TEXT NOT NULL PRIMARY KEY,
  name TEXT NOT NULL,
  sealed_secret BLOB NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX instance_secrets_name ON instance_secrets (name);

-- ---------------------------------------------------------------------------------------------
-- instance: domains of the instance (ADR-0018). Owned by internal/app/instance.
-- ---------------------------------------------------------------------------------------------

-- Resource: the host and account_domain that this database belongs to, one row written once. The
-- server refuses to start when the configuration differs, since other servers store ids and
-- handles made from them.
CREATE TABLE instance_domains (
  id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
  host TEXT NOT NULL,
  account_domain TEXT NOT NULL,
  recorded_at INTEGER NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- instance: TLS certificates that the server serves itself (ADR-0163 §4). Owned
-- by internal/app/instance. The current certificate of a host is the last one issued that is not
-- revoked, by created_at; the wait before the next order is derived from the attempts after it.
-- ---------------------------------------------------------------------------------------------

-- Resource: the ACME account of a CA, one per directory URL. Two processes that register at once
-- agree on the row inserted first. sealed_private_key is the account key (PKCS #8), sealed with the
-- Sealer bound to its id; `my kek rewrap` re-wraps it.
CREATE TABLE tls_acme_accounts (
  id TEXT NOT NULL PRIMARY KEY,
  directory_url TEXT NOT NULL,
  account_url TEXT NOT NULL,
  sealed_private_key BLOB NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX tls_acme_accounts_directory_url ON tls_acme_accounts (directory_url);

-- Event: an order of a certificate for host, written just before the order. profile is '' for the
-- CA's default; replaces_certificate_id is NULL for a first order. Its result is a row of
-- tls_order_failures or tls_certificates; an attempt whose process died has neither.
CREATE TABLE tls_order_attempts (
  id TEXT NOT NULL PRIMARY KEY,
  host TEXT NOT NULL,
  directory_url TEXT NOT NULL,
  profile TEXT NOT NULL,
  replaces_certificate_id TEXT,
  occurred_at INTEGER NOT NULL
) STRICT;
CREATE INDEX tls_order_attempts_host_occurred_at ON tls_order_attempts (host, occurred_at);

-- Event (subtype of the result of an attempt): the failure of an order. problem_type is the type
-- of the RFC 8555 problem, or the class of a failure on the client's side.
CREATE TABLE tls_order_failures (
  attempt_id TEXT NOT NULL PRIMARY KEY,
  problem_type TEXT NOT NULL,
  detail TEXT NOT NULL CHECK (length(detail) <= 1000),
  occurred_at INTEGER NOT NULL
) STRICT;

-- Resource (subtype of the result of an attempt), insert only: an issued certificate. chain_pem is
-- public; sealed_private_key is its key (PKCS #8), sealed with the Sealer bound to its id; `my kek
-- rewrap` re-wraps it.
CREATE TABLE tls_certificates (
  id TEXT NOT NULL PRIMARY KEY,
  attempt_id TEXT NOT NULL,
  host TEXT NOT NULL,
  directory_url TEXT NOT NULL,
  serial TEXT NOT NULL,
  not_before INTEGER NOT NULL,
  not_after INTEGER NOT NULL,
  chain_pem TEXT NOT NULL,
  sealed_private_key BLOB NOT NULL,
  created_at INTEGER NOT NULL,
  CHECK (not_after > not_before)
) STRICT;
CREATE UNIQUE INDEX tls_certificates_attempt_id ON tls_certificates (attempt_id);
CREATE INDEX tls_certificates_host_created_at ON tls_certificates (host, created_at);

-- Event: the revocation of a certificate, written after the CA revoked it. revoked_by_user_id is
-- NULL for the CLI.
CREATE TABLE tls_certificate_revocations (
  id TEXT NOT NULL PRIMARY KEY,
  certificate_id TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (reason IN ('key_compromise', 'superseded', 'operator')),
  revoked_by_user_id TEXT,
  occurred_at INTEGER NOT NULL
) STRICT;
CREATE INDEX tls_certificate_revocations_certificate_id ON tls_certificate_revocations (certificate_id);
