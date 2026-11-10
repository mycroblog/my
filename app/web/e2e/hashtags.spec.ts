// SPDX-License-Identifier: AGPL-3.0-only
// Followed hashtags in every width × locale project (ADR-0013 §24–§28; web-client/screens/tag.md, home.md): the viewer
// opens a hashtag from a post of the home timeline, follows it, and goes back to find on home, without loading the page
// again, a post of someone they do not follow with the mark of the followed hashtag. `vite preview` has no API, so the
// spec answers the token refresh and the client API with `page.route`, as a signed-in user's server would. Names come
// from the project's catalog, so the spec finds the same elements in every locale.
import type { Page } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Account = components["schemas"]["Account"];
type Status = components["schemas"]["Status"];

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

function account(n: number, username: string, name: string): Account {
  const id = `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`;
  return {
    id,
    username,
    acct: username,
    domain: "my.example",
    uri: `https://my.example/ap/actors/${id}`,
    display_name: name,
    note: "",
    fields: [],
    emojis: [],
    badges: [],
    url: `https://my.example/@${username}`,
    locked: false,
    bot: false,
    created_at: "2026-10-01T00:00:00Z",
  };
}

const alice = account(1, "alice", "Alice");
const bob = account(2, "bob", "Bob");

/** A post of `author` with the hashtag #bread, linked to its page on the server at `origin`. */
function breadPost(n: number, author: Account, origin: string, text: string): Status {
  const id = `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`;
  return {
    id,
    uri: `https://my.example/ap/statuses/${id}`,
    url: `https://my.example/@${author.username}/status/${id}`,
    account: author,
    content: `<p>${text} <a href="${origin}/tags/bread" class="mention hashtag" rel="tag">#<span>bread</span></a></p>`,
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    language: "en",
    created_at: new Date(Date.now() - n * 60_000).toISOString(),
    media_attachments: [],
    mentions: [],
    tags: [{ name: "bread", url: `${origin}/tags/bread` }],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
    filtered: [],
    favourited: false,
    boosted: false,
    bookmarked: false,
  };
}

/**
 * Answers the session refresh and the home timeline: Alice's own post with #bread, and once she follows #bread, Bob's
 * post with it too, which only the hashtag brings. Follows change the hashtag's state.
 */
async function signedInServer(page: Page) {
  let following = false;
  const json = (body: unknown) => ({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  const originOf = (url: string) => new URL(url).origin;
  await page.route("**/oauth/token/refresh", (route) =>
    route.fulfill(json({ access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 })),
  );
  await page.route("**/api/v1/accounts/verify_credentials", (route) =>
    route.fulfill(json({ ...alice, permissions: [] })),
  );
  await page.route("**/api/v1/notifications/marker", (route) => route.fulfill(json({ unread_count: 0 })));
  await page.route("**/api/v1/timelines/home*", (route) => {
    const origin = originOf(route.request().url());
    const own = breadPost(2, alice, origin, "Fresh from the oven.");
    const items = following ? [{ ...breadPost(1, bob, origin, "Rye today."), followed_tags: ["bread"] }, own] : [own];
    return route.fulfill(json({ items }));
  });
  await page.route("**/api/v1/timelines/tag/*", (route) =>
    route.fulfill(json({ items: [breadPost(1, bob, originOf(route.request().url()), "Rye today.")] })),
  );
  await page.route("**/api/v1/tags/**", (route) => {
    const url = route.request().url();
    const method = route.request().method();
    if (url.endsWith("/follow")) following = method === "POST";
    return route.fulfill(json({ name: "bread", url: `${originOf(url)}/tags/bread`, following }));
  });
}

test.describe("following a hashtag", () => {
  test.beforeEach(async ({ page }) => {
    await signedInServer(page);
    await page.goto("/home");
    await expect(page.getByRole("feed").getByRole("article")).toHaveCount(1);
  });

  test("brings its posts to home without a reload, marked with the hashtag", async ({ page, makeAxeBuilder }) => {
    await page.getByRole("feed").getByRole("link", { name: "#bread" }).click();
    const follow = page.getByRole("button", { name: message("tag.follow") });
    await expect(follow).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await follow.click();
    const following = page.getByRole("button", { name: message("tag.following") });
    await expect(following).toBeVisible();
    await expect(following).toBeFocused();

    // Back in the app's history: the same document, whose home timeline loads again with the hashtag's posts.
    const loads = await page.evaluate(() => performance.getEntriesByType("navigation").length);
    await page.getByRole("button", { name: message("shell.back") }).click();
    const articles = page.getByRole("feed").getByRole("article");
    await expect(articles).toHaveCount(2);
    const marked = articles.first();
    await expect(marked.getByRole("link", { name: "#bread" }).first()).toHaveAttribute("href", "/tags/bread");
    await expect(marked).toHaveAccessibleName(/#bread/u);
    expect(await page.evaluate(() => performance.getEntriesByType("navigation").length)).toBe(loads);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
