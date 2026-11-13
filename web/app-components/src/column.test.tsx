// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { Column, type ColumnTimeline } from "./column";
import type { PostStatus } from "./status";

const status = (id: string): PostStatus => ({
  id,
  account: { id: "1", username: "lena", acct: "lena", domain: "my.example", display_name: "Lena", emojis: [] },
  content: `<p>${id}</p>`,
  spoiler_text: "",
  visibility: "public",
  language: "en",
  created_at: new Date().toISOString(),
  media_attachments: [],
  mentions: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
});

const timeline = (over: Partial<ColumnTimeline> = {}): ColumnTimeline => ({
  statuses: [],
  status: "success",
  error: null,
  hasNextPage: false,
  isFetchingNextPage: false,
  fetchNextPage: () => undefined,
  refetch: () => undefined,
  ...over,
});

const render = (over: Partial<ColumnTimeline>, locale = "en", empty?: ReactNode) =>
  renderToStaticMarkup(
    <I18nProvider locale={locale}>
      <Column label="Home" {...timeline(over)} empty={empty} />
    </I18nProvider>,
  ).replace(/ class="[^"]*"/g, "");

/** The text of the notice after the feed, and its role. */
const notice = (html: string) => /<p role="(status|alert)">(.*?)<\/p>/.exec(html)?.slice(1);

describe("the feed", () => {
  test("its posts are the articles of a named feed, each with its position, in order", () => {
    const html = render({ statuses: [status("a"), status("b")] });
    expect(html).toMatch(/<div role="feed" aria-label="Home" aria-busy="false">/);
    expect(html.match(/<article [^>]*aria-posinset="\d" aria-setsize="2"/g)).toHaveLength(2);
    const [first, second] = html.split('aria-posinset="2"');
    expect(first).toContain(">a</");
    expect(second).toContain(">b</");
  });

  test("while more may follow, its size is unknown", () => {
    expect(render({ statuses: [status("a")], hasNextPage: true })).toContain('aria-posinset="1" aria-setsize="-1"');
  });
});

describe("its states", () => {
  test("loading the first posts, it is busy and says so", () => {
    const html = render({ status: "pending" });
    expect(html).toContain('aria-busy="true"');
    expect(notice(html)).toEqual(["status", "Loading posts…"]);
  });

  test("when the first posts failed, it alerts with a retry", () => {
    expect(render({ status: "error", error: new Error("x") })).toMatch(
      /<p role="alert">The posts could not be loaded\. <span>· <button [^>]*>Try again<\/button>/,
    );
  });

  test("with no posts, it shows the feature's own empty state when given one", () => {
    const html = render({}, "en", <p>Follow someone</p>);
    expect(html).toContain("<p>Follow someone</p>");
    expect(html).not.toContain("No posts yet.");
  });

  test("with no posts, it says there are none", () => {
    expect(notice(render({}))).toEqual(["status", "No posts yet."]);
  });

  test("loading more, it is busy under its posts", () => {
    const html = render({ statuses: [status("a")], hasNextPage: true, isFetchingNextPage: true });
    expect(html).toContain('aria-busy="true"');
    expect(notice(html)).toEqual(["status", "Loading more posts…"]);
  });

  test("when more failed, it alerts with a retry, and asks for no more by itself", () => {
    const html = render({ statuses: [status("a")], hasNextPage: true, error: new Error("x") });
    expect(html).toMatch(/<p role="alert">More posts could not be loaded\. <span>· <button [^>]*>Try again<\/button>/);
    // The mark the feed watches to ask for more is gone.
    expect(html).not.toContain('aria-hidden="true"></div>');
  });

  test("with posts and nothing to say, it shows no notice; it watches its end while more may follow", () => {
    expect(notice(render({ statuses: [status("a")] }))).toBeUndefined();
    expect(render({ statuses: [status("a")], hasNextPage: true })).toMatch(
      /<\/div><div aria-hidden="true"><\/div><\/div>$/,
    );
  });

  test("its words are in the reader's language", () => {
    expect(notice(render({ status: "pending" }, "ja"))).toEqual(["status", "投稿を読み込んでいます…"]);
  });
});
