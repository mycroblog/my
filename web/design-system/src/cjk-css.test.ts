// SPDX-License-Identifier: AGPL-3.0-only

// cjk.css as ADR-0009 §5 decides it: the faces of "my Noto Sans CJK JP", "… SC", "… TC" and "… KR", each a WOFF2
// slice that lies beside the stylesheet, of its own region, over a unicode-range, so that a page loads only the
// slices of the characters it shows; swapped in when it arrives; and the fonts' license beside them.

import { cjkRegions } from "@my/design-tokens";
import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";

const stylesheet = new URL("./cjk.css", import.meta.url);
const css = readFileSync(stylesheet, "utf8");

type Face = { family: string | undefined; display: string | undefined; src: string | undefined; range: string };

/** The declarations of each @font-face block of the stylesheet. */
function faces(source: string): Face[] {
  const blocks = [...source.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(/@font-face\s*\{([^}]*)\}/g)].map((m) => m[1]);
  return blocks.map((block = "") => {
    const decl = (name: string) => new RegExp(`(?:^|;)\\s*${name}\\s*:\\s*([^;]+)`).exec(block)?.[1]?.trim();
    return {
      family: decl("font-family"),
      display: decl("font-display"),
      src: decl("src"),
      range: decl("unicode-range") ?? "",
    };
  });
}

/** The URLs of a src descriptor, in their order. */
function urls(src: string): string[] {
  return [...src.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/g)].map(([, url = ""]) => url);
}

/** A unicode-range item: U+4E00, U+4E00-9FFF or U+4E??. */
const rangeItem = /^U\+(?:[0-9A-F]{1,6}(?:-[0-9A-F]{1,6})?|[0-9A-F]{0,5}\?{1,6})$/i;

const all = faces(css);

/** The region of a family name, "my Noto Sans CJK TC" → "tc". */
const regionOf = (family: string | undefined) =>
  /^["']my Noto Sans CJK (JP|SC|TC|KR)["']$/.exec(family ?? "")?.[1]?.toLowerCase();

test("cjk.css declares faces of each region of the design tokens, and of no other family", () => {
  expect(all.length).toBeGreaterThan(0);
  const families = new Set(all.map((f) => f.family?.replace(/^["']|["']$/g, "")));
  expect([...families].toSorted()).toEqual(cjkRegions.map((r) => r.family).toSorted());
});

describe.each(cjkRegions.map((r) => r.family))("the faces of %s", (family) => {
  const own = all.filter((f) => f.family?.replace(/^["']|["']$/g, "") === family);
  const region = regionOf(`"${family}"`);

  test("are slices: more than one face, each over its own unicode-range", () => {
    expect(own.length).toBeGreaterThan(1);
    expect(new Set(own.map((f) => f.range)).size).toBe(own.length);
  });

  test("swap in when they arrive", () => {
    expect(own.map((f) => f.display).filter((d) => d !== "swap")).toEqual([]);
  });

  test("each name one WOFF2 of their region that lies beside the stylesheet", () => {
    for (const face of own) {
      const [url, ...more] = urls(face.src ?? "");
      expect({ src: face.src, more }).toEqual({ src: face.src, more: [] });
      expect(url).toMatch(new RegExp(`^\\./fonts/cjk/noto-sans-cjk-${region ?? ""}-[a-z0-9-]+\\.woff2$`));
      expect({ url, exists: existsSync(new URL(url ?? "", stylesheet)) }).toEqual({ url, exists: true });
    }
  });

  test("each claim a unicode-range of well-formed items", () => {
    for (const face of own) {
      const items = face.range
        .split(",")
        .map((r) => r.trim())
        .filter((r) => r !== "");
      expect({ src: face.src, empty: items.length === 0 }).toEqual({ src: face.src, empty: false });
      expect(items.filter((item) => !rangeItem.test(item))).toEqual([]);
    }
  });
});

test("no two faces name the same file", () => {
  const files = all.flatMap((f) => urls(f.src ?? ""));
  expect(files.length).toBe(new Set(files).size);
});

test("the fonts' license lies beside the WOFF2 (OFL 1.1)", () => {
  expect(readFileSync(new URL("./fonts/cjk/OFL-NotoSansCJK.txt", stylesheet), "utf8")).toContain(
    "SIL OPEN FONT LICENSE Version 1.1",
  );
});
