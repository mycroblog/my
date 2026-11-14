// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { AccountRow, NoticeCaption, NoticeLine, OpenRow, TabRow, UnreadMark } from "./list";
import { Badge, ProfileHead } from "./profile";
import { ThreadReply } from "./thread";

const tabs = [
  { id: "all", name: "All" },
  { id: "mentions", name: "Mentions" },
] as const;

test("the tabs are a tablist named by their label, the current one selected, the list under them its panel", () => {
  const html = renderToStaticMarkup(
    <TabRow label="Show" tabs={tabs} current="mentions" onChange={() => undefined} end={<button>Read</button>}>
      <p>List</p>
    </TabRow>,
  );
  expect(html).toMatch(/aria-label="Show" role="tablist"/);
  expect(html).toMatch(/aria-selected="true"[^>]*>Mentions/);
  expect(html).toMatch(/role="tabpanel"[^>]*><p>List<\/p>/);
  // React Aria writes the collection into a template first: the row as shown is after it.
  expect(html.lastIndexOf(">Mentions<")).toBeLessThan(html.lastIndexOf(">Read<"));
});

test("the unread mark is a shape in the text's color with no hue, named for assistive technology", () => {
  const html = renderToStaticMarkup(<UnreadMark label="Unread" />);
  expect(html).toMatch(/role="img" aria-label="Unread"/);
  expect(html).toContain("bg-text");
  expect(html).not.toContain("accent");
});

test("a notice's line is in the text's color with its mark while unread, and muted with none once read", () => {
  const unread = renderToStaticMarkup(
    <NoticeLine icon="follow" date="2026-11-14T00:00:00Z" unread="Unread">
      Lena followed you
    </NoticeLine>,
  );
  const read = renderToStaticMarkup(
    <NoticeLine icon="follow" date="2026-11-14T00:00:00Z">
      Lena followed you
    </NoticeLine>,
  );
  expect(unread).toContain('aria-label="Unread"');
  expect(unread).not.toMatch(/text-muted-text">Lena followed you/);
  expect(read).not.toContain('aria-label="Unread"');
  expect(read).toMatch(/text-muted-text">Lena followed you/);
  const caption = renderToStaticMarkup(
    <NoticeCaption icon="reply" unread="Unread">
      Ken mentioned you
    </NoticeCaption>,
  );
  expect(caption.indexOf("Ken mentioned you")).toBeLessThan(caption.indexOf('aria-label="Unread"'));
});

test("an account's row puts what the reader may do at its end, its name leading to the profile", () => {
  const html = renderToStaticMarkup(
    <AccountRow avatar={<span />} name="Lena" handle="@lena@my.example" href="/@lena" end={<button>Accept</button>} />,
  );
  expect(html).toMatch(/href="\/@lena"[^>]*><bdi[^>]*>Lena<\/bdi><\/a>/);
  expect(html.indexOf("@lena@my.example")).toBeLessThan(html.indexOf(">Accept<"));
});

test("the row of what waits elsewhere is one link, its glyph mirrored toward the line's end", () => {
  const html = renderToStaticMarkup(
    <OpenRow href="/notifications/requests" icon="followers" title="Requests" hint="Held" />,
  );
  expect(html.match(/<a /g)).toHaveLength(1);
  expect(html).toContain("rtl:-scale-x-100");
});

test("a reply indents by its depth, up to three steps, and only where its column is wide enough", () => {
  const indent = (depth: number) => renderToStaticMarkup(<ThreadReply depth={depth}>x</ThreadReply>);
  expect(indent(1)).not.toContain("@reply:ms-");
  expect(indent(2)).toContain("@reply:ms-screen-reply-step");
  expect(indent(4)).toContain("@reply:ms-[calc(3*var(--screen-reply-step))]");
  expect(indent(9)).toBe(indent(4));
});

test("a profile's head is named by its name, the avatar's row before the names, in the reader's direction", () => {
  const html = renderToStaticMarkup(
    <I18nProvider locale="ar-XB">
      <ProfileHead
        nameId="n"
        avatar={<span>A</span>}
        actions={<button>Follow</button>}
        name="Alice"
        handle="@alice@example.social"
        badges={<Badge>Follows you</Badge>}
      />
    </I18nProvider>,
  );
  expect(html).toMatch(/<section [^>]*aria-labelledby="n" dir="rtl"/);
  expect(html.indexOf(">Follow<")).toBeLessThan(html.indexOf(">Alice<"));
  expect(html).toContain('<bdi dir="ltr">@alice@example.social</bdi>');
  expect(html).toContain('<bdi dir="auto"');
});
