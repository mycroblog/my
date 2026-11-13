// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { TextButton } from "./button";
import { Chip, ChipRow } from "./chip";

test("a chip is a toggle named by its label, its glyph and count hidden from assistive technology", () => {
  const html = renderToStaticMarkup(<Chip label="🎉, 5" glyph="🎉" count={5} pressed={false} locale="en" />);
  expect(html).toContain('aria-pressed="false"');
  expect(html).toContain('aria-label="🎉, 5"');
  expect(html).toMatch(/<span aria-hidden="true" class="[^"]*">🎉<\/span>/);
  expect(html).toContain('<bdi aria-hidden="true" dir="ltr" lang="en" class="tabular-nums">5</bdi>');
});

test("on, it is knocked out with the text's color and no hue, also while hovered or pressed", () => {
  const html = renderToStaticMarkup(<Chip label="x" glyph="x" count={1} pressed locale="en" />);
  expect(html).toContain('aria-pressed="true"');
  expect(html).toContain("aria-pressed:bg-text aria-pressed:text-surface");
  expect(html).toContain("aria-pressed:data-hovered:bg-text");
  expect(html).not.toContain("accent");
});

test("its count is compact in the reader's locale, and it lays out in the reader's direction", () => {
  const html = renderToStaticMarkup(
    <I18nProvider locale="ar-XB">
      <Chip label="x" glyph="x" count={12_840} pressed={false} locale="ja" />
    </I18nProvider>,
  );
  expect(html).toMatch(/^<button [^>]*dir="rtl"/);
  expect(html).toContain('dir="ltr" lang="ja" class="tabular-nums">1.3万</bdi>');
});

test("chips sit in a named group that wraps", () => {
  const html = renderToStaticMarkup(
    <ChipRow label="Reactions" lang="en">
      x
    </ChipRow>,
  );
  expect(html).toBe(
    '<div role="group" aria-label="Reactions" lang="en" class="flex flex-wrap gap-chip-spacing">x</div>',
  );
});

test("a text button takes the line's font and color, underlined, with the focus ring", () => {
  const html = renderToStaticMarkup(<TextButton>Show original</TextButton>);
  expect(html).toMatch(/^<button [^>]*class="[^"]*\[font:inherit\][^"]*underline[^"]*"/);
  expect(html).toContain("text-current");
  expect(html).toContain("data-focus-visible:shadow-focus");
  expect(html).toContain(">Show original</button>");
});
