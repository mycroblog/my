// SPDX-License-Identifier: AGPL-3.0-only
// The keyword filters (ADR-0157; web-client/screens/settings-filters.md) and the posts they fold: a filter is added from
// its settings page and listed, and a post of the home timeline that the server marked in `filtered` shows only what it
// matched until "Show", axe-clean and without horizontal scroll at every step. `vite preview` has no API, so the spec
// answers the token refresh and the client API with `page.route`, as a signed-in user's server would. Names come from
// the project's catalog, so the spec finds the same elements in every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { escapeRegExp, expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { preferencesOf } from "./preferences.ts";

type Account = components["schemas"]["Account"];
type KeywordFilter = components["schemas"]["KeywordFilter"];
type KeywordFilterRequest = components["schemas"]["KeywordFilterRequest"];
type Status = components["schemas"]["Status"];

/** The client message `key` in the project's locale, its arguments left as they are. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** A pattern of the message `key` whose arguments may be anything (a name in directional isolates). */
function pattern(key: string): RegExp {
  return new RegExp(
    `^${message(key)
      .split(/\{[a-z]+\}/u)
      .map(escapeRegExp)
      .join(".+")}$`,
    "u",
  );
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

/** A post of Bob that Alice's filter "Season finale" folds. */
const foldedPost: Status = {
  id: "0192f0c4-7a8e-7000-8000-000000000101",
  uri: "https://my.example/ap/statuses/0192f0c4-7a8e-7000-8000-000000000101",
  url: "https://my.example/@bob/status/0192f0c4-7a8e-7000-8000-000000000101",
  account: bob,
  content: "<p>The finale was a dream.</p>",
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 60 * 60_000).toISOString(),
  media_attachments: [],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
  filtered: [
    { filter: { id: "0192f0c4-7a8e-7000-8000-0000000f0001", title: "Season finale" }, keyword_matches: ["finale"] },
  ],
  favourited: false,
  boosted: false,
  bookmarked: false,
};

/** Answers the client API as Alice's session: she has no filter at first, and her home timeline has one folded post. */
async function serveApi(page: Page): Promise<void> {
  let filters: KeywordFilter[] = [];
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
    const method = request.method();
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
      return json(
        route,
        preferencesOf({ locale: test.info().project.use.locale ?? "en", default_post_language: "en" }),
      );
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [foldedPost] });
    if (path === "/filters" && method === "GET") return json(route, { items: filters });
    if (path === "/filters" && method === "POST") {
      const body = request.postDataJSON() as KeywordFilterRequest;
      const filter: KeywordFilter = {
        id: `0192f0c4-7a8e-7000-8000-0000000f${String(filters.length + 1).padStart(4, "0")}`,
        title: body.title,
        action: body.action,
        contexts: body.contexts,
        keywords: body.keywords,
        expired: false,
        created_at: new Date().toISOString(),
        ...(body.expires_at === undefined ? {} : { expires_at: body.expires_at }),
      };
      filters = [...filters, filter];
      return json(route, filter, 201);
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

test.describe("keyword filters", () => {
  test("a filter is added from its settings page, and a post it folds shows only what it matched until shown", async ({
    page,
    makeAxeBuilder,
  }) => {
    await serveApi(page);
    await page.goto("/settings/filters");
    await expect(page.getByRole("heading", { level: 1, name: message("filters.title") })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);

    // Add: the button, the name, a word, Add.
    const main = page.getByRole("main");
    await main.getByRole("button", { name: message("filters.add") }).click();
    const form = main.getByRole("region", { name: message("filters.add") });
    await form
      .getByRole("textbox", { name: new RegExp(escapeRegExp(message("filters.form.title"))) })
      .fill("Season finale");
    await form.getByRole("textbox", { name: pattern("filters.form.word") }).fill("finale");
    await expect(form.getByText(message("filters.form.whole_word.note"))).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await form.getByRole("button", { name: message("filters.form.add"), exact: true }).click();
    await expect(main.getByRole("status")).toHaveText(pattern("filters.added"));
    await expect(main.getByRole("listitem")).toHaveCount(1);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The home timeline: the folded post is an article named by what it matched, without the post's text.
    await page.goto("/home");
    const folded = page.getByRole("article", { name: pattern("filters.folded") });
    await expect(folded).toBeVisible();
    await expect(page.getByText("The finale was a dream.")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await folded.getByRole("button", { name: message("filters.folded.show") }).click();
    await expect(page.getByText("The finale was a dream.")).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
  });
});
