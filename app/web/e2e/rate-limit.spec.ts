// SPDX-License-Identifier: AGPL-3.0-only
// The server's rate limit in every width × locale project (web-client/09-app-shell.md §9 "外枠の読み込み" and "レート制限", ADR-0027):
// when what the shell reads on every page is refused with 429, a reloaded page stays in the user's locale, says in the
// top bar when it loads again (Retry-After), and loads again by itself then. `vite preview` has no API, so the spec
// answers the OAuth refresh and the client API with `page.route`. Texts come from the project's catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];
/** The project's locale, which the fixture writes into <html lang> as the instance's. */
function projectLocale(): string {
  return test.info().project.use.locale ?? "en";
}

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const catalog = readCatalog(projectLocale());
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${projectLocale()}`);
  return text;
}

/** `text` as a literal in a regular expression. */
function escapeRegExp(text: string): string {
  return text.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);
}

/** The rate-limit bar's text, with any time of day in place of {time}. */
function rateLimitedText(): RegExp {
  const [before = "", after = ""] = message("shell.rate_limited").split("{time}");
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
  statuses_count: 12,
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

/** Seconds the rate limit asks the app to wait. */
const retryAfter = 3;

/**
 * Answers the API of a signed-in user whose preferences are the project's locale; while `limited()` is true, the
 * preferences and the notification marker are refused by the rate limit. Returns how many preferences requests came.
 */
async function serveApi(page: Page, limited: () => boolean) {
  const counts = { preferences: 0 };
  const json = (route: Route, body: unknown) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  const tooMany = (route: Route) =>
    route.fulfill({
      status: 429,
      contentType: "application/problem+json",
      headers: { "Retry-After": String(retryAfter) },
      body: JSON.stringify({
        type: "about:blank",
        title: "Too many requests.",
        status: 429,
      } satisfies Schemas["Problem"]),
    });
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/accounts/verify_credentials", (route) => json(route, alice));
  await page.route("**/api/v1/preferences", (route) => {
    counts.preferences++;
    if (limited()) return tooMany(route);
    const preferences: Schemas["Preferences"] = {
      locale: projectLocale(),
      time_zone: "UTC",
      time_zone_chosen: true,
      default_visibility: "public",
      default_quote_policy: "public",
      default_post_language: "en",
      require_media_description: false,
      timeline_auto_insert: false,
      timeline_languages: [],
    };
    return json(route, preferences);
  });
  await page.route("**/api/v1/notifications/marker", (route) =>
    limited() ? tooMany(route) : json(route, { last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a1", unread_count: 0 }),
  );
  await page.route("**/api/v1/instance", (route) => json(route, instance));
  await page.route("**/api/v1/timelines/local*", (route) =>
    json(route, { items: [post] } satisfies Schemas["StatusPage"]),
  );
  return counts;
}

test("a reloaded page stays in the user's locale while the rate limit refuses the shell, says when it loads again, and does", async ({
  page,
  makeAxeBuilder,
}) => {
  let limited = false;
  const counts = await serveApi(page, () => limited);
  await page.goto("/local");
  await expect(page.getByRole("feed").getByRole("article")).toHaveCount(1);
  await expect.poll(() => counts.preferences).toBeGreaterThan(0);

  limited = true;
  await page.reload();
  await expect(page.getByRole("feed").getByRole("article")).toHaveCount(1);
  const bar = page.getByRole("status").filter({ hasText: rateLimitedText() });
  await expect(bar).toBeVisible();
  await expect(bar).toMatchAriaSnapshot(`
    - status:
      - paragraph: ${rateLimitedText().toString()}
  `);
  await expect(page.locator("html")).toHaveAttribute("lang", projectLocale());
  await expect(page.getByRole("navigation")).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);

  // At the time Retry-After names, the shell loads again by itself, and the bar goes.
  const refused = counts.preferences;
  limited = false;
  await expect(bar).toBeHidden({ timeout: (retryAfter + 7) * 1000 });
  expect(counts.preferences).toBeGreaterThan(refused);
  await expect(page.locator("html")).toHaveAttribute("lang", projectLocale());
});
