// SPDX-License-Identifier: AGPL-3.0-only

// The design tokens of the web client (ADR-0025 §2, ADR-0059): the values the design system reads. They start from
// the set the decider picked on the catalog page and grow with the components. `css` writes them as custom
// properties; `contrastPairs` are the pairs of colors a check holds to their minimum in both schemes.

/** The root font size the rem values are relative to, in px. */
export const rootPx = 16;

/** Latin from the system's sans, Japanese from a gothic; system stacks only, since the CSP allows no font CDN. */
export const fontFamily = {
  sans: 'system-ui, -apple-system, "Segoe UI", Roboto, "Hiragino Sans", "Noto Sans CJK JP", "Noto Sans JP", "Yu Gothic UI", "Yu Gothic", sans-serif',
} as const;

export type TextStep = {
  /** The font size, in rem. */
  size: number;
  lineHeight: number;
  fontWeight: number;
  letterSpacing?: string;
};

/** The type scale, from the largest; the tagline reads as a catch copy. */
export const text = {
  tagline: { size: 1.875, lineHeight: 1.2, fontWeight: 800, letterSpacing: "-0.01em" },
  title: { size: 1.125, lineHeight: 1.35, fontWeight: 700 },
  body: { size: 0.875, lineHeight: 1.55, fontWeight: 400 },
  caption: { size: 0.75, lineHeight: 1.45, fontWeight: 400 },
} as const satisfies Record<string, TextStep>;

export type TextName = keyof typeof text;

export type Scheme = "light" | "dark";

/** A color for each scheme, as #rrggbb. */
export type SchemeColor = Record<Scheme, string>;

/**
 * The colors. `accent` is the brand's coral as a fill, with `on-accent` on it; `accent-text` carries links and the
 * focus ring, darker than the fill in light, where the coral reaches only 2.8:1 on the surface.
 */
export const color = {
  surface: { light: "#ffffff", dark: "#221c1b" },
  text: { light: "#111111", dark: "#f2e9e7" },
  "muted-text": { light: "#74665f", dark: "#9c8f8b" },
  accent: { light: "#ff6b6b", dark: "#ff6b6b" },
  "on-accent": { light: "#111111", dark: "#111111" },
  "accent-text": { light: "#a64646", dark: "#ff6b6b" },
} as const satisfies Record<string, SchemeColor>;

export type ColorName = keyof typeof color;

/** A foreground color drawn on a background color, and the contrast ratio it needs in both schemes. */
export type ContrastPair = { fg: ColorName; bg: ColorName; minimum: number };

/**
 * The pairs a check holds: text needs 4.5:1, which also covers the focus ring's 3:1 (WCAG 1.4.11). The fill against
 * the surface is no pair: the button's text carries its meaning.
 */
export const contrastPairs: readonly ContrastPair[] = [
  { fg: "text", bg: "surface", minimum: 4.5 },
  { fg: "muted-text", bg: "surface", minimum: 4.5 },
  { fg: "accent-text", bg: "surface", minimum: 4.5 },
  { fg: "on-accent", bg: "accent", minimum: 4.5 },
];

const channels = (hex: string) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));

/** The WCAG 2 relative luminance of a #rrggbb color. */
const luminance = (hex: string) => {
  const [r = 0, g = 0, b = 0] = channels(hex).map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};

/** The WCAG 2 contrast ratio of two #rrggbb colors, from 1 to 21. */
export function contrast(a: string, b: string): number {
  const [hi = 0, lo = 0] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

const textProperties = (name: string, step: TextStep) => [
  `--text-${name}: ${step.size}rem;`,
  `--text-${name}--line-height: ${step.lineHeight};`,
  `--text-${name}--font-weight: ${step.fontWeight};`,
  ...(step.letterSpacing === undefined ? [] : [`--text-${name}--letter-spacing: ${step.letterSpacing};`]),
];

/**
 * The tokens as custom properties on :root: `--font-*`, `--text-*` (with Tailwind's `--line-height`,
 * `--font-weight` and `--letter-spacing` suffixes) and `--color-*` as `light-dark()`, which follows the color
 * scheme of the system or of `<html data-theme>`.
 */
export const css = [
  ":root {",
  "  color-scheme: light dark;",
  ...Object.entries(fontFamily).map(([name, value]) => `  --font-${name}: ${value};`),
  ...Object.entries(text).flatMap(([name, step]) => textProperties(name, step).map((line) => `  ${line}`)),
  ...Object.entries(color).map(([name, c]) => `  --color-${name}: light-dark(${c.light}, ${c.dark});`),
  "}",
  ':root[data-theme="light"] { color-scheme: light; }',
  ':root[data-theme="dark"] { color-scheme: dark; }',
  "",
].join("\n");
