// SPDX-License-Identifier: AGPL-3.0-only

// What a post gives the media viewer of its images: the size the API says of each original, which the viewer holds
// the picture at before the original has come. happy-dom loads no image, and the originals here never decode.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Post } from "./post";
import { inLocale, status } from "./test/post";

const decodeWas = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, "decode");

beforeEach(() => {
  // Every original is still on its way.
  Object.defineProperty(HTMLImageElement.prototype, "decode", {
    configurable: true,
    writable: true,
    value: () => new Promise<void>(() => undefined),
  });
});

afterEach(() => {
  if (decodeWas === undefined) Reflect.deleteProperty(HTMLImageElement.prototype, "decode");
  else Object.defineProperty(HTMLImageElement.prototype, "decode", decodeWas);
});

/**
 * The size an image has of its own before any file of it has loaded, as `<width>x<height>`: its `width` and `height`,
 * or the size of the SVG it is drawn from.
 */
function intrinsic(img: HTMLImageElement): string | undefined {
  const width = img.getAttribute("width");
  const height = img.getAttribute("height");
  if (width !== null && height !== null) return `${width}x${height}`;
  const src = img.getAttribute("src") ?? "";
  if (!src.startsWith("data:image/svg+xml")) return undefined;
  const svg = decodeURIComponent(src.slice(src.indexOf(",") + 1));
  const w = /\swidth=["']?(\d+)/.exec(svg)?.[1];
  const h = /\sheight=["']?(\d+)/.exec(svg)?.[1];
  return w === undefined || h === undefined ? undefined : `${w}x${h}`;
}

/** The picture the open viewer's stage shows. */
async function viewed() {
  const dialog = await screen.findByRole("dialog", { name: "Media" });
  const stage = dialog.querySelector<HTMLElement>("[data-viewer-stage]");
  if (stage === null) throw new Error("test: the viewer shows no stage");
  return { dialog, img: within(stage).getByRole<HTMLImageElement>("presentation") };
}

// The sizes of the API, which the base's type of a post's media may not hold yet: a value, not a fresh literal.
const ridge = {
  id: "a",
  type: "image" as const,
  description: "A ridge",
  preview_url: "https://m.example/a-small.webp",
  url: "https://m.example/a.webp",
  meta: { width: 1600, height: 1200 },
};
const pine = {
  id: "b",
  type: "image" as const,
  description: "A pine",
  preview_url: "https://m.example/b-small.webp",
  url: "https://m.example/b.webp",
  meta: { width: 600, height: 900 },
};
const fog = {
  id: "c",
  type: "image" as const,
  description: "Fog",
  preview_url: "https://m.example/c-small.webp",
  url: "https://m.example/c.webp",
};

test("the viewer a post opens holds each image at the size the API gives of its original, with its preview in it", async () => {
  const user = userEvent.setup();
  const media = [ridge, pine];
  inLocale(<Post status={status({ media_attachments: media })} onAction={() => undefined} />);

  await user.click(screen.getByRole("button", { name: "Open image: A ridge" }));
  const first = await viewed();

  expect(intrinsic(first.img)).toBe("1600x1200");
  expect(first.img.style.backgroundImage).toContain("https://m.example/a-small.webp");
  await user.keyboard("{ArrowRight}");

  const second = await viewed();
  expect(intrinsic(second.img)).toBe("600x900");
  expect(second.img.style.backgroundImage).toContain("https://m.example/b-small.webp");
});

test("an image the API gives no size of opens as its preview", async () => {
  const user = userEvent.setup();
  const media = [fog];
  inLocale(<Post status={status({ media_attachments: media })} onAction={() => undefined} />);

  await user.click(screen.getByRole("button", { name: "Open image: Fog" }));

  expect((await viewed()).img.getAttribute("src")).toBe("https://m.example/c-small.webp");
});
