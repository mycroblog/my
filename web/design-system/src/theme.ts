// SPDX-License-Identifier: AGPL-3.0-only

// Tailwind's theme, generated from @my/design-tokens (ADR-0025 §1, ADR-0059 §1), which styles.css loads with
// @plugin. It declares every token as a custom property on :root, and names each in the theme as a reference to that
// property, so that a utility reads `var(--color-surface)` and a theme's override of the property reaches it.

import { color, control, fontFamily, fontWeight, motion, properties, radius, text } from "@my/design-tokens";
import plugin from "tailwindcss/plugin";

const ref = (name: string) => `var(${name})`;
const names = <T extends object>(o: T) => Object.keys(o) as (keyof T & string)[];

// The theme's keys, each a token or a value derived from tokens only.
const theme = {
  colors: {
    ...Object.fromEntries(names(color).map((name) => [name, ref(`--color-${name}`)])),
    "accent-hover": "color-mix(in srgb, var(--press-shade) var(--press-hover), var(--color-accent))",
    "accent-pressed": "color-mix(in srgb, var(--press-shade) var(--press-pressed), var(--color-accent))",
    "spinner-track": "color-mix(in srgb, currentColor var(--spinner-track-share), transparent)",
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
    press: ref("--press-shift"),
    spinner: ref("--spinner-size"),
    "icon-action": ref("--icon-action"),
    "icon-caption": ref("--icon-caption"),
    "icon-tile-glyph": ref("--icon-tile-glyph"),
    "avatar-inline": ref("--avatar-inline"),
    "avatar-post": ref("--avatar-post"),
    "avatar-preview": ref("--avatar-preview"),
  },
  borderWidth: { 0: "0", spinner: ref("--spinner-stroke") },
  opacity: { disabled: ref("--press-disabled-opacity") },
  boxShadow: {
    focus: [
      "0 0 0 var(--focus-ring-gap) var(--color-surface)",
      "0 0 0 calc(var(--focus-ring-gap) + var(--focus-ring-width)) var(--color-accent-text)",
      "0 0 0 calc(var(--focus-ring-gap) + var(--focus-ring-width) + var(--focus-ring-halo)) color-mix(in srgb, var(--color-accent) var(--focus-ring-halo-share), transparent)",
    ].join(", "),
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
