// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { markedSurface } from "./button";
import { compactCount, IconButton } from "./icon-button";

test("the label carries the exact count, and the visible compact count is hidden", () => {
  const html = renderToStaticMarkup(<IconButton icon="favourite" label="Favourite" count={12_840} locale="en" />);
  expect(html).toContain('aria-label="Favourite, 12,840"');
  expect(html).toContain('<span aria-hidden="true" lang="en" class="tabular-nums">12.8K</span>');
});

test("zero shows no count", () => {
  const html = renderToStaticMarkup(<IconButton icon="reply" label="Reply" count={0} locale="en" />);
  expect(html).toContain('aria-label="Reply"');
  expect(html).not.toContain("tabular-nums");
});

test("a toggle says whether it is on, and an action that is no toggle says nothing", () => {
  expect(renderToStaticMarkup(<IconButton icon="boost" label="Boost" pressed />)).toContain('aria-pressed="true"');
  expect(renderToStaticMarkup(<IconButton icon="boost" label="Boost" pressed={false} />)).toContain(
    'aria-pressed="false"',
  );
  expect(renderToStaticMarkup(<IconButton icon="more" label="More" />)).not.toContain("aria-pressed=");
});

test("the count is compact in the reader's locale", () => {
  expect(compactCount(1_234, "en")).toBe("1.2K");
  expect(compactCount(12_840, "ja")).toBe("1.3万");
  expect(compactCount(999, "en")).toBe("999");
});

test("a marked surface carries the attribute its icon buttons answer, and an unmarked one none", () => {
  expect(markedSurface(true)).toEqual({ "data-surface-marked": "" });
  expect(markedSurface(false)).toEqual({ "data-surface-marked": undefined });
  const html = renderToStaticMarkup(<IconButton icon="reply" label="Reply" />);
  expect(html).toContain("[[data-surface-marked]_&amp;]:data-hovered:bg-surface-pressed");
});
