// SPDX-License-Identifier: AGPL-3.0-only

// The tokens of the instance theme as its editor groups them (ADR-0059 §2): each overridable token of
// @my/design-tokens (`themeTokens`) in a section of one concern, with its name and, for a color, what it colors, as the
// catalog says them; and the pairs a color makes with the others (`contrastPairs`), which the server holds to WCAG AA.
// A token the list has and no section names is not offered; one a section names and the list lacks fails the tests.

import { contrastPairs, type ColorName } from "@my/design-tokens";
import { themeTokens, type ThemeToken } from "@my/design-tokens/theme";
import type { MessageId } from "../../messages.gen";

/** A section of the editor: its heading (`staff_theme.section.<id>`) and its tokens, in their order. */
type ThemeSection = { readonly id: string; readonly heading: MessageId; readonly tokens: readonly string[] };

/** The sections of the editor, the colors first. */
export const themeSections: readonly ThemeSection[] = [
  {
    id: "text",
    heading: "staff_theme.section.text",
    tokens: ["color-text", "color-muted-text", "color-accent-text", "color-quote-bar"],
  },
  { id: "accent", heading: "staff_theme.section.accent", tokens: ["color-accent", "color-on-accent"] },
  {
    id: "surfaces",
    heading: "staff_theme.section.surfaces",
    tokens: [
      "color-surface",
      "color-surface-hover",
      "color-surface-pressed",
      "color-surface-raised",
      "color-surface-raised-hover",
      "color-surface-raised-pressed",
    ],
  },
  { id: "fonts", heading: "staff_theme.section.fonts", tokens: ["font-sans-override", "font-mono-override"] },
  {
    id: "sizes",
    heading: "staff_theme.section.sizes",
    tokens: [
      "text-tagline",
      "text-headline",
      "text-heading",
      "text-title",
      "text-reading",
      "text-body",
      "text-caption",
    ],
  },
  {
    id: "line_heights",
    heading: "staff_theme.section.line_heights",
    tokens: [
      "text-tagline--line-height",
      "text-headline--line-height",
      "text-heading--line-height",
      "text-title--line-height",
      "text-reading--line-height",
      "text-body--line-height",
      "text-caption--line-height",
    ],
  },
  { id: "shape", heading: "staff_theme.section.shape", tokens: ["radius-control", "radius-overlay", "duration-spin"] },
];

/** The name of each token, as its field and a refusal say it; a line height takes its size's name. */
export const tokenNames: Readonly<Record<string, MessageId>> = {
  "color-text": "staff_theme.token.color_text",
  "color-muted-text": "staff_theme.token.color_muted_text",
  "color-accent-text": "staff_theme.token.color_accent_text",
  "color-quote-bar": "staff_theme.token.color_quote_bar",
  "color-accent": "staff_theme.token.color_accent",
  "color-on-accent": "staff_theme.token.color_on_accent",
  "color-surface": "staff_theme.token.color_surface",
  "color-surface-hover": "staff_theme.token.color_surface_hover",
  "color-surface-pressed": "staff_theme.token.color_surface_pressed",
  "color-surface-raised": "staff_theme.token.color_surface_raised",
  "color-surface-raised-hover": "staff_theme.token.color_surface_raised_hover",
  "color-surface-raised-pressed": "staff_theme.token.color_surface_raised_pressed",
  "font-sans-override": "staff_theme.token.font_sans_override",
  "font-mono-override": "staff_theme.token.font_mono_override",
  "text-tagline": "staff_theme.token.text_tagline",
  "text-headline": "staff_theme.token.text_headline",
  "text-heading": "staff_theme.token.text_heading",
  "text-title": "staff_theme.token.text_title",
  "text-reading": "staff_theme.token.text_reading",
  "text-body": "staff_theme.token.text_body",
  "text-caption": "staff_theme.token.text_caption",
  "text-tagline--line-height": "staff_theme.token.text_tagline",
  "text-headline--line-height": "staff_theme.token.text_headline",
  "text-heading--line-height": "staff_theme.token.text_heading",
  "text-title--line-height": "staff_theme.token.text_title",
  "text-reading--line-height": "staff_theme.token.text_reading",
  "text-body--line-height": "staff_theme.token.text_body",
  "text-caption--line-height": "staff_theme.token.text_caption",
  "radius-control": "staff_theme.token.radius_control",
  "radius-overlay": "staff_theme.token.radius_overlay",
  "duration-spin": "staff_theme.token.duration_spin",
};

/** How a font family is written, the line under its field's name; a token named by what it colors has none. */
export const tokenAbouts: Readonly<Record<string, MessageId>> = {
  "font-sans-override": "staff_theme.token.font_sans_override.about",
  "font-mono-override": "staff_theme.token.font_mono_override.about",
};

/** The overridable tokens, by name. */
export const themeTokensByName: ReadonlyMap<string, ThemeToken> = new Map(themeTokens().map((t) => [t.name, t]));

/** The name of a color token's color (`color-accent` → `accent`); none for a token of another kind. */
const colorOf = (token: string): ColorName | undefined =>
  token.startsWith("color-") ? (token.slice("color-".length) as ColorName) : undefined;

/** A pair a color token makes: the other color's token, its place in the pair, and the ratio it needs. */
export type TokenPair = { readonly other: string; readonly role: "text" | "background"; readonly minimum: number };

/** The pairs the color `token` makes with the others, from its side. */
export function pairsOf(token: string): TokenPair[] {
  const name = colorOf(token);
  return contrastPairs.flatMap((pair): TokenPair[] => {
    if (pair.fg === name) return [{ other: `color-${pair.bg}`, role: "text", minimum: pair.minimum }];
    if (pair.bg === name) return [{ other: `color-${pair.fg}`, role: "background", minimum: pair.minimum }];
    return [];
  });
}
