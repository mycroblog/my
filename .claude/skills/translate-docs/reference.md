# Translate-docs reference

## Style

Write natural prose in the language, not a word-for-word rendering. Quote UI text as the locale does.

| Locale | Rules | UI quote |
| --- | --- | --- |
| `ja` | polite style (`です・ます`) on every page | `「…」` |
| `zh-CN` | address the reader as `你`; full-width punctuation in Chinese text; a half-width space between Chinese and Latin letters, digits, or code | `“…”` |
| `zh-TW` | as `zh-CN`, in Traditional Chinese with the words of Taiwan (`伺服器`, `設定`, `使用者`), never a character conversion of `zh-CN` | `「…」` |
| `es` | neutral Spanish for Spain and Latin America (no `ordenador` or `computadora`); `tú`, never `usted` or `vosotros`; open questions with `¿` | `«…»` |
| `pt-BR` | Brazilian words and spelling (`usuário`, `tela`, `arquivo`, `contato`, never `utilizador`, `ecrã`, `ficheiro`, `contacto`); `você`, never `tu` | `“…”` |
| `de` | `du`, never `Sie`; `„…“` for quotations | `„…“` |
| `fr` | `vous`, never `tu`; a narrow no-break space (U+202F) before `:`, `;`, `?`, `!` and inside `« »` in prose (not code); the apostrophe `’` | `« … »` |

## The landing page

Its translation is a copy of `index.mdx` with the prose translated: the frontmatter's `description`, the hero's
`tagline` and the `text` of its buttons, the cards, the alt texts, the labels of `DocLinks`. The first line of its
frontmatter is the marker `# source: <hash>`. Keep its imports, component names, `path`s, and image paths as in
English; the site adjusts them. The analytics notice is translated the same way.

## Adding a locale

Add the catalog first (`/translate-catalogs`). Then:

1. Add the locale to `translationLocales` in `app/docs/src/translations.ts`, by its catalog tag, and to `locales` in
   `app/docs/astro.config.ts`, keyed by the tag in lower case with the tag as its `lang`
   (`"zh-cn": { label: "简体中文", lang: "zh-CN" }`).
2. Add the site's UI strings: `app/docs/src/content/i18n/<locale>.json`, with the keys of `en.json`.
3. Translate `docs/guide/` and the landing page first, and add `sidebar.json`.
