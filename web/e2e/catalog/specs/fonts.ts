// SPDX-License-Identifier: AGPL-3.0-only

// The self-hosted fonts of a page (ADR-0009 §5, ADR-0091 §8–§10): the font files it fetched, the faces its
// stylesheets declare, and the fonts Chromium drew a piece of text with, which only the DevTools protocol tells
// (CSS.getPlatformFontsForNode): the computed font-family names the stack, not the font that drew the glyph.

import type { Page } from "@playwright/test";

/** A font that drew a piece of text: the family name inside the font, and whether it is a web font. */
export type DrawnWith = { familyName: string; isCustomFont: boolean };

/**
 * The fonts Chromium drew the text nodes holding `text` with, also inside a text field, each once, in a stable order:
 * the family name inside each font, and whether it is a web font (`isCustomFont`) or one of the system's.
 */
export async function drawing(page: Page, text: string): Promise<DrawnWith[]> {
  const cdp = await page.context().newCDPSession(page);
  try {
    await cdp.send("DOM.enable");
    await cdp.send("CSS.enable");
    await cdp.send("DOM.getDocument", { depth: -1, pierce: true });
    const { searchId, resultCount } = await cdp.send("DOM.performSearch", {
      query: text,
      includeUserAgentShadowDOM: true,
    });
    if (resultCount === 0) return [];
    const { nodeIds } = await cdp.send("DOM.getSearchResults", { searchId, fromIndex: 0, toIndex: resultCount });
    const fonts = new Map<string, DrawnWith>();
    for (const nodeId of nodeIds) {
      const { fonts: used } = await cdp.send("CSS.getPlatformFontsForNode", { nodeId });
      for (const { familyName, isCustomFont } of used)
        fonts.set(`${familyName}|${String(isCustomFont)}`, { familyName, isCustomFont });
    }
    return [...fonts.values()].toSorted((a, b) => a.familyName.localeCompare(b.familyName));
  } finally {
    await cdp.detach();
  }
}

/** The self-hosted Noto Color Emoji as `drawing` names it: a web font whose own name is the Noto's. */
export const notoEmoji: DrawnWith = { familyName: "Noto Color Emoji", isCustomFont: true };

/** The self-hosted Noto Sans as `drawing` names it. */
export const notoSans: DrawnWith = { familyName: "Noto Sans", isCustomFont: true };

/** The regions of Noto Sans CJK, as cjk.css names its files. */
export type CjkRegion = "jp" | "sc" | "tc" | "kr";

/** The self-hosted Noto Sans CJK of a region as `drawing` names it. */
export const notoCjk = (region: CjkRegion): DrawnWith => ({
  familyName: `Noto Sans CJK ${region.toUpperCase()}`,
  isCustomFont: true,
});

/** A font file of the build: assets/<name>-<hash>.woff2; the name is group 1. */
const built = /^\/assets\/(noto-[a-z0-9-]+)-[\w-]{8}\.woff2$/;

/** A slice of Noto Sans CJK by its file's name: the region is group 1, the slice group 2. */
const cjkSlice = /^noto-sans-cjk-(jp|sc|tc|kr)-([a-z0-9-]+)$/;

/** A slice of Noto Sans CJK: its region, its name in cjk.css ("han-common-03"), and its built file's path. */
export type CjkSlice = { region: CjkRegion; slice: string; path: string };

/** One item of a unicode-range as the CSSOM serializes it: U+4E00, U+4E00-9FFF or U+4E??. */
function range(item: string): [number, number] {
  const m = /^U\+([0-9A-F?]+)(?:-([0-9A-F]+))?$/i.exec(item.trim());
  if (m === null) throw new Error(`not a unicode-range item: ${item}`);
  const [, from = "", to] = m;
  if (from.includes("?")) return [parseInt(from.replaceAll("?", "0"), 16), parseInt(from.replaceAll("?", "F"), 16)];
  return [parseInt(from, 16), parseInt(to ?? from, 16)];
}

/** The font files of the self-hosted Noto fonts that a page fetches from the moment this is made. */
export class FontFiles {
  readonly page: Page;
  readonly #fetched: { name: string; path: string }[] = [];

  constructor(page: Page) {
    this.page = page;
    page.on("request", (request) => {
      const path = new URL(request.url()).pathname;
      if (!path.endsWith(".woff2") || !path.includes("/noto-")) return;
      // A file of no built name fails every assertion on the names, as it should.
      this.#fetched.push({ name: built.exec(path)?.[1] ?? path, path });
    });
  }

  /** Returns once the page has loaded every font it started to load (document.fonts.ready). */
  async settled(): Promise<void> {
    await this.page.evaluate(async () => {
      await document.fonts.ready;
    });
  }

  /** The emoji files fetched so far, by their names without the hash ("noto-emoji-colrv1-food-drink"). */
  get emoji(): readonly string[] {
    return this.#fetched.map((f) => f.name).filter((name) => name.startsWith("noto-emoji-"));
  }

  /** The slices of Noto Sans CJK fetched so far. */
  get cjk(): readonly CjkSlice[] {
    return this.#fetched.flatMap(({ name, path }) => {
      const [, region, slice] = cjkSlice.exec(name) ?? [];
      return region === undefined || slice === undefined ? [] : [{ region: region as CjkRegion, slice, path }];
    });
  }

  /** The faces of "my Noto Color Emoji" the page knows, from emoji.css, and how many of them it has loaded. */
  async emojiFaces(): Promise<{ declared: number; loaded: number }> {
    return this.page.evaluate(() => {
      const faces = [...document.fonts].filter((f) => f.family.replace(/^["']|["']$/g, "") === "my Noto Color Emoji");
      return { declared: faces.length, loaded: faces.filter((f) => f.status === "loaded").length };
    });
  }

  /** The faces of "my Noto Sans CJK …" in the page's stylesheets, each with its built file and its unicode-range. */
  async #cjkFaces(): Promise<{ path: string; ranges: [number, number][] }[]> {
    const faces = await this.page.evaluate(() => {
      const found: { src: string; base: string; range: string }[] = [];
      const visit = (rules: CSSRuleList, base: string) => {
        for (const rule of rules) {
          if (rule instanceof CSSFontFaceRule) {
            const family = rule.style.getPropertyValue("font-family").replace(/^["']|["']$/g, "");
            if (family.startsWith("my Noto Sans CJK "))
              found.push({
                src: rule.style.getPropertyValue("src"),
                base,
                range: rule.style.getPropertyValue("unicode-range"),
              });
          } else if ("cssRules" in rule) visit((rule as CSSGroupingRule).cssRules, base);
        }
      };
      // A url() of a stylesheet is relative to the stylesheet.
      for (const sheet of document.styleSheets) visit(sheet.cssRules, sheet.href ?? document.baseURI);
      return found;
    });
    return faces.map(({ src, base, range: ranges }) => ({
      path: new URL(/url\(\s*["']?([^"')]+)["']?\s*\)/.exec(src)?.[1] ?? "", base).pathname,
      ranges: ranges
        .split(",")
        .filter((r) => r.trim() !== "")
        .map(range),
    }));
  }

  /**
   * The slices of Noto Sans CJK fetched that claim none of the characters the page shows: a page loads only the
   * slices of the characters it shows.
   */
  async unclaimed(): Promise<readonly CjkSlice[]> {
    const shown = await this.page.evaluate(() => document.body.innerText);
    // Code points, each of a sequence on its own: a unicode-range claims code points, not sequences.
    const points = new Set<number>();
    for (const character of shown) points.add(character.codePointAt(0) ?? 0);
    const faces = await this.#cjkFaces();
    return this.cjk.filter((s) => {
      const face = faces.find((f) => f.path === s.path);
      return !(face?.ranges.some(([from, to]) => [...points].some((cp) => from <= cp && cp <= to)) ?? false);
    });
  }

  /** The slice of `region` whose face claims `character` and the browser checks first (the last declared). */
  async sliceOf(region: CjkRegion, character: string): Promise<string | undefined> {
    const cp = character.codePointAt(0) ?? 0;
    const faces = (await this.#cjkFaces()).filter((f) => f.path.includes(`/noto-sans-cjk-${region}-`));
    const face = faces.findLast((f) => f.ranges.some(([from, to]) => from <= cp && cp <= to));
    return face === undefined ? undefined : cjkSlice.exec(built.exec(face.path)?.[1] ?? "")?.[2];
  }
}
