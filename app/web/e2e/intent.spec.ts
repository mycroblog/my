// SPDX-License-Identifier: AGPL-3.0-only
// The page that the follow buttons of other servers open (the Follow intent of WebFinger, FEP-3b86,
// web-client/screens/intent.md), at every width: the address the other server sent fits the screen, the page
// looks nothing up until the button is pressed, and the button opens the account's profile to follow. `vite preview`
// has no API, so the spec answers the token refresh and the client API with `page.route`, as a signed-in user's
// server would. Names come from the project's catalog, so the spec finds the same elements in every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { preferencesOf } from "./preferences.ts";

type Account = components["schemas"]["Account"];
type Relationship = components["schemas"]["Relationship"];

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
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

/** A long host and path that have to fit a 320 px screen. */
const host = "a-rather-long-community-server-name.social.example.com";

const carol: Account = {
  id: "0192f0c4-7a8e-7000-8000-000000000003",
  username: "carol",
  acct: `carol@${host}`,
  domain: host,
  uri: `https://${host}/users/carol-the-painter-of-very-long-landscapes`,
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

const relationship: Relationship = {
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

/** Answers the client API as Alice's session; Carol is found by her actor's URL. Returns the lookups. */
async function serveApi(page: Page) {
  const lookups: string[] = [];
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
        full_text_search: { available: false, visibilities: [] },
        translation: { browser: false, providers: [] },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (path === "/preferences") {
      // The user's locale is the project's: the app displays itself in the signed-in user's locale.
      return json(
        route,
        preferencesOf({ locale: test.info().project.use.locale ?? "en", default_post_language: "en" }),
      );
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/accounts/lookup") {
      const acct = url.searchParams.get("acct") ?? "";
      lookups.push(`${acct} resolve=${url.searchParams.get("resolve") ?? "false"}`);
      if (acct === carol.uri) return json(route, carol);
      return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
    }
    if (path === "/accounts/relationships") return json(route, { items: [relationship] });
    if (path === `/accounts/${carol.id}/statuses`) return json(route, { items: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return { lookups };
}

test.describe("the page of the Follow intent", () => {
  test("shows the address it was sent and opens the profile to follow once the button is pressed", async ({
    page,
    makeAxeBuilder,
  }) => {
    const server = await serveApi(page);
    await page.goto(`/intent?follow=${encodeURIComponent(carol.uri)}`);

    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1, name: message("intent.title") })).toBeVisible();
    await expect(main.getByText(carol.uri, { exact: true })).toBeVisible();
    const open = main.getByRole("button", { name: message("intent.follow.submit"), exact: true });
    await expect(open).toBeVisible();
    // Opening the page looks nothing up.
    expect(server.lookups).toEqual([]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await open.press("Enter");

    await expect(page).toHaveURL((url) => url.pathname === `/@carol@${host}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Carol");
    await expect(main.getByRole("button", { name: message("profile.follow"), exact: true })).toBeVisible();
    expect(server.lookups).toEqual([`${carol.uri} resolve=true`]);
  });
});
