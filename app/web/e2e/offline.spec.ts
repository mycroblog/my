// SPDX-License-Identifier: AGPL-3.0-only
// Going offline and back in every width × locale project (web-client/16-look-states-and-wording.md G17, ADR-0027): the app's one offline bar
// comes and goes as a polite live region, a page opened offline says why it cannot load instead of loading
// forever, and back online it loads by itself; the home timeline says offline that it shows the posts loaded last
// (FR-604). `vite preview` has no API, so the spec answers GET /instance and
// the timelines with `page.route`. `context.setOffline` takes the network away; a route still answers then, so the
// spec's routes fail their requests while offline, as the network would.
// Texts come from the project's catalog, so the spec finds the same elements in every locale.
import type { BrowserContext, Page, Route } from "@playwright/test";
import type { components } from "../src/lib/api/schema.gen.ts";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, openFromNavigation as openFromNavigationItem, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Instance = components["schemas"]["Instance"];
type Status = components["schemas"]["Status"];
type StatusPage = components["schemas"]["StatusPage"];
/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

const bob: Status["account"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000002",
  display_name: "Bob",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@bob",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};

const post: Status = {
  id: "0192f0c4-7a8e-7000-8000-0000000000c1",
  uri: "https://my.example/ap/statuses/0192f0c4-7a8e-7000-8000-0000000000c1",
  url: "https://my.example/@bob/status/0192f0c4-7a8e-7000-8000-0000000000c1",
  account: bob,
  content: "<p>A post to read while online.</p>",
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 7 * 60_000).toISOString(),
  media_attachments: [],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
};

/** The instance's status page, on another site (FR-810), which the offline bar links to. */
const statusPageUrl = "https://status.my.example/";

const instance: Instance = {
  domain: "my.example",
  name: "my example",
  version: "0.0.0-e2e",
  source_url: "https://github.com/mycroblog/my",
  registrations: { mode: "open" },
  limits: { max_post_characters: 500, require_media_description: false },
  timelines: { local: "everyone", federated: "everyone" },
  federation: { mode: "both" },
  sends_email: false,
  search_indexing: "deny",
  ai_usage: "deny",
  finance_visibility: "private",
  income_visibility: "private",
  supporters_enabled: false,
  supporter_list_visibility: "off",
  rules: [],
  status_page_url: statusPageUrl,
};

/** Whether the spec has taken the network away. */
let offline = false;

async function setOffline(context: BrowserContext, value: boolean): Promise<void> {
  offline = value;
  await context.setOffline(value);
}

/** Answers the API; the federated timeline fails the first time, so that its page has nothing cached to show. */
async function serveApi(page: Page): Promise<void> {
  const json = (route: Route, body: unknown, status = 200) =>
    offline
      ? route.abort("internetdisconnected")
      : route.fulfill({ status, contentType: status < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
  let publicRequests = 0;
  await page.route("**/api/v1/instance", (route) => json(route, instance));
  await page.route("**/api/v1/timelines/local*", (route) => json(route, { items: [post] } satisfies StatusPage));
  await page.route("**/api/v1/timelines/public*", (route) => {
    if (!offline) publicRequests++;
    return publicRequests === 1
      ? json(route, { type: "about:blank", title: "Service Unavailable", status: 503 }, 503)
      : json(route, { items: [post] } satisfies StatusPage);
  });
}

/** Opens a page from the navigation, as the user would. */
async function openFromNavigation(page: Page, path: string, hasTouch: boolean): Promise<void> {
  await openFromNavigationItem(page, path, hasTouch);
  await expect(page).toHaveURL(new RegExp(`${path}$`));
}

/** The offline bar: the status region with its message (the announcer's region is another one). */
function offlineBar(page: Page) {
  return page.getByRole("status").filter({ hasText: message("shell.offline") });
}

test.beforeEach(async ({ page, hasTouch }) => {
  offline = false;
  await serveApi(page);
  // Both pages' code is loaded while online, as a Service Worker would serve it offline (e2e/pwa.spec.ts); the
  // federated timeline failed, so it has no posts cached.
  await page.goto("/public");
  await expect(page.getByRole("main").getByRole("button")).toBeVisible();
  await openFromNavigation(page, "/local", hasTouch);
  await expect(page.getByRole("feed").getByRole("article")).toHaveCount(1);
});

test("shows the offline bar while offline and announces it", async ({ page, context, makeAxeBuilder }) => {
  // The live region is in the page, empty, before anything is to be announced.
  const region = page.locator('[role="status"]:not([data-live-announcer])').first();
  await expect(region).toBeAttached();
  await expect(region).toBeEmpty();

  await setOffline(context, true);
  await expect(offlineBar(page)).toBeVisible();
  // A polite live region of the message alone, so a screen reader reads it once it appears.
  await expect(offlineBar(page)).toMatchAriaSnapshot(`
    - status:
      - paragraph: ${JSON.stringify(message("shell.offline"))}
  `);
  // The page read before stays.
  await expect(page.getByRole("feed").getByRole("article")).toHaveCount(1);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);

  await setOffline(context, false);
  await expect(offlineBar(page)).toHaveCount(0);
  await expect(region).toBeEmpty();
});

test("the offline bar links to the server's status page read while online (FR-810)", async ({ page, context, makeAxeBuilder }) => {
  const link = page.locator(`a[href="${statusPageUrl}"]`);
  await expect(link).toHaveCount(0);

  await setOffline(context, true);
  await expect(offlineBar(page)).toBeVisible();
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  // After the message, outside its live region: what is announced is the message alone.
  await expect(offlineBar(page).locator("a")).toHaveCount(0);
  await expect(offlineBar(page).locator("xpath=..")).toMatchAriaSnapshot(`
    - status:
      - paragraph: ${JSON.stringify(message("shell.offline"))}
    - link
  `);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);

  await setOffline(context, false);
  await expect(link).toHaveCount(0);
});

test("a page opened offline says why it cannot load, and loads back online", async ({ page, context, hasTouch }) => {
  await setOffline(context, true);
  await expect(offlineBar(page)).toBeVisible();

  await openFromNavigation(page, "/public", hasTouch);
  await expect(page.getByRole("main").getByText(message("api.error.offline"))).toBeVisible();
  await expect(page.getByRole("main").getByRole("progressbar")).toHaveCount(0);

  await setOffline(context, false);
  await expect(offlineBar(page)).toHaveCount(0);
  await expect(page.getByRole("feed").getByRole("article")).toHaveCount(1);
});

test("the home timeline says that it shows the posts loaded last while offline (FR-604)", async ({ page, context, makeAxeBuilder }) => {
  // A signed-in session. Registered after serveApi's routes, so Playwright asks this one first; it leaves the
  // instance and the public timelines to them.
  const json = (route: Route, body: unknown, status = 200) =>
    offline
      ? route.abort("internetdisconnected")
      : route.fulfill({ status, contentType: status < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow" }));
  await page.route("**/api/v1/**", (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^\/api\/v1/, "");
    if (path === "/instance" || path.startsWith("/timelines/local") || path.startsWith("/timelines/public")) return route.fallback();
    if (path.startsWith("/timelines/home")) return json(route, { items: [post] } satisfies StatusPage);
    if (path === "/accounts/verify_credentials") return json(route, { ...bob, source: { note: "", fields: [] }, permissions: [] });
    if (path === "/preferences") {
      const locale = test.info().project.use.locale ?? "en";
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "en", require_media_description: false, timeline_languages: [] });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  await page.goto("/home");
  const feed = page.getByRole("feed");
  await expect(feed.getByRole("article")).toHaveCount(1);
  // The notice's text before the time it was loaded, in the project's locale.
  const notice = page.getByRole("main").getByText(message("timeline.offline.home").split("{time}")[0]?.trim() ?? "", { exact: false });
  await expect(notice).toHaveCount(0);

  await setOffline(context, true);
  await expect(offlineBar(page)).toBeVisible();
  await expect(notice).toBeVisible();
  await expect(feed.getByRole("article")).toHaveCount(1);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);

  await setOffline(context, false);
  await expect(notice).toHaveCount(0);
  await expect(feed.getByRole("article")).toHaveCount(1);
});
