-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The translation module: translations of posts, and how many characters went to the providers
-- (ADR-0148, data-model section 4.17).

-- Current value (cache): the translation of a revision by one provider. It is derived from the
-- revision, may be deleted at any time, and is not a projection, since the output of a provider
-- cannot be rebuilt. provider has no CHECK, so that adding a provider does not change the schema.
CREATE TABLE status_translations (
  id TEXT NOT NULL PRIMARY KEY,
  status_id TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  target_language TEXT NOT NULL,
  provider TEXT NOT NULL,
  source_language TEXT NOT NULL,
  content_html TEXT NOT NULL,
  spoiler_text TEXT NOT NULL,
  created_at INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX status_translations_revision_id_target_language_provider ON status_translations (revision_id, target_language, provider);
CREATE INDEX status_translations_status_id ON status_translations (status_id);
CREATE INDEX status_translations_created_at ON status_translations (created_at);

-- Current value (cache, child): the translated alt text of an attachment of the revision.
CREATE TABLE status_translation_attachments (
  translation_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  description TEXT NOT NULL,
  PRIMARY KEY (translation_id, position)
) STRICT;

-- Current value (cache, child): the translated title of an option of the revision's poll.
CREATE TABLE status_translation_poll_options (
  translation_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  title TEXT NOT NULL,
  PRIMARY KEY (translation_id, position)
) STRICT;

-- Current value: the characters of one account sent to providers in a UTC month (YYYY-MM).
CREATE TABLE translation_account_usage (
  account_id TEXT NOT NULL,
  month TEXT NOT NULL,
  characters INTEGER NOT NULL CHECK (characters >= 0),
  PRIMARY KEY (account_id, month)
) STRICT;
CREATE INDEX translation_account_usage_month ON translation_account_usage (month);

-- Current value: the characters of the whole server sent to providers in a UTC month (YYYY-MM),
-- and how many requests that month were answered from the cache and how many went to a provider.
CREATE TABLE translation_server_usage (
  month TEXT NOT NULL PRIMARY KEY,
  characters INTEGER NOT NULL DEFAULT 0 CHECK (characters >= 0),
  cache_hits INTEGER NOT NULL DEFAULT 0 CHECK (cache_hits >= 0),
  cache_misses INTEGER NOT NULL DEFAULT 0 CHECK (cache_misses >= 0)
) STRICT;
