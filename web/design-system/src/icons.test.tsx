// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { Icon, icons, type IconName } from "./icons";

test("every icon is hidden from assistive technology and sized by its place", () => {
  for (const name of Object.keys(icons) as IconName[]) {
    for (const on of [false, true]) {
      const html = renderToStaticMarkup(<Icon name={name} place="caption" on={on} />);
      expect(html).toMatch(/^<(svg|span)[^>]*aria-hidden="true"/);
      expect(html).toContain("size-icon-caption");
      if (html.startsWith("<svg")) expect(html).toContain('focusable="false"');
    }
  }
});

test("a glyph that bears a direction mirrors in right-to-left text, and only such a glyph", () => {
  expect(renderToStaticMarkup(<Icon name="back" place="action" />)).toContain("rtl:-scale-x-100");
  expect(renderToStaticMarkup(<Icon name="boost" place="action" />)).not.toContain("rtl:");
});

test("an on state differs by shape: a filled glyph, or the glyph knocked out of a solid tile", () => {
  const star = renderToStaticMarkup(<Icon name="favourite" place="action" />);
  expect(renderToStaticMarkup(<Icon name="favourite" place="action" on />)).not.toBe(star);
  const boost = renderToStaticMarkup(<Icon name="boost" place="action" on />);
  expect(boost).toMatch(/^<span aria-hidden="true" class="[^"]*bg-current[^"]*"><svg[^>]*focusable="false"/);
});
