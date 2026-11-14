// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { markedSurface } from "./button";
import { compactCount, IconButton } from "./icon-button";

test("the name carries the count as shown, the description the exact one, and the visible count is hidden", () => {
  const html = renderToStaticMarkup(<IconButton icon="favourite" label="Favourite" count={12_840} locale="en" />);
  expect(html).toContain('aria-label="Favourite, 12.8K"');
  expect(html).toContain('aria-description="12,840"');
  expect(html).toContain('<span aria-hidden="true" lang="en" class="tabular-nums">12.8K</span>');
});

test("a count shown whole has no description", () => {
  const html = renderToStaticMarkup(<IconButton icon="reply" label="Reply" count={999} locale="en" />);
  expect(html).toContain('aria-label="Reply, 999"');
  expect(html).not.toContain("aria-description");
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

test("it reads the locale and the direction of the I18nProvider; a locale prop names the count's locale alone", () => {
  const under = (provided: string, locale?: string) =>
    renderToStaticMarkup(
      <I18nProvider locale={provided}>
        <IconButton icon="favourite" label="Favourite" count={12_840} locale={locale} />
      </I18nProvider>,
    );
  expect(under("ar-XB")).toMatch(/^<button [^>]*dir="rtl"/);
  expect(under("ar-XB")).toContain('lang="ar-XB"');
  expect(under("ja")).toMatch(/^<button [^>]*dir="ltr"/);
  expect(under("ja")).toContain('lang="ja" class="tabular-nums">1.3万</span>');
  expect(under("ar-XB", "en")).toMatch(/^<button [^>]*dir="rtl"/);
  expect(under("ar-XB", "en")).toContain('lang="en" class="tabular-nums">12.8K</span>');
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
