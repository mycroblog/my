// SPDX-License-Identifier: AGPL-3.0-only

// Taking a screenshot of a page that has come to rest: its fonts and the pictures in view loaded, two shots in a row
// the same, the PNG encoded as WebP by the browser itself, so that the images need no other tool.

import type { Page } from "@playwright/test";

/** The quality of the WebP encoding, 0 to 1: the screens are flat colours and text, which stay sharp at 0.85. */
const quality = 0.85;

/** Waits until the fonts and every picture in the viewport have loaded; the pictures below it may load lazily. */
export async function picturesDrawn(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  await page.waitForFunction(
    () =>
      [...document.images]
        .filter((image) => image.checkVisibility() && image.getBoundingClientRect().top < window.innerHeight)
        .every((image) => image.complete && image.naturalWidth > 0),
    undefined,
    { timeout: 30_000 },
  );
}

/**
 * A screenshot of `page` once two in a row, a quarter of a second apart, are the same: a picture that the page swaps
 * for a larger one, or decodes late, has settled.
 */
export async function stillShot(page: Page): Promise<Buffer> {
  let last = await page.screenshot({ animations: "disabled", caret: "hide" });
  for (let attempt = 0; attempt < 20; attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 250));
    const next = await page.screenshot({ animations: "disabled", caret: "hide" });
    if (next.equals(last)) return next;
    last = next;
  }
  throw new Error("shots: the page never came to rest");
}

/** Encodes a PNG as WebP in a blank page of the browser, which no Content Security Policy keeps from the image. */
export async function toWebP(page: Page, png: Buffer): Promise<Buffer> {
  const blank = await page.context().newPage();
  try {
    const dataURL = await blank.evaluate(
      async ({ base64, quality }) => {
        const image = new Image();
        image.src = `data:image/png;base64,${base64}`;
        await image.decode();
        const canvas = document.createElement("canvas");
        canvas.width = image.naturalWidth;
        canvas.height = image.naturalHeight;
        canvas.getContext("2d")?.drawImage(image, 0, 0);
        return canvas.toDataURL("image/webp", quality);
      },
      { base64: png.toString("base64"), quality },
    );
    if (!dataURL.startsWith("data:image/webp;base64,")) throw new Error("shots: the browser did not encode WebP");
    return Buffer.from(dataURL.slice(dataURL.indexOf(",") + 1), "base64");
  } finally {
    await blank.close();
  }
}
