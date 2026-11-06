-- SPDX-License-Identifier: AGPL-3.0-only
--
-- The translation module: translations of posts, and how many characters went to the providers
-- (ADR-0148, data-model section 4.17).

-- Current value (cache): the translation of a revision by one provider. It is derived from the
-- revision, may be deleted at any time, and is not a projection, since the output of a provider
-- cannot be rebuilt. provider has no CHECK, so that adding a provider does not change the schema.
CREATE TABLE status_translations (
  id uuid NOT NULL PRIMARY KEY,
  status_id uuid NOT NULL,
  revision_id uuid NOT NULL,
  target_language text NOT NULL,
  provider text NOT NULL,
  source_language text NOT NULL,
  content_html text NOT NULL,
  spoiler_text text NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX status_translations_revision_id_target_language_provider ON status_translations (revision_id, target_language, provider);
CREATE INDEX status_translations_status_id ON status_translations (status_id);
CREATE INDEX status_translations_created_at ON status_translations (created_at);

-- Current value (cache, child): the translated alt text of an attachment of the revision.
CREATE TABLE status_translation_attachments (
  translation_id uuid NOT NULL,
  position integer NOT NULL,
  description text NOT NULL,
  PRIMARY KEY (translation_id, position)
);

-- Current value (cache, child): the translated title of an option of the revision's poll.
CREATE TABLE status_translation_poll_options (
  translation_id uuid NOT NULL,
  position integer NOT NULL,
  title text NOT NULL,
  PRIMARY KEY (translation_id, position)
);

-- Current value: the characters of one account sent to providers in a UTC month (YYYY-MM).
CREATE TABLE translation_account_usage (
  account_id uuid NOT NULL,
  month text COLLATE "C" NOT NULL CHECK (month ~ '^[0-9]{4}-[0-1][0-9]$'),
  characters bigint NOT NULL CHECK (characters >= 0),
  PRIMARY KEY (account_id, month)
);
CREATE INDEX translation_account_usage_month ON translation_account_usage (month);

-- Current value: the characters of the whole server sent to providers in a UTC month (YYYY-MM),
-- and how many requests that month were answered from the cache and how many went to a provider.
CREATE TABLE translation_server_usage (
  month text COLLATE "C" NOT NULL PRIMARY KEY CHECK (month ~ '^[0-9]{4}-[0-1][0-9]$'),
  characters bigint NOT NULL DEFAULT 0 CHECK (characters >= 0),
  cache_hits bigint NOT NULL DEFAULT 0 CHECK (cache_hits >= 0),
  cache_misses bigint NOT NULL DEFAULT 0 CHECK (cache_misses >= 0)
);
