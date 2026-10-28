// SPDX-License-Identifier: AGPL-3.0-only
// The account search and the follow of an account on another server in every width × locale project
// (FR-305, FR-301, ADR-0027): from the keyboard only, the search resolves a handle, opens the profile, and
// sends a follow request that waits for the other server. `vite preview` has no API, so the spec answers
// the token refresh and the client API with `page.route`, as a signed-in user's server would. Names come
// from the project's catalog, so the spec finds the same elements in every locale.
import type { Locator, Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Account = components["schemas"]["Account"];
type Relationship = components["schemas"]["Relationship"];
/** The client message `key` in the project's locale, with its placeholders replaced by `values`. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return Object.entries(values).reduce((result, [name, value]) => result.replace(`{${name}}`, value), text);
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

/** A long host that has to fit a 320 px screen. */
const host = "a-rather-long-community-server-name.social.example.com";

const carol: Account = {
  id: "0192f0c4-7a8e-7000-8000-000000000003",
  username: "carol",
  acct: `carol@${host}`,
  domain: host,
  uri: `https://${host}/users/carol`,
  display_name: "Carol",
  note: "<p>Painter on another server.</p>",
  fields: [],
  emojis: [],
  badges: [],
  url: `https://${host}/@carol`,
  locked: false,
  bot: false,
  created_at: "2026-09-01T00:00:00Z",
  followers_count: 3,
  following_count: 4,
};

/**
 * Answers the client API as Alice's session. Carol is unknown until a search resolves her; a follow of her
 * is a request until her server accepts it, which it does not here. Returns the searches and the changes.
 */
async function serveApi(page: Page) {
  const searches: string[] = [];
  const changes: string[] = [];
  let relationship: Relationship = {
    id: carol.id,
    following: false,
    requested: false,
    followed_by: false,
    requested_by: false,
    show_boosts: true,
    notify: false,
    blocking: false,
    muting: false,
    muting_notifications: false,
  };
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
    const url = new URL(route.request().url());
    const path = url.pathname.replace("/api/v1", "");
    if (path === "/accounts/verify_credentials") return json(route, alice);
    if (path === "/instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0-e2e",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
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
        default_post_language: "en",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [] });
    if (path === "/accounts/lookup") {
      const acct = url.searchParams.get("acct") ?? "";
      searches.push(`${acct} resolve=${url.searchParams.get("resolve") ?? "false"}`);
      if (acct === `@${carol.acct}` || acct === carol.acct) return json(route, carol);
      return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
    }
    if (path === "/accounts/relationships") return json(route, [relationship]);
    if (path === `/accounts/${carol.id}/follow` && route.request().method() === "POST") {
      changes.push("follow");
      relationship = { ...relationship, requested: true };
      return json(route, relationship);
    }
    if (path === `/accounts/${carol.id}/unfollow` && route.request().method() === "POST") {
      changes.push("unfollow");
      relationship = { ...relationship, requested: false, following: false };
      return json(route, relationship);
    }
    if (path === `/accounts/${carol.id}/statuses`) return json(route, { items: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return { searches, changes };
}

/** Presses `key` until `target` has the focus, as a keyboard user moves through the page. */
async function pressUntilFocused(page: Page, key: "Tab" | "Shift+Tab", target: Locator): Promise<void> {
  for (let i = 0; i < 30; i++) {
    if (await target.evaluate((element) => element === document.activeElement)) return;
    await page.keyboard.press(key);
  }
  await expect(target).toBeFocused();
}

test.describe("the account search", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/search");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
  });

  test("resolves a handle and follows the account from the keyboard only, waiting for its server", async ({
    page,
    makeAxeBuilder,
  }) => {
    const server = await serveApi(page);
    await page.reload();
    const field = page.getByRole("searchbox", { name: message("search.label") });
    await expect(field).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await pressUntilFocused(page, "Tab", field);
    await page.keyboard.type(`@${carol.acct}`);
    await page.keyboard.press("Enter");

    await expect(page).toHaveURL((url) => url.pathname === `/@carol@${host}`);
    const heading = page.getByRole("heading", { level: 1 });
    await expect(heading).toHaveText("Carol");
    await expect(heading).toBeFocused();
    expect(server.searches).toEqual([`@${carol.acct} resolve=true`]);

    // The relationship buttons come before the name in the header.
    const follow = page.getByRole("main").getByRole("button", { name: message("profile.follow"), exact: true });
    await pressUntilFocused(page, "Shift+Tab", follow);
    await page.keyboard.press("Enter");

    const requested = page
      .getByRole("main")
      .getByRole("button", { name: message("profile.follow.requested"), exact: true });
    await expect(requested).toBeVisible();
    await expect(requested).toHaveAccessibleDescription(
      message("profile.follow.pending_remote", { host: `⁨${host}⁩` }),
    );
    await expect(requested).toBeFocused();
    expect(server.changes).toEqual(["follow"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Going back shows the search that found her.
    await page.goBack();
    await expect(page.getByRole("searchbox")).toHaveValue(`@${carol.acct}`);
  });

  test("marks the field and says why when nothing is found", async ({ page, makeAxeBuilder }) => {
    await serveApi(page);
    await page.reload();
    const field = page.getByRole("searchbox", { name: message("search.label") });
    await field.fill("@nobody@elsewhere.example");
    await field.press("Enter");

    await expect(field).toHaveAttribute("aria-invalid", "true");
    await expect(field).toHaveAccessibleDescription(
      new RegExp(message("search.not_found").replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`)),
    );
    await expect(field).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("is a search landmark with its field and button", async ({ page }) => {
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - heading [level=1]
      - search:
        - searchbox
        - button
    `);
  });
});
