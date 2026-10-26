// SPDX-License-Identifier: AGPL-3.0-only
// The federated timeline in every width × locale project (FR-202, ADR-0027): its navigation item, which
// the instance setting federated_timeline_access takes away, and its feed of posts of this server and of
// another one. `vite preview` has no client API, so the spec answers GET /instance and the timeline with
// `page.route`, shaped like api/openapi.yaml. Names change with the locale, so the navigation item is
// found by its address and the feed by the page heading.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, openFromNavigation, test } from "./fixtures";

type Account = components["schemas"]["Account"];
type Instance = components["schemas"]["Instance"];
type Status = components["schemas"]["Status"];
type StatusPage = components["schemas"]["StatusPage"];

function account(username: string, displayName: string, domain: string): Account {
  const local = domain === "my.example";
  return {
    id: `0192f0c4-7a8e-7000-8000-${local ? "000000000002" : "000000000003"}`,
    username,
    acct: local ? username : `${username}@${domain}`,
    domain,
    uri: `https://${domain}/users/${username}`,
    display_name: displayName,
    note: "",
    fields: [],
    emojis: [],
    badges: [],
    url: `https://${domain}/@${username}`,
    locked: false,
    bot: false,
    created_at: "2026-10-01T00:00:00Z",
  };
}

const bob = account("bob", "Bob", "my.example");
/** A long host that has to fit a 320 px card. */
const carol = account("carol", "Carol", "a-rather-long-community-server-name.social.example.com");

function status(n: number, author: Account): Status {
  return {
    id: `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`,
    uri: `https://${author.domain}/statuses/${String(n)}`,
    url: `https://${author.domain}/@${author.username}/${String(n)}`,
    account: author,
    content: `<p>Post number ${String(n)} on the federated timeline.</p>`,
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    language: "en",
    created_at: new Date(Date.now() - n * 7 * 60_000).toISOString(),
    media_attachments: [],
    mentions: [],
    tags: [],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
  };
}

function instance(federated: Instance["timelines"]["federated"]): Instance {
  return {
    domain: "my.example",
    name: "my example",
    version: "0.0.0-e2e",
    source_url: "https://github.com/mycroblog/my",
    registrations: { mode: "open" },
    limits: { max_post_characters: 500, require_media_description: false },
    timelines: { local: "everyone", federated },
    federation: { mode: "both" },
    sends_email: false,
    search_indexing: "deny",
    ai_usage: "deny",
    finance_visibility: "private",
    income_visibility: "private",
    supporters_enabled: false,
    supporter_list_visibility: "off",
    voice_enabled: false,
    voice_room_starters: "permitted",
    rules: [],
  };
}

/**
 * Answers a signed-in user's session (the home timeline and the user's navigation need one), GET /instance, the
 * home and the local timeline, and the federated timeline, which is a 404 while it is off.
 */
async function serveApi(page: Page, federated: boolean): Promise<void> {
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({ status: statusCode, contentType: statusCode < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }));
  await page.route("**/api/v1/timelines/home*", (route) => json(route, { items: [] } satisfies StatusPage));
  await page.route("**/api/v1/instance", (route) => json(route, instance(federated ? "everyone" : "off")));
  // The local timeline, which phones open first: its page has the federated timeline as its other tab.
  await page.route("**/api/v1/timelines/local*", (route) => json(route, { items: [status(1, bob)] } satisfies StatusPage));
  await page.route("**/api/v1/timelines/public*", (route) =>
    federated
      ? json(route, { items: [status(1, bob), status(2, carol), status(3, carol)] } satisfies StatusPage)
      : json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404),
  );
}

/** The navigation item of the federated timeline, in the tab bar below md and in the rail from md. */
function federatedLink(page: Page) {
  return page.getByRole("navigation").locator('a[href="/public"]');
}

/** Whether the project's viewport has the phones' tab bar rather than the rail (Tailwind's md). */
function hasTabBar(page: Page): boolean {
  return (page.viewportSize()?.width ?? 0) < 768;
}

test.describe("the federated timeline while the server serves it", () => {
  test.beforeEach(async ({ page }) => {
    await serveApi(page, true);
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  });

  test("opens from the navigation as a feed named by the page heading", async ({ page, hasTouch }) => {
    const heading = page.getByRole("heading", { level: 1 });
    // An item of its own at every width, the tab bar's included.
    await expect(federatedLink(page)).toBeVisible();
    const previous = (await heading.textContent()) ?? "";
    await openFromNavigation(page, "/public", hasTouch);

    await expect(page).toHaveURL(/\/public$/);
    // Where focus goes after a navigation is the shell's, and e2e/shell.spec.ts checks it.
    await expect(federatedLink(page)).toHaveAttribute("aria-current", "page");
    await expect(page.getByRole("navigation").locator('a[href="/local"]').filter({ visible: true })).not.toHaveAttribute("aria-current");
    // A screen of its own: no tabs to the local timeline.
    await expect(page.getByRole("main").getByRole("tablist")).toHaveCount(0);
    // The URL and the navigation can change before the new page renders its heading.
    await expect(heading).not.toHaveText(previous);
    const name = (await heading.textContent()) ?? "";
    const feed = page.getByRole("feed", { name });
    await expect(feed.getByRole("article")).toHaveCount(3);
    await expect(feed).toMatchAriaSnapshot(`
      - feed:
        - article:
          - link /Bob/
          - link:
            - time
          - paragraph
        - article:
          - link /Carol/
          - link:
            - time
          - paragraph
          - link /a-rather-long-community-server-name/
        - article:
          - link /Carol/
          - link:
            - time
          - paragraph
          - link /a-rather-long-community-server-name/
    `);
  });

  test("has no axe violations and does not scroll horizontally", async ({ page, makeAxeBuilder }) => {
    await page.goto("/public");
    await expect(page.getByRole("feed").getByRole("article")).toHaveCount(3);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("fits every item of the navigation in its width, with every label whole", async ({ page }) => {
    const nav = page.getByRole("navigation");
    // Six in the phones' tab bar (Home, Local, Federated, Notifications, Search and the menu button), eight in the
    // rail. A label that does not fit wraps, inside a word if it must, and is never cut.
    const items = nav.getByRole("listitem").filter({ visible: true });
    await expect(items).toHaveCount(hasTabBar(page) ? 6 : 8);
    await expect(nav.getByRole("button")).toBeVisible({ visible: hasTabBar(page) });
    const navBox = await nav.boundingBox();
    expect(navBox).not.toBeNull();
    for (const item of await items.all()) {
      const control = item.locator("a, button");
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      if (box === null || navBox === null) continue;
      expect(box.x).toBeGreaterThanOrEqual(navBox.x - 0.5);
      expect(box.x + box.width).toBeLessThanOrEqual(navBox.x + navBox.width + 0.5);
      // The label is never cut: nothing of it overflows its box ("Noti…" in a seven-item bar).
      const label = control.locator("span").last();
      const overflow = await label.evaluate((element) => element.scrollWidth - element.clientWidth);
      expect(overflow, `the label "${(await label.textContent()) ?? ""}" overflows`).toBeLessThanOrEqual(0);
    }
    await expectNoHorizontalScroll(page);
  });
});

test.describe("the federated timeline while the server has turned it off", () => {
  test.beforeEach(async ({ page }) => {
    await serveApi(page, false);
  });

  test("is not in the navigation", async ({ page }) => {
    const answered = page.waitForResponse("**/api/v1/instance");
    await page.goto("/home");
    await answered;
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    // The local timeline is there; the federated one never appears.
    await expect(page.getByRole("navigation").locator('a[href="/local"]').filter({ visible: true })).toBeVisible();
    await expect(federatedLink(page)).toHaveCount(0);
    // Home, Local, Notifications, Search and the menu button.
    if (hasTabBar(page)) await expect(page.getByRole("navigation").getByRole("listitem").filter({ visible: true })).toHaveCount(5);
  });

  test("says so at its address, without axe violations", async ({ page, makeAxeBuilder }) => {
    await page.goto("/public");
    await expect(page.getByRole("main").getByRole("link")).toHaveAttribute("href", "/home");
    await expect(page.getByRole("feed")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("the federated timeline while the server keeps it for signed-in users", () => {
  test.beforeEach(async ({ page }) => {
    // A visitor: no session to resume, and the timeline is a 401 without one.
    const json = (route: Route, body: unknown, statusCode = 200) =>
      route.fulfill({ status: statusCode, contentType: statusCode < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
    await page.route("**/api/v1/instance", (route) => json(route, instance("logged_in")));
    await page.route("**/api/v1/timelines/local*", (route) => json(route, { items: [status(1, bob)] } satisfies StatusPage));
    await page.route("**/api/v1/timelines/public*", (route) =>
      json(route, { type: "/api/v1/problems/unauthenticated", title: "Unauthenticated", status: 401 }, 401),
    );
  });

  test("is not in a visitor's navigation, and its address asks to sign in, without axe violations", async ({ page, makeAxeBuilder }) => {
    const answered = page.waitForResponse("**/api/v1/instance");
    await page.goto("/public");
    await answered;
    await expect(page.getByRole("main").locator('a[href="/login?return_to=%2Fpublic"]')).toBeVisible();
    await expect(page.getByRole("feed")).toHaveCount(0);
    await expect(page.getByRole("navigation").locator('a[href="/local"]').filter({ visible: true })).toBeVisible();
    await expect(federatedLink(page)).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
