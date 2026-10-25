// SPDX-License-Identifier: AGPL-3.0-only
// The list of the user's bookmarks in every width × locale project (FR-113, ADR-0027, ADR-0039): opened from the
// navigation (the rail from md, the menu's sheet below it), its feed and its empty state are axe-clean and do not
// scroll sideways, and a post whose bookmark is removed is gone when the list opens again. `vite preview` has no
// API, so the spec answers the token refresh and the client API with `page.route`, as a signed-in user's server
// would. Names come from the project's catalog, so the spec finds the same elements in every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, openFromNavigation, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Status = components["schemas"]["Status"];
/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

const alice: Status["account"] = {
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
const bob: Status["account"] = { ...alice, id: "0192f0c4-7a8e-7000-8000-000000000002", username: "bob", acct: "bob", display_name: "Bob", url: "https://my.example/@bob" };

function status(n: number, content: string): Status {
  const id = `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`;
  return {
    id,
    uri: `https://my.example/ap/statuses/${id}`,
    url: `https://my.example/@bob/status/${id}`,
    account: bob,
    content,
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    language: "en",
    created_at: new Date(Date.now() - n * 60 * 60_000).toISOString(),
    media_attachments: [],
    mentions: [],
    tags: [],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
    favourited: false,
    boosted: false,
    bookmarked: true,
  };
}

/**
 * Answers the client API as Alice's session with two bookmarked posts; removing a bookmark takes the post out of
 * the list the server answers.
 */
async function serveApi(page: Page): Promise<void> {
  const bookmarks = new Map([status(1, "<p>The trail map for the weekend walk.</p>"), status(2, "<p>A recipe worth keeping.</p>")].map((s) => [s.id, s]));
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({ status: statusCode, contentType: statusCode < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace("/api/v1", "");
    if (path === "/accounts/verify_credentials") return json(route, alice);
    if (path === "/instance") {
      return json(route, { domain: "my.example", name: "my example", version: "0.0.0-e2e", source_url: "https://github.com/mycroblog/my", registrations: { mode: "open" }, federation: { mode: "both" }, limits: { max_post_characters: 500, require_media_description: false }, timelines: { local: "everyone", federated: "everyone" }, rules: [] });
    }
    if (path === "/preferences") {
      // The user's locale is the project's: the app displays itself in the signed-in user's locale.
      return json(route, { locale: test.info().project.use.locale ?? "en", time_zone: "UTC", default_visibility: "public", default_post_language: "en", require_media_description: false, timeline_languages: [] });
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [] });
    if (path === "/bookmarks") return json(route, { items: [...bookmarks.values()] });
    const bookmark = /^\/statuses\/([^/]+)\/bookmark$/.exec(path);
    if (bookmark !== null && request.method() === "DELETE") {
      const removed = bookmarks.get(bookmark[1] ?? "");
      if (removed === undefined) return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
      bookmarks.delete(removed.id);
      return json(route, { ...removed, bookmarked: false });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

test.describe("the bookmarks", () => {
  test.beforeEach(async ({ page, hasTouch }) => {
    await serveApi(page);
    await page.goto("/home");
    // The first page keeps focus where the browser puts it; only a navigation moves it to the heading.
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await openFromNavigation(page, "/bookmarks", hasTouch);
    await expect(page.getByRole("heading", { level: 1, name: message("bookmarks.title") })).toBeFocused();
    await expect(page.getByRole("feed").getByRole("article")).toHaveCount(2);
  });

  test("lists the bookmarked posts in a feed named by the page, axe-clean and without horizontal scroll", async ({ page, makeAxeBuilder }) => {
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - heading [level=1]
      - feed:
        - article /Bob/:
          - paragraph: The trail map for the weekend walk.
          - button "${message("status.action.bookmark")}" [pressed]
        - article /Bob/:
          - paragraph: A recipe worth keeping.
          - button "${message("status.action.bookmark")}" [pressed]
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("drops the posts whose bookmark is removed once opened again, then says the list is empty", async ({ page, hasTouch, makeAxeBuilder }) => {
    for (const article of await page.getByRole("feed").getByRole("article").all()) {
      const toggle = article.getByRole("button", { name: message("status.action.bookmark") });
      await toggle.focus();
      await page.keyboard.press("Enter");
      // The list being read keeps the post, shown unbookmarked, so that the press can be undone.
      await expect(toggle).toHaveAttribute("aria-pressed", "false");
    }
    await openFromNavigation(page, "/home", hasTouch);
    await expect(page.getByRole("heading", { level: 1 })).toBeFocused();
    await openFromNavigation(page, "/bookmarks", hasTouch);

    await expect(page.getByRole("heading", { level: 2, name: message("timeline.empty.bookmarks.heading") })).toBeVisible();
    await expect(page.getByRole("feed")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
