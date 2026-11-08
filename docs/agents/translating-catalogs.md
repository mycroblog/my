# Translating the UI catalogs (playbook)

How an agent translates the message catalogs of `locales/` into a locale other than `en` and `ja`, and keeps it
current. Policy background: [ADR-0009](../adr/0009-internationalization.md) §1, §2 and §8. `en` is the source locale;
the maintainer updates `ja` in the same change as `en`. The other locales (`zh-CN`, `zh-TW`, `es`, `pt-BR`, and the languages of the
"Languages" milestone after it) are machine translations that an agent writes in runs started by hand.

## What is translated

Every file `locales/en/<namespace>/<area>.json` has a translation `locales/<locale>/<namespace>/<area>.json`: the
same keys in the same order, each with its translated `defaultMessage` and the English `description` copied as it is.
A key missing from a translation shows the English text, and `scripts/task.sh i18n:check` warns about it; it never
fails the check, so a change to `en` needs no translation in the same pull request. A key removed from `en` or renamed
is removed from every locale in the same change, since a key that `en` does not have fails the check.

A locale's directory is the language tag alone (`es`, `de`), with a region only where the language is written
differently by region (`zh-CN`, `zh-TW`, `pt-BR`). The server and the web client find a new directory on their own;
the tags that resolve to it are in `parentLocales` of `internal/i18n/locale.go` and of `app/web/src/lib/i18n.tsx`.

## Rules of a translation

- Keep every argument name, `select` key, and rich-text tag (`<link>…</link>`) as in English. Translate the text inside
  the branches and the tags.
- Keep only the plural categories the locale has in CLDR, plus exact selectors (`=0`, `=1`); `i18n:check` rejects a
  category the locale does not have.
- Keep product names, commands, settings, environment variables, and URLs in English.
- Follow the locale's punctuation and wording rules (web-client
  [§16 G19–G21](../architecture/web-client/16-look-states-and-wording.md)) and its terms in
  `tools/i18n/glossary.json`. `i18n:check` applies the rules it can check and rejects the glossary's avoided terms.
- Write natural text in the language, as its widely used apps do, not a word-for-word rendering. The `description`
  says where the text appears; `locales/ja/` shows how another translation reads the same key.

## Steps

1. List the untranslated keys of the locale:

   ```sh
   scripts/task.sh i18n:check 2>&1 | grep 'untranslated' | grep 'zh-CN'
   ```

   A key whose English text changed after it was translated is not listed: compare the `git log` of the English file
   with the translation's when a run catches up after many changes.
2. Translate the missing keys into the locale's files, in the order of the English file. For a large catch-up, split
   the work by area files.
3. Run `scripts/task.sh i18n:gen` and `scripts/task.sh i18n:check`, and fix every error.
4. Open a pull request with the translations only.

## Adding a locale

1. Add the style rules of the language to web-client §16 G20, its terms to `tools/i18n/glossary.json`, and the checks
   of `tools/i18n/src/check.ts` that the rules need. The decider reviews them in the pull request.
2. Add the tags that resolve to the locale to `parentLocales` of `internal/i18n/locale.go` and
   `app/web/src/lib/i18n.tsx`, with their tests, when they are not there yet.
3. Add the locale to `GOLDEN_LOCALES` and `CLDR_LOCALES` of `tools/i18n/src/generate.ts`, and one of the messages of
   `internal/i18n/testdata/cases.json` if the catalogs differ in plurals.
4. Translate every file as above, then run `scripts/task.sh i18n:gen`.
5. Add the locale to the documentation site ([Translating the documentation site](translating-docs.md#adding-a-locale)).

## Before a release

Before tagging a release, translate the untranslated keys of every locale and merge them, so that the release shows
as little English as possible ([Releasing](releasing.md)). Untranslated keys do not block the release.
