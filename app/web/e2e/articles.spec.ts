// SPDX-License-Identifier: AGPL-3.0-only
// Reading articles (FR-122, ADR-0160): an article's card on the home timeline, of this server and of another one, and
// the article's page with its title as the page heading and its body in full, each axe-clean and without horizontal
// scrolling (also at 320 px: the spec is in `widthSpecs`). `vite preview` has no API, so the spec answers the token
// refresh and the client API with `page.route`. Names come from the project's catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, servedImage, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { preferencesOf } from "./preferences.ts";

type Status = components["schemas"]["Status"];
type MediaAttachment = components["schemas"]["MediaAttachment"];

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
const carol: Status["account"] = {
  ...alice,
  id: "0192f0c4-7a8e-7000-8000-000000000003",
  username: "carol",
  acct: "carol@remote.example",
  domain: "remote.example",
  uri: "https://remote.example/users/carol",
  display_name: "Carol",
  url: "https://remote.example/@carol",
};

const coverUrl = servedImage(
  `data:image/svg+xml,${encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900"><rect width="1600" height="900" fill="#5c7a4f"/><circle cx="800" cy="450" r="220" fill="#e0533d"/></svg>',
  )}`,
);
const cover: MediaAttachment = {
  id: "0192f0c4-7a8e-7000-8000-0000000000c1",
  type: "image",
  state: "ready",
  meta: { width: 1600, height: 900, focus: { x: 0, y: 0 } },
  url: coverUrl,
  preview_url: coverUrl,
  description: "A tomato on green leaves",
};

/** A status of the timeline with `overrides`. */
function statusOf(id: string, account: Status["account"], overrides: Partial<Status>): Status {
  return {
    id,
    uri: `https://${account.domain}/statuses/${id}`,
    account,
    content: "",
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
    filtered: [],
    favourited: false,
    boosted: false,
    bookmarked: false,
    ...overrides,
  };
}

const articleId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const title = "What I learned growing tomatoes on a balcony, three summers in a row";
const summary = "Two pots, one stubborn plant, and a long list of what the soil, the water and the sun asked for.";
/** Alice's article as lists carry it: no body. */
const article = statusOf(articleId, alice, { article: { title, summary, cover } });
/** The same article as GET /statuses/{id} answers it, with a body whose table is wider than a phone. */
const articleInFull: Status = {
  ...article,
  content: [
    "<h2>Soil</h2><p>Use a deep pot, and mix compost into the soil before the first planting of the season.</p>",
    "<hr>",
    "<h3>What to plant when</h3>",
    "<table><thead><tr><th>Month</th><th>Seedlings</th><th>Watering</th><th>Notes for a balcony that faces south</th></tr></thead>",
    "<tbody><tr><td>April</td><td>Indoors, by the window</td><td>Every other day</td><td>Keep them out of the cold nights</td></tr></tbody></table>",
  ].join(""),
};
/** Carol's sensitive article of another server, without a title. */
const remoteArticleId = "0192f0c4-7a8e-7000-8000-0000000000f2";
const remoteArticle = statusOf(remoteArticleId, carol, {
  url: "https://remote.example/@carol/articles/1",
  sensitive: true,
  article: { title: "", summary: "A note on the harvest.", cover },
});

/** Answers the client API as Alice's session. */
async function serveApi(page: Page): Promise<void> {
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
    const path = new URL(route.request().url()).pathname.replace("/api/v1", "");
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
        limits: { max_post_characters: 500 },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (path === "/preferences") {
      return json(
        route,
        preferencesOf({ locale: test.info().project.use.locale ?? "en", default_post_language: "en" }),
      );
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [article, remoteArticle] });
    if (path === "/custom_emojis") return json(route, { items: [] });
    if (path === `/statuses/${articleId}/context`) return json(route, { ancestors: [], descendants: [] });
    if (path === `/statuses/${articleId}`) return json(route, articleInFull);
    if (path === `/statuses/${remoteArticleId}/context`) return json(route, { ancestors: [], descendants: [] });
    if (path === `/statuses/${remoteArticleId}`) {
      return json(route, { ...remoteArticle, content: "<p>The harvest was small but sweet.</p>" });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

test.describe("articles on the home timeline", () => {
  test.beforeEach(async ({ page }) => {
    await serveApi(page);
    await page.goto("/home");
  });

  test("are cards with their title, summary and cover, axe-clean and without horizontal scrolling", async ({
    page,
    makeAxeBuilder,
  }) => {
    const feed = page.getByRole("main").getByRole("feed");
    const local = feed.getByRole("article").filter({ has: page.getByRole("link", { name: title, exact: true }) });
    await expect(local.getByRole("link", { name: title, exact: true })).toHaveAttribute(
      "href",
      `/@alice/status/${articleId}`,
    );
    await expect(local.getByText(summary, { exact: true })).toBeVisible();
    await expect(local.getByRole("img", { name: "A tomato on green leaves", exact: true })).toBeVisible();

    // Another server's article reads the same; without a title, and sensitive, its cover stays hidden.
    const untitled = message("status.article.untitled");
    const remote = feed.getByRole("article").filter({ has: page.getByRole("link", { name: untitled, exact: true }) });
    await expect(remote.getByText("A note on the harvest.", { exact: true })).toBeVisible();
    await expect(remote.getByRole("img", { name: "A tomato on green leaves", exact: true })).toHaveCount(0);
    await expect(remote.getByRole("button", { name: message("timeline.media.reveal"), exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("open the article's page, headed by its title, with its body", async ({ page, makeAxeBuilder }) => {
    await page.getByRole("main").getByRole("link", { name: title, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/@alice/status/${articleId}$`));
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1, name: title, exact: true })).toBeVisible();
    await expect(main.getByRole("heading", { level: 2, name: "Soil", exact: true })).toBeVisible();
    await expect(main.getByRole("table")).toBeVisible();
    await expect(main.getByRole("img", { name: "A tomato on green leaves", exact: true })).toBeVisible();
    await expect(
      main.getByRole("heading", { level: 2, name: message("status.thread.replies"), exact: true }),
    ).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("the page of a sensitive article", () => {
  test("shows its summary and folds its cover and body until asked", async ({ page, makeAxeBuilder }) => {
    await serveApi(page);
    await page.goto(`/@carol@remote.example/status/${remoteArticleId}`);
    const main = page.getByRole("main");
    await expect(
      main.getByRole("heading", { level: 1, name: message("status.article.untitled"), exact: true }),
    ).toBeVisible();
    await expect(main.getByText("A note on the harvest.", { exact: true })).toBeVisible();
    const body = main.getByText("The harvest was small but sweet.", { exact: true });
    await expect(body).toBeHidden();
    await expectNoAxeViolations(makeAxeBuilder);

    const show = main.getByRole("button", { name: message("status.article.show"), exact: true });
    await show.click();
    await expect(show).toHaveAttribute("aria-expanded", "true");
    await expect(body).toBeVisible();
    await expect(main.getByRole("img", { name: "A tomato on green leaves", exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
