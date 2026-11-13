// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { MediaGrid, type MediaItem } from "./media";

const items = (n: number): MediaItem[] =>
  Array.from({ length: n }, (_, i) => ({ id: `m${i}`, src: `https://m.example/${i}.webp`, alt: `Image ${i}` }));

test("no images draw nothing", () => {
  expect(renderToStaticMarkup(<MediaGrid items={[]} />)).toBe("");
});

test("one image alone, two side by side, three or four in two rows, in a box 16:9 of its width up to a height", () => {
  const grid = (n: number) => renderToStaticMarkup(<MediaGrid items={items(n)} />);
  expect(grid(1)).toContain("grid-cols-[minmax(0,1fr)] grid-rows-[minmax(0,1fr)]");
  expect(grid(2)).toContain("grid-cols-[minmax(0,1fr)_minmax(0,1fr)] grid-rows-[minmax(0,1fr)]");
  expect(grid(4)).toContain("grid-cols-[minmax(0,1fr)_minmax(0,1fr)] grid-rows-[minmax(0,1fr)_minmax(0,1fr)]");
  expect(grid(1)).toContain("h-[min(calc(100cqw*var(--media-ratio)),var(--media-max-height))]");
  expect(grid(1)).toContain('<div class="@container"><div class="grid');
});

test("each image has its description as its text, and no border", () => {
  const html = renderToStaticMarkup(<MediaGrid items={items(2)} />);
  expect(html).toContain('src="https://m.example/1.webp" alt="Image 1"');
  expect(html).not.toContain("border");
});

test("as a preview, each image is a small tile in a row", () => {
  const html = renderToStaticMarkup(<MediaGrid items={items(2)} size="preview" />);
  expect(html).toContain('<div class="flex flex-wrap gap-media"><img');
  expect(html).toContain("h-media-preview-height w-media-preview-width");
  expect(html).not.toContain("@container");
});
