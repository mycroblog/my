// SPDX-License-Identifier: AGPL-3.0-only

// The self-hosted Noto Color Emoji (ADR-0091 §8–§10) and Noto Sans CJK (ADR-0009 §5) of the page: the WOFF2 of their
// splits and slices that the page fetched, the faces that emoji.css and cjk.css declare, and the fonts Chromium drew a
// piece of text with, which only the DevTools protocol tells (CSS.getPlatformFontsForNode): the computed font-family
// names the stack, not the font that drew the glyph.

import type { Page } from "@playwright/test";

/** The splits of ADR-0091 §9, as the WOFF2 of each is named. */
const splits = [
  "smileys-emotion",
  "people-body",
  "component",
  "animals-nature",
  "food-drink",
  "travel-places",
  "activities",
  "objects",
  "symbols",
  "flags",
  "country-flags",
];

/** A built split, assets/noto-emoji-<format>-<split>-<hash>.woff2; the name without the hash is group 1. */
const builtSplit = new RegExp(`^/assets/(noto-emoji-(?:colrv1|svg)-(?:${splits.join("|")}))-[\\w-]+\\.woff2$`);

/** The emoji fonts that a page fetches from the moment it is made. */
export class EmojiFonts {
  readonly page: Page;
  readonly #fetched: string[] = [];

  constructor(page: Page) {
    this.page = page;
    page.on("request", (request) => {
      const path = new URL(request.url()).pathname;
      if (!path.includes("noto-emoji")) return;
      // A split by its name without the hash ("noto-emoji-colrv1-food-drink.woff2"); anything else whole.
      const split = builtSplit.exec(path)?.[1];
      this.#fetched.push(split === undefined ? path : `${split}.woff2`);
    });
  }

  /** The emoji files fetched so far, in order; read it once `settled()` has returned. */
  get fetched(): readonly string[] {
    return [...this.#fetched];
  }

  /** Returns once the page has loaded every font it started to load (document.fonts.ready). */
  async settled(): Promise<void> {
    await this.page.evaluate(async () => {
      await document.fonts.ready;
    });
  }

  /** The faces of "my Noto Color Emoji" the page knows, from emoji.css, and how many of them it has loaded. */
  async faces(): Promise<{ declared: number; loaded: number }> {
    return this.page.evaluate(() => {
      const faces = [...document.fonts].filter((f) => f.family.replace(/^["']|["']$/g, "") === "my Noto Color Emoji");
      return { declared: faces.length, loaded: faces.filter((f) => f.status === "loaded").length };
    });
  }

  /**
   * The fonts Chromium drew the text nodes holding `text` with, also inside a text field: the family name inside each
   * font, and whether it is a web font (`isCustomFont`) or one of the system's.
   */
  async drawing(text: string): Promise<DrawnWith[]> {
    return drawing(this.page, text);
  }
}

/** A font that drew a piece of text: the family name inside the font, and whether it is a web font. */
export type DrawnWith = { familyName: string; isCustomFont: boolean };

/** The fonts Chromium drew the text nodes holding `text` with, also inside a text field. */
async function drawing(page: Page, text: string): Promise<DrawnWith[]> {
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
    const fonts = [];
    for (const nodeId of nodeIds) {
      const { fonts: used } = await cdp.send("CSS.getPlatformFontsForNode", { nodeId });
      fonts.push(...used.map(({ familyName, isCustomFont }) => ({ familyName, isCustomFont })));
    }
    return fonts;
  } finally {
    await cdp.detach();
  }
}

/** The self-hosted Noto Color Emoji as `EmojiFonts.drawing` names it: a web font whose own name is the Noto's. */
export const selfHostedNoto = { familyName: "Noto Color Emoji", isCustomFont: true } as const;

/**
 * Applies a theme's stylesheet over the page, as an administrator previews a draft: from a URL of the page's origin,
 * since the CSP allows no inline style.
 */
export async function applyTheme(page: Page, stylesheet: string): Promise<void> {
  const path = "/e2e-theme-preview.css";
  // The context's route, which also answers a request that the Service Worker passes on.
  await page.context().route(`**${path}`, (route) => route.fulfill({ contentType: "text/css", body: stylesheet }));
  await page.addStyleTag({ url: path });
}

/** The self-hosted Noto Sans (ADR-0009 §5) as `CjkFonts.drawing` names it: a web font whose own name is the Noto's. */
export const selfHostedNotoSans = { familyName: "Noto Sans", isCustomFont: true } as const;

/** The regions of Noto Sans CJK (ADR-0009 §5), as cjk.css names its files. */
export type CjkRegion = "jp" | "sc" | "tc" | "kr";

/** The self-hosted Noto Sans CJK of a region as `CjkFonts.drawing` names it: a web font whose own name is the Noto's. */
export const selfHostedCjk = (region: CjkRegion) =>
  ({ familyName: `Noto Sans CJK ${region.toUpperCase()}`, isCustomFont: true }) as const;

/** A built slice, assets/noto-sans-cjk-<region>-<slice>-<hash>.woff2: the region is group 1, the slice group 2. */
const builtCjkSlice = /^\/assets\/noto-sans-cjk-(jp|sc|tc|kr)-([a-z0-9-]+)-[\w-]{8}\.woff2$/;

/** A slice of Noto Sans CJK: its region, its name in cjk.css ("han-common-03"), and its built file's path. */
export type CjkSlice = { region: CjkRegion; slice: string; path: string };

/** A face of Noto Sans CJK that the page's stylesheets declare: its built file and its code points. */
type CjkFace = { path: string; ranges: [number, number][] };

/** One item of a unicode-range as the CSSOM serializes it: U+4E00, U+4E00-9FFF or U+4E??. */
function range(item: string): [number, number] {
  const m = /^U\+([0-9A-F?]+)(?:-([0-9A-F]+))?$/i.exec(item.trim());
  if (m === null) throw new Error(`not a unicode-range item: ${item}`);
  const [, from = "", to] = m;
  if (from.includes("?")) return [parseInt(from.replaceAll("?", "0"), 16), parseInt(from.replaceAll("?", "F"), 16)];
  return [parseInt(from, 16), parseInt(to ?? from, 16)];
}

/** The self-hosted Noto Sans CJK of the page (ADR-0009 §5): the slices it fetched, and the faces that claim them. */
export class CjkFonts {
  readonly page: Page;
  readonly #fetched: CjkSlice[] = [];

  constructor(page: Page) {
    this.page = page;
    page.on("request", (request) => {
      const path = new URL(request.url()).pathname;
      if (!path.includes("noto-sans-cjk")) return;
      const [, region, slice] = builtCjkSlice.exec(path) ?? [];
      // A file of no slice name fails every assertion on the slices, as it should.
      this.#fetched.push({ region: (region ?? "unknown") as CjkRegion, slice: slice ?? path, path });
    });
  }

  /** The slices fetched so far, in order; read it once `settled()` has returned. */
  get fetched(): readonly CjkSlice[] {
    return [...this.#fetched];
  }

  /** Returns once the page has loaded every font it started to load (document.fonts.ready). */
  async settled(): Promise<void> {
    await this.page.evaluate(async () => {
      await document.fonts.ready;
    });
  }

  /** The faces of "my Noto Sans CJK …" in the page's stylesheets, each with its built file and its unicode-range. */
  async #faces(): Promise<CjkFace[]> {
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
    return faces.map(({ src, base, range: ranges }) => {
      const url = /url\(\s*["']?([^"')]+)["']?\s*\)/.exec(src)?.[1] ?? "";
      return {
        path: new URL(url, base).pathname,
        ranges: ranges
          .split(",")
          .filter((r) => r.trim() !== "")
          .map(range),
      };
    });
  }

  /**
   * The slices fetched that claim none of the characters the page shows (its rendered text, and the placeholders of
   * its fields): a page loads only the slices of the characters it shows.
   */
  async unclaimed(): Promise<readonly CjkSlice[]> {
    const shown = await this.page.evaluate(() =>
      [
        document.body.innerText,
        ...[...document.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>("[placeholder]")].map(
          (f) => f.placeholder,
        ),
      ].join(""),
    );
    // Code points, each of a sequence on its own: a unicode-range claims code points, not sequences.
    const points = new Set<number>();
    for (let i = 0; i < shown.length;) {
      const cp = shown.codePointAt(i) ?? 0;
      points.add(cp);
      i += cp > 0xffff ? 2 : 1;
    }
    const faces = await this.#faces();
    return this.fetched.filter((s) => {
      const face = faces.find((f) => f.path === s.path);
      return !(face?.ranges.some(([from, to]) => [...points].some((cp) => from <= cp && cp <= to)) ?? false);
    });
  }

  /** The slice of `region` whose face claims `character` and the browser checks first (the last declared). */
  async sliceOf(region: CjkRegion, character: string): Promise<string | undefined> {
    const cp = character.codePointAt(0) ?? 0;
    const faces = (await this.#faces()).filter((f) => f.path.includes(`/noto-sans-cjk-${region}-`));
    const face = faces.findLast((f) => f.ranges.some(([from, to]) => from <= cp && cp <= to));
    return face === undefined ? undefined : builtCjkSlice.exec(face.path)?.[2];
  }

  /** The fonts Chromium drew the text nodes holding `text` with (as `EmojiFonts.drawing`). */
  async drawing(text: string): Promise<DrawnWith[]> {
    return drawing(this.page, text);
  }
}
