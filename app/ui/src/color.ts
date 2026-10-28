// SPDX-License-Identifier: AGPL-3.0-only
// Color math for the token checks and the server theme editor: parses the color syntaxes theme.css
// uses (oklch() and hex), and computes WCAG 2.2 relative luminance and contrast ratios. The Go
// domain (internal/domain/instance/theme.go) computes contrast with the same formulas.
// Formulas: https://www.w3.org/TR/WCAG22/#dfn-relative-luminance and
// https://bottosson.github.io/posts/oklab/ (OKLab to linear sRGB).

/** A color in linear-light sRGB with straight alpha; channels may fall outside 0..1 when out of gamut. */
export type LinearRgba = { r: number; g: number; b: number; alpha: number };

/** How far a channel may stray outside 0..1 before the color counts as out of the sRGB gamut. */
const gamutEpsilon = 0.0005;

function parseNumber(token: string, percentScale: number): number {
  const value = Number.parseFloat(token);
  if (Number.isNaN(value)) throw new Error(`not a number: ${token}`);
  return token.endsWith("%") ? (value / 100) * percentScale : value;
}

function toLinear(channel: number): number {
  return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
}

function toGamma(channel: number): number {
  return channel <= 0.0031308 ? channel * 12.92 : 1.055 * channel ** (1 / 2.4) - 0.055;
}

function parseOklch(body: string): LinearRgba {
  const [channels = "", alphaToken] = body.split("/").map((part) => part.trim());
  const [lToken, cToken, hToken] = channels.split(/\s+/);
  if (lToken === undefined || cToken === undefined || hToken === undefined) throw new Error(`bad oklch(): ${body}`);
  const l = parseNumber(lToken, 1);
  const c = parseNumber(cToken, 0.4);
  const h = hToken === "none" ? 0 : (parseNumber(hToken, 1) * Math.PI) / 180;
  const a = c * Math.cos(h);
  const b = c * Math.sin(h);
  const l3 = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m3 = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s3 = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return {
    r: 4.0767416621 * l3 - 3.3077115913 * m3 + 0.2309699292 * s3,
    g: -1.2684380046 * l3 + 2.6097574011 * m3 - 0.3413193965 * s3,
    b: -0.0041960863 * l3 - 0.7034186147 * m3 + 1.707614701 * s3,
    alpha: alphaToken === undefined ? 1 : parseNumber(alphaToken, 1),
  };
}

function parseHex(hex: string): LinearRgba {
  const digits = hex.length <= 4 ? hex.replace(/./g, (d) => d + d) : hex;
  if (!/^[0-9a-f]{6}([0-9a-f]{2})?$/i.test(digits)) throw new Error(`bad hex color: #${hex}`);
  const channel = (i: number) => Number.parseInt(digits.slice(i, i + 2), 16) / 255;
  return {
    r: toLinear(channel(0)),
    g: toLinear(channel(2)),
    b: toLinear(channel(4)),
    alpha: digits.length === 8 ? channel(6) : 1,
  };
}

/** Parses `oklch(L C H [/ A])` or `#rgb[a]` / `#rrggbb[aa]`. */
export function parseColor(text: string): LinearRgba {
  const value = text.trim().toLowerCase();
  const oklch = /^oklch\((.*)\)$/.exec(value);
  if (oklch?.[1] !== undefined) return parseOklch(oklch[1]);
  if (value.startsWith("#")) return parseHex(value.slice(1));
  throw new Error(`unsupported color syntax (use oklch() or hex): ${text}`);
}

/** Whether the color is displayable in sRGB without gamut mapping, so its contrast is the same on every screen. */
export function inSrgbGamut(color: LinearRgba): boolean {
  return [color.r, color.g, color.b].every((c) => c >= -gamutEpsilon && c <= 1 + gamutEpsilon);
}

/** Paints `top` over the opaque `bottom` the way browsers composite: in gamma-encoded sRGB. */
function composite(top: LinearRgba, bottom: LinearRgba): LinearRgba {
  const mix = (t: number, b: number) => toLinear(toGamma(t) * top.alpha + toGamma(b) * (1 - top.alpha));
  return { r: mix(top.r, bottom.r), g: mix(top.g, bottom.g), b: mix(top.b, bottom.b), alpha: 1 };
}

/** WCAG relative luminance of an opaque color. */
function luminance(color: LinearRgba): number {
  const clamp = (c: number) => Math.min(1, Math.max(0, c));
  return 0.2126 * clamp(color.r) + 0.7152 * clamp(color.g) + 0.0722 * clamp(color.b);
}

/** WCAG contrast ratio of `fg` drawn on the opaque `bg`, from 1 to 21. */
export function contrast(fg: LinearRgba, bg: LinearRgba): number {
  const a = luminance(composite(fg, bg));
  const b = luminance(bg);
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

/** The `#rrggbb` (or `#rrggbbaa` when translucent) form of a color in the sRGB gamut. */
export function toHex(color: LinearRgba): string {
  const byte = (c: number) =>
    Math.round(Math.min(1, Math.max(0, c)) * 255)
      .toString(16)
      .padStart(2, "0");
  const rgb = [color.r, color.g, color.b].map((c) => byte(toGamma(Math.min(1, Math.max(0, c))))).join("");
  return `#${rgb}${color.alpha < 1 ? byte(color.alpha) : ""}`;
}
