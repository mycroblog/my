// SPDX-License-Identifier: AGPL-3.0-only
// The layout of the PNG renditions of a brand image (ADR-0116 §3): each canvas has the size the server asked for, the
// opaque ones are filled with the background first, the maskable icon keeps the mark in the middle 80%, and a logo
// keeps its proportions, centered, in the link preview and the mail header.
import { describe, expect, test, vi } from "vitest";
import type { BrandRenditionSize } from "../api/brand";
import { containBox, type DrawingContext, drawRendition, planRendition, sizedSvg } from "./brandRenditions";

const square = { width: 64, height: 64 };
const wide = { width: 160, height: 40 };

describe("planRendition", () => {
  test.each<[BrandRenditionSize, boolean]>([
    [{ rendition: "favicon-32", width: 32, height: 32 }, false],
    [{ rendition: "favicon-48", width: 48, height: 48 }, false],
    [{ rendition: "apple-touch-180", width: 180, height: 180 }, true],
    [{ rendition: "pwa-192", width: 192, height: 192 }, false],
    [{ rendition: "pwa-512", width: 512, height: 512 }, false],
  ])("draws the icon over the whole canvas of %o (opaque: %s)", (size, opaque) => {
    expect(planRendition(size, square)).toEqual({
      ...size,
      opaque,
      box: { x: 0, y: 0, width: size.width, height: size.height },
    });
  });

  test("keeps the maskable icon's mark in the middle 80%, on an opaque canvas", () => {
    const plan = planRendition({ rendition: "maskable-512", width: 512, height: 512 }, square);
    expect(plan.opaque).toBe(true);
    expect(plan.box.x).toBeCloseTo(51.2);
    expect(plan.box.y).toBeCloseTo(51.2);
    expect(plan.box.width).toBeCloseTo(409.6);
    expect(plan.box.height).toBeCloseTo(409.6);
  });

  test("fills the mail header's canvas with a logo of the same proportions, transparent", () => {
    expect(planRendition({ rendition: "logo-mail", width: 384, height: 96 }, wide)).toEqual({
      rendition: "logo-mail",
      width: 384,
      height: 96,
      opaque: false,
      box: { x: 0, y: 0, width: 384, height: 96 },
    });
    expect(planRendition({ rendition: "logo-dark-mail", width: 384, height: 96 }, wide).opaque).toBe(false);
  });

  test("centers the logo in the link preview, with a margin, on an opaque canvas", () => {
    const plan = planRendition({ rendition: "preview-1200x630", width: 1200, height: 630 }, wide);
    expect(plan).toMatchObject({ width: 1200, height: 630, opaque: true });
    // 80% of the width (960) by half of the height (315): a 4:1 logo is as wide as the area allows.
    expect(plan.box.width).toBeCloseTo(960);
    expect(plan.box.height).toBeCloseTo(240);
    expect(plan.box.x).toBeCloseTo(120);
    expect(plan.box.y).toBeCloseTo(195);
  });

  test("lets a nearly square logo be as high as the preview's area", () => {
    const plan = planRendition({ rendition: "preview-1200x630", width: 1200, height: 630 }, { width: 40, height: 40 });
    expect(plan.box.height).toBeCloseTo(315);
    expect(plan.box.width).toBeCloseTo(315);
    expect(plan.box.x).toBeCloseTo((1200 - 315) / 2);
  });
});

describe("containBox", () => {
  test("fits the proportions inside the area, centered", () => {
    expect(containBox({ x: 10, y: 0, width: 100, height: 100 }, 2)).toEqual({ x: 10, y: 25, width: 100, height: 50 });
    expect(containBox({ x: 0, y: 0, width: 100, height: 50 }, 1)).toEqual({ x: 25, y: 0, width: 50, height: 50 });
  });
});

/** A context that records what is drawn on it. */
function recordingContext() {
  const calls: string[] = [];
  const context = {
    fillStyle: "",
    fillRect: vi.fn((x: number, y: number, width: number, height: number) => {
      calls.push(`fill ${context.fillStyle} ${x},${y} ${width}x${height}`);
    }),
    drawImage: vi.fn((_image: CanvasImageSource, x: number, y: number, width: number, height: number) => {
      calls.push(`draw ${x},${y} ${width}x${height}`);
    }),
  };
  return { context: context as unknown as DrawingContext, calls };
}

describe("drawRendition", () => {
  const image = {} as CanvasImageSource;

  test("fills an opaque rendition with the background before drawing the picture", () => {
    const { context, calls } = recordingContext();
    drawRendition(
      context,
      image,
      planRendition({ rendition: "apple-touch-180", width: 180, height: 180 }, square),
      "#fffaf2",
    );
    expect(calls).toEqual(["fill #fffaf2 0,0 180x180", "draw 0,0 180x180"]);
  });

  test("leaves a transparent rendition transparent", () => {
    const { context, calls } = recordingContext();
    drawRendition(context, image, planRendition({ rendition: "favicon-32", width: 32, height: 32 }, square), "#fffaf2");
    expect(calls).toEqual(["draw 0,0 32x32"]);
  });

  test("draws the maskable icon inside its safe zone over the background", () => {
    const { context, calls } = recordingContext();
    drawRendition(
      context,
      image,
      planRendition({ rendition: "maskable-512", width: 512, height: 512 }, square),
      "#000000",
    );
    expect(calls[0]).toBe("fill #000000 0,0 512x512");
    expect(calls[1]).toMatch(/^draw 51\.\d+,51\.\d+ 409\.\d+x409\.\d+$/u);
  });
});

describe("sizedSvg", () => {
  test("gives the root element the size to draw at", () => {
    expect(sizedSvg('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 1"><g/></svg>', 384, 96)).toBe(
      '<svg width="384" height="96" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 1"><g/></svg>',
    );
  });

  test("replaces a size of the root, and leaves the sizes of the shapes", () => {
    expect(sizedSvg(`<svg width="100%" height='10' viewBox="0 0 1 1"><rect width="1" height="1"/></svg>`, 32, 32)).toBe(
      '<svg width="32" height="32" viewBox="0 0 1 1"><rect width="1" height="1"/></svg>',
    );
  });
});
