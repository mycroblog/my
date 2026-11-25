// SPDX-License-Identifier: AGPL-3.0-only

// emoji.css as ADR-0091 §9, §10 and §13 decide it: one @font-face of "my Noto Color Emoji" per split, COLRv1 first
// and sbix second, each chosen by tech(), swapped in when it arrives, over the emoji code points alone, so that
// the keycap bases and the symbols that running text uses (# * 0-9 © ® ™ ‼ ⁉ ↔ ↕) stay with the text fonts, and the
// emoji that hold one fall back to the OS emoji font; and the WOFF2 of every face and the font's license beside them.

import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";

const css = readFileSync(new URL("./emoji.css", import.meta.url), "utf8");

/** Each split with emoji of its group of emoji-test.txt. */
const samples = {
  "smileys-emotion": "😀😂🥹❤️🫠",
  "people-body": "👋👍🏽🧑‍💻👩🏿‍🦰🫶",
  component: "🏻🏿🦰",
  "animals-nature": "🐱🐶🌸🍀🪼",
  "food-drink": "🍣🍜🍙🍵🫖",
  "travel-places": "🗻🚃☀️🌈🏯",
  activities: "🎉⚽🎮🎨🪁🪩",
  objects: "📱💡📚🔑🔔",
  symbols: "✅❌♻️🆗",
  flags: "🏁🏳️‍🌈🏴‍☠️",
  "country-flags": "🇯🇵🇺🇸🇧🇷🇪🇺",
} as const;
const splits = Object.keys(samples) as (keyof typeof samples)[];

/** The code points of `s`, each of a sequence on its own: a unicode-range claims code points, not sequences. */
function codePoints(s: string): number[] {
  const points: number[] = [];
  for (let i = 0; i < s.length;) {
    const cp = s.codePointAt(i) ?? 0;
    points.push(cp);
    i += cp > 0xffff ? 2 : 1;
  }
  return points;
}

/** The code points that running text uses, which no face may claim (ADR-0091 §10). */
const textCodePoints = codePoints("#*0123456789©®™‼⁉↔↕");

type Face = {
  family: string | undefined;
  display: string | undefined;
  src: string | undefined;
  ranges: [number, number][];
};

/** The declarations of each @font-face block of the stylesheet. */
function faces(source: string): Face[] {
  const blocks = [...source.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(/@font-face\s*\{([^}]*)\}/g)].map((m) => m[1]);
  return blocks.map((block = "") => {
    const decl = (name: string) => new RegExp(`(?:^|;)\\s*${name}\\s*:\\s*([^;]+)`).exec(block)?.[1]?.trim();
    return {
      family: decl("font-family"),
      display: decl("font-display"),
      src: decl("src"),
      ranges: (decl("unicode-range") ?? "")
        .split(",")
        .map((r) => r.trim())
        .filter((r) => r !== "")
        .map(range),
    };
  });
}

/** One item of a unicode-range: U+1F600, U+1F600-1F64F or U+1F6??. */
function range(item: string): [number, number] {
  const m = /^U\+([0-9A-F?]+)(?:-([0-9A-F]+))?$/i.exec(item);
  if (m === null) throw new Error(`not a unicode-range item: ${item}`);
  const [, from = "", to] = m;
  if (from.includes("?")) return [parseInt(from.replaceAll("?", "0"), 16), parseInt(from.replaceAll("?", "F"), 16)];
  return [parseInt(from, 16), parseInt(to ?? from, 16)];
}

/** The URLs of a src descriptor with the format and tech of each, in their order. */
function sources(src: string): { url: string; format?: string; tech?: string }[] {
  return [
    ...src.matchAll(
      /url\(\s*["']?([^"')]+)["']?\s*\)\s*(?:format\(\s*["']?([^"')]+)["']?\s*\))?\s*(?:tech\(\s*([^)]+)\))?/g,
    ),
  ].map(([, url = "", format, tech]) => ({
    url,
    ...(format ? { format } : {}),
    ...(tech ? { tech: tech.trim() } : {}),
  }));
}

const all = faces(css);

test("emoji.css holds one face of each split", () => {
  expect(all).toHaveLength(splits.length);
  const files = all.map((f) => /noto-emoji-colrv1-([a-z-]+)\.woff2/.exec(f.src ?? "")?.[1]);
  expect(files.toSorted()).toEqual([...splits].toSorted());
});

describe.each(splits)("the face of %s", (split) => {
  const face = all.find((f) => f.src?.includes(`noto-emoji-colrv1-${split}.woff2`));

  test('is of the family "my Noto Color Emoji" and swaps in when it arrives', () => {
    expect(face?.family).toMatch(/^["']my Noto Color Emoji["']$/);
    expect(face?.display).toBe("swap");
  });

  test("offers the COLRv1 WOFF2, then the sbix one, each by tech()", () => {
    const src = sources(face?.src ?? "");
    expect(src.map((s) => [s.url.replace(/^.*\//, ""), s.format, s.tech])).toEqual([
      [`noto-emoji-colrv1-${split}.woff2`, "woff2", "color-COLRv1"],
      [`noto-emoji-sbix-${split}.woff2`, "woff2", "color-sbix"],
    ]);
    for (const { url } of src) expect(existsSync(new URL(url, new URL("./emoji.css", import.meta.url)))).toBe(true);
  });

  test("claims every code point of its emoji", () => {
    const ranges = face?.ranges ?? [];
    const missed = codePoints(samples[split]).filter((cp) => !ranges.some(([from, to]) => from <= cp && cp <= to));
    expect(missed.map((cp) => `U+${cp.toString(16).toUpperCase()}`)).toEqual([]);
  });

  test("claims none of the text code points # * 0-9 © ® ™ ‼ ⁉ ↔ ↕", () => {
    const ranges = face?.ranges ?? [];
    const claimed = textCodePoints.filter((cp) => ranges.some(([from, to]) => from <= cp && cp <= to));
    expect(claimed.map((cp) => `U+${cp.toString(16).toUpperCase().padStart(4, "0")}`)).toEqual([]);
  });
});

test("the emoji that hold a text code point after their start fall back whole to the OS emoji font", () => {
  // A face draws a sequence only when it claims every code point of it, and one that claimed ↔ would draw the arrows
  // of running text as emoji (TEXT_FALLBACK_SEQUENCES of scripts/emoji-fonts).
  const claimedWhole = (s: string) =>
    all.some((f) => codePoints(s).every((cp) => f.ranges.some(([from, to]) => from <= cp && cp <= to)));
  expect(["🙂‍↔️", "🙂‍↕️"].filter(claimedWhole)).toEqual([]);
});

test("the country flags' face claims the regional indicators, and no other face does (ADR-0091 §9)", () => {
  const regional = (f: Face) => f.ranges.some(([from, to]) => from <= 0x1f1ff && to >= 0x1f1e6);
  expect(all.filter(regional).map((f) => /noto-emoji-colrv1-([a-z-]+)\.woff2/.exec(f.src ?? "")?.[1])).toEqual([
    "country-flags",
  ]);
});

test("the font's license lies beside the WOFF2 (ADR-0091 §13)", () => {
  expect(readFileSync(new URL("./fonts/emoji/OFL.txt", import.meta.url), "utf8")).toContain(
    "SIL OPEN FONT LICENSE Version 1.1",
  );
});
