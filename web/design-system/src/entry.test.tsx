// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { ActionRow, Byline, CaptionLine, CaptionName, Embed, Entry, Stack, Stamp } from "./entry";
import { IconButton } from "./icon-button";
import { Preview } from "./preview";

test("an entry is a focusable article, found by its feed, with no surface at rest", () => {
  const html = renderToStaticMarkup(
    <Entry lang="ja" labelledBy="name">
      x
    </Entry>,
  );
  expect(html).toMatch(/^<article lang="ja" dir="ltr" tabindex="0" aria-labelledby="name" data-entry=""/);
  expect(html).not.toContain("data-surface-marked=");
  expect(html).not.toContain("data-focus-visible=");
  // Its surface shows only while marked, and reaches past the content so that the content stays where it is.
  expect(html).toContain("data-surface-marked:bg-surface-hover");
  expect(html).toContain("-m-entry-reach p-entry-reach");
});

test("hovered, it marks its surface; focused from the keyboard, it adds the ring; its buttons stay at rest", () => {
  const shown = (state: "hover" | "focus-visible") =>
    renderToStaticMarkup(
      <Preview state={state}>
        <Entry>
          <IconButton icon="reply" label="Reply" />
        </Entry>
      </Preview>,
    );
  expect(shown("hover")).toContain('data-surface-marked=""');
  expect(shown("hover")).not.toContain('data-focus-visible=""');
  expect(shown("focus-visible")).toMatch(/^<article [^>]*data-focus-visible="true" data-surface-marked=""/);
  expect(shown("hover")).not.toContain('data-hovered="true"');
  expect(shown("focus-visible").match(/data-focus-visible="true"/g)).toHaveLength(1);
});

test("it lays out in the reader's direction", () => {
  const html = renderToStaticMarkup(
    <I18nProvider locale="ar-XB">
      <Entry lang="en">x</Entry>
    </I18nProvider>,
  );
  expect(html).toContain('lang="en" dir="rtl"');
});

test("the byline isolates the name and keeps the handle left-to-right, truncating at its end", () => {
  const html = renderToStaticMarkup(
    <Byline avatar={<span>A</span>} name="سارا" nameId="n" handle="@sara@example.social" end={<span>E</span>} />,
  );
  expect(html).toMatch(/<bdi id="n" class="[^"]*truncate[^"]*">سارا<\/bdi><span class="ms-auto[^"]*"><span>E<\/span>/);
  // The handle's line follows the page's direction, so the handle sits at its start; the handle truncates at its end.
  expect(html).toMatch(
    /<span class="block min-w-0 [^"]*"><bdi dir="ltr" class="inline-block max-w-full truncate[^"]*">@sara@example.social<\/bdi><\/span>/,
  );
  // The name's line ends with what follows it; the handle is under it.
  expect(html.indexOf("<span>E</span>")).toBeLessThan(html.indexOf("@sara@example.social"));
});

test("a compact byline puts the name, the handle and the end on one line", () => {
  const html = renderToStaticMarkup(<Byline compact avatar={null} name="Ravi" handle="@ravi@x.example" end="now" />);
  expect(html).toMatch(
    /<bdi [^>]*>Ravi<\/bdi><span class="[^"]*flex-1[^"]*"><bdi dir="ltr" [^>]*>@ravi@x.example<\/bdi><\/span><span [^>]*>now<\/span>/,
  );
});

test("the stamp names its glyph and isolates the time", () => {
  const html = renderToStaticMarkup(
    <Stamp glyph={{ icon: "public", label: "Public" }} date={new Date(Date.now() - 3 * 60_000)} />,
  );
  expect(html).toMatch(/<span role="img" aria-label="Public" [^>]*><svg [^>]*tabler-icon-world/);
  expect(html).toMatch(/<bdi><time [^>]*>3m ago<\/time><\/bdi>/);
  expect(renderToStaticMarkup(<Stamp date={new Date()} />)).not.toContain('role="img"');
});

test("a caption line keeps its glyph on the first line, and its action with the middle dot", () => {
  const html = renderToStaticMarkup(
    <CaptionLine icon="translate" lang="en" action={<button type="button">Show original</button>}>
      Translated from Japanese
    </CaptionLine>,
  );
  expect(html).toMatch(/^<p class="[^"]*items-start[^"]*" lang="en">/);
  expect(html).toContain('Translated from Japanese <span class="whitespace-nowrap">· <button');
  const alone = renderToStaticMarkup(
    <CaptionLine icon="translate" action={<button type="button">Translate</button>} />,
  );
  expect(alone).not.toContain("·");
});

test("a one-line caption truncates its name and keeps the words around it with their spaces", () => {
  const html = renderToStaticMarkup(
    <CaptionLine icon="boost" oneLine>
      <CaptionName>A very long name</CaptionName> boosted
    </CaptionLine>,
  );
  expect(html).toContain(
    '<span class="min-w-0 flex-1 flex whitespace-pre"><bdi class="min-w-0 truncate">A very long name</bdi> boosted</span>',
  );
  expect(renderToStaticMarkup(<CaptionLine icon="boost">x</CaptionLine>)).not.toContain("whitespace-pre");
});

test("the action row reaches a tile's padding past the body and puts `end` at its end", () => {
  const html = renderToStaticMarkup(<ActionRow end={<i>more</i>}>a</ActionRow>);
  expect(html).toContain("-mx-icon-button-inline");
  expect(html).toContain('<span class="ms-auto inline-flex"><i>more</i></span>');
});

test("a stack and an embed lay their parts one under another; an embed sits on the raised surface", () => {
  expect(renderToStaticMarkup(<Stack gap="line">x</Stack>)).toContain("gap-entry-line");
  expect(renderToStaticMarkup(<Stack>x</Stack>)).toContain('gap-entry"');
  expect(renderToStaticMarkup(<Embed lang="ja">x</Embed>)).toMatch(/class="[^"]*bg-surface-raised[^"]*" lang="ja"/);
});
