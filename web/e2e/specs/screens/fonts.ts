// SPDX-License-Identifier: AGPL-3.0-only

// The self-hosted Noto Color Emoji of the page (ADR-0091 §8–§10): the WOFF2 of its splits that the page fetched, the
// faces that emoji.css declares, and the fonts Chromium drew a piece of text with, which only the DevTools protocol
// tells (CSS.getPlatformFontsForNode): the computed font-family names the stack, not the font that drew the glyph.

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
  async drawing(text: string): Promise<{ familyName: string; isCustomFont: boolean }[]> {
    const cdp = await this.page.context().newCDPSession(this.page);
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
}

/** The self-hosted Noto Color Emoji as `EmojiFonts.drawing` names it: a web font whose own name is the Noto's. */
export const selfHostedNoto = { familyName: "Noto Color Emoji", isCustomFont: true } as const;
