// SPDX-License-Identifier: AGPL-3.0-only

// Tailwind's theme, generated from @my/design-tokens (ADR-0025 §1, ADR-0059 §1), which styles.css loads with
// @plugin. It declares every token as a custom property on :root, and names each in the theme as a reference to that
// property, so that a utility reads `var(--color-surface)` and a theme's override of the property reaches it.

import {
  breakpoint,
  cjkRegions,
  badgeColor,
  color,
  latinFamily,
  control,
  emojiPicker,
  fontFamily,
  fontWeight,
  formPage,
  motion,
  properties,
  radius,
  screen,
  shell,
  text,
  toggle,
  voice,
  event,
} from "@my/design-tokens";
import plugin from "tailwindcss/plugin";
import { transitionRules } from "./motion-css";

const ref = (name: string) => `var(${name})`;

/** The rings of who speaks, the halo `swell` past the ring of the text's color. */
const speaking = (swell: string) =>
  [
    "0 0 0 var(--voice-ring-gap) var(--voice-cut, var(--color-surface))",
    "0 0 0 calc(var(--voice-ring-gap) + var(--voice-ring-width)) var(--color-text)",
    `0 0 0 calc(var(--voice-ring-gap) + var(--voice-ring-width) + ${swell}) color-mix(in srgb, var(--color-text) var(--voice-halo-share), transparent)`,
  ].join(", ");
const names = <T extends object>(o: T) => Object.keys(o) as (keyof T & string)[];

/** A token's camel-case name as its custom property's: `gapWide` as `gap-wide`. */
const kebab = (name: string) => name.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);

// The theme's keys, each a token or a value derived from tokens only. The breakpoints (`tablet:`, `desktop:`,
// `wide:`) are for the frames of a page alone: the app's (shell.tsx, ADR-0110 §10) and a page of its own outside it
// (form-page.tsx); a part lays itself out by its container.
const theme = {
  screens: Object.fromEntries(names(breakpoint).map((name) => [name, `${breakpoint[name]}rem`])),
  colors: {
    ...Object.fromEntries(names(color).map((name) => [name, ref(`--color-${name}`)])),
    ...Object.fromEntries(names(badgeColor).map((name) => [name, ref(`--color-${name}`)])),
    "accent-hover": "color-mix(in srgb, var(--press-shade) var(--press-hover), var(--color-accent))",
    "accent-pressed": "color-mix(in srgb, var(--press-shade) var(--press-pressed), var(--color-accent))",
    "spinner-track": "color-mix(in srgb, currentColor var(--spinner-track-share), transparent)",
    "diff-added": "color-mix(in srgb, var(--color-text) var(--diff-added-share), transparent)",
    mark: ref("--composer-mark"),
    scrim: ref("--dialog-scrim"),
    "media-badge": ref("--media-badge-backing"),
    "viewer-scrim": ref("--viewer-scrim"),
    // What is drawn over a picture, on its badge's backing: white at any scheme.
    "on-media": "#ffffff",
    transparent: "transparent",
    current: "currentColor",
  },
  fontFamily: Object.fromEntries(names(fontFamily).map((name) => [name, ref(`--font-${name}`)])),
  fontWeight: Object.fromEntries(names(fontWeight).map((name) => [name, ref(`--font-weight-${name}`)])),
  fontSize: Object.fromEntries(
    names(text).map((name) => [
      name,
      [
        ref(`--text-${name}`),
        { lineHeight: ref(`--text-${name}--line-height`), fontWeight: ref(`--text-${name}--font-weight`) },
      ],
    ]),
  ),
  borderRadius: {
    ...Object.fromEntries(names(radius).map((name) => [name, ref(`--radius-${name}`)])),
    "icon-tile": ref("--icon-tile-radius"),
    entry: ref("--entry-radius"),
    full: "50%",
  },
  spacing: {
    0: "0",
    ...Object.fromEntries(
      names(control).flatMap((size) => [
        [`control-${size}`, ref(`--control-${size}-height`)],
        [`control-${size}-inline`, ref(`--control-${size}-padding-inline`)],
        [`control-${size}-block`, ref(`--control-${size}-padding-block`)],
      ]),
    ),
    // The room above and below one line of text in a box as tall as a control, or as the least press target: the block
    // padding of a box of that height whose text may wrap, which then grows by its lines.
    ...Object.fromEntries(
      names(control).map((size) => [`control-${size}-line`, `calc((var(--control-${size}-height) - 1lh) / 2)`]),
    ),
    "target-line": "max(0px, calc((var(--target-minimum) - 1lh) / 2))",
    // The same around a text as tall as its font's size (a link in a line that sets no leading of its own).
    "target-em": "calc((var(--target-minimum) - 1em) / 2)",
    "icon-button-inline": ref("--icon-button-padding-inline"),
    "icon-button-gap": ref("--icon-button-gap"),
    "icon-button-touch": ref("--icon-button-touch"),
    press: ref("--press-shift"),
    spinner: ref("--spinner-size"),
    "icon-action": ref("--icon-action"),
    "icon-caption": ref("--icon-caption"),
    "icon-navigation": ref("--icon-navigation"),
    ...Object.fromEntries(names(shell).map((name) => [`shell-${kebab(name)}`, ref(`--shell-${kebab(name)}`)])),
    ...Object.fromEntries(names(screen).map((name) => [`screen-${kebab(name)}`, ref(`--screen-${kebab(name)}`)])),
    ...Object.fromEntries(
      names(voice)
        .filter((name) => typeof voice[name] === "number" && name !== "haloShare")
        .map((name) => [`voice-${kebab(name)}`, ref(`--voice-${kebab(name)}`)]),
    ),
    ...Object.fromEntries(names(event).map((name) => [`event-${kebab(name)}`, ref(`--event-${kebab(name)}`)])),
    // The room above an entry of a feed from what stands before it outside the feed (a heading, the tabs): the
    // entry's surface reaches past its content and its focus ring past the surface, neither touching what is above.
    "entry-room":
      "calc(var(--entry-reach) + var(--focus-ring-gap) + var(--focus-ring-width) + var(--focus-ring-halo) + var(--screen-air))",
    "icon-tile-glyph": ref("--icon-tile-glyph"),
    "avatar-inline": ref("--avatar-inline"),
    "avatar-row": ref("--avatar-row"),
    "avatar-post": ref("--avatar-post"),
    "avatar-preview": ref("--avatar-preview"),
    "menu-padding": ref("--menu-padding"),
    "menu-group-gap": ref("--menu-group-gap"),
    "menu-icon-gap": ref("--menu-icon-gap"),
    "menu-min-width": ref("--menu-min-width"),
    "menu-described-block": ref("--menu-described-block"),
    "menu-wrapped-block": ref("--menu-wrapped-block"),
    // What a sheet holds under its head lines up with the head's name, as a menu's items do.
    "menu-form-inline": "calc(var(--control-timeline-padding-inline) - 0.25rem)",
    "diff-image-frame": ref("--diff-image-frame"),
    "disclosure-gap": ref("--disclosure-gap"),
    "disclosure-padding": ref("--disclosure-padding"),
    "disclosure-start": ref("--disclosure-padding-start"),
    // Beside the label of the summary's toggle: a timeline button's own, unless the frame of the page says otherwise
    // (`--disclosure-toggle-inline`, which the app's frame sets on a phone, shell.tsx).
    "disclosure-toggle": "var(--disclosure-toggle-inline, var(--control-timeline-padding-inline))",
    // Above and below the summary's text, so that its first line is level with the toggle's label.
    "disclosure-text": "calc((var(--control-timeline-height) - 1lh) / 2)",
    // The search field's input, past its glyph at the start.
    "search-field-start": "calc(var(--control-timeline-padding-inline) * 2 + var(--icon-action))",
    "content-paragraph": ref("--content-paragraph-gap"),
    "content-indent": ref("--content-indent"),
    "content-quote": ref("--content-quote-indent"),
    "content-code-block": ref("--content-code-padding-block"),
    "content-code-inline": ref("--content-code-padding-inline"),
    "content-code-chip": ref("--content-code-inline"),
    "content-emoji": ref("--content-emoji"),
    "content-emoji-side": "0.05em",
    "content-emoji-line": "calc((1em - var(--content-emoji)) / 2)",
    "content-link-reach": "calc((var(--target-minimum) - 1em) / 2)",
    "article-block": ref("--article-block-gap"),
    "article-heading": ref("--article-heading-gap"),
    "article-cover-max": ref("--article-cover-max-height"),
    "article-card-thumb": ref("--article-card-thumb"),
    "article-measure": ref("--article-measure"),
    "article-field-block": ref("--article-field-block"),
    "article-tags-inset": ref("--article-tags-inset"),
    "article-tag-input": ref("--article-tag-input"),
    entry: ref("--entry-gap"),
    "entry-line": ref("--entry-line-gap"),
    "entry-reach": ref("--entry-reach"),
    "entry-avatar": ref("--entry-avatar-gap"),
    "entry-name": ref("--entry-name-gap"),
    "entry-caption": ref("--entry-caption-gap"),
    // Where a byline's name starts, past its avatar: what stands under the name starts there too.
    "entry-name-start": "calc(var(--avatar-post) + var(--entry-avatar-gap))",
    "entry-action": ref("--entry-action-gap"),
    "entry-scroll": ref("--entry-scroll-margin"),
    feed: ref("--feed-gap"),
    "feed-scroll": ref("--feed-scroll-margin"),
    "feed-tight": ref("--feed-tight-gap"),
    embed: ref("--embed-padding"),
    "embed-gap": ref("--embed-gap"),
    "poll-padding": ref("--poll-padding"),
    "poll-option-inline": ref("--poll-option-inline"),
    "poll-result-block": ref("--poll-result-block"),
    // What stands inside the mark of a poll's option that is chosen: a checkbox's check, a radio's dot.
    "poll-mark-check": "calc(var(--form-page-check) * 2 / 3)",
    "poll-mark-dot": "calc(var(--form-page-check) / 3)",
    media: ref("--media-gap"),
    "media-preview-width": ref("--media-preview-width"),
    "media-preview-height": ref("--media-preview-height"),
    "media-badge": ref("--media-badge-inset"),
    "media-play": ref("--media-play"),
    "media-card-thumb": ref("--media-card-thumb"),
    "media-card-max": ref("--media-card-max-height"),
    "media-mark": "0.375em",
    "media-track": "0.1875rem",
    "media-line": "calc((var(--control-timeline-height) - 1lh) / 2)",
    "viewer-bar": ref("--viewer-bar"),
    "viewer-inset": ref("--viewer-inset"),
    "viewer-side-inline": ref("--viewer-side-inline"),
    "viewer-specimen": ref("--viewer-specimen"),
    "viewer-foot": "calc((var(--viewer-bar) - var(--control-timeline-height)) / 2)",
    chip: ref("--chip-height"),
    "chip-inline": ref("--chip-padding-inline"),
    "chip-gap": ref("--chip-gap"),
    "chip-spacing": ref("--chip-spacing"),
    ...Object.fromEntries(names(toggle).map((name) => [`toggle-${name}`, ref(`--toggle-${name}`)])),
    ...Object.fromEntries(
      names(emojiPicker).map((name) => [`emoji-picker-${kebab(name)}`, ref(`--emoji-picker-${kebab(name)}`)]),
    ),
    // The popover's width: the picker's, or the viewport less the dialog's margins.
    "emoji-picker-popover": "min(var(--emoji-picker-width), calc(100vw - 2 * var(--dialog-margin)))",
    // The sheet's height: its own, or the room it is in when that is less; its foot clears the device's safe area.
    "emoji-picker-sheet-height": "min(100%, var(--emoji-picker-sheet))",
    "emoji-picker-sheet-foot": "calc(var(--emoji-picker-gap) + env(safe-area-inset-bottom))",
    // The search field's padding at its start: the glyph's, and the room it keeps on both sides.
    "emoji-picker-search-start": "calc(var(--control-timeline-padding-inline) * 2 + var(--icon-action))",
    dialog: ref("--dialog-padding"),
    // The bottom padding of a sheet from the viewport's bottom edge: a dialog's, clear of the device's safe area.
    "dialog-safe-bottom": "calc(var(--dialog-padding) + env(safe-area-inset-bottom))",
    "dialog-gap": ref("--dialog-gap"),
    "dialog-row": ref("--dialog-row-gap"),
    "dialog-margin": ref("--dialog-margin"),
    // The least height of the composer's text: its lines, or one, at its size and leading.
    "composer-room": "calc(var(--composer-text-lines) * var(--composer-text-leading) * var(--composer-text-size))",
    "composer-line": "calc(var(--composer-text-leading) * var(--composer-text-size))",
    // What a button at the end of a line takes: its glyph and its padding on both sides.
    "icon-button-end": "calc(var(--icon-action) + 2 * var(--icon-button-padding-inline))",
    "composer-thumb": ref("--composer-thumb"),
    "composer-inset": ref("--composer-inset"),
    "composer-field-block": ref("--composer-field-block"),
    "composer-remove-room": ref("--composer-remove-room"),
    "composer-top": ref("--composer-top"),
    "composer-bottom": ref("--composer-bottom"),
    kbd: ref("--kbd-height"),
    "kbd-min-width": ref("--kbd-min-width"),
    "kbd-inline": ref("--kbd-padding-inline"),
    "kbd-gap": ref("--kbd-gap"),
    ...Object.fromEntries(
      names(formPage)
        .filter((name) => name !== "bannerRatio")
        .map((name) => [`form-page-${kebab(name)}`, ref(`--form-page-${kebab(name)}`)]),
    ),
    // Between the rows of a footer's links that wrap, so that the targets of two rows never overlap.
    "form-page-footer-rows": "max(var(--form-page-field-gap), calc(var(--target-minimum) - 1lh))",
  },
  // The widths of a container from which a part lays itself out otherwise: a thread's replies indent (`@reply:`).
  containers: { reply: `${screen.replyFrom}rem` },
  aspectRatio: { banner: ref("--form-page-banner-ratio") },
  lineHeight: { entry: ref("--entry-leading") },
  borderWidth: { 0: "0", spinner: ref("--spinner-stroke"), "content-quote": ref("--content-quote-bar") },
  opacity: { disabled: ref("--press-disabled-opacity") },
  boxShadow: {
    focus: [
      "0 0 0 var(--focus-ring-gap) var(--color-surface)",
      "0 0 0 calc(var(--focus-ring-gap) + var(--focus-ring-width)) var(--color-accent-text)",
      "0 0 0 calc(var(--focus-ring-gap) + var(--focus-ring-width) + var(--focus-ring-halo)) color-mix(in srgb, var(--color-accent) var(--focus-ring-halo-share), transparent)",
    ].join(", "),
    // Inside a menu, whose rows touch, the ring is drawn within the item.
    "focus-inset": "inset 0 0 0 var(--focus-ring-width) var(--color-accent-text)",
    // A face in a row of faces, cut out of the one before by a ring of the surface under it (`--face-cut-color`).
    "face-cut": "0 0 0 var(--screen-face-cut) var(--face-cut-color, var(--color-surface))",
    overlay: ref("--shadow-overlay"),
    bar: ref("--shadow-bar"),
    // The rings around the avatar of who speaks in a voice chat room: the surface under it (`--voice-cut`), the
    // text's color, and the halo, under the ring until it swells (`animate-speaking`).
    speaking: speaking("0px"),
  },
  animation: {
    ...Object.fromEntries(names(motion).map((name) => [name, `my-${name} var(--duration-${name}) linear infinite`])),
    // A voice chat room's: a reaction rises and fades once, or, with reduced motion, fades in and out where it is; the
    // halo of who speaks swells and settles while they speak.
    reaction: "my-reaction var(--voice-reaction-duration) ease-out both",
    "reaction-still": "my-reaction-still var(--voice-reaction-duration) linear both",
    speaking: "my-speaking var(--voice-speaking-duration) ease-in-out infinite alternate",
  },
};

export default plugin(
  (api) => {
    api.addBase({
      ":root": { colorScheme: "light dark", ...properties },
      // Noto Sans CJK of the text's language (ADR-0009 §5): an element with a language sets the stacks again, from its
      // own region, and draws its text with them, since an inherited font-family keeps the region of the parent's. In
      // a CJK language, Noto Sans leaves to Noto Sans CJK the punctuation it draws full-width.
      ...Object.fromEntries(
        cjkRegions.map(({ family, langs }) => [
          langs.map((lang) => `:lang(${lang})`).join(", "),
          { "--font-cjk": `"${family}"`, "--font-latin": `"${latinFamily.besideCjk}"` },
        ]),
      ),
      "[lang]": {
        "--font-sans": fontFamily.sans,
        "--font-mono": fontFamily.mono,
        fontFamily: "var(--font-sans)",
      },
      // Code draws with the mono stack, its emoji and Han with the self-hosted fonts, also where it or a part inside it
      // has a language, which the rule above would turn back to the sans stack. A part that wants the sans stack, such
      // as a key of the shortcuts, says so with its class.
      "code, pre, kbd, samp, :is(code, pre, kbd, samp)[lang], :is(code, pre, kbd, samp) [lang]": {
        fontFamily: "var(--font-mono)",
      },
      ':root[data-theme="light"]': { colorScheme: "light" },
      ':root[data-theme="dark"]': { colorScheme: "dark" },
      "@keyframes my-spin": { to: { transform: "rotate(360deg)" } },
      "@keyframes my-reaction": {
        from: { opacity: "0", transform: "translateY(0) scale(0.6)" },
        "12%": { opacity: "1", transform: "translateY(calc(var(--voice-rise) * -0.12)) scale(1)" },
        "70%": { opacity: "1" },
        to: { opacity: "0", transform: "translateY(calc(var(--voice-rise) * -1)) scale(1)" },
      },
      "@keyframes my-reaction-still": {
        from: { opacity: "0" },
        "12%": { opacity: "1" },
        "70%": { opacity: "1" },
        to: { opacity: "0" },
      },
      "@keyframes my-speaking": { to: { boxShadow: speaking("var(--voice-halo)") } },
    });
    api.addBase(transitionRules);
  },
  { theme },
);
