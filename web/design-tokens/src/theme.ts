// SPDX-License-Identifier: AGPL-3.0-only

// The tokens a server theme may override (ADR-0059 §2), with their kinds, defaults and bounds: the one list the theme
// editor of the administration draws its fields from and the Go domain's generated list is written from
// (theme-tokens.ts). The contrast pairs an override is checked against are `contrastPairs` of tokens.ts.
import { color, contrastPairs, properties } from "./tokens";

/** The kinds of value the Go domain validates an override against (instance.ThemeTokenKind). */
export type ThemeTokenKind = "color" | "length" | "number" | "font-family" | "duration";

/**
 * How a custom property is treated: overridable with a kind and, for a length (rem), a number or a duration (ms),
 * the bounds that keep the screens usable; or fixed, without a kind. Every property matches one rule, so a new
 * token is classified before it reaches the Go list.
 */
type Rule = { pattern: RegExp; kind?: ThemeTokenKind; min?: number; max?: number };

const rules: readonly Rule[] = [
  // Fixed: a pair with an avatar holds its contrast only with this value.
  { pattern: /^--color-avatar-/ },
  // Fixed: a role's badge has the same colors on every server (BadgeColor of the client API).
  { pattern: /^--color-(on-)?badge-/ },
  // Fixed: the stream's dot means the same on every server.
  { pattern: /^--color-stream-/ },
  { pattern: /^--color-/, kind: "color" },
  // A theme adds its fonts to the stacks; the stacks, the Latin family and the CJK region are fixed, so that a theme
  // never drops the emoji and text web fonts.
  { pattern: /^--font-(sans|mono)-override$/, kind: "font-family" },
  { pattern: /^--font-(sans|mono|latin|cjk)$/ },
  { pattern: /^--text-[a-z]+$/, kind: "length", min: 0.625, max: 3 },
  { pattern: /^--text-[a-z]+--line-height$/, kind: "number", min: 1, max: 2.5 },
  { pattern: /^--radius-/, kind: "length", min: 0, max: 2 },
  { pattern: /^--duration-/, kind: "duration", min: 0, max: 1000 },
  // Fixed: the easing of a motion, which its keyframes are drawn for.
  { pattern: /^--ease-/ },
  // Fixed: the motion of a change of screen, whose kinds read as one set only with these values.
  { pattern: /^--transition-/ },
  // Fixed: the weights and letter spacing of the type scale, and the sizes and mixes of the components, whose
  // values hold a touch target, a ring around a control, or a contrast of a mix.
  { pattern: /^--text-[a-z]+--(font-weight|letter-spacing)$/ },
  { pattern: /^--font-weight-/ },
  {
    pattern:
      /^--(control|icon-button|target|focus-ring|press|spinner|icon|avatar|menu|disclosure|content|article|entry|feed|embed|poll|media|viewer|chip|toggle|emoji-picker|dialog|composer|diff|kbd|shell|screen|voice|event|form-page|landing)-/,
  },
  // Fixed: Tailwind maps a shadow to its utilities.
  { pattern: /^--shadow-/ },
];

export type ThemeToken = {
  /** The token's name without the leading `--` (`color-accent`). */
  name: string;
  kind: ThemeTokenKind;
  /** A color's default in each scheme, #rrggbb. */
  light?: string;
  dark?: string;
  /** Another kind's default, as the stylesheet writes it. */
  value?: string;
  /** The bounds of a length (rem), a number, or a duration (ms). */
  min?: number;
  max?: number;
};

/** The overridable tokens, in the order of `properties`, named without the leading `--`. */
export function themeTokens(): ThemeToken[] {
  const tokens: ThemeToken[] = [];
  for (const [property, value] of Object.entries(properties)) {
    const rule = rules.find((r) => r.pattern.test(property));
    if (rule === undefined) throw new Error(`${property} matches no rule of src/theme-tokens.ts: overridable or fixed`);
    if (rule.kind === undefined) continue;
    const token: ThemeToken = { name: property.slice(2), kind: rule.kind };
    if (rule.kind === "color") {
      const c = color[property.slice("--color-".length) as keyof typeof color];
      token.light = c.light;
      token.dark = c.dark;
    } else {
      token.value = value;
    }
    if (rule.min !== undefined) token.min = rule.min;
    if (rule.max !== undefined) token.max = rule.max;
    tokens.push(token);
  }
  for (const { fg, bg } of contrastPairs) {
    for (const name of [fg, bg]) {
      if (!tokens.some((t) => t.name === `color-${name}`))
        throw new Error(`the contrast pair ${fg} on ${bg} names --color-${name}, which is not overridable`);
    }
  }
  return tokens;
}
