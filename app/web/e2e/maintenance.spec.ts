// SPDX-License-Identifier: AGPL-3.0-only
// The server's maintenance in every width × locale project (ADR-0084 §3, web-client/09-app-shell.md §9 "メンテナンス"): when
// the client API and the OAuth endpoints answer with the maintenance problem, a reloaded page (the shell that a Service
// Worker or the preview still serves) shows the maintenance in place of the app, with what the server set, and once
// the API answers again after Retry-After it returns to the page by itself, still signed in. `vite preview` has no API,
// so the spec answers the OAuth refresh and the client API with `page.route`. Texts come from the project's catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** `text` as a literal in a regular expression. */
function escapeRegExp(text: string): string {
  return text.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);
}

/** The text of the message `key` with anything in place of its one placeholder `{name}`. */
function withAnything(key: string, name: string): RegExp {
  const [before = "", after = ""] = message(key).split(`{${name}}`);
  return new RegExp(`^${escapeRegExp(before)}.+${escapeRegExp(after)}$`);
}

const alice: Schemas["CredentialAccount"] = {
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
  discoverable: true,
  indexable: true,
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: [],
};

const post: Schemas["Status"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000c1",
  uri: "https://my.example/ap/statuses/0192f0c4-7a8e-7000-8000-0000000000c1",
  url: "https://my.example/@alice/status/0192f0c4-7a8e-7000-8000-0000000000c1",
  account: alice,
  content: "<p>A post on the local timeline.</p>",
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

const instance: Schemas["Instance"] = {
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
  voice_enabled: false,
  voice_room_starters: "permitted",
  rules: [],
};

/** Seconds the maintenance asks the app to wait. */
const retryAfter = 2;
const notice = "We are upgrading the database. Posts will be back soon.";
const statusPageUrl = "https://status.my.example/";

/**
 * Answers the API of a signed-in user; while `maintenance()` is true, every request to the client API and the OAuth
 * endpoints gets the maintenance problem instead. Returns the requests that carried the user's token.
 */
async function serveApi(page: Page, maintenance: () => boolean) {
  const seen = { authorized: 0 };
  const json = (route: Route, body: unknown) => {
    if (route.request().headers().authorization === "Bearer e2e-token") seen.authorized++;
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  };
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/accounts/verify_credentials", (route) => json(route, alice));
  await page.route("**/api/v1/preferences", (route) =>
    json(route, {
      locale: test.info().project.use.locale ?? "en",
      time_zone: "UTC",
      time_zone_chosen: true,
      default_visibility: "public",
      default_quote_policy: "public",
      default_post_language: "en",
      require_media_description: false,
      timeline_auto_insert: false,
      timeline_languages: [],
    } satisfies Schemas["Preferences"]),
  );
  await page.route("**/api/v1/notifications/marker", (route) =>
    json(route, { last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a1", unread_count: 0 }),
  );
  await page.route("**/api/v1/instance", (route) => json(route, instance));
  await page.route("**/api/v1/timelines/local*", (route) =>
    json(route, { items: [post] } satisfies Schemas["StatusPage"]),
  );
  // Registered last, so it is tried first: the server's maintenance mode (internal/interface/maintenance).
  const underMaintenance = (route: Route) => {
    if (!maintenance()) return route.fallback();
    return route.fulfill({
      status: 503,
      contentType: "application/problem+json",
      headers: { "Retry-After": String(retryAfter), "Cache-Control": "no-store" },
      body: JSON.stringify({
        type: "/api/v1/problems/maintenance",
        title: "Under maintenance",
        status: 503,
        message: notice,
        until: new Date(Date.now() + 60 * 60_000).toISOString(),
        status_page_url: statusPageUrl,
      }),
    });
  };
  await page.route("**/api/v1/**", underMaintenance);
  await page.route("**/oauth/**", underMaintenance);
  return seen;
}

test("the maintenance replaces the app with what the server says, and the app returns after it, still signed in", async ({
  page,
  makeAxeBuilder,
}) => {
  let maintenance = false;
  const seen = await serveApi(page, () => maintenance);
  await page.goto("/local");
  const feed = page.getByRole("feed").getByRole("article");
  await expect(feed).toHaveCount(1);

  maintenance = true;
  await page.reload();
  const heading = page.getByRole("heading", { level: 1, name: message("maintenance.title") });
  await expect(heading).toBeVisible();
  await expect(heading).toBeFocused();
  const main = page.getByRole("main");
  await expect(main).toContainText(message("maintenance.body"));
  const noticeText = main.getByText(notice);
  await expect(noticeText).toHaveAttribute("dir", "auto");
  await expect(main.getByText(withAnything("maintenance.until", "time"))).toBeVisible();
  await expect(
    main.getByRole("link", { name: new RegExp(escapeRegExp(message("maintenance.status_page"))) }),
  ).toHaveAttribute("href", statusPageUrl);
  await expect(main.getByRole("button", { name: message("maintenance.reload") })).toBeVisible();
  await expect(page.getByRole("navigation")).toHaveCount(0);
  await expect(page.getByRole("feed")).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);

  // After Retry-After the app asks again; once the API answers, it loads the page again, signed in as before.
  const before = seen.authorized;
  maintenance = false;
  await expect(feed).toHaveCount(1, { timeout: (retryAfter + 8) * 1000 });
  await expect(heading).toBeHidden();
  await expect.poll(() => seen.authorized).toBeGreaterThan(before);
});
