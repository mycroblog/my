// SPDX-License-Identifier: AGPL-3.0-only
// The PNG renditions of a brand image, drawn in the administrator's browser (ADR-0116 §3): the sanitized SVG is loaded
// into an <img> from a blob: URL (the shell's CSP allows img-src blob:), drawn on a <canvas> at each size the server
// asked for, and encoded as PNG. The browser draws the SVG as faithfully as it draws it for the viewers (favicons, the
// home screen), so the PNGs show the same picture; the SVG is never put into the page itself. The layout of each
// rendition is a pure function (planRendition), tested apart from the canvas.
import type { BrandRendition, BrandRenditionSize } from "../api/brand";

/** A rectangle on the canvas, in pixels. */
export type Box = { x: number; y: number; width: number; height: number };

/** How to draw one rendition: the canvas size, whether its background is filled, and where the picture goes. */
export type RenditionPlan = {
  rendition: BrandRendition;
  width: number;
  height: number;
  /** Whether the canvas is filled with the background first: the renditions that must be opaque. */
  opaque: boolean;
  /** Where the SVG is drawn, keeping its proportions (contain, centered). */
  box: Box;
};

/** The renditions that must have no transparent pixel: the iOS home screen, the maskable icon, the link preview. */
const opaqueRenditions: ReadonlySet<BrandRendition> = new Set(["apple-touch-180", "maskable-512", "preview-1200x630"]);

/** The share of the maskable icon that the mark fills: the safe zone of a maskable icon is the middle 80%. */
const maskableSafeZone = 0.8;

/** The share of the link preview's width and height that the logo may fill, so that it has a margin around it. */
const previewArea = { width: 0.8, height: 0.5 } as const;

/** The largest box of the proportions `aspect` (width / height) inside `area`, centered in it. */
export function containBox(area: Box, aspect: number): Box {
  const width = Math.min(area.width, area.height * aspect);
  const height = width / aspect;
  return { x: area.x + (area.width - width) / 2, y: area.y + (area.height - height) / 2, width, height };
}

/** Plans one rendition of an SVG whose viewBox is `viewBox`, at the size the server gave. */
export function planRendition(size: BrandRenditionSize, viewBox: { width: number; height: number }): RenditionPlan {
  const { rendition, width, height } = size;
  const aspect = viewBox.width / viewBox.height;
  let area: Box = { x: 0, y: 0, width, height };
  if (rendition === "maskable-512") {
    const margin = (1 - maskableSafeZone) / 2;
    area = {
      x: width * margin,
      y: height * margin,
      width: width * maskableSafeZone,
      height: height * maskableSafeZone,
    };
  } else if (rendition === "preview-1200x630") {
    const areaWidth = width * previewArea.width;
    const areaHeight = height * previewArea.height;
    area = { x: (width - areaWidth) / 2, y: (height - areaHeight) / 2, width: areaWidth, height: areaHeight };
  }
  return { rendition, width, height, opaque: opaqueRenditions.has(rendition), box: containBox(area, aspect) };
}

/** The part of a 2D context that drawing a rendition uses. */
export type DrawingContext = Pick<CanvasRenderingContext2D, "fillStyle" | "fillRect" | "drawImage">;

/** Draws `image` (the SVG, sized to the plan's box) on a context of the plan's size. */
export function drawRendition(
  context: DrawingContext,
  image: CanvasImageSource,
  plan: RenditionPlan,
  background: string,
): void {
  if (plan.opaque) {
    context.fillStyle = background;
    context.fillRect(0, 0, plan.width, plan.height);
  }
  context.drawImage(image, plan.box.x, plan.box.y, plan.box.width, plan.box.height);
}

/**
 * The SVG with the size of its root element set to `width` × `height`, so that every browser draws it at that size on
 * a canvas (an SVG without a size has none of its own to draw, and Firefox refuses to draw it). The viewBox and
 * preserveAspectRatio keep the picture in proportion.
 */
export function sizedSvg(svg: string, width: number, height: number): string {
  return svg.replace(/<svg\b[^>]*>/u, (tag) => {
    const bare = tag.replace(/\s(?:width|height)\s*=\s*(?:"[^"]*"|'[^']*')/gu, "");
    return bare.replace(/^<svg\b/u, `<svg width="${width}" height="${height}"`);
  });
}

/** A drawn rendition: its PNG and a blob: URL of it for the previews. */
export type DrawnRendition = { rendition: BrandRendition; width: number; height: number; png: Blob; url: string };

/** Why the browser could not draw the SVG or encode a PNG. */
export class RasterizeError extends Error {
  override name = "RasterizeError";
}

/** Loads `svg` into an image from a blob: URL. */
async function loadSvg(svg: string): Promise<HTMLImageElement> {
  const url = URL.createObjectURL(new Blob([svg], { type: "image/svg+xml" }));
  try {
    const image = new Image();
    image.src = url;
    await image.decode();
    return image;
  } catch (error) {
    throw new RasterizeError("the browser could not load the SVG", { cause: error });
  } finally {
    URL.revokeObjectURL(url);
  }
}

/** Draws one rendition on a new canvas and encodes it as PNG. */
async function drawOne(svg: string, plan: RenditionPlan, background: string): Promise<DrawnRendition> {
  const image = await loadSvg(sizedSvg(svg, plan.box.width, plan.box.height));
  const canvas = document.createElement("canvas");
  canvas.width = plan.width;
  canvas.height = plan.height;
  const context = canvas.getContext("2d");
  if (context === null) throw new RasterizeError("the browser has no 2D canvas");
  drawRendition(context, image, plan, background);
  const png = await new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, "image/png");
  });
  if (png === null) throw new RasterizeError(`the browser could not encode ${plan.rendition}`);
  return { rendition: plan.rendition, width: plan.width, height: plan.height, png, url: URL.createObjectURL(png) };
}

/**
 * Draws every rendition the server asked for (`renditions` of the sanitize answer) from the sanitized SVG. The caller
 * revokes the URLs of the result when it no longer shows them (revokeRenditions).
 */
export async function drawRenditions(
  svg: string,
  viewBox: { width: number; height: number },
  renditions: readonly BrandRenditionSize[],
  background: string,
): Promise<DrawnRendition[]> {
  const drawn: DrawnRendition[] = [];
  try {
    for (const size of renditions) drawn.push(await drawOne(svg, planRendition(size, viewBox), background));
  } catch (error) {
    revokeRenditions(drawn);
    throw error;
  }
  return drawn;
}

/** Releases the blob: URLs of drawn renditions. */
export function revokeRenditions(drawn: readonly DrawnRendition[]): void {
  for (const rendition of drawn) URL.revokeObjectURL(rendition.url);
}

/** A `#rrggbb` color, as the server takes the background of an icon. */
export const hexColor = /^#[0-9a-f]{6}$/u;

/**
 * The theme's light surface color as `#rrggbb`: the default background of the icon's opaque renditions, as the server
 * uses when none is sent. A canvas turns any CSS color into its pixels; without one (tests), white.
 */
export function lightSurfaceColor(): string {
  const fallback = "#ffffff";
  const probe = document.createElement("span");
  probe.style.colorScheme = "light";
  probe.style.color = "var(--color-surface)";
  document.body.append(probe);
  const color = getComputedStyle(probe).color;
  probe.remove();
  const canvas = document.createElement("canvas");
  canvas.width = 1;
  canvas.height = 1;
  let context: CanvasRenderingContext2D | null = null;
  try {
    context = canvas.getContext("2d");
  } catch {
    return fallback;
  }
  if (context === null || color === "") return fallback;
  context.fillStyle = color;
  context.fillRect(0, 0, 1, 1);
  const [r = 255, g = 255, b = 255] = context.getImageData(0, 0, 1, 1).data;
  return `#${[r, g, b].map((channel) => channel.toString(16).padStart(2, "0")).join("")}`;
}
