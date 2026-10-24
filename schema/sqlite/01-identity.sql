-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Desired-state schema for SQLite (ADR-0023): the tables of the identity module
-- (docs/architecture/data-model/04-tables/01-identity.md). schema/README.md describes the layout
-- and the conventions of this directory.

-- identity (data-model §4.1) -----------------------------------------------------------------

-- Resource: a login principal.
CREATE TABLE users (
  id TEXT NOT NULL PRIMARY KEY,
  created_at INTEGER NOT NULL
) STRICT;

-- Event (revision): the newest row of a user is the current password.
CREATE TABLE password_credentials (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  set_at INTEGER NOT NULL
) STRICT;
CREATE INDEX password_credentials_user_id_id ON password_credentials (user_id, id DESC);

-- Event: a user removed their password (FR-004), which then no longer signs in. It ends the
-- password credential it names, the newest one of the user; a newer credential sets a password
-- again.
CREATE TABLE password_credential_removals (
  password_credential_id TEXT NOT NULL PRIMARY KEY,
  removed_at INTEGER NOT NULL
) STRICT;

-- Event: a TOTP authenticator a user enrolled (FR-005). The secret is sealed with kernel.Sealer
-- (ADR-0019 §6). The active enrollment is the newest one without a revocation.
CREATE TABLE totp_enrollments (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  secret_enc BLOB NOT NULL,
  enrolled_at INTEGER NOT NULL
) STRICT;
CREATE INDEX totp_enrollments_user_id_id ON totp_enrollments (user_id, id DESC);

-- Event: a TOTP enrollment ended (disabled by its user, replaced, or removed by an administrator).
CREATE TABLE totp_enrollment_revocations (
  enrollment_id TEXT NOT NULL PRIMARY KEY,
  revoked_at INTEGER NOT NULL
) STRICT;

-- Resource: a recovery code of a TOTP enrollment, stored as the SHA-256 digest of its normalized form.
CREATE TABLE totp_recovery_codes (
  enrollment_id TEXT NOT NULL,
  code_hash BLOB NOT NULL,
  PRIMARY KEY (enrollment_id, code_hash)
) STRICT;

-- Event: a recovery code was used. The primary key makes each code work once.
CREATE TABLE totp_recovery_code_uses (
  enrollment_id TEXT NOT NULL,
  code_hash BLOB NOT NULL,
  used_at INTEGER NOT NULL,
  PRIMARY KEY (enrollment_id, code_hash)
) STRICT;

-- Resource: a passkey (a WebAuthn discoverable credential) that a user registered to sign in
-- with (FR-004): the authenticator's ID of the credential, unique among all, and its COSE public
-- key. A passkey with a revocation no longer signs in.
CREATE TABLE passkey_credentials (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  credential_id BLOB NOT NULL,
  public_key BLOB NOT NULL,
  backup_eligible INTEGER NOT NULL CHECK (backup_eligible IN (0, 1)),
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX passkey_credentials_credential_id ON passkey_credentials (credential_id);
CREATE INDEX passkey_credentials_user_id_id ON passkey_credentials (user_id, id);

-- Event: a passkey ended (removed by its user).
CREATE TABLE passkey_credential_revocations (
  passkey_id TEXT NOT NULL PRIMARY KEY,
  revoked_at INTEGER NOT NULL
) STRICT;

-- Resource: an email address a user added.
CREATE TABLE user_email_addresses (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  address TEXT NOT NULL,
  address_normalized TEXT NOT NULL,
  added_at INTEGER NOT NULL
) STRICT;
CREATE INDEX user_email_addresses_user_id_id ON user_email_addresses (user_id, id);

-- Projection: the primary email address of each user, for login and against duplicates.
CREATE TABLE user_primary_emails (
  user_id TEXT NOT NULL PRIMARY KEY,
  email_address_id TEXT NOT NULL,
  address_normalized TEXT NOT NULL
) STRICT;
CREATE UNIQUE INDEX user_primary_emails_address_normalized ON user_primary_emails (address_normalized);

-- Resource: a role, such as the built-in admin and moderator.
CREATE TABLE roles (
  id TEXT NOT NULL PRIMARY KEY,
  key TEXT NOT NULL,
  position INTEGER NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX roles_key ON roles (key);

-- Current value: the permissions a role grants.
CREATE TABLE role_permissions (
  role_id TEXT NOT NULL,
  permission TEXT NOT NULL CHECK (permission IN (
    'manage_reports', 'manage_accounts', 'manage_domains', 'manage_rules',
    'manage_settings', 'manage_roles', 'manage_invites', 'view_audit', 'manage_jobs',
    'manage_login_recovery', 'manage_custom_emojis', 'manage_webhooks', 'manage_announcements',
    'manage_finance', 'view_finance', 'manage_supporters', 'create_custom_emojis'
  )),
  PRIMARY KEY (role_id, permission)
) STRICT;

-- Event: a role granted to a user. granted_by_user_id is NULL for a grant from the CLI.
CREATE TABLE user_role_grants (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  role_id TEXT NOT NULL,
  granted_by_user_id TEXT,
  granted_at INTEGER NOT NULL
) STRICT;

-- Event: a grant revoked. revoked_by_user_id is NULL for a revocation from the CLI.
CREATE TABLE user_role_revocations (
  grant_id TEXT NOT NULL PRIMARY KEY,
  revoked_by_user_id TEXT,
  revoked_at INTEGER NOT NULL
) STRICT;

-- Projection: the roles a user holds now (grants without a revocation).
CREATE TABLE user_roles (
  user_id TEXT NOT NULL,
  role_id TEXT NOT NULL,
  grant_id TEXT NOT NULL,
  PRIMARY KEY (user_id, role_id)
) STRICT;

-- Current value: the roles that another module manages (ADR-0072 §5): the role of a supporter
-- plan, which only the supporters module grants and revokes, with member permissions only and a
-- position below every staff role.
CREATE TABLE managed_roles (
  role_id TEXT NOT NULL PRIMARY KEY,
  manager TEXT NOT NULL CHECK (manager IN ('supporters')),
  created_at INTEGER NOT NULL
) STRICT;

-- Current value: the badge of a role (ADR-0071): a row means the role has one.
CREATE TABLE role_badges (
  role_id TEXT NOT NULL PRIMARY KEY,
  label TEXT NOT NULL,
  color TEXT NOT NULL CHECK (color IN (
    'neutral', 'red', 'orange', 'yellow', 'green', 'teal', 'blue', 'purple', 'pink'
  )),
  updated_at INTEGER NOT NULL
) STRICT;

-- Current value: the badges a user shows on their profile (ADR-0071): a row means the user shows
-- the badge of a role they hold.
CREATE TABLE user_role_badge_displays (
  user_id TEXT NOT NULL,
  role_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, role_id)
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- identity: user preferences (data-model/04-tables/01-identity.md §4.1, FR-007, FR-208).
-- ---------------------------------------------------------------------------------------------

-- Current value: a user's settings. locale and default_post_language are canonical BCP 47
-- tags; require_media_description is 0/1.
CREATE TABLE user_preferences (
  user_id TEXT NOT NULL PRIMARY KEY,
  locale TEXT NOT NULL,
  default_visibility TEXT NOT NULL CHECK (default_visibility IN ('public', 'local', 'unlisted', 'private', 'direct')),
  -- Who may quote the user's new statuses (ADR-0069 §1).
  default_quote_policy TEXT NOT NULL DEFAULT 'public' CHECK (default_quote_policy IN ('public', 'followers', 'nobody')),
  default_post_language TEXT NOT NULL,
  require_media_description INTEGER NOT NULL CHECK (require_media_description IN (0, 1)),
  -- Whether the moderator notifications (admin_report, admin_sign_up) also come by email.
  email_moderator_notifications INTEGER NOT NULL CHECK (email_moderator_notifications IN (0, 1)),
  -- Whether the web client inserts new timeline posts without asking (ADR-0077).
  timeline_auto_insert INTEGER NOT NULL DEFAULT 0 CHECK (timeline_auto_insert IN (0, 1)),
  updated_at INTEGER NOT NULL
) STRICT;

-- Current value: the time zone a user chose, an IANA name. No row means the user has not chosen
-- one yet: dates are shown in UTC and the web client offers the browser's zone.
CREATE TABLE user_time_zones (
  user_id TEXT NOT NULL PRIMARY KEY,
  time_zone TEXT NOT NULL
) STRICT;

-- Current value: the languages a user's timelines show; no rows show every language.
CREATE TABLE user_timeline_languages (
  user_id TEXT NOT NULL,
  language TEXT NOT NULL,
  PRIMARY KEY (user_id, language)
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- identity: browser sessions of the server-rendered screens (data-model/04-tables/01-identity.md §4.1, ADR-0019 §4).
-- ---------------------------------------------------------------------------------------------

-- Resource: a sign-in. token_hash is the SHA-256 digest of the session cookie's value.
CREATE TABLE sessions (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  token_hash BLOB NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX sessions_token_hash ON sessions (token_hash);
CREATE INDEX sessions_user_id ON sessions (user_id);

-- Event: a session ended before it expired.
CREATE TABLE session_revocations (
  session_id TEXT NOT NULL PRIMARY KEY,
  reason TEXT NOT NULL CHECK (reason IN ('logout', 'replaced', 'password_changed', 'moderation', 'expired_cleanup', 'account_deleted')),
  revoked_at INTEGER NOT NULL
) STRICT;

-- Current value: when a session was last used, updated at most once per interval.
CREATE TABLE session_activity (
  session_id TEXT NOT NULL PRIMARY KEY,
  last_seen_at INTEGER NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- identity: OAuth 2.0 authorization server (data-model/04-tables/01-identity.md §4.1, ADR-0019 §4, ADR-0030).
-- Every client_id column except oauth_clients.client_id refers to oauth_clients.id.
-- ---------------------------------------------------------------------------------------------

-- Resource: a registered client. client_secret_hash is NULL for public clients. registration is the
-- route by which the client was registered (ADR-0090 §1), fixed for its life.
CREATE TABLE oauth_clients (
  id TEXT NOT NULL PRIMARY KEY,
  client_id TEXT NOT NULL,
  client_secret_hash BLOB,
  name TEXT NOT NULL,
  website TEXT,
  scope TEXT NOT NULL,
  registration TEXT NOT NULL CHECK (registration IN ('first_party', 'user', 'metadata_document', 'dynamic')),
  owner_user_id TEXT,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX oauth_clients_client_id ON oauth_clients (client_id);
CREATE INDEX oauth_clients_owner_user_id ON oauth_clients (owner_user_id);

-- Event: a client was disabled (ADR-0090 §1, an administrator's decision). disabled_by_user_id is
-- NULL for the CLI. A client is disabled while one of its disablements has no lift.
CREATE TABLE oauth_client_disablements (
  id TEXT NOT NULL PRIMARY KEY,
  client_id TEXT NOT NULL,
  disabled_by_user_id TEXT,
  disabled_at INTEGER NOT NULL
) STRICT;
CREATE INDEX oauth_client_disablements_client_id ON oauth_client_disablements (client_id);

-- Event: a disablement was lifted, at most once. lifted_by_user_id is NULL for the CLI.
CREATE TABLE oauth_client_disablement_lifts (
  disablement_id TEXT NOT NULL PRIMARY KEY,
  lifted_by_user_id TEXT,
  lifted_at INTEGER NOT NULL
) STRICT;

-- Event: a client was deleted, at most once; it never works again. reason is who deleted it: the
-- user who registered it (owner) or the clean-up of unused dynamically registered clients
-- (unused). deleted_by_user_id is NULL for the clean-up.
CREATE TABLE oauth_client_deletions (
  client_id TEXT NOT NULL PRIMARY KEY,
  reason TEXT NOT NULL CHECK (reason IN ('owner', 'unused')),
  deleted_by_user_id TEXT,
  deleted_at INTEGER NOT NULL
) STRICT;

-- Current value: the redirect URIs a client may use, matched exactly.
CREATE TABLE oauth_client_redirect_uris (
  client_id TEXT NOT NULL,
  redirect_uri TEXT NOT NULL,
  PRIMARY KEY (client_id, redirect_uri)
) STRICT;

-- Event: the user who registered a client changed it (ADR-0090 §1). The newest revision (by id)
-- is the client's name, website, and scope; a client without one keeps those it was registered
-- with.
CREATE TABLE oauth_client_revisions (
  id TEXT NOT NULL PRIMARY KEY,
  client_id TEXT NOT NULL,
  name TEXT NOT NULL,
  website TEXT,
  scope TEXT NOT NULL,
  revised_at INTEGER NOT NULL
) STRICT;
CREATE INDEX oauth_client_revisions_client_id ON oauth_client_revisions (client_id, id);

-- Event: the user who registered a confidential client issued it a new secret (ADR-0090 §2). Only
-- the newest secret (by id) works; a client without a rotation has the secret it was registered
-- with.
CREATE TABLE oauth_client_secret_rotations (
  id TEXT NOT NULL PRIMARY KEY,
  client_id TEXT NOT NULL,
  secret_hash BLOB NOT NULL,
  rotated_at INTEGER NOT NULL
) STRICT;
CREATE INDEX oauth_client_secret_rotations_client_id ON oauth_client_secret_rotations (client_id, id);
-- Current value (cache): the Client ID Metadata Document of a client whose registration is
-- metadata_document (ADR-0090 §1), as last fetched from its client_id URL. Every fetch replaces the
-- row and the client's oauth_client_redirect_uris; refetching rebuilds it. client_uri and logo_uri
-- are '' when the document has none. expires_at is when the HTTP cache directives of the response
-- let the document be reused until (fetched_at for no-store and no-cache).
CREATE TABLE oauth_client_metadata_documents (
  client_id TEXT NOT NULL PRIMARY KEY,
  client_name TEXT NOT NULL,
  client_uri TEXT NOT NULL,
  logo_uri TEXT NOT NULL,
  scope TEXT NOT NULL,
  fetched_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
) STRICT;

-- Event: an administrator denied the Client ID Metadata Documents of a host (ADR-0090 §1): no
-- client_id URL on the host is fetched or used while one of its denials has no lift. host is the
-- lowercase host name without the port. denied_by_user_id is NULL for the CLI.
CREATE TABLE oauth_client_host_denials (
  id TEXT NOT NULL PRIMARY KEY,
  host TEXT NOT NULL,
  denied_by_user_id TEXT,
  denied_at INTEGER NOT NULL
) STRICT;
CREATE INDEX oauth_client_host_denials_host ON oauth_client_host_denials (host);

-- Event: a host denial was lifted, at most once. lifted_by_user_id is NULL for the CLI.
CREATE TABLE oauth_client_host_denial_lifts (
  denial_id TEXT NOT NULL PRIMARY KEY,
  lifted_by_user_id TEXT,
  lifted_at INTEGER NOT NULL
) STRICT;

-- Resource: an authorization code, keyed by its SHA-256 digest. session_id is the browser session
-- that authorized it (NULL when it was issued outside one).
CREATE TABLE oauth_authorization_codes (
  code_hash BLOB NOT NULL PRIMARY KEY,
  client_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  redirect_uri TEXT NOT NULL,
  scope TEXT NOT NULL,
  code_challenge TEXT NOT NULL,
  session_id TEXT,
  issued_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
) STRICT;

-- Event: a code was exchanged; the primary key makes it one-time. token_id is the first token
-- of the family it started.
CREATE TABLE oauth_authorization_code_redemptions (
  code_hash BLOB NOT NULL PRIMARY KEY,
  token_id TEXT NOT NULL,
  redeemed_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX oauth_authorization_code_redemptions_token_id ON oauth_authorization_code_redemptions (token_id);

-- Resource: an access token with its refresh token. A rotation issues a new row in the same
-- family (family_id is the id of the family's first row).
CREATE TABLE oauth_access_tokens (
  id TEXT NOT NULL PRIMARY KEY,
  family_id TEXT NOT NULL,
  client_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  access_token_hash BLOB NOT NULL,
  refresh_token_hash BLOB,
  scope TEXT NOT NULL,
  rotated_from_token_id TEXT,
  issued_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  refresh_expires_at INTEGER,
  CONSTRAINT oauth_access_tokens_refresh_pair CHECK ((refresh_token_hash IS NULL) = (refresh_expires_at IS NULL))
) STRICT;
CREATE UNIQUE INDEX oauth_access_tokens_access_token_hash ON oauth_access_tokens (access_token_hash);
CREATE UNIQUE INDEX oauth_access_tokens_refresh_token_hash ON oauth_access_tokens (refresh_token_hash);
CREATE INDEX oauth_access_tokens_family_id ON oauth_access_tokens (family_id);
CREATE INDEX oauth_access_tokens_user_id ON oauth_access_tokens (user_id);
CREATE INDEX oauth_access_tokens_client_id ON oauth_access_tokens (client_id);

-- Event: a token row (its access and refresh token) stopped working.
CREATE TABLE oauth_token_revocations (
  token_id TEXT NOT NULL PRIMARY KEY,
  reason TEXT NOT NULL CHECK (reason IN ('user', 'rotation', 'refresh_reuse', 'code_reuse', 'password_changed', 'moderation', 'account_deleted', 'client_disabled', 'client_deleted')),
  revoked_at INTEGER NOT NULL
) STRICT;

-- Current value: when a token family last authenticated an API request, at the precision of one
-- hour (ADR-0090 §5). The row of a family exists once it has been used.
CREATE TABLE oauth_token_family_last_used (
  family_id TEXT NOT NULL PRIMARY KEY,
  last_used_at INTEGER NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- identity: registration, invites, email verification, and one-time tokens
-- (data-model/04-tables/01-identity.md §4.1, FR-001, FR-002).
-- ---------------------------------------------------------------------------------------------

-- Event: an email address was verified, at most once per address.
CREATE TABLE user_email_verifications (
  email_address_id TEXT NOT NULL PRIMARY KEY,
  verified_at INTEGER NOT NULL
) STRICT;

-- Resource: a sign-up from the registration page, one per user. reason is the applicant's
-- answer in approval mode; invite_id is the invite it redeemed in invite mode.
CREATE TABLE registration_applications (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  reason TEXT,
  invite_id TEXT,
  submitted_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX registration_applications_user_id ON registration_applications (user_id);

-- Event: an application was approved or rejected. decided_by_user_id is NULL for the system
-- and for a decision from the CLI.
CREATE TABLE registration_decisions (
  id TEXT NOT NULL PRIMARY KEY,
  application_id TEXT NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('approved', 'rejected')),
  decided_by TEXT NOT NULL CHECK (decided_by IN ('moderator', 'system')),
  decided_by_user_id TEXT,
  note TEXT,
  decided_at INTEGER NOT NULL
) STRICT;
CREATE INDEX registration_decisions_application_id_id ON registration_decisions (application_id, id DESC);

-- Projection: the registration state of each user who signed up; users without a row (created
-- with the CLI) are approved.
CREATE TABLE user_registration_states (
  user_id TEXT NOT NULL PRIMARY KEY,
  state TEXT NOT NULL CHECK (state IN ('pending', 'approved', 'rejected')),
  since INTEGER NOT NULL
) STRICT;
CREATE INDEX user_registration_states_state_since ON user_registration_states (state, since);

-- Resource: an invite code. created_by_user_id is NULL for an invite from the CLI; max_uses
-- and expires_at are NULL for no limit.
CREATE TABLE invites (
  id TEXT NOT NULL PRIMARY KEY,
  code TEXT NOT NULL,
  created_by_user_id TEXT,
  max_uses INTEGER,
  expires_at INTEGER,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX invites_code ON invites (code);

-- Event: one use of an invite. seq is 1 + the uses before it, so that the primary key keeps
-- concurrent sign-ups from exceeding max_uses.
CREATE TABLE invite_redemptions (
  invite_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  user_id TEXT NOT NULL,
  redeemed_at INTEGER NOT NULL,
  PRIMARY KEY (invite_id, seq)
) STRICT;

-- Event: an invite was revoked. revoked_by_user_id is NULL for a revocation from the CLI.
CREATE TABLE invite_revocations (
  invite_id TEXT NOT NULL PRIMARY KEY,
  revoked_by_user_id TEXT,
  revoked_at INTEGER NOT NULL
) STRICT;

-- Resource: a one-time token sent to a user. token_hash is the SHA-256 digest of the token.
CREATE TABLE user_tokens (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  purpose TEXT NOT NULL CHECK (purpose IN ('email_verification', 'password_reset')),
  email_address_id TEXT,
  token_hash BLOB NOT NULL,
  expires_at INTEGER NOT NULL,
  issued_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX user_tokens_token_hash ON user_tokens (token_hash);

-- Event: a token was used; the primary key makes every token single-use.
CREATE TABLE user_token_redemptions (
  token_id TEXT NOT NULL PRIMARY KEY,
  redeemed_at INTEGER NOT NULL
) STRICT;

-- ---------------------------------------------------------------------------------------------
-- identity: where sign-ins and registrations came from (data-model/04-tables/01-identity.md §4.1, ADR-0038). Kept
-- apart from sessions and users so that the purge erases them after their retention.
-- ---------------------------------------------------------------------------------------------

-- Resource (child of a session): the client that started a session. ip and user_agent are NULL
-- when unknown; a session with neither has no row.
CREATE TABLE session_origins (
  session_id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  ip TEXT,
  user_agent TEXT,
  created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX session_origins_ip ON session_origins (ip);
CREATE INDEX session_origins_created_at ON session_origins (created_at);
CREATE INDEX session_origins_user_id_created_at ON session_origins (user_id, created_at DESC);

-- Resource: the IP address of a registration from the registration page. A registration from
-- an unknown address has no row.
CREATE TABLE registration_origins (
  user_id TEXT NOT NULL PRIMARY KEY,
  ip TEXT NOT NULL,
  recorded_at INTEGER NOT NULL
) STRICT;
CREATE INDEX registration_origins_ip ON registration_origins (ip);
CREATE INDEX registration_origins_recorded_at ON registration_origins (recorded_at);

-- ---------------------------------------------------------------------------------------------
-- identity: the exports and imports of a user's data (data-model/04-tables/01-identity.md §4.1, FR-008).
-- ---------------------------------------------------------------------------------------------

-- Resource: a user's request for an archive of their data.
CREATE TABLE data_exports (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  requested_at INTEGER NOT NULL
) STRICT;
CREATE INDEX data_exports_user_id_id ON data_exports (user_id, id DESC);

-- Event (child of an export): the end of its build. A ready archive is the private blob under
-- blob_key; both outcomes are removed at expires_at.
CREATE TABLE data_export_results (
  export_id TEXT NOT NULL PRIMARY KEY,
  outcome TEXT NOT NULL CHECK (outcome IN ('ready', 'failed')),
  blob_key TEXT,
  file_size INTEGER CHECK (file_size >= 0),
  expires_at INTEGER NOT NULL,
  completed_at INTEGER NOT NULL,
  CHECK ((outcome = 'ready') = (blob_key IS NOT NULL)),
  CHECK ((outcome = 'ready') = (file_size IS NOT NULL))
) STRICT;
CREATE INDEX data_export_results_expires_at ON data_export_results (expires_at);

-- Resource: a user's import of a Mastodon CSV file of follows, blocks, mutes, bookmarks, or
-- muted domains.
CREATE TABLE data_imports (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('following', 'blocks', 'mutes', 'bookmarks', 'domain_mutes')),
  mode TEXT NOT NULL CHECK (mode IN ('merge', 'overwrite')),
  created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX data_imports_user_id_id ON data_imports (user_id, id DESC);
CREATE INDEX data_imports_created_at ON data_imports (created_at);

-- Resource (child of an import): one thing the import does. An 'add' item is a line of the file,
-- target its address or post URL as written; a 'remove' item is what an overwrite removes,
-- target the ID of the account or status.
CREATE TABLE data_import_items (
  import_id TEXT NOT NULL,
  position INTEGER NOT NULL CHECK (position >= 0),
  action TEXT NOT NULL CHECK (action IN ('add', 'remove')),
  target TEXT NOT NULL,
  show_boosts INTEGER NOT NULL CHECK (show_boosts IN (0, 1)),
  notify INTEGER NOT NULL CHECK (notify IN (0, 1)),
  hide_notifications INTEGER NOT NULL CHECK (hide_notifications IN (0, 1)),
  PRIMARY KEY (import_id, position)
) STRICT;

-- Event (child of an item): how the item ended. failure says why exactly when it failed.
CREATE TABLE data_import_item_results (
  import_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  outcome TEXT NOT NULL CHECK (outcome IN ('done', 'failed')),
  failure TEXT CHECK (failure IN ('unresolvable', 'unavailable', 'blocked', 'restricted', 'self', 'limit')),
  processed_at INTEGER NOT NULL,
  PRIMARY KEY (import_id, position),
  CHECK ((outcome = 'failed') = (failure IS NOT NULL))
) STRICT;
