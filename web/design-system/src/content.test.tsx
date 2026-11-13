// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { ContentBody, ContentLink, CustomEmoji, UrlPart } from "./content";

test("the body lays its blocks a paragraph's gap apart in body text that wraps anywhere, with its language", () => {
  const html = renderToStaticMarkup(
    <ContentBody lang="ja">
      <p>一</p>
      <p>二</p>
    </ContentBody>,
  );
  expect(html).toMatch(/^<div class="[^"]*gap-content-paragraph[^"]*" lang="ja"><p>一<\/p><p>二<\/p><\/div>$/);
  expect(html).toContain("text-body");
  expect(html).toContain("wrap-anywhere");
  // The plain elements it holds lose the browser's margins; quotes and lists indent on the start side.
  expect(html).toContain("[&amp;_p]:m-0");
  expect(html).toContain("[&amp;_ul]:ps-content-indent");
  expect(html).not.toMatch(/\b(pl|ml|left)-/);
});

test("a link takes the accent with no underline, opens in a new tab and sends no referrer", () => {
  const html = renderToStaticMarkup(
    <ContentLink href="https://example.com/" kind="link">
      example.com
    </ContentLink>,
  );
  expect(html).toContain('href="https://example.com/"');
  expect(html).toContain('rel="nofollow noopener noreferrer"');
  expect(html).toContain('target="_blank"');
  expect(html).toContain("text-accent-text no-underline");
  expect(html).not.toContain("whitespace-nowrap");
});

test("a mention or a hashtag never breaks inside, and keeps its translate and lang", () => {
  for (const kind of ["mention", "hashtag"] as const) {
    const html = renderToStaticMarkup(
      <ContentLink href="https://example.com/@alice" kind={kind} translate="no" lang="en">
        @alice
      </ContentLink>,
    );
    expect(html).toContain("whitespace-nowrap");
    expect(html).toContain('translate="no"');
    expect(html).toContain('lang="en"');
  }
});

test("a hidden part of a URL takes no room but stays in the text; the cut part ends in an ellipsis", () => {
  const hidden = renderToStaticMarkup(<UrlPart part="hidden">https://</UrlPart>);
  expect(hidden).toMatch(/^<span class="[^"]*size-0[^"]*">https:\/\/<\/span>$/);
  expect(hidden).toContain("text-[0px]");
  expect(renderToStaticMarkup(<UrlPart part="cut">example.com/a</UrlPart>)).toContain("after:content-[&#x27;…&#x27;]");
});

test("a custom emoji reads as its shortcode and moves only when the reader allows motion", () => {
  const animated = renderToStaticMarkup(
    <CustomEmoji shortcode="blobcat" url="https://m.example/a.gif" staticUrl="https://m.example/s.png" />,
  );
  expect(animated).toContain(
    '<source media="(prefers-reduced-motion: no-preference)" srcSet="https://m.example/a.gif"/>',
  );
  expect(animated).toContain('src="https://m.example/s.png"');
  expect(animated).toContain('alt=":blobcat:"');
  expect(animated).toContain('title=":blobcat:"');
  expect(animated).toContain('draggable="false"');
  expect(animated).toContain("size-content-emoji");
  const still = renderToStaticMarkup(
    <CustomEmoji shortcode="wave" url="https://m.example/w.png" staticUrl="https://m.example/w.png" />,
  );
  expect(still).not.toContain("<source");
});
