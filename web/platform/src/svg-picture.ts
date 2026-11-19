// SPDX-License-Identifier: AGPL-3.0-only

// Drawing an SVG as PNG pictures of given sizes, in the browser (ADR-0059 §10): the server sanitizes a brand image's
// SVG and draws nothing, and the administrator's browser draws the PNGs of the places that take no SVG (favicons, home
// screens, emails, link previews) with the same renderer that shows the SVG. The SVG is read as an image from a `blob:`
// URL, never put into the page, and each picture is drawn on a canvas.

/** How one picture is drawn: its size, the color under it (none leaves it transparent), and the share it fills. */
export type SvgPictureSpec = {
  readonly width: number;
  readonly height: number;
  /** The `#rrggbb` color under the SVG, for a picture that must be opaque. */
  readonly background?: string | undefined;
  /** The share of the width and of the height the SVG may fill, centered (1 by default): `[0.8, 0.5]`. */
  readonly fit?: readonly [number, number] | undefined;
};

/** Reads `svg` as an image, through a `blob:` URL that it lets go once read. */
async function imageOf(svg: string): Promise<HTMLImageElement> {
  const url = URL.createObjectURL(new Blob([svg], { type: "image/svg+xml" }));
  try {
    const image = new Image();
    image.decoding = "async";
    image.src = url;
    await image.decode();
    return image;
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** A canvas's picture as a PNG. */
const pngOf = (canvas: HTMLCanvasElement): Promise<Blob> =>
  new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob === null) reject(new Error("platform: the browser drew no picture"));
      else resolve(blob);
    }, "image/png");
  });

/**
 * Draws the SVG `svg`, whose viewBox is `viewBox` wide and high, as a PNG of each of `specs`, in their order: the SVG
 * as large as it fits in its share of the picture, centered, its proportions kept. Rejects where the browser cannot
 * read the SVG or draw a picture.
 */
export async function drawSvgPictures(
  svg: string,
  viewBox: { readonly width: number; readonly height: number },
  specs: readonly SvgPictureSpec[],
): Promise<Blob[]> {
  const image = await imageOf(svg);
  const pictures: Blob[] = [];
  for (const spec of specs) {
    const canvas = document.createElement("canvas");
    canvas.width = spec.width;
    canvas.height = spec.height;
    const context = canvas.getContext("2d");
    if (context === null) throw new Error("platform: the browser gives no canvas to draw on");
    if (spec.background !== undefined) {
      context.fillStyle = spec.background;
      context.fillRect(0, 0, spec.width, spec.height);
    }
    const [fw, fh] = spec.fit ?? [1, 1];
    const scale = Math.min((spec.width * fw) / viewBox.width, (spec.height * fh) / viewBox.height);
    const w = viewBox.width * scale;
    const h = viewBox.height * scale;
    context.drawImage(image, (spec.width - w) / 2, (spec.height - h) / 2, w, h);
    pictures.push(await pngOf(canvas));
  }
  return pictures;
}
