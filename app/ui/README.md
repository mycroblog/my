# @my/ui

The shared UI kit of the web client and the Tauri app ([ADR-0026](../../docs/adr/0026-client-workspace-and-component-architecture.md)).
It ships TypeScript source and one stylesheet, `@my/ui/theme.css`, which holds the design tokens.
The rules for writing components are in [web-client/README.md](../../docs/architecture/web-client/README.md).

`app/web/src` writes no Tailwind ([web-client/README.md A4](../../docs/architecture/web-client/README.md)): no JSX `className`, also none passed to a component
of this kit, and no import of `tailwind-variants`, `tailwind-merge` or `clsx`. Oxlint's `restrict/classes` (app/.oxlintrc.json, proved by
`app/oxlint/restrictions.test.js`) fails `scripts/task.sh web:check` on both, in every file of `web/src` and its tests. A look or a layout the web needs is a prop
of a component here, or a new prop or component; the one exception, a browser- or platform-specific adjustment that no component can express, disables
the rule on its line with the reason. The layout components take no `className` or `style` at all. Components built on React Aria Components keep
the `className` of its props, merged into their root, for use inside this package only.

## Using the tokens

An app's Tailwind entry imports the theme right after Tailwind and scans the kit's source:

```css
@import "tailwindcss";
@import "@my/ui/theme.css";
@source "../../ui/src";
@source not "../../ui/src/**/*.stories.tsx";
```

Components use only the semantic tokens below as Tailwind utilities (`bg-surface-raised`, `text-fg-muted`, `border-border-strong`)
or, where a value is computed, as `var(--color-*)` in React's `style` ([D8](../../docs/architecture/web-client/README.md#6-レスポンシブとスタイル)).
`theme.css` resets `--color-*`, so Tailwind's default palette (`bg-red-500`, `text-white`) produces no CSS.

## App-wide providers

The kit re-exports two React Aria providers that an app renders once at its root, so that the app needs no dependency on
`react-aria-components` of its own:

- `I18nProvider` with the app's locale: the components' built-in strings, number and date formats, and the direction of arrow keys.
- `RouterProvider` with the app router's `navigate`: every `Link` (and menu item or tab) with a same-origin `href` navigates
  through it instead of loading a new document. The kit itself knows no router.

## Colors

Colors have two layers in [src/theme.css](src/theme.css):

- `--palette-*` is the primitive palette of the my brand (cream, taupe, charcoal, cocoa, coral, mango, berry, mint, amber; the role badges
  keep OKLCH steps), inside the sRGB gamut so that a contrast ratio is the same on every screen.
  It is not a Tailwind namespace, so it has no utilities; only the semantic tokens refer to it.
- `--color-*` is the semantic layer. Each token is `light-dark(<light>, <dark>)` of palette colors.

| Token | Use |
| --- | --- |
| `surface` | Page background |
| `surface-raised` | Cards, menus, popovers, dialogs |
| `surface-sunken` | Inputs, wells, hover fills, media cells |
| `soft` | The selection tint: the current navigation item, secondary buttons, the reader's own reaction |
| `fg` | Body text and icons |
| `fg-muted` | Secondary text: metadata, placeholders, hints |
| `accent` / `accent-hover` | Coral as text and thin marks: links, selected tabs and toggles, checkboxes, switches |
| `accent-fg` | Text and icons on `accent` / `accent-hover` |
| `accent-fill` / `accent-fill-hover` | The bright brand coral as a fill: primary buttons, count badges, `NewItemsButton` |
| `accent-fill-fg` | Text and icons on `accent-fill` / `accent-fill-hover`: charcoal, never white on coral |
| `danger` / `success` / `warning` | Status text on a surface, or a solid status background |
| `danger-fg` / `success-fg` / `warning-fg` | Text on the matching solid status background |
| `danger-bg` / `success-bg` / `warning-bg` | The tinted box of a status message (`Callout`, `StatusMessage`), with `fg` or the matching status text on it |
| `toast` / `toast-fg` | The dark pill of `Toast` and `Tooltip`, the same in both themes, and its text |
| `toast-action` / `toast-success` / `toast-danger` / `toast-muted` | An action, a success mark, an error mark and secondary text on `toast` |
| `border` | Decorative separators between regions that are already distinct |
| `border-strong` | Boundaries of controls (inputs, checkboxes) that must be seen (WCAG 1.4.11) |
| `focus` | The focus ring |
| `overlay` | The translucent scrim behind modal dialogs and sheets |
| `alt-overlay` | The translucent scrim under the alt text over a picture (`MediaViewer`); decorative, its contrast depends on the picture |
| `avatar-<tint>` / `avatar-<tint>-fg` | The fill and the initial of an `Avatar` without a picture, for each of `lilac`, `peach`, `mint`, `lavender`, `yellow` (`avatarTints`); the same in both themes, fixed |
| `spark` | The mango mark after a heading (`Spark`); decorative |
| `pressed` / `pressed-fg` | The pressed button of `Segmented`: charcoal with white in light, coral with charcoal in dark |
| `badge-<color>` / `badge-<color>-fg` | The background and the text of a role badge (`RoleBadge`, ADR-0071), for each of `neutral`, `red`, `orange`, `yellow`, `green`, `teal`, `blue`, `purple`, `pink` (`badgeColors`) |

### Using the colors

- `accent` is coral as text and thin marks; `accent-fill` is the one bright primary fill of a view (one primary action per view). Text on
  `accent-fill` is charcoal (`accent-fill-fg`): never white on coral.
- Mango is a small spark only (the heading spark, `spark`, and the focus ring in the dark theme), never a fill or a text color.
- Selection is shown by `accent` plus a change of shape (a filled icon, a check, a dot), not by color alone.

### Contrast pairs

[src/tokens.ts](src/tokens.ts) lists every foreground / background pair the kit may draw, and `scripts/task.sh web:check` (`scripts/task.sh web:tokens`)
verifies each against WCAG 2.2 AA in both themes: 4.5:1 for text, 3:1 for control boundaries and the focus ring.

- On each of `surface`, `surface-raised`, `surface-sunken`:
  `fg`, `fg-muted`, `accent`, `accent-hover`, `danger`, `success`, `warning` (text) and `border-strong`, `focus` (UI).
- `accent-fg` on `accent` and `accent-hover`; `accent-fill-fg` on `accent-fill` and `accent-fill-hover`; `danger-fg` on `danger`; `success-fg` on `success`; `warning-fg` on `warning` (text).
- `fg`, `fg-muted`, `accent` on `soft`; `fg` and the status text on `danger-bg` / `success-bg` / `warning-bg`; `toast-fg`, `toast-action`
  (text) and `toast-muted`, `toast-success`, `toast-danger` (UI) on `toast`.
- `badge-<color>-fg` on `badge-<color>` for every badge color (text). A server theme cannot override the badge tokens, so their
  pairs hold under every theme; the color only decorates, and the badge's label carries its meaning (WCAG 1.4.1).
- `border` and `overlay` are decorative and have no minimum.

A new semantic color needs its pairs (or a place in the decorative list) in `tokens.ts`; the check fails otherwise.
A color that only meets AA on some surfaces is not a token.

## Light and dark

`:root` declares `color-scheme: light dark`, so `light-dark()` follows the OS setting (`prefers-color-scheme`) by default.
CSS resolves it before the first paint, so the default never flashes.

The theme hook is the `data-theme` attribute on `<html>`:

| `<html>` | Theme |
| --- | --- |
| no `data-theme` | The OS setting |
| `data-theme="light"` | Light |
| `data-theme="dark"` | Dark |

The attribute is set only on `<html>`; a subtree cannot switch themes.
The `dark:` variant follows the same rule, but components rarely need it because the tokens switch by themselves.

A stored theme choice must reach `<html>` before the first paint, or the page flashes in the OS theme first.
The CSP allows no inline script, and the HTML shell is shared and cached across viewers
([ADR-0028](../../docs/adr/0028-html-shell-link-previews-and-pwa.md)), so the server cannot write a per-viewer attribute (from a cookie) into it.
The choice is therefore applied by a same-origin, render-blocking classic script (`<script src>` in `<head>`, without `defer`,
`async` or `type="module"`) that reads it from storage and sets `document.documentElement.dataset.theme`.
In `app/web` that script is `public/theme.js` (served at `/theme.js`, loaded before the stylesheets), and `src/lib/theme.ts`
changes the choice and the attribute together; both use the localStorage key `my:display:theme` that `src/lib/storage.ts` declares.

## Typography

- `font-sans` starts with `"my Noto Color Emoji"`, the one web font, and is OS fonts after it: the platform UI font, then Japanese,
  Chinese and Korean system fonts (Hiragino, Yu Gothic UI, Meiryo, PingFang, Microsoft YaHei UI, Malgun Gothic), Arabic system fonts
  (Geeza Pro, Segoe UI Arabic, Tahoma), the Noto families of Linux and Android, and the OS emoji fonts. `font-mono` also starts with the
  emoji font.
- Unicode emoji are drawn with Noto Color Emoji everywhere ([ADR-0095](../../docs/adr/0095-noto-color-emoji.md)). `@my/ui/emoji.css`
  holds its `@font-face` rules: one per group of `emoji-test.txt`, each with a COLRv1 and an OpenType-SVG WOFF2 of `src/fonts/emoji/`
  that the browser picks with `tech()`, and a `unicode-range` of emoji code points only (not `#`, `*`, the digits, `©`, `®`, `™` and the
  other symbols that body text uses), so a page loads only the splits of the emoji it shows and the OS fonts draw every other character.
  `theme.css` does not import it: the app and Storybook import it after `theme.css`, and the server-rendered pages do not, and keep the OS emoji ([ADR-0096](../../docs/adr/0096-emoji-css-import.md)). `scripts/task.sh emoji:generate`
  writes the fonts and `emoji.css`, never edit them by hand; the fonts are under the SIL Open Font License 1.1 (`src/fonts/emoji/OFL.txt`).
- `font-serif` is `Georgia, "Noto Serif JP", serif`, for brand moments only: display headlines, page titles, `EmptyState` headings and avatar
  initials. Body text and controls stay `font-sans`.
- Sizes are `text-xs` (0.75rem), `text-sm` (0.875rem), `text-base` (1rem), `text-lg` (1.125rem), `text-xl` (1.25rem), `text-2xl` (1.5rem)
  and `text-3xl` (1.875rem). Line heights are taller than Tailwind's defaults (1.7 for `text-base`) for CJK and Arabic, and `body`
  uses the `text-base` line height.

### Type roles

Text takes its size from its role, never from the screen it is on
([web-client/16-look-states-and-wording.md §16.2](../../docs/architecture/web-client/16-look-states-and-wording.md#162-文字)):

| Role | Classes | Used for |
| --- | --- | --- |
| Page title | `text-2xl font-semibold` | The `h1` of `PageHeading`, one per page |
| Section title | `text-lg font-semibold` | `h2` of a region (replies, settings groups), the heading of `EmptyState` and of dialogs |
| Focused content | `text-lg` | The body of the post a detail page is about |
| Body | `text-base` | Post bodies, profile text, form values, menu items |
| Secondary | `text-sm text-fg-muted` | Handles, times, counts, descriptions and hints of fields |
| Label | `text-xs` | Badges, the labels of the bottom tab bar and the side rail |

`text-xl` and `text-3xl` have no role and are not used by screens. Names (`font-semibold`) are the only other weight;
emphasis inside running text comes from the content, not from the UI.

### Names in rows

A user's name gets `dir="auto"` (Y10), which also turns `text-align: start` around for a right-to-left name.
When the `dir="auto"` element is a block (a `p`, or a `span` inside a flex or grid row), it also takes
`align-match-parent` (`text-align: match-parent`, with the `-webkit-` value Chromium needs), so it lines up with the page's
start like the rows around it and only the text inside it runs right to left
([G9](../../docs/architecture/web-client/16-look-states-and-wording.md#162-文字)). Post bodies and content warnings keep their own direction's alignment.
The utility is not named `text-*` because `tailwind-merge` would read that as a color and drop the element's color class.

## Spacing, radius, shadow

- Spacing is Tailwind's 0.25rem step (`p-1` = 0.25rem, `gap-4` = 1rem). Padding and gaps are limited to `0.5`, `1`, `1.5`, `2`, `3`, `4`, `6`, `8`
  and `10`; margins and sizes hold geometry (a description aligned with a switch's label) rather than a role and may use other steps.
  Each role has one value:

  | Role | Value |
  | --- | --- |
  | Inside a small control: a badge, a toggle button, a label over media | `px-1.5`, `gap-1.5` |
  | Icon and its text, a name and its handle | `gap-1` |
  | Controls in a row (actions of a post, buttons of a dialog) | `gap-2` |
  | Avatar and the content beside it; lines inside a card | `gap-3` |
  | Inside a row of a list or a card | `px-4 py-3` |
  | Page gutter (set by the app layout only) | `px-4` |
  | Fields of a form; blocks inside a page | `gap-4` |
  | Sections of a page | `gap-6` (`gap-8` from `@3xl`) |
  | Around an `EmptyState` or a page-level spinner | `py-10` |

- Radius: `rounded-sm`, `rounded-md`, `rounded-lg`, `rounded-xl`, `rounded-2xl`, `rounded-avatar` and `rounded-full`. The brand design's radii
  are drawn on these steps, never as arbitrary values:

  | Token | Value | Used for (design radius) |
  | --- | --- | --- |
  | `rounded-sm` | 0.375rem (6px) | Checkboxes, small marks |
  | `rounded-md` | 0.625rem (10px) | Toggles, list items, text fields (the design's 11px), quotes (12px) |
  | `rounded-lg` | 1rem (16px) | Menus, popovers, media |
  | `rounded-xl` | 1.1875rem (19px) | Cards, posts, dialogs, toasts (the design's 18px) |
  | `rounded-2xl` | 1.5rem (24px) | Sheets, the tile of an `EmptyState` |
  | `rounded-avatar` | 21% | Avatars: a share of the side, so a server theme cannot override it (`radius-avatar` is in `fixedTokens`) |
  | `rounded-full` | pill | Buttons, badges, search fields |

- Shadow: `shadow-card` (cards and posts), `shadow-sm`, `shadow-md`, `shadow-lg`, warm-tinted. In the dark theme raised surfaces are told apart
  by `surface-raised`, not by shadow: a server theme's shadow is one value for both themes (no `light-dark()`), so the black-based dark
  shadows of the design are not used.
  Tailwind copies a `--shadow-*` value into its utilities, so the values live in `--elevation-sm|md|lg`, which the `--shadow-*` tokens refer to
  and a server theme overrides (`--elevation-card`, `--elevation-sm|md|lg`).
- Radius, shadow, duration, and easing utilities take tokens only: no arbitrary values (`rounded-[3px]`, `shadow-[…]`, `duration-[…]`).

## Shape, elevation and states

- **Pills** (`rounded-full`): buttons, badges, tags, toasts and the search field. **Cards** (`rounded-xl`): posts, cards and dialogs,
  each with a 1px `border` and `shadow-card`. **List items** and toggles are `rounded-md`. **Avatars** are rounded squares
  (`rounded-avatar`), never circles.
- Borders come first: a card is told apart by its `border`, with the barely-there `shadow-card`. Floating layers use `shadow-sm` (media badges),
  `shadow-md` (lists, tooltips, `NewItemsButton`) and `shadow-lg` (dialogs, menus, popovers, toasts).
- **Serif** (`font-serif`) is for brand moments only: the display line of the brand, page titles, `EmptyState` headings and avatar
  initials. All other UI is sans in heavy weights; the serif stays light.
- States: hover is a `surface-sunken` fill or `accent-hover`; selected is `accent` plus a change of shape; invalid is a `danger` border plus a
  message with a warning icon; disabled is half opacity; pending is a `Spinner` while the control keeps its focus.
- Status messages, `RegionError` and `OfflineBanner` are tinted boxes with an icon, not a start-edge bar; `Toast` and `Tooltip` are dark pills.

## Focus

`focus-ring` is a 3px `focus` outline with a 2px offset. Apply it as `focus-visible:focus-ring`, or `data-focus-visible:focus-ring`
on React Aria Components. It is an outline, so it stays visible in forced-colors mode. Every `:focus-visible` element also gets the
same ring from the base layer when it sets none of its own.

## Motion

Transitions and animations go under `motion-safe:` only ([D6](../../docs/architecture/web-client/README.md#6-レスポンシブとスタイル)),
for example `motion-safe:transition-colors`; `scripts/task.sh web:tokens` fails on `transition-*` / `animate-*` without it (`*-none` is allowed).

- Durations: `duration-fast` (100ms), `duration-normal` (200ms), `duration-slow` (300ms). A bare `transition-*` uses 200ms.
- Easing: `ease-standard` (the default), `ease-enter` (things appearing), `ease-exit` (things leaving).

## Icons

Icons are part of the kit, so that every screen draws the same shapes at the same sizes
([web-client/16-look-states-and-wording.md §16.3](../../docs/architecture/web-client/16-look-states-and-wording.md#163-アイコン)):

- The shapes are [Phosphor](https://phosphoricons.com/)'s, through `react-icons/pi` ([ADR-0043](../../docs/adr/0043-icon-set.md)).
  `src/Icons.tsx` is the only file that imports `react-icons`, and only `react-icons/pi` (lint enforces both). Its
  `iconTable` maps each meaningful name (`ReplyIcon`) to its Phosphor icon off and on and to whether it has a direction;
  each export renders one row, and the `Foundations/Icons` gallery lists the whole table with the Phosphor names.
- The base weight is Regular. Neither a screen nor a component draws SVG of its own (lint rejects `<svg>`, `<path>` and
  the other SVG elements in JSX); a missing icon is a new row of the table (look the shape up by name in the Phosphor
  catalog) and a new export, which the gallery then shows. The exceptions, listed with their reasons in
  `app/.oxlintrc.json`, are the marks of a control's own drawing (the check of `Checkbox`, the chevrons of `Select`
  and `ContentWarning`, the arrow of `Tooltip`, the silhouette of `Avatar`) and stand-in content in tests and stories.
- They are decorative: `aria-hidden="true"` and `focusable="false"`. The meaning is in the visible label or the accessible
  name of the control. An icon-only control has an `aria-label` and a `Tooltip` with the same text.
- The control sizes its icon: `Button`, `ToggleButton` and `Link` size a child `svg` (`size-5` in buttons, also in a
  `toolbar` button among toggles, `size-4` in a small button and next to a link's text), and a `navigation` link sizes
  every `svg` in it at `size-6`. An icon sets no size of its own, and neither does the screen that places it, so it never
  renders at 0 × 0.
- A toggle shows its state by the icon's shape as well as its color: on is Phosphor's Fill weight for a shape that closes
  (star, bookmark, warning) and the Bold weight for one that does not (boost).
- Icons with a direction (reply, back, external link) mirror in right-to-left text with `rtl:-scale-x-100`; Phosphor's
  `mirrored` ignores the writing direction, so it is not used.

## Logo

The wordmark is the hand-lettered artwork in `src/logos/`, used as given and never redrawn or recolored. `App-Icon` is for app and
avatar slots; the three spark strokes (`mango`, `logo-spark-coral`, `logo-spark-amber`) stay exactly as drawn:

- **The inks are fixed SVG fills, not tokens.** The logo is a brand mark, not UI: its colors (`coral`, `logo-red`, the three
  spark inks) must not follow a server theme override, and an `<img>` of the file cannot take a token anyway. The parts load the
  files with `<img>`, so no `<svg>` is drawn in JSX and the token check has nothing to inspect.
- `Wordmark` picks the variant by ground: `Wordmark-Primary-Color` on light, `Wordmark-DarkBg` on dark (the `dark:` variant,
  so `data-theme` wins over the OS setting), `tone="monochrome"` for the single charcoal ink. `Wordmark-LightBg` (red letters
  for light grounds where coral looks weak) is not bundled; add it with a `tone` when a screen needs it.
- It is sized by height (`sm` / `md` / `lg`); the width follows the 1250 x 700 artwork. `name` is the accessible name ("my",
  from the locale files); `decorative` is required, and `true` gives `alt=""` when the server name is shown next to it.
- `BrandMark` is the wordmark next to the server name (`instanceName`, `dir="auto"`, truncated when long).
- The favicon and PWA icons in `app/web/public` are made from `App-Icon.svg`.

## States of a region

The kit gives every region of data the same four states
([web-client/16-look-states-and-wording.md §16.4](../../docs/architecture/web-client/16-look-states-and-wording.md#164-空読み込み中エラーオフライン)):

| State | Component | Notes |
| --- | --- | --- |
| Empty | `EmptyState` | A heading of one short fact, a description of what appears here, and an action that leads on. `headingLevel` is one below the region's heading. |
| Loading | `PostSkeleton`, `AccountRowSkeleton`, `NotificationRowSkeleton` (or `Skeleton` shapes) for the first page of a list; `Spinner` for a button or a small region | Never both in one region. The region carries `aria-busy` and one loading label. |
| Error | `RegionError` | What happened (`heading`), the reason (`reason`) and the retry button (`retryLabel`, `onRetry`, `isRetrying`). It replaces only the region; content already loaded stays. |
| Offline | `OfflineBanner` | One `role="status"` bar from the app layout; regions show their own error only when a request fails. |

- The skeleton rows are hidden from assistive technology (`aria-hidden`) as a whole, so a region that repeats them
  still has only the one loading label it gives itself (a `VisuallyHidden` text, or a `Spinner`).
  `PostSkeleton` and `NotificationRowSkeleton` have the padding, bottom border and container layout of `PostCard`;
  `AccountRowSkeleton` has no border, because account lists separate their rows with `divide-y`.
- `RegionError` is not a live region by default: a failure that came with the page is found by reading.
  `alert` makes it `role="alert"`, only for the result of the user's own action, such as a retry that failed again
  ([APG alert](https://www.w3.org/WAI/ARIA/apg/patterns/alert/)). Without `headingLevel` the message is a paragraph
  (the failure of the next page of a list); with it, a heading one below the region's heading.
- `OfflineBanner` is rendered once for the whole session, empty while `isOffline` is false, so that going offline is
  announced (a live region announces only changes made after it is in the page). It sticks to the top edge
  (`position: sticky`) and pads itself for `env(safe-area-inset-top)`, so the app renders it before any element that
  already pads for that inset.

## Server themes

An administrator can override the tokens other than the palette and the badge colors for the whole server and add a custom CSS
([ADR-0059](../../docs/adr/0059-design-tokens-and-instance-theme.md), [operations](../../docs/operations/instance-settings.md#テーマ)).
The server serves them as one stylesheet at `/theme/{revision}.css`: a `:root` rule with the overrides, then the custom CSS, both outside
any cascade layer, so they win over the `@layer theme` that Tailwind emits the defaults into and over the utilities. That works only while
the utilities refer to the variables, so no token is declared in `@theme inline`.

[scripts/theme-tokens.ts](scripts/theme-tokens.ts) (`scripts/task.sh web:tokens-gen`) generates the list of the overridable tokens from `theme.css`, with
each token's kind, defaults (colors resolved to `#rrggbb` per theme), and bounds from `tokenRules` in [src/tokens.ts](src/tokens.ts), and the
contrast pairs whose two colors are overridable (`themeColorPairs`): `src/themeTokens.gen.ts` for the theme editor and `internal/domain/instance/theme_tokens.gen.go` for the server, which checks
every theme against them. A token that matches no rule (and is not in `fixedTokens`) fails the generator. Only the Go file is committed (ADR-0081):
the tasks that read `src/themeTokens.gen.ts` (type checks, tests, builds, Storybook) generate it first with `scripts/task.sh web:tokens-gen-ts`.

| Tokens | Kind | Override |
| --- | --- | --- |
| `color-*` (but `color-badge-*`) | color | `#rrggbb` per theme (`#rrggbbaa` for `color-overlay`); every contrast pair must keep AA in both themes |
| `font-sans`, `font-mono`, `font-serif` | font family | Family names, quoted or plain, comma-separated (at most 30); keep `"my Noto Color Emoji"` first in `font-sans` and `font-mono`, or the web client draws emoji with the OS fonts ([ADR-0096](../../docs/adr/0096-emoji-css-import.md)) |
| `text-*` | length | `0` or rem / px, 0.625rem to 3rem |
| `text-*--line-height` | number | 1 to 2.5 |
| `spacing` | length | 0.125rem to 0.5rem (half to twice the default step) |
| `radius-*` (but `radius-avatar`) | length | 0 to 2rem |
| `elevation-*` | shadow | `none` or up to 4 `[inset] <x> <y> [<blur> [<spread>]] <#color>` layers, lengths within 4rem |
| `transition-duration-*`, `default-transition-duration` | duration | ms / s, 0 to 1000ms |
| `ease-*`, `default-transition-timing-function` | easing | `linear`, `ease`, `ease-in`, `ease-out`, `ease-in-out`, or `cubic-bezier()` with x in 0..1 |

The quality gates (axe, screenshots, horizontal scroll) check the default theme; one story shows representative components under a
sample theme (see "Stories"). A theme's color tokens keep AA because the server checks them; the other overrides and the custom CSS can still break layout, tap targets, or right-to-left text, which is the administrator's
responsibility. `themeContrast` computes what the server checks, with the same formulas as [src/color.ts](src/color.ts) and the Go domain.

## The token check

`scripts/task.sh web:tokens` runs [scripts/check-tokens.ts](scripts/check-tokens.ts). It fails when:

- a pair in `tokens.ts` is below AA in either theme, a background in a pair is translucent, or a semantic color is in no pair and not decorative;
- a palette color is outside the sRGB gamut, or a semantic color is not `oklch()`, hex, a `var(--palette-*)` or `light-dark()` of those;
- a source file Tailwind scans for `src/storybook.css` or `app/web/src/index.css` uses a default-palette color (`text-gray-500`),
  an arbitrary color (`bg-[#fff]`, `text-(--palette-gray-50)`, `[color:red]`), motion outside `motion-safe:`, an arbitrary radius, shadow,
  duration, or easing (`rounded-[3px]`), or a padding or gap step outside the list under "Spacing, radius, shadow";
- the committed list of the overridable tokens (`internal/domain/instance/theme_tokens.gen.go`) differs from what
  `scripts/task.sh web:tokens-gen` writes from `theme.css` and `tokens.ts`, or `theme.css` uses `@theme inline`;
- the Storybook's sample theme (`.storybook/sampleTheme.ts`) fails what the server checks of a saved theme: a token that is not overridable,
  a color that is not `#rrggbb`, a length out of its bounds, or a contrast pair below AA in either theme.

Other unknown class names produce no CSS; catching those is the job of the Tailwind lint rules ([D9](../../docs/architecture/web-client/README.md#6-レスポンシブとスタイル)).

## Form controls

[src/index.ts](src/index.ts) exports these wrappers of React Aria Components (RAC). Each takes RAC's props, is controlled or
uncontrolled as RAC is (`value` / `onChange` or `defaultValue`; `isSelected` / `defaultSelected` for toggles), and receives every
string through props or children.

| Component | Wraps | Variants and extras |
| --- | --- | --- |
| `Button` | `Button` | `variant`: `primary` / `secondary` (default) / `ghost` / `danger`; `size`: `sm` / `md` (default); `isPending` |
| `Link` | `Link` | `variant`: `inline` (default, underlined in running text) / `standalone` |
| `TextField` | `TextField` + `Input` | `label`, `description`, `errorMessage`, `placeholder`; `grow` takes the room left in a row; `highlights` (below) |
| `TextArea` | `TextField` + `TextArea` | as `TextField`, plus `rows` and the character count below |
| `TextAreaAutocomplete` | `Autocomplete` + `TextField` + `TextArea`, `Popover` + `ListBox` | as `TextArea`, plus the completion of the word after a trigger character (below) |
| `TextFieldAutocomplete` | `Autocomplete` + `TextField` + `Input`, `Popover` + `ListBox` | as `TextField`, plus the same completion on one line (below) |
| `Checkbox` | `CheckboxField` + `CheckboxButton` | children as the label, `description`, `errorMessage`, `isIndeterminate` |
| `Switch` | `SwitchField` + `SwitchButton` | children as the label, `description`, `errorMessage` |
| `RadioGroup`, `Radio` | `RadioGroup`, `RadioField` + `RadioButton` | `orientation`: `vertical` (default) / `horizontal`; a `description` per `Radio` |
| `Select`, `SelectItem` | `Select` + `Popover` + `ListBox`, `ListBoxItem` | `items` with a render function, or `SelectItem` children; single selection. `SelectItem`'s `description` adds a line under the label (the option's accessible description) that the closed select does not show. `variant="toolbar"` (with `aria-label` and an `icon`) is a ghost `toolbar`-sized button for a row of tools that shows the chosen option |
| `ReasonField` | `TextArea` | The reason for an operator's action: three rows and a required character count (`characterLimit`, `formatRemaining`) |
| `DurationSelect` | `Select` | `durations` in seconds with `formatDuration`; `indefiniteLabel` adds "no end", last; the value is seconds or `null` for no end |
| `Form` | `Form` | The layout props of `Stack` (`gap`, `align`, `justify`, and the box props: `container`, `width`, …); `validationErrors` from the server show on the named fields. A plain `<form>` without them can be `<Stack as="form">` |

- Each component has one `tailwind-variants` recipe; its variant props are typed from the recipe (`VariantProps`).
  `className` (a string or RAC's function of the render props) is merged into the root with `tailwind-merge`, so a later class wins.
- RAC links the label, the description and the error message to the control (`aria-labelledby` / `aria-describedby`).
  The error message shows while the field is invalid: `isInvalid`, a failing `validate`, or a native constraint (`isRequired`).
- States are styled from RAC's `data-*` attributes (`data-hovered:`, `data-pressed:`, `data-selected:`, `data-invalid:`).
  Keyboard focus shows `focus-ring`; text inputs show it on any focus. Controls are at least 24 px tall and 44 px under `pointer-coarse:`.
- `TextArea` with `characterLimit` and `formatRemaining` (the count message for a remaining count, negative when over) shows a count
  after the control, as the [GOV.UK character count](https://design-system.service.gov.uk/components/character-count/) does:
  - the count is counted in Unicode code points, like the server ([data-model/README.md](../../docs/architecture/data-model/README.md));
  - the visible count is linked to the text area with `aria-describedby` and hidden from the reading order, so it is read with the field;
  - a visually hidden `role="status"` region announces it when typing pauses for a second, and at once when the remaining count
    reaches 50, 20 or 0 ([ADR-0027](../../docs/adr/0027-ui-quality-gates.md));
  - longer text is kept, not cut off: the count turns `danger` and the field becomes invalid.
- `TextAreaAutocomplete` completes the word after a trigger character (`@`, `#`, `:`) in a native textarea
  ([ADR-0092](../../docs/adr/0092-composer-autocomplete-highlight.md) §3). It knows no domain: it takes `triggers` (each trigger
  character with the fewest query characters it needs), `findWord` (the word at the caret; the web passes `activeWord` of
  `lib/textTokens.ts`), `loadSuggestions(word, signal)` (sync or async; asked after typing pauses for `queryDelay`, 200 ms by
  default or a function of the word, so local data can answer at once; aborted when the word changes, a rejection just closes the list), `renderSuggestion`, and the strings `suggestionsLabel`,
  `formatSuggestionCount`, `loadingLabel` and `emptyLabel`. A suggestion is `{ id, value, textValue }`; `value` replaces the word.
  - The textarea stays a `textbox` with `aria-autocomplete="list"`, `aria-controls` and `aria-activedescendant`; the chosen option
    has `aria-selected="true"`; a `role="status"` region announces the count, loading (after 300 ms) and no results, which the
    popover also shows. Spell checking, autocorrect and the return key stay on.
  - While the list is open: ↑ / ↓ move (stopping at the ends), Enter and Tab accept, Esc closes it without closing an
    enclosing `Sheet`, and the first option is chosen whenever the options change. Every other key, every key while it is
    closed and during an IME composition (`isComposing`, `keyCode` 229), and Ctrl / Cmd + Enter reach the textarea and the form.
  - Accepting replaces the word with `value` and a space (none when whitespace follows) through `execCommand("insertText")`,
    one undo step, or `setRangeText` where that fails.
  - The popover is as wide as the text area, under the caret's line, or above it when the visual viewport has no room
    below. The caret's line comes from `TextAreaMirror`, an `aria-hidden` copy of the text laid out with the textarea's own
    font, spacing, line height, padding and borders behind the transparent textarea, which the highlight layer also uses.
    The mirrored textarea is `dir="auto"` and has a whole-pixel line height, so the scrolled mirror lines up in every engine.
  - `endAction` (the emoji picker's button) is rendered outside Autocomplete's contexts, which would otherwise reach the
    picker through its portal and take over its search field, menus and grid.
- `TextFieldAutocomplete` is the same completion on a one-line `TextField` (a content warning, a display name): it takes
  every prop of `TextField` and the completion's own, its mirror is the single-line one, and the popover opens under the
  whole input (above it without room below).
- `TextArea`, `TextField` and `TextAreaAutocomplete` take `highlights`, the ranges to highlight while typing
  ([ADR-0092](../../docs/adr/0092-composer-autocomplete-highlight.md) §4): `{ kind, start, end }` in UTF-16 units, where `kind`
  is `mention`, `hashtag`, `url`, `shortcode` or `over` (text over the limit, which wins where it overlaps a token). The kit
  knows no rules; the web builds the ranges (`lib/textHighlights.ts`).
  - They are painted as backgrounds (`highlight-<kind>` tokens) on `TextAreaMirror`, the `aria-hidden` layer behind the
    transparent control, which draws the text itself. The control becomes `dir="auto"` with a whole-pixel line height; an
    input's layer lays out one line (`white-space: pre`), centred, and follows its sideways scroll.
  - Forced-colors mode drops backgrounds, so the ranges are underlined with `CanvasText` there (dashed over the limit). The
    layer opts out of forced colors and moves in front of the control, whose text Chromium backs with a `Canvas` plate.

## Overlays and announcements

The overlay components wrap React Aria Components, which handle focus, Esc and the reading direction
([WAI-ARIA APG](https://www.w3.org/WAI/ARIA/apg/patterns/)). All strings, including the accessible names of × buttons, come in as props.

| Component | Pattern | Notes |
| --- | --- | --- |
| `Dialog` / `Sheet` in a `DialogTrigger` | Modal dialog | Traps focus, closes on Esc and on a press on the scrim (`isDismissable={false}` for a form whose input that would lose), returns focus to the trigger. `closeLabel` (the × button) is required; only `role="alertdialog"`, the confirmations whose content has a cancel button, passes `null`; `isOpen` / `onOpenChange` without a trigger. `Sheet` sits on the bottom edge below a 32rem container width, with safe-area padding and a height capped in `svh`. |
| `ConfirmDialog` in a `DialogTrigger` | Alert dialog | The confirmation before an action that cannot be undone (see below). |
| `Popover` in a `PopoverTrigger` | Non-modal dialog | Named by `title` or `aria-label`. |
| `Menu`, `MenuItem`, `MenuSeparator` in a `MenuTrigger`; `SubmenuTrigger` | Menu button | `intent="danger"` for destructive items. Submenus open towards the inline end. |
| `Tooltip` in a `TooltipTrigger` | Tooltip | Describes its trigger; never the trigger's only name. |
| `Tabs`, `TabList`, `Tab`, `TabPanel` | Tabs | `selectedKey` / `onSelectionChange` let a route keep the tab in the URL. Tabs that do not fit scroll sideways in the list, which has no scrollbar and fades out (statically, a 2rem mask) at each inline edge with hidden tabs (`data-overflow-start` / `data-overflow-end`); the focused or selected tab scrolls into view, clear of the fade. `TabPanel` takes `gap` (its content one above another, as a `Stack`) and `offset` (`3` / `4`: `pt-3` / `pt-4` more room under the list than the `gap-4` of `Tabs`). |
| `AnnouncerProvider`, `useAnnouncer` | Live regions | One polite `role="status"` and one assertive `role="alert"` region for the app. |
| `ToastProvider`, `useToast` | Toasts | Announced through the Announcer; the visible list is a labelled region. |
| `StatusMessage` | Status | The result of an action that stays on the page: a `role="status"` region, empty until it has a message. |

Mount `AnnouncerProvider` once at the app root and `ToastProvider` inside it. `announce(message, politeness)` and
`toast.show(...)` are called from event handlers and mutation callbacks, and never move focus. Each announcement is a new node in its
region, so a repeated message is read again, and it is removed after `clearAfter` (7 s). The regions carry `data-live-announcer`, so they
stay exposed while a modal hides the rest of the page. The kit has its own regions and toast queue because React Aria Components' Toast is
`UNSTABLE_` and its live announcer is internal to react-aria. A toast without an action closes after `defaultTimeout` (5 s), but not
while the pointer is over the toasts or focus is in them; a toast with an action stays until it is dismissed.

`ConfirmDialog` follows the [APG alert dialog](https://www.w3.org/WAI/ARIA/apg/patterns/alertdialog/) on `Dialog`
(`role="alertdialog"`, named by `title`, described by `description`):

- the title is a question, the description says what happens and whether it can be undone, and the buttons are verbs
  (G22 in [web-client/README.md](../../docs/architecture/web-client/README.md)); `tone` is `danger` (default) or `primary`;
- on open, focus moves to the first field of `children` (a `ReasonField`, a `DurationSelect`), or else to the cancel button, never to the
  destructive one ([APG dialog, initial focus](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)); Esc and cancel close it and focus
  returns to the trigger; there is no × button and a press outside does not close it;
- the body is a form: the confirm button submits it, the fields' constraints (`isRequired`) are checked first, and only then
  `onConfirm(close)` runs. The dialog stays open until `close` is called, `isPending` makes the confirm button wait, and `errorMessage`
  shows a failure in the dialog (`role="alert"`) so that the action can be tried again.

`StatusMessage` is rendered before the action, empty, and gets the result as children once the action has succeeded: a live region
announces only changes made after it is in the page. A failure is not a status; it goes to `ConfirmDialog`'s `errorMessage` or a toast.

## Display

| Component | Notes |
| --- | --- |
| `Picture`, `AutoplayContext` | An image in the rendition that fits (`srcset` / `sizes` from `ImageSources`). An animation's moving rendition is a `<source>` for `prefers-reduced-motion: no-preference` only, and with `AutoplayContext` off it moves only while pointed at or touched (ADR-0054 §3). `Avatar` (`sources`) and `MediaGrid` (`MediaItem.sources`) use it. |
| `DescriptionList`, `DescriptionItem` | The facts of a record: a `<dl>` that is its own size container, with up to `columns` (1–4) columns as it widens; values wrap anywhere. |
| `DetailSection` | A titled region of a detail page: a `<section>` named by its `h2`; with `headingRef` the heading takes focus from script. |
| `Table`, `TableHead`, `TableColumnHeader`, `TableBody`, `TableRow`, `TableRowHeader`, `TableCell` | A table to read: the native `table`, which needs no dependency beyond ADR-0031's list (a table to select or sort in would wrap React Aria's `Table`). It sits in a focusable `group` named like the table that scrolls sideways by itself when the columns do not fit the width, so the page never does. `caption` names it and is visually hidden; without it, `aria-labelledby` names it by a visible heading. `density`: `regular` (body size, a rule under every row) / `compact` (`text-sm`, fixed columns, rules between rows; `TableColumnHeader`'s `width`: `narrow` `w-16` / `wide` `w-2/5`). `TableCell` takes `numeric`, `wrap="anywhere"` and `align="matchParent"`; `TableRowHeader` the last two. |
| `Segmented`, `SegmentedItem` | A pill switcher for two to four views of the same content (`value` / `onChange` by segment `id`): a React Aria single-selection `ToggleButtonGroup`, so a radiogroup that the arrow keys move through; pressed is `pressed` / `pressed-fg`. |
| `Tag` | A short word that qualifies the value beside it ("Estimated", `outlined`: `rounded-full border border-border px-2 text-sm text-fg-muted`) or labels an account (`variant="pill"`, `as="li"` in a `ul`). |
| `ProfileHeader`, `ProfileHeaderImage`, `ProfileFields` / `ProfileField` | The heading of a profile: header image (or band), overlapping avatar, actions slot, name, handle, badges, tag pills, text, metadata fields, joined line and counts; `ProfileHeaderImage size="preview"` is the 3:1 image of the profile form. |
| `Poll`, `PollResults`, `PollChoices`, `PollSummary`, `PollHint` | A poll (FR-112): `Poll` is the named section; `PollResults` the list of shares with a decorative bar (`aria-hidden`; the percentage and the "your vote" word carry the meaning); `PollChoices` the radio or checkbox form with the vote button; `PollSummary` the total and deadline line; `PollHint` a note. Titles take `dir="auto"`, `lang` and custom emoji. |

## Layout and text

Screens and feature components write no `className`: they lay themselves out with the primitives below and set their
text with `Heading`, `Text`, `Hint` and `InlineError` ([web-client/16-look-states-and-wording.md §16.1, §16.2](../../docs/architecture/web-client/16-look-states-and-wording.md#16-見た目状態文言)).
None of these components takes `className` or `style`. Every value is a typed prop: a gap or a padding is a step of G1
(`Space`: `"0"`, `"0.5"`, `"1"`, `"2"`, `"3"`, `"4"`, `"6"`, `"8"`, `"10"`) picked by its role under "Spacing" above, never an
arbitrary value. A look that no prop gives is a new prop or component in the kit, not a class in the app.

| Component | Renders | Its own props |
| --- | --- | --- |
| `Stack` | `flex flex-col` | `gap`; `align` (`stretch` default / `start` / `center` / `end` / `baseline`); `justify` (`start` / `center` / `end` / `between`); `divided` (`divide-y divide-border`: rows of a list, no gap, G3); `marker` (`disc` / `decimal` with `ps-6`, `discInside`); `rowAt` (`sm` / `md`: a row with centered items from that container width) |
| `Cluster` | `flex flex-wrap` | `gap` (both axes); `rowGap` (between wrapped lines, when it differs from `gap`); `align`; `justify`; `wrap={false}` for one line; `inline` (`inline-flex`, in the line of the text around it) |
| `Grid` | `grid grid-cols-1`, then `columns` (`2` default / `3`) from the container width `at` (`sm` default / `md`) | `columns`; `at`; `gap` (both axes); `rowGap` |
| `Box` | A block with no layout of its own | Only the box props below. It stays beside `Card`: a `Box` is a layout primitive (a padded row, a size container, a capped width, an optional frame) with no surface, whereas a `Card` is the one raised surface of the brand (`bg-surface-raised`, `border`, `rounded-xl`, `shadow-card`) |
| `Card` | A raised surface: `bg-surface-raised border rounded-xl shadow-card` (no shadow in dark), a `div` (or `as`: `section`, `article`, `aside`, `li`) | `padding` (`0` for rows with their own padding / `3` / `4` side panel / `6` page panel, default / `8` settings pane) |
| `SectionHeader` | The heading (children) and `actions` (in a `flex flex-wrap gap-2` row): stacked with `items-start gap-2`, on one line with `justify-between` from `@md` | `actions` (nothing, `null` or `false` leaves the heading alone) |
| `DialogActions` | `flex flex-wrap justify-end gap-2`: the buttons at the end of a dialog, cancel first | — |
| `Heading` | `h1`–`h6` by `level` | `level`; `size` (`page` `text-2xl font-bold` / `section` `text-lg font-bold` / `subsection` `font-bold` / `label` `text-sm font-bold` / `medium` `font-bold`; by default level 1 is `page`, 2 `section`, the others `subsection`); `tone` (`fg` / `muted`); `wrap` (`anywhere` / `truncate` / `pretty`); `mono`; `shrink` (`min-w-0`); `focusable` (`tabIndex={-1}` and `outline-none`, for a heading focus moves to from script); `spark` (`✳` / `✧`: a decorative mango mark after the text, `aria-hidden`; `PageHeading` has it too, and sets the text in the serif `page-title`) |
| `Text` | A `span` (or `as`: `p`, `div`, `li`, `dt`, `dd`, `code`, `time`, `strong`, `small`, `label`, `legend`) | `size` (`xs` / `sm` / `base` / `lg`), `tone` (`fg` / `muted` / `danger` / `warning` / `success` / `accent`), `weight` (`normal` / `medium` / `semibold`), each inherited when unset; `wrap` (`anywhere` / `truncate` / `pretty`); `whitespace` (`preWrap` / `preLine`); `numeric` (`tabular-nums`); `mono`; `block`; `align` (`start` / `center` / `matchParent`); `lineClamp` (`2` / `3`); `selectAll`; `shrink`; `eyebrow` (small upper-case `text-accent` label above a heading) |
| `Hint` | Secondary text (G6), a `p` (or `as` `span` / `div`): `text-sm text-fg-muted` | `size="inherit"` drops `text-sm`; the other props of `Text` but `tone` |
| `InlineError` | An error in place, a `p` (or `as` `span` / `div`): `text-sm text-danger` | `size="inherit"` drops `text-sm`; `alert` (`role="alert"`, only for the result of the user's own action, G16); the other props of `Text` but `tone` |

`Stack`, `Cluster`, `Grid` and `Box` share the box props:

| Prop | Classes |
| --- | --- |
| `as` | The element: `div` (default), `section`, `article`, `aside`, `header`, `footer`, `nav`, `main`, `form`, `fieldset`, `ul`, `ol`, `li`, `dl`, `dd`, `p`, `span`, `label`. The element's attributes (`aria-*`, `id`, `onSubmit`, `ref`) pass through. |
| `container` | `@container`, so the `@sm:` / `@md:` variants inside (`rowAt`, `Grid`'s `at`, `SectionHeader`) answer to its width |
| `width` | `prose` `max-w-prose`; `sm` `w-full max-w-sm`; `xs` `w-full max-w-xs`; `full` `w-full` |
| `shrink`, `grow` | `min-w-0`, `flex-1` |
| `self` | `start` / `center` / `end`: `self-*` |
| `padding`, `paddingX`, `paddingY` | `p-*`, `px-*`, `py-*` in a spacing step (a row of a list or a card is `paddingX="4" paddingY="3"`) |
| `paddingTop`, `paddingBottom`, `marginTop`, `marginStart` | `pt-*`, `pb-*`, `mt-*`, `ms-*` in a spacing step |
| `fullRow` | `basis-full`: takes a whole line of a wrapping `Cluster` |
| `border` | `md` `rounded-md border border-border`; `lg` `rounded-lg border border-border`; `y` `border-y border-border`; `top` `border-t border-border`; `bottom` `border-b border-border`; `start` `border-s-2 border-border ps-3` |
| `background` | `surface` `bg-surface` (a row that stands on the page background); `sunken` `bg-surface-sunken` (a well inside a raised surface, such as the quote of the post a form replies to) |
| `pushEnd` | `ms-auto`: moves the element to the inline end of its row, also when the row wraps |
| `tone` | `fg` / `muted` / `danger` / `warning`: the text color inside |
| `textSize` | `xs` / `sm` / `base` / `lg`: the text size inside |

### Notification rows and quotes

`NotificationSentence` is the sentence of a notification in the author slot of a `PostCard` (`isUnread` adds the dot, the heavier
weight and the hidden `unreadLabel`), `NotificationOthers` the row of the other people of a group above it (`PostCardAvatarLink`s
around `xs` avatars), and `NotificationLinks` the links that lead on, in the actions slot. `QuoteBlock` is lines with a rule at the
start (a moderation notice, a reason); `PostExcerpt` is that rule on a link to a post, around a `PostExcerptText` (its direction,
language and line breaks, cut after three lines). The form's quote of the post being replied to uses the same parts.

### Settings pieces

`IconLabel` is an icon and a word on one line (a page in a list of pages). `Callout` is a raised box with a strong start edge for a
note and the one button that acts on it. `CodeList` is a raised box of monospaced codes to copy down (recovery codes). `Button`'s
`size="icon"` is an icon-only button (as wide as tall, a tap target on a coarse pointer); `Text`'s `inset` adds `px-1` to a `legend`
over a bordered group; `Stack`'s `marker="decimalInside"` puts numbers inside the text.

`BannerImage` is a page-width image capped in height. `Button` takes `variant="subtle"` (ghost in the muted color) and `size="compact"`;
`Heading` takes `size="meta"`; `Tag` takes `variant="solid"` (the accent mark); `NewItemsButton` takes `sticky`; `PostContent` takes
`lineClamp`; `Link` takes `size="sm"` and `wrap="anywhere"`.

### Account rows

`AccountItem` is the avatar, the display name and the handle of an account: a link to the profile with `href`, plain text
without. `nameContent` shows the name with custom emoji, `layout` is `row` (default), `padded` (a whole-row target with a
hover background) or `inline` (as wide as its text), `details` adds lines under the handle and `trailing` an action or a
badge at the end (`trailingPlacement="below"` puts it under the account until the container is `@md` wide). A list of
accounts is `<Stack as="ul" divided>` of `<Box as="li">`.

### Which component for which pattern

The common class strings of `app/web/src` and what draws the same thing. A string that is not listed is composed from
the props above. `Stack` and `Cluster` always add `flex`: a block whose children flow as text (an `li` with a name and
a time side by side as inline text) is a `Box`, not a `Stack`.

| Class string (element) | Replacement |
| --- | --- |
| `flex flex-col gap-3` (`div`) | `<Stack gap="3">` |
| `flex flex-col gap-3` (`section`, `article`, `ul`, `form`) | `<Stack as="section" gap="3">` (`as="article"`, `as="ul"`, `as="form"`) |
| `flex flex-col gap-1` / `gap-2` / `gap-4` / `gap-6` / `gap-8` | `<Stack gap="1">` / `gap="2"` / `gap="4"` / `gap="6"` / `gap="8"` |
| `flex flex-col` | `<Stack>` |
| `flex flex-col items-start gap-2` (also `gap-1`, `gap-3`, `gap-4`) | `<Stack align="start" gap="2">` |
| `@container flex flex-col gap-4` / `gap-8` | `<Stack container gap="4">` / `<Stack container gap="8">` |
| `@container flex min-w-0 flex-col gap-2` (`fieldset`) | `<Stack as="fieldset" container shrink gap="2">` |
| `flex min-w-0 flex-col` / `flex min-w-0 flex-1 flex-col` | `<Stack shrink>` / `<Stack shrink grow>` |
| `flex max-w-prose flex-col gap-4` | `<Stack width="prose" gap="4">` |
| `flex flex-col gap-2 text-fg-muted` | `<Stack gap="2" tone="muted">` |
| `flex flex-col gap-1 text-sm text-danger` | `<Stack gap="1" tone="danger" textSize="sm">` |
| `flex flex-col gap-1 border-s-2 border-border ps-3` | `<Stack gap="1" border="start">` |
| `flex flex-col gap-2 rounded-md border border-border p-3` | `<Stack gap="2" border="md" padding="3">` |
| `flex flex-col gap-3 @md:flex-row @md:items-center` | `<Stack gap="3" rowAt="md">` |
| `flex flex-col gap-8 pt-4` / `flex flex-col gap-4 pt-3` (on a `TabPanel`) | `<TabPanel gap="8" offset="4">` / `<TabPanel gap="4" offset="3">` |
| `flex flex-col gap-6` and the other stack strings on a `<form>` | `<Stack as="form" gap="6">`; with server errors on fields, `<Form gap="6" validationErrors={…}>` |
| `className` on React Aria's `Form` | `<Form>` from the kit with the same layout props as `Stack` |
| `flex flex-col divide-y divide-border` (`ul`) | `<Stack as="ul" divided>` |
| `flex flex-col divide-y divide-border rounded-lg border border-border` (`ul`) | `<Stack as="ul" divided border="lg">` |
| `flex flex-col divide-y divide-border border-y border-border` (`ul`) | `<Stack as="ul" divided border="y">` |
| `@container py-3` (`li`) | `<Box as="li" container paddingY="3">` |
| `@container flex flex-col gap-2 py-3` / `flex flex-col gap-1 py-3` (`li`) | `<Stack as="li" container gap="2" paddingY="3">` / `<Stack as="li" gap="1" paddingY="3">` |
| `flex flex-col px-4 py-3` (`li`) / `px-4 py-3` | `<Stack as="li" paddingX="4" paddingY="3">` / `<Box paddingX="4" paddingY="3">` |
| `rounded-md border border-border p-3` / `rounded-lg border border-border p-4` | `<Box border="md" padding="3">` / `<Box border="lg" padding="4">` |
| `w-full max-w-sm` / `w-full max-w-xs` / `w-full` | `<Box width="sm">` / `<Box width="xs">` / `<Box width="full">` |
| `min-w-0` / `min-w-0 flex-1` | `<Box shrink>` / `<Box shrink grow>` |
| `flex list-disc flex-col gap-1 ps-6` (`ul`) / `flex list-decimal flex-col gap-2 ps-6` (`ol`) | `<Stack as="ul" marker="disc" gap="1">` / `<Stack as="ol" marker="decimal" gap="2">` |
| `flex list-inside list-disc flex-col gap-1` (`ul`) | `<Stack as="ul" marker="discInside" gap="1">` |
| `flex flex-wrap gap-2` / `flex flex-wrap gap-4` | `<Cluster gap="2">` / `<Cluster gap="4">` |
| `flex flex-wrap items-center gap-2` | `<Cluster align="center" gap="2">` |
| `flex flex-wrap justify-end gap-2` (the buttons of a dialog; also `mt-4 …` after a paragraph) | `<DialogActions>` (in a `<Stack gap="4">` for the `mt-4`) |
| `flex flex-wrap justify-end gap-2` (elsewhere) / `flex justify-end` / `flex justify-center` | `<Cluster justify="end" gap="2">` / `<Cluster justify="end">` / `<Cluster justify="center">` |
| `flex flex-wrap gap-x-2` / `flex flex-wrap gap-x-1` (`li`, `span`, `p`) | `<Cluster as="li" gap="2" rowGap="0">` / `<Cluster as="span" gap="1" rowGap="0">` |
| `flex flex-wrap gap-x-4` / `flex flex-wrap gap-x-4 gap-y-1` (`ul`) | `<Cluster as="ul" gap="4" rowGap="0">` / `<Cluster as="ul" gap="4" rowGap="1">` |
| `flex flex-wrap items-baseline gap-x-3` (`span`) | `<Cluster as="span" align="baseline" gap="3" rowGap="0">` |
| `flex items-center gap-3` / `flex min-w-0 items-center gap-3` | `<Cluster wrap={false} align="center" gap="3">` / add `shrink` |
| `flex min-w-0 flex-wrap items-center gap-1` | `<Cluster shrink align="center" gap="1">` |
| `self-start` / `self-end` (a wrapper) | `<Box self="start">` / `<Box self="end">` |
| `flex flex-col items-start gap-2 @md:flex-row @md:items-center @md:justify-between` with a heading and actions | `<SectionHeader actions={…}><Heading level={3}>…</Heading></SectionHeader>`; the actions' own `flex flex-wrap gap-2` wrapper goes away |
| `grid grid-cols-1 gap-x-6 gap-y-1 @sm:grid-cols-2` (`ul`) | `<Grid as="ul" gap="6" rowGap="1">` |
| `grid gap-2 @md:grid-cols-2` / `grid grid-cols-1 gap-4 @md:grid-cols-2` | `<Grid at="md" gap="2">` / `<Grid at="md" gap="4">` |
| `text-2xl font-semibold` (`h1`) | `<Heading level={1}>` (the page title is the app's `PageHeading`) |
| `text-lg font-semibold` (`h2`) | `<Heading level={2}>` |
| `font-semibold` (`h3`, `h4`) | `<Heading level={3}>` / `<Heading level={4}>` |
| `font-semibold` (`h2`) / `font-semibold wrap-anywhere` (`h2`) | `<Heading level={2} size="subsection">` / add `wrap="anywhere"` |
| `text-sm font-medium text-fg` (`h2`) | `<Heading level={2} size="label" tone="fg">` |
| `font-semibold text-fg` (`h3` in a muted region) | `<Heading level={3} tone="fg">` |
| `font-semibold wrap-anywhere outline-none` / `font-semibold outline-none` (`h3` with `tabIndex={-1}`) | `<Heading level={3} wrap="anywhere" focusable>` / `<Heading level={3} focusable>` |
| `text-lg font-semibold outline-none` (`h2` with `tabIndex={-1}`) | `<Heading level={2} focusable>` |
| `min-w-0 font-semibold wrap-anywhere` / `min-w-0 font-mono font-semibold wrap-anywhere` (`h3`) | `<Heading level={3} shrink wrap="anywhere">` / add `mono` |
| `font-mono font-semibold wrap-anywhere` (`h3`) | `<Heading level={3} mono wrap="anywhere">` |
| `text-sm text-fg-muted` (`p`) / (`span`) | `<Hint>` / `<Hint as="span">` |
| `text-fg-muted` (`p`) | `<Hint size="inherit">` |
| `text-fg-muted` (`span`) | `<Text tone="muted">` |
| `text-sm text-danger` (`p`) / `text-danger` (`p`) | `<InlineError>` / `<InlineError size="inherit">` |
| `role="alert"` on such a `p` | add `alert`: `<InlineError alert>` |
| `text-warning` / `text-sm text-warning` (`p`) | `<Text as="p" tone="warning">` / `<Text as="p" size="sm" tone="warning">` |
| `font-semibold` / `font-medium` (`span`, `p`) | `<Text weight="semibold">` / `<Text as="p" weight="medium">` |
| `text-sm font-medium text-fg` (`span`, `legend`) | `<Text size="sm" weight="medium" tone="fg">` (`as="legend"`) |
| `text-sm` / `text-lg` (`p`, `span`) | `<Text size="sm">` / `<Text as="p" size="lg">` |
| `tabular-nums` (`span`) | `<Text numeric>` |
| `wrap-anywhere` (`span`, `p`) / `text-sm wrap-anywhere` | `<Text wrap="anywhere">` / `<Text size="sm" wrap="anywhere">` |
| `font-mono wrap-anywhere` / `font-mono wrap-anywhere select-all` (`p`) | `<Text mono wrap="anywhere">` / `<Text as="p" mono wrap="anywhere" selectAll>` |
| `whitespace-pre-wrap wrap-anywhere` / `whitespace-pre-line` | `<Text whitespace="preWrap" wrap="anywhere">` / `<Text whitespace="preLine">` |
| `block text-start wrap-anywhere whitespace-pre-wrap` (`span`) | `<Text block align="start" wrap="anywhere" whitespace="preWrap">` |
| `whitespace-pre-line wrap-anywhere align-match-parent` (`p dir="auto"`) | `<Text as="p" dir="auto" whitespace="preLine" wrap="anywhere" align="matchParent">` |
| `line-clamp-2 text-start wrap-anywhere whitespace-pre-wrap` (`p`) | `<Text as="p" lineClamp={2} align="start" wrap="anywhere" whitespace="preWrap">` |
| `truncate text-start font-semibold` / `truncate text-start text-sm text-fg-muted` (`span`) | `<Text wrap="truncate" align="start" weight="semibold">` / `<Text wrap="truncate" align="start" size="sm" tone="muted">` |
| `text-start text-sm font-normal wrap-anywhere text-fg-muted` (`span`) | `<Text align="start" size="sm" weight="normal" wrap="anywhere" tone="muted">` |
| `rounded-full border border-border px-2 text-sm text-fg-muted` (`span`) | `<Tag>` |
| `<table className="w-full text-start">`, `th` `py-2 pe-3 text-start font-semibold`, rows `border-b border-border align-top` | `<Table caption={…}>` with `TableHead` › `TableColumnHeader`, `TableBody` › `TableRow` › `TableRowHeader` / `TableCell` |
| `<table className="w-full table-fixed border-collapse text-sm">` with `tbody` `divide-y` | `<Table density="compact" aria-labelledby={…}>`, `TableColumnHeader width="narrow"` / `"wide"` for `w-16` / `w-2/5` |
| `sr-only` | `VisuallyHidden` |

## App shell

The frame around every page ([web-client/09-app-shell.md §9](../../docs/architecture/web-client/09-app-shell.md#9-アプリの外枠)). The app decides what goes in each slot
(the items, their permissions and counts, the strings); the look is here. `AppShell` and `AppNavigation` are the kit's only components that
answer to the viewport (`md:`, `lg:`, `xl:` and the `short:` variant of [theme.css](src/theme.css)), because they lay out the screen itself;
so does the region of `ToastProvider` (`clearsTabBar`), which stands above the tab bar they draw (D2). The lint exempts those three files from the rule
against viewport breakpoints.

| Component | What it draws | Its props |
| --- | --- | --- |
| `AppShell` | The skip link, the navigation, `main` (padded at its end for the tab bar and the floating button), the third column (xl) and the floating slot | `navigation`, `children`, `skipLinkLabel`, `mainId`, `mainRef`, `aside` (only where the third column shows), `floating`, `floatingVisible` (sets `data-compose-fab` on `main`, which `with-compose-fab:` and the app's `scroll-padding-block-end` follow), `navigationPlacement` (`side` / `top`: the signed-out top bar above the page) |
| `AsidePanel` | A section of the third column: an `aside` named by its small muted `h2` | `heading`, `children` |
| `AppNav` | The `nav` landmark: bottom tab bar, rail from md, sidebar from lg | `label`, `header` (the wordmark and server name: `AppNavBrand`), `action` (the rail's button: `AppNavAction`), `footer` (sidebar only), `placement` (`side` / `top`), the items as children |
| `AppNavItem` | An item: a `RailItem` (icon, label, count's `Badge`; soft fill when current) | `href`, `icon`, `label`, `current`, `badge` (`AppNavBadge`: `count`, `label`, `name`, the link's name while it shows), `shows` (`always` / `tab-bar` / `rail`) |
| `FloatingButton` | The round primary button that starts the page's main action (below md; the rail has `AppNavAction`): `label`, `icon`, `iconOnly` (a 3rem circle named by `aria-label`, with the label as a tooltip); its place on the page is `AppShell`'s | `label`, `icon`, `iconOnly`, `onPress` |
| `AppNavAction` | The rail's one `primary` button after the items: icon alone in the md rail, with its label from lg | `label`, `icon`, `onPress` |
| `RailItem` | A navigation link: icon, label, optional count; `aria-current="page"` is soft-filled with an accent label | `href`, `icon`, `label`, `current`, `badge`, `form` (`tab` / `row` / `top`), `prominent`, `classNames` |
| `AppNavMenu` | The tab bar's last item, a button that opens a `Sheet` | `label`, `badge`, `isOpen`, `onOpenChange`, `title`, `closeLabel`, the sheet's content |
| `AppNavMenuList`, `AppNavMenuLink`, `AppNavMenuSection` | The sheet's list and its rows; a block under them, with a line above (`separator` `"3"` / `"4"`) or without | as `AppNavItem` but `shows`; `separator` |
| `AppNavAccount` | The sidebar's signed-in account: avatar, display name and handle | `href`, `avatar`, `name`, `handle`, `description` |
| `ServerNameLink` | The server's name as the link to `/`, in a `dir="auto"` paragraph | `name`, `description`, `current`, `size` (`lg` at the head of a column) |
| `PageHeading` | The page's `h1`, focusable by script | `id`, `visuallyHidden` (`sr-only`) |
| `BackLinkHeading` | A heading with the way back under it (a page of the settings) | `backHref`, `backLabel`, the heading as children |
| `RouteLoading` | A large spinner in the page's column | `label` |
| `StandalonePage` | A `main` without the shell, centered, prose width (the layout's own error) | `children` |

## Component tests

Each component has Testing Library tests next to it (`src/<Component>.test.tsx`) that find elements by role and drive them with
`user-event`: keyboard operation and the label / description / error association. They run in jsdom in the `unit` project of
[vitest.config.ts](vitest.config.ts) (`bun run test:unit`), which `scripts/task.sh ui:unit-test` (part of `scripts/task.sh check`) runs.

## Stories

Every component has a story next to it (`src/<Component>.stories.tsx`, CSF 3). `scripts/task.sh ui:storybook` opens Storybook 10 on port 6006;
its toolbar switches the locale (`lang` / `dir` on `<html>` and React Aria's `I18nProvider`), the theme (`data-theme`), and the instance
theme: the default tokens, or the sample server theme of [.storybook/sampleTheme.ts](.storybook/sampleTheme.ts) (teal accents on warm
surfaces, a looser spacing step, and rounder corners). The preview renders the sample's overrides as the server renders a theme's stylesheet
(a `:root` rule outside any cascade layer, colors as `light-dark()`) and adopts it as a constructed stylesheet, as the Tauri app applies a
theme (ADR-0059 §6). [src/ThemeSample.stories.tsx](src/ThemeSample.stories.tsx) (`Foundations/Theme sample`) fixes the sample theme
for its own story, which shows buttons, a toggle, a card with form controls, links, muted text and badges, so its screenshots catch a
component that stops following the overrides; every other story is tested with the default tokens only.
[src/Tokens.stories.tsx](src/Tokens.stories.tsx) shows the color pairs, the type scale in English, Japanese and Arabic, radius,
shadow and the focus ring. [src/storybook.css](src/storybook.css) is the Storybook preview stylesheet.

Stories are also the kit's browser tests ([vitest.config.ts](vitest.config.ts)). `scripts/task.sh ui:test` runs each story in Chromium in
`en` and `ar-XB`, light and dark (four projects); a story fails on any axe violation or when it differs from its screenshots at 320, 640 and
1024 px container widths. A story of the app shell sets a viewport instead (`globals: atViewport("phone")` with
`parameters: shellParameters`, [src/storyViewports.ts](src/storyViewports.ts)), since the shell answers to the viewport, and is captured
once at it; its content stays within the viewport's height. The screenshots are taken with `prefers-reduced-motion: reduce` emulated, and a story fails when
anything in it still animates then; stories render and run their `play` functions (which find elements by role, with
`storybook/test`) without it. The screenshots in `src/__screenshots__/` come only from the pinned Playwright image: regenerate
them with `scripts/task.sh ui:snapshots-update`, and compare like CI with `scripts/task.sh ui:test-docker` (both need Docker).
A story's `play` function also tests behaviour in the real browser (focus trap and return, Esc, arrow keys in RTL, announcements that
keep focus); it runs before axe and the screenshots, in every project. The overlay components' jsdom tests cover the same
behaviour in `scripts/task.sh check`. Overlays portal into the story frame, which has layout
containment, so a modal covers the frame and appears in its screenshots.
The test procedure is in [web-client/README.md §7](../../docs/architecture/web-client/README.md#7-テスト).

## Parts for the slots of PostCard and QuoteCard

| Part | Slot | What it is |
| --- | --- | --- |
| `PostCardAuthorLink` | `author` | The profile link around a `PostCardAuthor` (`href`, `name`, `handle`). |
| `PostCardTimeLink` | `time`, `details` | The muted link on a `<time>` or on the mark of an edited post; `dense` drops the 24 px minimum height (QuoteCard's header). |
| `PostCardAvatarLink` | `avatar` | The avatar as a second link to the profile: out of the tab order, `aria-hidden`. |
| `PostCardNameLink` | `context`, a sentence | A name inside a sentence ("Bob boosted"), in a `<bdi>`; notification rows use it too. |
| `PostCardBoostContext` | `context` | The boost icon and the sentence. |
| `PostCardOrigin` | `actions` | The link from a post of another server to its own page (`href`, `host`, `label`). |

Counts in `details` are `<Text numeric>`; several parts under a body are a `<Stack gap="3" shrink>`; `PostContent` takes `size="lg"` for the post a page is about.

### Screen-specific pieces

| Component | Renders |
| --- | --- |
| `EmojiTile` | A custom emoji (`emoji`) on a 3rem tile, hidden from assistive technology, beside the heading that names it |
| `CodeBlock` | `pre` > `code` on a sunken surface, wrapping anywhere (a payload, a log) |
| `PreviewImage` | A 16:9 `img` preview of a file the user chose |
| `AttachmentItem` | An image of a draft: the `li` of a `<Stack as="ul">` with the preview, a `progressLabel` band (spinner) over it, and the description field, error and actions as children; a column below `@md` of the container |
| `OutboxItem` | A post waiting to be sent: the `li` of a `<Stack as="ul">` with its text; `sendingLabel` (`aria-busy`, spinner) while it is sent, else `error` and the retry / edit / discard buttons as children |
| `Fieldset` | A `fieldset` framed with a `legend` (`gap-3` between its controls); other attributes (`aria-describedby`) pass to the `fieldset` |
| `ThemePreview` | A surface, card, text, link, button and error drawn with the theme `tokens` (custom properties); strings are props |
| `PostContent` `framed` | A border and padding around a post body shown as a preview |
| `PostContent` `size="sm"` `lineClamp={3}` | An excerpt of a post in a form (the reply target), cut after three lines |

## Reactions

`ReactionRow` is the wrapping `role="group"` of a post's emoji reactions with the button that lists who reacted (`label`, `listLabel`,
`onOpenList`; the dialog goes in `after`). Its children are `ReactionChip`s: a toggle named by `label` and described by the formatted
`count`, outlined and accented while `isSelected`; `onChange={null}` shows it only (an emoji nobody here can react with). The emoji is
`image` (a custom emoji) or `text`; `ReactionEmoji` draws the same emoji for the rows of the list of reactors.
