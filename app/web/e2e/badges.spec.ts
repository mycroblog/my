// SPDX-License-Identifier: AGPL-3.0-only
// The role badges of a profile in every width × locale project (ADR-0071, ADR-0027): a named list next to the name,
// with a long badge name and one in a right-to-left script, passing axe without scrolling sideways, and absent from
// the author lines of the account's posts. `vite preview` has no API, so the spec plays the signed-in user's server
// with page.route.
import type { Page } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

type Account = components["schemas"]["Account"];
type Status = components["schemas"]["Status"];

const alice: Account = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};

/** Bob shows two badges on his profile: a long name, and one in a right-to-left script. */
const bob: Account = {
  ...alice,
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000002",
  display_name: "Bob",
  url: "https://my.example/@bob",
  followers_count: 3,
  following_count: 1,
  badges: [
    { role_id: "0192f0c4-7a8e-7000-8000-0000000000b1", label: "Supporter of the large plan 2026", color: "purple" },
    { role_id: "0192f0c4-7a8e-7000-8000-0000000000b2", label: "داعم", color: "teal" },
  ],
};

/** A post of Bob's: the server sends no badges on embedded accounts. */
const post: Status = {
  id: "0192f0c4-7a8e-7000-8000-000000000101",
  uri: "https://my.example/ap/statuses/101",
  account: { ...bob, badges: [] },
  content: "<p>Fresh bread.</p>",
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: "2026-10-20T00:00:00Z",
  media_attachments: [],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
};

async function signedInServer(page: Page) {
  const json = (body: unknown) => ({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) =>
    route.fulfill(json({ access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 })),
  );
  await page.route("**/api/v1/accounts/verify_credentials", (route) => route.fulfill(json(alice)));
  await page.route("**/api/v1/notifications/marker", (route) => route.fulfill(json({ unread_count: 0 })));
  await page.route("**/api/v1/accounts/lookup*", (route) => route.fulfill(json(bob)));
  await page.route("**/api/v1/accounts/relationships*", (route) =>
    route.fulfill(
      json([
        {
          id: bob.id,
          following: false,
          requested: false,
          followed_by: false,
          requested_by: false,
          show_boosts: true,
          notify: false,
          blocking: false,
          muting: false,
          muting_notifications: false,
        },
      ]),
    ),
  );
  await page.route(`**/api/v1/accounts/${bob.id}/statuses*`, (route) =>
    new URL(route.request().url()).searchParams.get("pinned") === "true"
      ? route.fulfill(json({ items: [] }))
      : route.fulfill(json({ items: [post] })),
  );
}

test.use({ reducedMotion: "reduce" });

test("a profile's badges are a named list next to the name, and not on the posts", async ({ page, makeAxeBuilder }) => {
  await signedInServer(page);
  await page.goto(`/@${bob.acct}`);
  const header = page.getByRole("main").locator("header").first();
  const badges = header.getByRole("list").filter({ hasText: "داعم" });
  await expect(badges).toMatchAriaSnapshot(`
    - list:
      - listitem: Supporter of the large plan 2026
      - listitem: داعم
  `);
  await expect(badges.getByText("داعم")).toHaveAttribute("dir", "auto");
  const feed = page.getByRole("feed");
  await expect(feed.getByRole("article")).toHaveCount(1);
  await expect(feed.getByText("Supporter of the large plan 2026")).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});
