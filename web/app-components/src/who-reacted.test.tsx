// SPDX-License-Identifier: AGPL-3.0-only

// Who reacted to a post, under the post on its own page (FR-116): a row per emoji that someone still reacted with,
// each named by its emoji and how many reacted, in the reader's language, and leading to that emoji's list.

import { expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import type { PostAccount, PostStatus } from "./status";
import { WhoReacted, type Reactors } from "./who-reacted";

const lena: PostAccount = {
  id: "1",
  username: "lena",
  acct: "lena",
  domain: "my.example",
  display_name: "Lena Novak",
  emojis: [],
};

const status: PostStatus = {
  id: "10",
  account: lena,
  content: "<p>Quiet morning.</p>",
  spoiler_text: "",
  visibility: "public",
  language: "en",
  created_at: "2026-11-14T03:00:00Z",
  media_attachments: [],
  mentions: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
};

const reactors: Reactors[] = [
  { reaction: { name: "🎉", count: 3 }, accounts: [lena] },
  {
    reaction: {
      name: "teacup",
      shortcode: "teacup",
      emoji: { shortcode: "teacup", url: "https://my.example/teacup.png", static_url: "https://my.example/teacup.png" },
      count: 1,
    },
    accounts: [lena],
  },
  // Taken back since the post loaded.
  { reaction: { name: "👍", count: 0 }, accounts: [] },
];

const rows = (locale: string) =>
  renderToStaticMarkup(
    <I18nProvider locale={locale}>
      <WhoReacted status={status} reactors={reactors} />
    </I18nProvider>,
  );

test("a row per emoji someone reacted with, named by the emoji and the count, leads to that emoji's list", () => {
  const html = rows("en");
  expect(html).toContain("🎉 3 people reacted");
  expect(html).toContain(":teacup: 1 person reacted");
  expect(html).toContain(`href="/@lena/status/10/reactions/${encodeURIComponent("🎉")}"`);
  expect(html).toContain(`href="/@lena/status/10/reactions/teacup"`);
  expect(html).not.toContain("👍");
  expect(html.match(/<a /g)?.length).toBe(2);
});

test("the rows are named in the reader's language", () => {
  const html = rows("ja");
  expect(html).toContain("🎉 3人がリアクション");
  expect(html).toContain(":teacup: 1人がリアクション");
});
