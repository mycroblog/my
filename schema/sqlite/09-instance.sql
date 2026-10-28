-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the instance module
-- (docs/architecture/data-model/04-tables/09-instance.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- ---------------------------------------------------------------------------------------------
-- instance: settings and texts (data-model §4.9). Owned by internal/app/instance.
-- Keys are defined in internal/domain/instance; add a key there and to its CHECK here together.
-- ---------------------------------------------------------------------------------------------

-- Event (revision): one change of a setting. value is the canonical string form of the key's
-- scalar type. revised_by_user_id is NULL for a change from the CLI.
CREATE TABLE instance_setting_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  key TEXT NOT NULL CHECK (key IN (
    'registration_mode', 'max_status_length', 'default_locale', 'local_timeline_access', 'federated_timeline_access',
    'preview_image', 'contact_account', 'contact_email', 'thumbnail', 'remote_media_policy', 'remote_media_cache_ttl', 'authorized_fetch',
    'domain_actions_visibility', 'domain_actions_rationale_visibility', 'appeal_window',
    'retention_session_origins', 'retention_registration_origins', 'retention_reports',
    'retention_moderation_actions', 'retention_audit_log',
    'search_indexing', 'ai_usage', 'indexable_default', 'security_contact', 'require_media_description', 'link_cards_enabled',
    'finance_visibility', 'finance_viewer_item_names', 'finance_viewer_item_memos', 'finance_viewer_income',
    'supporter_list_visibility', 'income_visibility', 'federation_mode', 'oauth_metadata_document_clients', 'dynamic_client_registration',
    'user_app_registration', 'voice_room_starters', 'voice_public_summary', 'retention_voice_participants',
    'invite_creators', 'member_invite_max_active', 'member_invite_max_uses', 'member_invite_max_expires_in'
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
-- instance: theme (data-model §4.9, ADR-0059, FR-605). Owned by internal/app/instance.
-- ---------------------------------------------------------------------------------------------

-- Event (revision): one version of the instance theme: the administrator's custom CSS, with its
-- token overrides in instance_theme_tokens. A revision without tokens and CSS resets the theme.
-- revised_by_user_id is NULL for a change from the CLI.
CREATE TABLE instance_theme_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  custom_css TEXT NOT NULL,
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL
) STRICT;

-- Event (child of a revision): the value of one overridden design token in one color scheme
-- (light and dark for a color token, all for the others). The token list is generated from the
-- web client's theme.css (internal/domain/instance/theme_tokens.gen.go).
CREATE TABLE instance_theme_tokens (
  theme_revision_id TEXT NOT NULL,
  token TEXT NOT NULL CHECK (token IN (
    'color-surface', 'color-surface-raised', 'color-surface-sunken', 'color-soft', 'color-fg',
   'color-fg-muted', 'color-accent', 'color-accent-hover', 'color-accent-fg', 'color-accent-fill',
   'color-accent-fill-hover', 'color-accent-fill-fg', 'color-danger', 'color-danger-fg',
   'color-danger-bg', 'color-success', 'color-success-fg', 'color-success-bg', 'color-warning',
   'color-warning-fg', 'color-warning-bg', 'color-border', 'color-border-strong', 'color-focus',
   'color-toast', 'color-toast-fg', 'color-toast-action', 'color-toast-success',
   'color-toast-danger', 'color-toast-muted', 'color-overlay', 'color-alt-overlay', 'color-highlight-mention', 'color-highlight-hashtag', 'color-highlight-url',
   'color-highlight-shortcode', 'color-highlight-over', 'color-pressed', 'color-pressed-fg', 'color-media-chip', 'color-wave', 'color-spark', 'color-data-1', 'color-data-2', 'color-data-3', 'font-sans',
   'font-mono', 'font-serif', 'text-xs', 'text-xs--line-height', 'text-sm', 'text-sm--line-height',
   'text-base', 'text-base--line-height', 'text-lg', 'text-lg--line-height', 'text-xl',
   'text-xl--line-height', 'text-2xl', 'text-2xl--line-height', 'text-3xl', 'text-3xl--line-height',
   'spacing', 'radius-sm', 'radius-md', 'radius-lg', 'radius-xl', 'radius-2xl', 'elevation-card',
   'elevation-sm', 'elevation-md', 'elevation-lg', 'ease-standard', 'ease-enter', 'ease-exit', 'ease-celebrate',
   'transition-duration-fast', 'transition-duration-normal', 'transition-duration-slow',
   'default-transition-duration', 'default-transition-timing-function'
  )),
  scheme TEXT NOT NULL CHECK (scheme IN ('light', 'dark', 'all')),
  value TEXT NOT NULL,
  PRIMARY KEY (theme_revision_id, token, scheme)
) STRICT;

-- Projection: the current theme revision, one row once a theme has been saved.
CREATE TABLE instance_theme (
  id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
  revision_id TEXT NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- instance: rules (data-model §4.9, FR-705). Owned by internal/app/instance.
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
-- instance: server-wide announcements (data-model §4.9, FR-726). Owned by internal/app/instance.
-- ---------------------------------------------------------------------------------------------

-- Resource: an announcement to every local user.
CREATE TABLE announcements (
  id TEXT NOT NULL PRIMARY KEY,
  created_at INTEGER NOT NULL
) STRICT;

-- Event (revision): one version of an announcement: its body in Markdown and the window in which
-- it shows once published. A NULL starts_at shows it from its publication, a NULL ends_at until
-- it is withdrawn. revised_by_user_id is NULL for a change from the CLI.
CREATE TABLE announcement_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  announcement_id TEXT NOT NULL,
  body TEXT NOT NULL,
  starts_at INTEGER,
  ends_at INTEGER,
  revised_by_user_id TEXT,
  revised_at INTEGER NOT NULL,
  CHECK (starts_at IS NULL OR ends_at IS NULL OR ends_at > starts_at)
) STRICT;
CREATE INDEX announcement_revisions_announcement_id_id ON announcement_revisions (announcement_id, id DESC);

-- Event: an announcement was published or withdrawn; a withdrawal is for good.
-- changed_by_user_id is NULL for the CLI.
CREATE TABLE announcement_state_changes (
  id TEXT NOT NULL PRIMARY KEY,
  announcement_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('published', 'withdrawn')),
  changed_by_user_id TEXT,
  changed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX announcement_state_changes_announcement_id_id ON announcement_state_changes (announcement_id, id DESC);

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
-- instance: the service closure (data-model §4.9, FR-808, ADR-0088). Owned by
-- internal/app/instance. The transitions are checked in internal/domain/instance (closure.go).
-- ---------------------------------------------------------------------------------------------

-- Event: one change of the service closure: an announcement (or a change of its dates), its
-- withdrawal, the closure, and the end of the closure's deliveries. by_user_id is NULL for the
-- CLI and for the server itself.
CREATE TABLE service_closure_events (
  id TEXT NOT NULL PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('announced', 'withdrawn', 'closed', 'delivery_completed')),
  by_user_id TEXT,
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
