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
 * focus ring, darker than the fill in light, where the coral reaches only 2.8:1 on the surface. Coral means the
 * primary action and links only. `surface-raised` and its hovered and pressed steps are the neutral surface of a
 * control, drawn from the warmth of the palette; `surface-hover` and `surface-pressed` answer a control without a
 * surface, lighter than `surface-raised`, so that it never reads as one with a surface.
 */
export const color = {
  surface: { light: "#ffffff", dark: "#221c1b" },
  text: { light: "#111111", dark: "#f2e9e7" },
  "muted-text": { light: "#74665f", dark: "#9c8f8b" },
  accent: { light: "#ff6b6b", dark: "#ff6b6b" },
  "on-accent": { light: "#111111", dark: "#111111" },
  "accent-text": { light: "#a64646", dark: "#ff6b6b" },
  "surface-raised": { light: "#f4efed", dark: "#372826" },
  "surface-raised-hover": { light: "#e6ddd9", dark: "#43302d" },
  "surface-raised-pressed": { light: "#d9cdc8", dark: "#503835" },
  "surface-hover": { light: "#f9f6f5", dark: "#2b201f" },
  "surface-pressed": { light: "#f1ebe9", dark: "#332624" },
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
  { fg: "text", bg: "surface-raised", minimum: 4.5 },
  { fg: "text", bg: "surface-raised-hover", minimum: 4.5 },
  { fg: "text", bg: "surface-raised-pressed", minimum: 4.5 },
  { fg: "text", bg: "surface-hover", minimum: 4.5 },
  { fg: "text", bg: "surface-pressed", minimum: 4.5 },
  // An icon button's muted count, hovered, and hovered in a marked post; pressed, it takes the text.
  { fg: "muted-text", bg: "surface-hover", minimum: 4.5 },
  { fg: "muted-text", bg: "surface-pressed", minimum: 4.5 },
];

/** The weight of a control's label. */
export const fontWeight = { label: 600 } as const;

/** The corner radius of a control, in rem. */
export const radius = { control: 0.375 } as const;

/** A control's height and inline padding at each size, in rem: compact in a timeline, roomier in a form. */
export const control = {
  timeline: { height: 2, paddingInline: 0.75 },
  form: { height: 2.5, paddingInline: 1 },
} as const satisfies Record<string, { height: number; paddingInline: number }>;

/**
 * The icon button of a post's actions, in rem: its tile is a timeline control's height, with a narrow inline padding
 * so that the tile and the focus ring stay inside a marked post's surface, a gap between the glyph and the count, and
 * the height its hit area reaches on a touch screen, beyond the tile.
 */
export const iconButton = { paddingInline: 0.375, gap: 0.25, touch: 2.75 } as const;

/**
 * The focus ring, in px: a gap of the surface, a ring of `accent-text`, and a halo of the accent at `haloShare`, so
 * that it marks focus around any control, also one without a surface.
 */
export const focusRing = { gap: 2, width: 2, halo: 3, haloShare: 0.35 } as const;

/**
 * How a control answers the pointer: a fill mixes in `hover` and `pressed` of `shade` (on-accent keeps 4.5:1, a test
 * holds it), a press moves it down `shift` px, and a disabled control shows at `disabledOpacity`.
 */
export const press = { shade: "#000000", hover: 0.1, pressed: 0.2, shift: 1, disabledOpacity: 0.4 } as const;

/** A motion: its duration in ms, and whether it stands `still` or keeps turning with reduced motion. */
export type Motion = { duration: number; reduced: "still" | "turn" };

/**
 * Motion. The spinner of a control in flight turns once in `duration` ms; with reduced motion it stands `still`,
 * since a ring with one arc reads as busy without turning.
 */
export const motion: { readonly spin: Motion } = { spin: { duration: 800, reduced: "still" } };

/** The spinner, relative to the label's size, in em: its diameter and stroke, and the share its track shows. */
export const spinner = { size: 1.125, stroke: 0.125, trackShare: 0.25 } as const;

/**
 * An icon's size where it sits, in rem: in an action of a timeline, or beside a caption. An on state the icon set
 * cannot draw by shape is the glyph knocked out of a solid tile: the tile's corner radius and the glyph's size within
 * it, as shares of the tile.
 */
export const icon = { action: 1.125, caption: 0.875, tile: { radius: 0.22, glyph: 0.72 } } as const;

/**
 * An avatar's size where it sits, in rem: inline beside a name (a mention, a reply context), at the head of a post,
 * and in a profile preview; and the size of the initial that stands in for a missing image, as a share of the avatar.
 */
export const avatar = { inline: 1.25, post: 2.5, preview: 4, initial: 0.44 } as const;

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

/** `over` mixed over `under` at `share` (0 to 1) in sRGB, as CSS color-mix does, as #rrggbb. */
export function mix(over: string, under: string, share: number): string {
  const u = channels(under);
  const mixed = channels(over).map((v, i) => Math.round(v * share + (u[i] ?? 0) * (1 - share)));
  return `#${mixed.map((v) => v.toString(16).padStart(2, "0")).join("")}`;
}

const textProperties = (name: string, step: TextStep): [string, string][] => [
  [`--text-${name}`, `${step.size}rem`],
  [`--text-${name}--line-height`, `${step.lineHeight}`],
  [`--text-${name}--font-weight`, `${step.fontWeight}`],
  ...(step.letterSpacing === undefined
    ? []
    : [[`--text-${name}--letter-spacing`, step.letterSpacing] as [string, string]]),
];

const percent = (share: number) => `${share * 100}%`;

/**
 * The tokens as custom properties, each name with its value: `--font-*`, `--text-*` (with Tailwind's
 * `--line-height`, `--font-weight` and `--letter-spacing` suffixes), `--color-*` as `light-dark()`, which follows the
 * color scheme of the system or of `<html data-theme>`, and `--font-weight-*`, `--radius-*`, `--control-*`, `--icon-button-*`, `--focus-ring-*`,
 * `--press-*`, `--duration-*`, `--spinner-*`, `--icon-*` and `--avatar-*`. The design system's stylesheet declares them on :root.
 */
export const properties: Readonly<Record<string, string>> = Object.fromEntries<string>([
  ...Object.entries(fontFamily).map(([name, value]): [string, string] => [`--font-${name}`, value]),
  ...Object.entries(text).flatMap(([name, step]) => textProperties(name, step)),
  ...Object.entries(color).map(([name, c]): [string, string] => [
    `--color-${name}`,
    `light-dark(${c.light}, ${c.dark})`,
  ]),
  ...Object.entries(fontWeight).map(([name, w]): [string, string] => [`--font-weight-${name}`, `${w}`]),
  ...Object.entries(radius).map(([name, r]): [string, string] => [`--radius-${name}`, `${r}rem`]),
  ...Object.entries(control).flatMap(([size, c]): [string, string][] => [
    [`--control-${size}-height`, `${c.height}rem`],
    [`--control-${size}-padding-inline`, `${c.paddingInline}rem`],
  ]),
  ["--icon-button-padding-inline", `${iconButton.paddingInline}rem`],
  ["--icon-button-gap", `${iconButton.gap}rem`],
  ["--icon-button-touch", `${iconButton.touch}rem`],
  ["--focus-ring-gap", `${focusRing.gap}px`],
  ["--focus-ring-width", `${focusRing.width}px`],
  ["--focus-ring-halo", `${focusRing.halo}px`],
  ["--focus-ring-halo-share", percent(focusRing.haloShare)],
  ["--press-shade", press.shade],
  ["--press-hover", percent(press.hover)],
  ["--press-pressed", percent(press.pressed)],
  ["--press-shift", `${press.shift}px`],
  ["--press-disabled-opacity", `${press.disabledOpacity}`],
  ...Object.entries(motion).map(([name, m]): [string, string] => [`--duration-${name}`, `${m.duration}ms`]),
  ["--spinner-size", `${spinner.size}em`],
  ["--spinner-stroke", `${spinner.stroke}em`],
  ["--spinner-track-share", percent(spinner.trackShare)],
  ["--icon-action", `${icon.action}rem`],
  ["--icon-caption", `${icon.caption}rem`],
  ["--icon-tile-radius", percent(icon.tile.radius)],
  ["--icon-tile-glyph", percent(icon.tile.glyph)],
  ["--avatar-inline", `${avatar.inline}rem`],
  ["--avatar-post", `${avatar.post}rem`],
  ["--avatar-preview", `${avatar.preview}rem`],
  ["--avatar-initial", `${avatar.initial}`],
]);
