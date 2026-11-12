---
name: translate-docs
description: Translate the documents of the documentation site (docs/guide, docs/operations, the landing page) into a locale of app/docs/i18n, or translate again the sections that went stale. Use when asked to translate the docs or the documentation site, or to bring its translations up to date.
---

# Translate-docs skill

Arguments: the sources to translate (repository paths such as `docs/operations/backup.md`, or a directory), and
`--locale <locale>` (`ja` by default), by the tag of its catalog in `locales/`. Without sources, take what `status` lists.

Policy: ADR-0150 §9. The English documents are authoritative: a translation never adds, drops, or corrects a fact.
Translated: `docs/guide/`, `docs/operations/`, the landing page and the analytics notice of `app/docs/src/content/docs/`
(one section each), and the headings of `docs/README.md` (`sidebar.json`). Not `docs/reference/`, `docs/developers/`,
`docs/plugins/`. Read [reference.md](reference.md) for the style of each locale, the landing page, and adding a locale.

1. `scripts/task.sh docs:site-translations -- status [source...]`: the sources and sections missing, stale, or no
   longer in the source.
2. For each source, `-- todo <source>` and `-- glossary <source>`.
3. Write the translation file, or replace its stale sections, each under the marker `todo` printed. Delete the sections
   `todo` says the source no longer has; leave current ones as they are.
   - Copy the markers exactly. Never compute or edit a hash by hand. No text before the first marker.
   - One section is one heading and its text, at the English heading's level, in the English order. Never merge,
     split, or add a heading, and never add `{#id}`: the site gives each heading its English anchor.
   - Keep every code block, code span, and link target as in English, `#anchors` included, written as in the English
     document (not relative to the translation file). Translate link text and image alt text.
   - Name the UI by the text `glossary` prints; when it offers several, pick the one of the screen the sentence is
     about. Keep names of products, commands, settings, environment variables, metrics, and job types in English.
     A table keeps its columns and rows.
4. Update `app/docs/i18n/<locale>/sidebar.json` (keys the English headings) when a document is new or a heading of
   `docs/README.md` changed.
5. `scripts/task.sh docs:site-translations-check`, then `scripts/task.sh docs:site-build`; fix every error. When the
   layout matters (tables, lists, images, the landing page), read the pages with `scripts/task.sh docs:site-dev`.
6. Never change an English document to fit a translation. If the English is wrong, say so and fix the English in a
   separate commit (boy scout rule).
7. Commit as `docs(i18n): translate <what> into <locale>`, translations only; for a large batch, one pull request per
   few documents, for example by the groups of `docs/README.md`.

Before a release, translate what `status` lists for every locale; stale sections never block it.

Report the sources translated, the sections left stale or missing, and any English that looked wrong.
