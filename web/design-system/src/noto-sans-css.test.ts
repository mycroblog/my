// SPDX-License-Identifier: AGPL-3.0-only

// noto-sans.css as ADR-0009 §5 decides it: the faces of "my Noto Sans", for a text in no CJK language, and of "my Noto
// Sans beside CJK", which leaves to Noto Sans CJK the punctuation it draws full-width; each a WOFF2 slice of Google
// Fonts' subsets that lies beside the stylesheet, swapped in when it arrives, with the one-em box of the other fonts;
// and the font's license beside them.

import { latinFamily } from "@my/design-tokens";
import { expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";

const stylesheet = new URL("./noto-sans.css", import.meta.url);
const css = readFileSync(stylesheet, "utf8");

type Face = { family: string; decl: (name: string) => string | undefined; ranges: [number, number][] };

/** One item of a unicode-range: U+0100 or U+0100-024F. */
function range(item: string): [number, number] {
  const m = /^U\+([0-9A-F]+)(?:-([0-9A-F]+))?$/i.exec(item.trim());
  if (m === null) throw new Error(`not a unicode-range item: ${item}`);
  const [, from = "", to] = m;
  return [parseInt(from, 16), parseInt(to ?? from, 16)];
}

const all: Face[] = [...css.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(/@font-face\s*\{([^}]*)\}/g)].map(
  ([, block = ""]) => {
    const decl = (name: string) => new RegExp(`(?:^|;)\\s*${name}\\s*:\\s*([^;]+)`).exec(block)?.[1]?.trim();
    return {
      family: (decl("font-family") ?? "").replace(/^["']|["']$/g, ""),
      decl,
      ranges: (decl("unicode-range") ?? "")
        .split(",")
        .filter((r) => r.trim() !== "")
        .map(range),
    };
  },
);

const file = (f: Face) => /noto-sans-([a-z-]+)\.woff2/.exec(f.decl("src") ?? "")?.[1];
const claims = (faces: Face[], ch: string) => {
  const cp = ch.codePointAt(0) ?? 0;
  return faces.some((f) => f.ranges.some(([from, to]) => from <= cp && cp <= to));
};
const of = (family: string) => all.filter((f) => f.family === family);

test("noto-sans.css declares the slices of Google Fonts' subsets for each family, and the wide slice for one", () => {
  const subsets = ["cyrillic", "cyrillic-ext", "greek", "greek-ext", "latin", "latin-ext", "vietnamese"];
  expect(of(latinFamily.family).map(file).toSorted()).toEqual([...subsets, "wide"].toSorted());
  expect(of(latinFamily.besideCjk).map(file).toSorted()).toEqual(subsets);
  expect(all.length).toBe(subsets.length * 2 + 1);
});

test("each face swaps in, with the one-em box of the other fonts, over one WOFF2 beside the stylesheet", () => {
  for (const face of all) {
    const src = face.decl("src") ?? "";
    const url = /url\(\s*["']?([^"')]+)["']?\s*\)/.exec(src)?.[1] ?? "";
    expect({
      src,
      display: face.decl("font-display"),
      ascent: face.decl("ascent-override"),
      descent: face.decl("descent-override"),
      exists: existsSync(new URL(url, stylesheet)),
    }).toEqual({ src, display: "swap", ascent: "88%", descent: "12%", exists: true });
  }
});

test("the letters that Noto Sans CJK lacks draw from Noto Sans in either family", () => {
  for (const family of [latinFamily.family, latinFamily.besideCjk])
    expect(
      ["ą", "ł", "ż", "č", "ř", "ş", "ğ", "і", "ї", "đ", "ơ", "ư"].filter((ch) => !claims(of(family), ch)),
    ).toEqual([]);
});

test("beside CJK, the punctuation that Noto Sans CJK draws full-width stays with it", () => {
  const wide = ["…", "※", "×", "“", "”", "‥"];
  expect(wide.filter((ch) => claims(of(latinFamily.besideCjk), ch))).toEqual([]);
  expect(wide.filter((ch) => !claims(of(latinFamily.family), ch))).toEqual([]);
});

test("the font's license lies beside the WOFF2 (OFL 1.1)", () => {
  expect(readFileSync(new URL("./fonts/noto-sans/OFL-NotoSans.txt", stylesheet), "utf8")).toContain(
    "SIL OPEN FONT LICENSE Version 1.1",
  );
});
