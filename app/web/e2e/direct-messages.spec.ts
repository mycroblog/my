// SPDX-License-Identifier: AGPL-3.0-only
// The direct messages in every width × locale project (ADR-0154 §6; web-client/screens/direct-messages.md): opened
// from the navigation (the rail from md, the menu's sheet below it), whose item counts the unread conversations; the
// list is axe-clean and does not scroll horizontally; opening a conversation goes to its thread and marks it read, and
// a reply there can only be direct. `vite preview` has no API, so the spec answers the token refresh and the client API
// with `page.route`, as a signed-in user's server would. Names come from the project's catalog, so the spec finds the
// same elements in every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import {
  escapeRegExp,
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  openFromNavigation,
  test,
} from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Account = components["schemas"]["Account"];
type Conversation = components["schemas"]["Conversation"];
type Status = components["schemas"]["Status"];

/** The client message `key` in the project's locale, its arguments left as they are. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** A pattern for a name that starts with `text`, which may hold the brackets of a pseudo-locale. */
function startsWith(text: string): RegExp {
  return new RegExp(`^${escapeRegExp(text)}`, "u");
}

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
const bob: Account = {
  ...alice,
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  display_name: "Bob",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000002",
  url: "https://my.example/@bob",
};

const postId = "0192f0c4-7a8e-7000-8000-000000000101";
const bobsMessage: Status = {
  id: postId,
  uri: `https://my.example/ap/statuses/${postId}`,
  url: `https://my.example/@bob/status/${postId}`,
  account: bob,
  content: '<p><a href="https://my.example/@alice">@alice</a> Are you coming to the bakery club on Saturday?</p>',
  spoiler_text: "",
  sensitive: false,
  visibility: "direct",
  language: "en",
  created_at: new Date(Date.now() - 30 * 60_000).toISOString(),
  media_attachments: [],
  mentions: [{ id: alice.id, username: "alice", acct: "alice", url: alice.url }],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
  favourited: false,
  boosted: false,
  bookmarked: false,
};

/** Answers the client API as Alice's session, with one unread conversation with Bob until it is marked read. */
async function serveApi(page: Page): Promise<{ marked: () => number }> {
  let conversation: Conversation = {
    id: "0192f0c4-7a8e-7000-8000-00000000d001",
    accounts: [bob],
    last_status: bobsMessage,
    unread: true,
  };
  let marked = 0;
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({
      status: statusCode,
      contentType: statusCode < 400 ? "application/json" : "application/problem+json",
      body: JSON.stringify(body),
    });
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace("/api/v1", "");
    if (path === "/accounts/verify_credentials") return json(route, { ...alice, permissions: [] });
    if (path === "/instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0-e2e",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        full_text_search: { available: false, visibilities: [] },
        translation: { browser: false, providers: [] },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (path === "/preferences") {
      // The user's locale is the project's: the app displays itself in the signed-in user's locale.
      return json(route, {
        locale: test.info().project.use.locale ?? "en",
        time_zone: "UTC",
        default_visibility: "public",
        default_interaction_policy: {
          reply: { automatic: "everyone" },
          announce: { automatic: "everyone" },
          quote: { automatic: "everyone" },
        },
        default_post_language: "en",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [] });
    if (path === "/conversations") return json(route, { items: [conversation] });
    if (path === `/conversations/${conversation.id}/read` && request.method() === "POST") {
      marked += 1;
      conversation = { ...conversation, unread: false };
      return json(route, conversation);
    }
    if (path === `/statuses/${postId}/context`) return json(route, { ancestors: [], descendants: [] });
    if (path === `/statuses/${postId}`) return json(route, bobsMessage);
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return { marked: () => marked };
}

test.describe("the direct messages", () => {
  test("count the unread conversations, list them, and open one as its thread, read", async ({
    page,
    hasTouch,
    makeAxeBuilder,
  }) => {
    const api = await serveApi(page);
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await openFromNavigation(page, "/direct-messages", hasTouch);
    await expect(page.getByRole("heading", { level: 1, name: message("direct_messages.title") })).toBeFocused();

    const feed = page.getByRole("feed", { name: message("direct_messages.title") });
    const row = feed.getByRole("article");
    await expect(row).toHaveCount(1);
    await expect(row).toHaveAccessibleName(startsWith(message("direct_messages.unread")));
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The excerpt opens the thread and marks the conversation read.
    await row.getByRole("link", { name: /bakery club/u }).click();
    await expect(page.getByRole("heading", { level: 1, name: message("status.title") })).toBeFocused();
    await expect.poll(() => api.marked()).toBe(1);

    // A reply in the thread stays direct: the other visibilities are disabled.
    const main = page.getByRole("main");
    await main
      .getByRole("button", { name: startsWith(message("status.action.reply")) })
      .first()
      .click();
    const visibility = page.getByRole("button", {
      name: message("compose.visibility.button").replace("{visibility}", message("compose.visibility.direct")),
    });
    await visibility.click();
    const choices = page.getByRole("radiogroup", { name: message("compose.visibility.title") });
    await expect(choices.getByRole("radio", { name: startsWith(message("compose.visibility.direct")) })).toBeChecked();
    await expect(choices.getByRole("radio", { name: startsWith(message("compose.visibility.public")) })).toBeDisabled();
    await expectNoAxeViolations(makeAxeBuilder);
  });
});
