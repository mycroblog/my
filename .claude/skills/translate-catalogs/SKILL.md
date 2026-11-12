---
name: translate-catalogs
description: Translate the UI message catalogs of locales/ into a machine-translated locale (zh-CN, zh-TW, es, pt-BR, de, fr), catch up its untranslated keys, or add a new locale. Use when asked to translate the UI, the catalogs, or the messages into a locale other than en and ja.
---

# Translate-catalogs skill

Arguments: `--locale <locale>`, the tag of its directory in `locales/`. Policy: ADR-0009 §1, §2, §8. `en` is the
source; the maintainer updates `ja` with `en`. The other locales are machine translations written in runs started by
hand. A key missing from them shows English and only warns, so they never block a change or a release.

1. List the untranslated keys: `scripts/i18n.sh check --locale <locale> --warnings | grep untranslated`.
2. Translate them. `scripts/i18n.sh show <key or prefix>` prints the English, the Japanese, and the description;
   `scripts/i18n.sh set <namespace>:<key> --<locale> "<text>"` checks one translation and writes it in its place. For
   a large catch-up, write the area files in the order of the English files and split the work by them.
   - Keep every argument name, `select` key, and rich-text tag (`<link>…</link>`) as in English; translate the text
     inside the branches and the tags.
   - Keep only the plural categories the locale has in CLDR, plus exact selectors (`=0`, `=1`).
   - Keep product names, commands, settings, environment variables, and URLs in English.
   - Follow the locale's rules in `docs/architecture/web-client/16-look-states-and-wording.md` G19–G21 and its terms in `tools/i18n/glossary.json`.
   - Write natural text, as the language's widely used apps do, not a word-for-word rendering. The `description` says
     where the text appears; `locales/ja/` shows how another translation reads the same key.
3. `scripts/task.sh i18n:gen` and `scripts/i18n.sh check --changed --locale <locale>`; fix every error.
4. Open a pull request with the translations only.

Before a release, catch up every locale this way; untranslated keys never block it.

## Adding a locale

The directory is the language tag alone (`es`, `de`), with a region only where the language is written differently by
region (`zh-CN`, `zh-TW`, `pt-BR`). The server and the web client find a new directory on their own.

1. Add the language's style rules to web-client §16 G20, its terms to `tools/i18n/glossary.json`, and the checks of
   `tools/i18n/src/check.ts` the rules need. The decider reviews them in the pull request.
2. Add the tags that resolve to the locale to `parentLocales` of `internal/i18n/locale.go` and
   `app/web/src/lib/i18n.tsx`, with their tests, when they are not there yet.
3. Add the locale to `GOLDEN_LOCALES` and `CLDR_LOCALES` of `tools/i18n/src/generate.ts`, and a message to
   `internal/i18n/testdata/cases.json` if the catalogs differ in plurals.
4. Translate every file as above, then `scripts/task.sh i18n:gen`.
5. Add the locale to the documentation site (`/translate-docs`, its reference.md).

Report the locale, the keys translated, and the keys left untranslated.
