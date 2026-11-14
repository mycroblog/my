// SPDX-License-Identifier: AGPL-3.0-only

// Tailwind's theme, generated from @my/design-tokens (ADR-0025 §1, ADR-0059 §1), which styles.css loads with
// @plugin. It declares every token as a custom property on :root, and names each in the theme as a reference to that
// property, so that a utility reads `var(--color-surface)` and a theme's override of the property reaches it.

import {
  breakpoint,
  color,
  control,
  fontFamily,
  fontWeight,
  motion,
  properties,
  radius,
  shell,
  text,
} from "@my/design-tokens";
import plugin from "tailwindcss/plugin";

const ref = (name: string) => `var(${name})`;
const names = <T extends object>(o: T) => Object.keys(o) as (keyof T & string)[];

/** A token's camel-case name as its custom property's: `gapWide` as `gap-wide`. */
const kebab = (name: string) => name.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);

// The theme's keys, each a token or a value derived from tokens only. The breakpoints (`tablet:`, `desktop:`,
// `wide:`) are for the app's frame alone (shell.tsx, ADR-0110 §10); a part lays itself out by its container.
const theme = {
  screens: Object.fromEntries(names(breakpoint).map((name) => [name, `${breakpoint[name]}rem`])),
  colors: {
    ...Object.fromEntries(names(color).map((name) => [name, ref(`--color-${name}`)])),
    "accent-hover": "color-mix(in srgb, var(--press-shade) var(--press-hover), var(--color-accent))",
    "accent-pressed": "color-mix(in srgb, var(--press-shade) var(--press-pressed), var(--color-accent))",
    "spinner-track": "color-mix(in srgb, currentColor var(--spinner-track-share), transparent)",
    scrim: ref("--dialog-scrim"),
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
      ]),
    ),
    "icon-button-inline": ref("--icon-button-padding-inline"),
    "icon-button-gap": ref("--icon-button-gap"),
    "icon-button-touch": ref("--icon-button-touch"),
    press: ref("--press-shift"),
    spinner: ref("--spinner-size"),
    "icon-action": ref("--icon-action"),
    "icon-caption": ref("--icon-caption"),
    "icon-navigation": ref("--icon-navigation"),
    ...Object.fromEntries(names(shell).map((name) => [`shell-${kebab(name)}`, ref(`--shell-${kebab(name)}`)])),
    "icon-tile-glyph": ref("--icon-tile-glyph"),
    "avatar-inline": ref("--avatar-inline"),
    "avatar-post": ref("--avatar-post"),
    "avatar-preview": ref("--avatar-preview"),
    "menu-padding": ref("--menu-padding"),
    "menu-group-gap": ref("--menu-group-gap"),
    "menu-icon-gap": ref("--menu-icon-gap"),
    "disclosure-gap": ref("--disclosure-gap"),
    "disclosure-padding": ref("--disclosure-padding"),
    "disclosure-start": ref("--disclosure-padding-start"),
    "content-paragraph": ref("--content-paragraph-gap"),
    "content-indent": ref("--content-indent"),
    "content-quote": ref("--content-quote-indent"),
    "content-code-block": ref("--content-code-padding-block"),
    "content-code-inline": ref("--content-code-padding-inline"),
    "content-code-chip": ref("--content-code-inline"),
    "content-emoji": ref("--content-emoji"),
    entry: ref("--entry-gap"),
    "entry-line": ref("--entry-line-gap"),
    "entry-reach": ref("--entry-reach"),
    "entry-avatar": ref("--entry-avatar-gap"),
    "entry-name": ref("--entry-name-gap"),
    "entry-caption": ref("--entry-caption-gap"),
    "entry-action": ref("--entry-action-gap"),
    "entry-scroll": ref("--entry-scroll-margin"),
    feed: ref("--feed-gap"),
    "feed-scroll": ref("--feed-scroll-margin"),
    embed: ref("--embed-padding"),
    "embed-gap": ref("--embed-gap"),
    media: ref("--media-gap"),
    "media-preview-width": ref("--media-preview-width"),
    "media-preview-height": ref("--media-preview-height"),
    chip: ref("--chip-height"),
    "chip-inline": ref("--chip-padding-inline"),
    "chip-gap": ref("--chip-gap"),
    "chip-spacing": ref("--chip-spacing"),
    dialog: ref("--dialog-padding"),
    "dialog-gap": ref("--dialog-gap"),
    "dialog-row": ref("--dialog-row-gap"),
    "dialog-margin": ref("--dialog-margin"),
    kbd: ref("--kbd-height"),
    "kbd-inline": ref("--kbd-padding-inline"),
    "kbd-gap": ref("--kbd-gap"),
  },
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
    overlay: ref("--shadow-overlay"),
    bar: ref("--shadow-bar"),
  },
  animation: Object.fromEntries(
    names(motion).map((name) => [name, `my-${name} var(--duration-${name}) linear infinite`]),
  ),
};

export default plugin(
  (api) => {
    api.addBase({
      ":root": { colorScheme: "light dark", ...properties },
      ':root[data-theme="light"]': { colorScheme: "light" },
      ':root[data-theme="dark"]': { colorScheme: "dark" },
      "@keyframes my-spin": { to: { transform: "rotate(360deg)" } },
    });
  },
  { theme },
);
