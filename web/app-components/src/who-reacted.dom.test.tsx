// SPDX-License-Identifier: AGPL-3.0-only

// Who reacted to a post, under the post on its own page (FR-116): a row per emoji that someone still reacted with,
// each named by its emoji and how many reacted, in the reader's language, and leading to that emoji's list.

import { expect, test } from "bun:test";
import { cleanup, screen } from "@testing-library/react";
import type { PostAccount } from "./status";
import { inLocale, status } from "./test/post";
import { WhoReacted, type Reactors } from "./who-reacted";

const lena: PostAccount = {
  id: "1",
  username: "lena",
  acct: "lena",
  domain: "my.example",
  display_name: "Lena Novak",
  emojis: [],
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

/** The rows for a reader of `locale`: each link's name and where it leads. */
function rows(locale: string) {
  cleanup();
  inLocale(<WhoReacted status={status({ account: lena })} reactors={reactors} />, locale);
  return screen
    .getAllByRole("link")
    .map((link) => [link.getAttribute("aria-label") ?? link.textContent, link.getAttribute("href")]);
}

test("a row per emoji someone reacted with, named by the emoji and the count, leads to that emoji's list", () => {
  expect(rows("en")).toEqual([
    ["🎉 3 people reacted", `/@lena/status/10/reactions/${encodeURIComponent("🎉")}`],
    [":teacup: 1 person reacted", "/@lena/status/10/reactions/teacup"],
  ]);
  expect(screen.getByRole("link", { name: "🎉 3 people reacted" })).not.toBeNull();
  expect(screen.getByRole("link", { name: ":teacup: 1 person reacted" })).not.toBeNull();
});

test("the rows are named in the reader's language", () => {
  expect(rows("ja").map(([name]) => name)).toEqual(["🎉 3人がリアクション", ":teacup: 1人がリアクション"]);
});
