// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { Disclosure } from "./disclosure";

const folded = (locale?: string) =>
  renderToStaticMarkup(
    <Disclosure summary="Spoilers for the finale" icon="hidden" locale={locale}>
      <p>The ending.</p>
    </Disclosure>,
  );

test("folded, the button shows, is collapsed and controls the hidden panel, described by the summary", () => {
  const html = folded("en");
  expect(html).toContain(">Show</span>");
  expect(html).toContain('aria-expanded="false"');
  const controls = /aria-controls="([^"]+)"/.exec(html)?.[1];
  expect(controls).toBeDefined();
  expect(html).toMatch(new RegExp(`id="${controls}"[^>]* hidden=""`));
  const described = /aria-describedby="([^"]+)"/.exec(html)?.[1];
  expect(html).toMatch(new RegExp(`id="${described}" class="[^"]*">Spoilers for the finale</div>`));
});

test("the glyph and the toggle line up with the summary's first line, not its middle", () => {
  const html = folded("en");
  expect(html).toContain("items-start");
  expect(html).not.toContain("items-center gap-disclosure-gap");
  expect(html).toContain("tabler-icon-eye-off");
});

test("expanded, the button hides and the panel shows what it folds", () => {
  const html = renderToStaticMarkup(
    <Disclosure summary="Spoilers" defaultExpanded locale="en">
      <p>The ending.</p>
    </Disclosure>,
  );
  expect(html).toContain(">Hide</span>");
  expect(html).toContain('aria-expanded="true"');
  expect(html).toContain('aria-hidden="false"');
  expect(html).not.toMatch(/ hidden(=|>| )/);
  expect(html).toContain("<p>The ending.</p>");
});

test("the label is in the reader's locale, marked with its language; another language reads en", () => {
  expect(folded("ja")).toContain('<span lang="ja">表示</span>');
  expect(folded("ja-JP")).toContain('<span lang="ja">表示</span>');
  expect(folded("de")).toContain('<span lang="en">Show</span>');
});

test("the summary sits on a raised surface with its glyph muted; the glyph is optional", () => {
  expect(folded("en")).toContain("bg-surface-raised");
  expect(folded("en")).toContain("text-muted-text");
  const plain = renderToStaticMarkup(
    <Disclosure summary="Spoilers" locale="en">
      x
    </Disclosure>,
  );
  expect(plain).not.toContain("<svg");
});

test("the label reads the pseudo-locales made from en, so a string left out of the catalog shows", () => {
  expect(folded("en-XA")).toMatch(/<span lang="en-XA">\[[^\]]+\]<\/span>/);
  expect(folded("en-XA")).not.toContain(">Show<");
  expect(folded("ar-XB")).toMatch(/<span lang="ar-XB">‮[^<]+‬<\/span>/);
});
