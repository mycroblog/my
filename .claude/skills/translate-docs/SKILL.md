---
name: translate-docs
description: Translate the documents of the documentation site (docs/guide, docs/operations, the landing page) into a locale of app/docs/i18n, or translate again the sections that went stale. Use when asked to translate the docs or the documentation site, or to bring its translations up to date.
---

# Translate-docs skill

Arguments: the sources to translate (repository paths such as `docs/operations/backup.md`, or a directory), and `--locale <locale>` (`ja` by default). Without sources, take what `status` lists.

Follow `docs/agents/translating-docs.md` exactly. In short:

1. `scripts/task.sh docs:site-translations -- status [source...]` to see which sources and sections are missing or stale.
2. For each source, `scripts/task.sh docs:site-translations -- todo <source>` and `-- glossary <source>`; write each section under the marker `todo` printed, with the UI terms of `glossary`, keeping code, code spans, link targets, and section headings as the playbook says.
3. Update `app/docs/i18n/<locale>/sidebar.json` when a document is new or a heading of `docs/README.md` changed.
4. `scripts/task.sh docs:site-translations-check`, then `scripts/task.sh docs:site-build`; fix every error.
5. Never change an English document to fit a translation. If the English is wrong, say so and fix the English in a separate commit (boy scout rule).
6. Commit as `docs(i18n): translate <what> into <locale>`; for a large batch, open one pull request per few documents.

Report the sources translated, the sections left stale or missing, and any English that looked wrong.
