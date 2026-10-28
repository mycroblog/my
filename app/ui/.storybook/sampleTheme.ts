// SPDX-License-Identifier: AGPL-3.0-only
// A sample server theme (ADR-0059) for the Storybook preview: token overrides in the shape the admin API
// takes, rendered the way the server renders a theme's stylesheet (internal/domain/instance, Stylesheet).
// The `Instance theme` toolbar of .storybook/preview.tsx applies it, and the story tests run every story
// with it too (vitest.config.ts). `scripts/task.sh web:tokens` (scripts/check-tokens.ts) checks it as the server
// would check a saved theme: known tokens, values of their kinds, and AA contrast of every color pair.

/** One token override: a color per theme as `#rrggbb[aa]`, or the value of a non-color token. */
export type ThemeOverride = { token: string; light: string; dark: string } | { token: string; value: string };

/**
 * A teal theme on warm surfaces with a looser spacing step and rounder corners, so that a screenshot shows
 * colors, spacing and radius all following the overrides.
 */
export const sampleTheme: readonly ThemeOverride[] = [
  { token: "color-surface", light: "#fbfaf6", dark: "#0c1210" },
  { token: "color-surface-raised", light: "#ffffff", dark: "#151d1b" },
  { token: "color-surface-sunken", light: "#f6f5f0", dark: "#050807" },
  { token: "color-accent", light: "#0f766e", dark: "#2dd4bf" },
  { token: "color-accent-hover", light: "#115e59", dark: "#5eead4" },
  { token: "color-accent-fg", light: "#ffffff", dark: "#042f2e" },
  { token: "color-focus", light: "#0f766e", dark: "#5eead4" },
  { token: "spacing", value: "0.3rem" },
  { token: "radius-sm", value: "0.5rem" },
  { token: "radius-md", value: "0.75rem" },
  { token: "radius-lg", value: "1rem" },
  { token: "radius-xl", value: "1.5rem" },
];

/** The stylesheet of a theme: one :root rule outside any cascade layer, colors as light-dark(). */
export function themeStylesheet(overrides: readonly ThemeOverride[]): string {
  const declarations = overrides.map(
    (o) => `  --${o.token}: ${"value" in o ? o.value : `light-dark(${o.light}, ${o.dark})`};\n`,
  );
  return `:root {\n${declarations.join("")}}\n`;
}
