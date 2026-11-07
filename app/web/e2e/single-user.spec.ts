// SPDX-License-Identifier: AGPL-3.0-only
// A one-person server (ADR-0152; docs/architecture/web-client/screens/landing.md, profile.md, 09-app-shell.md) in every
// width × locale project: its front page is its operator's profile, with signing in and the server's information and
// documents, and the navigation offers only what the server does: the home timeline of the timelines, and no
// sign-up, suggestions or invites. `vite preview` has no API, so the spec plays it with page.route.
// Names change with the locale, so elements are found by role, structure and addresses.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

type Account = components["schemas"]["Account"];
type Instance = components["schemas"]["Instance"];

/** Olive, the operator: the server's one local account. */
const olive: Account = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "olive",
  acct: "olive",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Olive",
  note: "<p>Gardening, mostly.</p>",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@olive",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  followers_count: 12,
  following_count: 0,
};

/** The paths of the API that a one-person server answers with a 404 of the type single-user-mode. */
const communityPaths = ["/timelines/local", "/timelines/federated", "/accounts/suggestions", "/invites", "/admin/"];

/**
 * Plays the client API of a one-person server of Olive: GET /instance says so, and her account, posts and profile
 * answer; a session resumes when `signedIn`. Returns the paths that the app asked for.
 */
async function serveOnePersonServer(page: Page, origin: string, signedIn: boolean): Promise<string[]> {
  const asked: string[] = [];
  const instance: Instance = {
    domain: "my.example",
    name: "my example",
    version: "0.0.0-e2e",
    source_url: "https://github.com/mycroblog/my",
    login_providers_only: false,
    registrations: { mode: "closed" },
    limits: { max_post_characters: 500, require_media_description: false },
    timelines: { local: "off", federated: "off" },
    federation: { mode: "both" },
    sends_email: true,
    search_indexing: "deny",
    ai_usage: "deny",
    link_cards: true,
    link_cards_ignore_robots_txt: false,
    finance_visibility: "private",
    income_visibility: "private",
    supporters_enabled: false,
    supporter_list_visibility: "off",
    voice_enabled: false,
    voice_room_starters: "permitted",
    full_text_search: { available: false, visibilities: [] },
    translation: { browser: false, providers: [] },
    single_user: true,
    single_user_account: olive,
    rules: [],
    terms_url: `${origin}/about/terms`,
    privacy_policy_url: `${origin}/about/privacy`,
  };
  const json = (route: Route, body: unknown, status = 200) =>
    route.fulfill({
      status,
      contentType: status < 400 ? "application/json" : "application/problem+json",
      body: JSON.stringify(body),
    });
  if (signedIn) {
    await page.route("**/oauth/token/refresh", (route) =>
      json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
    );
  }
  await page.route("**/api/v1/**", (route) => {
    const path = new URL(route.request().url()).pathname.replace("/api/v1", "");
    asked.push(path);
    if (path === "/instance") return json(route, instance);
    if (path === "/accounts/lookup") return json(route, olive);
    if (signedIn && path === "/accounts/verify_credentials")
      return json(route, {
        ...olive,
        statuses_count: 0,
        permissions: [],
        follow_requests_count: 0,
        posting: { visibility_ceiling: "public", visibilities: ["public", "unlisted", "private", "direct"] },
      });
    if (signedIn && path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home" || path === "/notifications" || path.endsWith("/statuses"))
      return json(route, { items: [] });
    if (communityPaths.some((community) => path.startsWith(community)))
      return json(route, { type: "/api/v1/problems/single-user-mode", title: "Not Found", status: 404 }, 404);
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return asked;
}

/** The paths that a one-person server does not offer among `asked`. */
function community(asked: readonly string[]): string[] {
  return asked.filter((path) => communityPaths.some((prefix) => path.startsWith(prefix)));
}

test.describe("the front page of a one-person server", () => {
  let asked: string[] = [];

  test.beforeEach(async ({ page, baseURL }) => {
    asked = await serveOnePersonServer(page, new URL(baseURL ?? "").origin, false);
    await page.goto("/");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toHaveText("Olive");
  });

  test("is the operator's profile, with signing in and the server's information and documents", async ({ page }) => {
    await expect(page).toHaveURL(/\/@olive$/);
    const navigation = page.getByRole("navigation").first();
    // Signing in is the bar's call to action, shown at every width.
    await expect(navigation.locator('a[href^="/login"]').filter({ visible: true }).first()).toBeVisible();
    await expect(navigation.locator('a[href$="/auth/sign_up"]')).toHaveCount(0);
    await expect(navigation.locator('a[href="/local"]')).toHaveCount(0);
    await expect(navigation.locator('a[href="/public"]')).toHaveCount(0);
    const footer = page.getByRole("contentinfo");
    await expect(footer.locator('a[href="/about"]')).toBeVisible();
    await expect(footer.locator('a[href$="/about/terms"]')).toBeVisible();
    await expect(footer.locator('a[href$="/about/privacy"]')).toBeVisible();
    expect(community(asked)).toEqual([]);
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("the navigation of a one-person server", () => {
  test("offers the home timeline but neither public timeline, and asks for nothing the server does not offer", async ({
    page,
    baseURL,
  }) => {
    const asked = await serveOnePersonServer(page, new URL(baseURL ?? "").origin, true);
    await page.goto("/home");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeAttached();
    const navigation = page.getByRole("navigation").first();
    await expect(navigation.locator('a[href="/home"]').first()).toBeAttached();
    await expect(navigation.locator('a[href="/local"]')).toHaveCount(0);
    await expect(navigation.locator('a[href="/public"]')).toHaveCount(0);

    // The settings list no invites.
    await page.goto("/settings");
    await expect(page.getByRole("main").locator('a[href="/settings/profile"]')).toBeVisible();
    await expect(page.locator('a[href="/settings/invites"]')).toHaveCount(0);
    expect(community(asked)).toEqual([]);
  });
});
