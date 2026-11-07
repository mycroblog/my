# Translating the documentation site (playbook)

How an agent translates the documents of the documentation site, and keeps the translations current, section by section.
Policy background: [ADR-0150](../adr/0150-documentation-site.md) §9. The English documents are authoritative; a
translation follows them and never adds, drops, or corrects a fact. A wrong fact is fixed in the English document first.

## What is translated

| Source | Translation | Translated |
| --- | --- | --- |
| `docs/guide/*.md` | `app/docs/i18n/<locale>/guide/*.md` | yes |
| `docs/operations/*.md` | `app/docs/i18n/<locale>/operations/*.md` | yes |
| `app/docs/src/content/docs/index.mdx` (the landing page) | `app/docs/i18n/<locale>/index.mdx` | yes, as one section |
| The `## Headings` and `### Headings` of `docs/README.md` (the sidebar groups) | `app/docs/i18n/<locale>/sidebar.json` | yes |
| `docs/reference/`, `docs/developers/`, `docs/plugins/` | none: the site shows the English page with a notice | no |

The site serves a translation under `/<locale>/`, the locale's tag in lower case (`/next/ja/guide/posting/`,
`/next/zh-cn/guide/posting/`). A page with no translation file, and a section with no translation, is shown in English.
The locales are listed in `app/docs/src/translations.ts` (`translationLocales`), by the tag of their catalog in
`locales/` (`ja`, `zh-CN`); the commands and the directories of `app/docs/i18n/` use that tag.

## How a translation is written

A source document is split into sections at each heading outside fenced code. Each section of a translation starts with
a marker that names the section by the anchor of its English heading and the hash of the English text it translates:

```markdown
<!-- source: who-can-see-a-post 46a2eddaeddf -->
## Who can see a post, in the locale

The translated text of the section.
```

- Copy the markers exactly as `todo` prints them. Never compute or edit a hash by hand.
- One section is one heading and its text, the heading at the same level as the English one. Never merge or split sections, and never add a heading.
- Keep every code block, code span, and link target exactly as in English, including `#anchors`. Links are written as in the English document (`safety.md`, `../reference/configuration.md#voice`, `images/visibility.svg`), not relative to the translation file. Translate the text of links and the alt text of images.
- Do not add `{#id}` to headings: the site gives each heading the anchor of its English heading.
- Keep the sections in the order of the English document.
- Text before the first marker is an error.

The landing page is one section. Its translation is a copy of `index.mdx` with the prose translated (the frontmatter's
`description`, the hero's `tagline` and the `text` of its buttons, the cards, the alt texts, the labels of `DocLinks`),
and the first line of its frontmatter is the marker `# source: <hash>`. Keep its imports, component names, `path`s, and
the paths of its images as in English; the site adjusts them.

### Style

- Write natural prose in the target language, not a word-for-word rendering. For `ja`, use the polite style
  (`です・ます`) for every page. For `zh-CN`, address the reader as `你`, use full-width punctuation in Chinese text,
  and put a half-width space between Chinese and Latin letters, digits, or code.
- Name the UI by its text in the locale's catalog (`locales/<locale>/`): the English documents quote it
  (`"Write a post"`), and `glossary` prints its text in the locale. Quote it as the locale does (for `ja`, `「…」`;
  for `zh-CN`, `“…”`).
  When `glossary` offers more than one text, pick the one of the screen the sentence is about. A phrase with no UI
  string is translated like any other text.
- Keep names of products, commands, settings, environment variables, metrics, and job types in English.
- A table keeps its columns and rows; translate its cells.

## Steps

All commands run through `scripts/task.sh docs:site-translations -- <command>` (`--locale ja` by default). A source is
named by its repository path.

1. Find what to translate. `status` lists every source that has no translation, and the sections of the others that are
   missing, stale (their English text changed), or no longer in the source:

   ```sh
   scripts/task.sh docs:site-translations -- status
   scripts/task.sh docs:site-translations -- status docs/operations/backup.md
   ```

2. For one source, print the sections to translate, each with its marker and English text, and the UI terms it quotes:

   ```sh
   scripts/task.sh docs:site-translations -- todo docs/operations/backup.md
   scripts/task.sh docs:site-translations -- glossary docs/operations/backup.md
   ```

3. Write the translation file, or replace the stale sections of it: each section under the marker `todo` printed for it.
   Delete the sections `todo` says the source no longer has. Leave the current sections as they are.
4. Check the translations, and build the site, whose link check covers the translated pages:

   ```sh
   scripts/task.sh docs:site-translations-check
   scripts/task.sh docs:site-build
   ```

   Read the translated pages with `scripts/task.sh docs:site-dev` (`http://localhost:4321/ja/...`) when the layout
   matters: tables, lists, images, and the landing page.
5. When a new document is translated, or a `## Heading` or `### Heading` of `docs/README.md` changes, update
   `sidebar.json`: its keys are the English headings, its values their translation.
6. Open a pull request with the translations only. For many documents, split the work into pull requests of a few
   documents each, for example by the groups of `docs/README.md`.

`docs:site-translations-check` runs in the merge queue whenever the site changes. It fails on a marker that does not parse,
a section that does not start with its heading, a current section whose code blocks, code spans, or link targets differ
from English, and a translation file whose source was moved or removed (move or delete it along with the source). A stale
or missing section never fails it.

## Before a release

Before tagging a release, run `status`, translate the stale and missing sections, and merge the translations, so that the
documentation of the version is translated as far as possible ([Releasing](releasing.md)). Stale sections do not block
the release: each page lists them and links the English page.

## Adding a locale

1. Add the locale to `translationLocales` in `app/docs/src/translations.ts`, by the tag of its catalog in `locales/`, and
   to `locales` in `app/docs/astro.config.ts`, keyed by the tag in lower case with the tag as its `lang`
   (`"zh-cn": { label: "简体中文", lang: "zh-CN" }`).
2. Add the site's UI strings for it: `app/docs/src/content/i18n/<locale>.json`, with the keys of `en.json`.
3. Translate as above, starting with `docs/guide/` and the landing page, and add `sidebar.json`.
