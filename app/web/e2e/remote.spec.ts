// SPDX-License-Identifier: AGPL-3.0-only
// Accounts and posts of other servers in every width × locale project (FR-006, ADR-0027): the full handle,
// the links to the profile and the post on their own server, the content warning, and the pictures that
// come through the media proxy, one without a description. `vite preview` has no API, so the spec answers
// the token refresh and the client API with `page.route`, as a signed-in user's server would. Names come
// from the project's catalog, so the spec finds the same elements in every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, servedImage, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Account = components["schemas"]["Account"];
type MediaAttachment = components["schemas"]["MediaAttachment"];
type Status = components["schemas"]["Status"];
/** The client message `key` in the project's locale, with its `{host}` replaced. */
function message(key: string, host: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text.replace("{host}", host);
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

/** A long host that has to fit a 320 px card. */
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
};

/** A picture served by the page's fixture, so that no request leaves the preview; it stands in for the media proxy URL. */
const picture = servedImage(`data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 90"><rect width="160" height="90" fill="#4f6d8f"/></svg>')}`);

/** An image of a remote post: no renditions here, the proxy's URL as `url` and `preview_url`. */
function remoteMedia(id: string, description: string | undefined): MediaAttachment {
  return {
    id: `0192f0c4-7a8e-7000-8000-0000000003${id}`,
    type: "image",
    state: "ready",
    meta: { focus: { x: 0, y: 0 } },
    url: picture,
    preview_url: picture,
    remote_url: `https://${host}/files/${id}.png`,
    ...(description === undefined ? {} : { description }),
  };
}

const postId = "0192f0c4-7a8e-7000-8000-0000000003c1";
const post: Status = {
  id: postId,
  uri: `https://${host}/users/carol/statuses/1`,
  url: `https://${host}/@carol/1`,
  account: carol,
  content: "<p>Two new paintings, finished this week.</p>",
  spoiler_text: "Paintings",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 60 * 60_000).toISOString(),
  media_attachments: [remoteMedia("d1", "A blue square on canvas"), remoteMedia("d2", undefined)],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
  favourited: false,
  boosted: false,
  bookmarked: false,
};

/** An older post of Carol, which this server stored when it fetched her recent posts from her server. */
const fetchedPost: Status = {
  ...post,
  id: "0192f0c4-7a8e-7000-8000-0000000003c2",
  uri: `https://${host}/users/carol/statuses/0`,
  url: `https://${host}/@carol/0`,
  content: "<p>A sketch from last month.</p>",
  spoiler_text: "",
  media_attachments: [],
  created_at: new Date(Date.now() - 30 * 24 * 60 * 60_000).toISOString(),
};

/**
 * Answers the client API as Alice's session, with Carol's profile and post. With `fetching`, the first page
 * of Carol's posts starts fetching her recent posts from her server and the later pages hold the post it
 * stored too (ADR-0044). Returns the number of requests for her posts so far.
 */
async function serveApi(page: Page, { fetching = false } = {}): Promise<{ statuses: number }> {
  const requests = { statuses: 0 };
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({ status: statusCode, contentType: statusCode < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }));
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace("/api/v1", "");
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
    if (path === "/accounts/lookup" && url.searchParams.get("acct") === carol.acct) return json(route, { ...carol, followers_count: 3, following_count: 4 });
    if (path === "/accounts/relationships") {
      return json(route, [
        { id: carol.id, following: false, requested: false, followed_by: false, requested_by: false, show_boosts: true, notify: false, blocking: false, muting: false, muting_notifications: false },
      ]);
    }
    if (path === `/accounts/${carol.id}/statuses`) {
      // Carol pinned nothing that this server knows of.
      if (url.searchParams.get("pinned") === "true") return json(route, { items: [] });
      requests.statuses += 1;
      if (!fetching) return json(route, { items: [post] });
      return requests.statuses === 1 ? json(route, { items: [post], remote_fetch_started: true }) : json(route, { items: [post, fetchedPost] });
    }
    if (path === `/statuses/${postId}`) return json(route, post);
    if (path === `/statuses/${postId}/context`) return json(route, { ancestors: [], descendants: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return requests;
}

test.describe("a profile of another server", () => {
  test.beforeEach(async ({ page }) => {
    await serveApi(page);
    await page.goto(`/@${carol.acct}`);
    await expect(page.getByRole("feed").getByRole("article")).toBeVisible();
  });

  test("says that only the posts this server received are shown, links to the profile and the post on their server, without axe violations or sideways scrolling", async ({ page, makeAxeBuilder }) => {
    const main = page.getByRole("main");
    await expect(main.getByText(`@${carol.acct}`).first()).toBeVisible();
    await expect(main.getByText(message("profile.remote.notice", host))).toBeVisible();
    // The host inside the sentence is wrapped in directional isolates (web-client/README.md Y10).
    await expect(main.getByRole("link", { name: message("profile.remote.original", `\u2068${host}\u2069`) })).toHaveAttribute("href", carol.url);
    const article = page.getByRole("feed").getByRole("article");
    await expect(article.getByRole("link", { name: message("status.original", host) })).toHaveAttribute("href", post.url ?? "");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("names the author's link by the full handle, which is cut on the domain's side when it does not fit", async ({ page }) => {
    const article = page.getByRole("feed").getByRole("article");
    await expect(article.getByRole("link", { name: `Carol @${carol.acct}` })).toBeVisible();
  });

  test("shows the post's pictures behind its content warning, naming the one without a description for screen readers only", async ({ page, makeAxeBuilder }) => {
    const article = page.getByRole("feed").getByRole("article");
    // The first collapsed button is the content warning's; the menu button of the actions is collapsed too.
    await article.getByRole("button", { expanded: false }).first().click();
    const images = article.getByRole("img");
    await expect(images).toHaveCount(2);
    await expect(images.first()).toHaveAttribute("alt", "A blue square on canvas");
    await expect(images.first()).toHaveAttribute("src", picture);
    // The picture without a description is named by the app's text, which the post does not show (web-client/README.md Y9).
    const name = message("timeline.media.no_description", host);
    await expect(images.nth(1)).toHaveAttribute("alt", name);
    await expect(article.getByText(name, { exact: true })).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("is the heading, the full handle, the notice with the link to the original profile, and the post with its link", async ({ page }) => {
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - heading "Carol" [level=1]
      - paragraph: "@carol@${host}"
      - paragraph: Painter on another server.
      - tabpanel:
        - paragraph
        - paragraph:
          - link /${host.replaceAll(".", "\\.")}/
        - feed:
          - article:
            - link /Carol/
            - link:
              - time
            - paragraph: Paintings
            - button
            - link /${host.replaceAll(".", "\\.")}/
    `);
  });
});

test.describe("a profile of another server whose recent posts are being fetched", () => {
  test("says so until it has read the posts again, once, without axe violations or sideways scrolling", async ({ page, makeAxeBuilder }) => {
    const requests = await serveApi(page, { fetching: true });
    await page.goto(`/@${carol.acct}`);
    const main = page.getByRole("main");
    const fetchingLine = main.getByText(message("profile.remote.fetching", host));
    await expect(fetchingLine).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // A few seconds later the posts are read again, with the one the server fetched, and the line goes.
    await expect(page.getByRole("feed").getByRole("article")).toHaveCount(2, { timeout: 10_000 });
    await expect(fetchingLine).toBeHidden();
    await expect(main.getByText(message("profile.remote.notice", host))).toBeVisible();
    expect(requests.statuses).toBe(2);
  });
});

test.describe("a post of another server", () => {
  test("has its link on its page, without axe violations or sideways scrolling", async ({ page, makeAxeBuilder }) => {
    await serveApi(page);
    await page.goto(`/@${carol.acct}/status/${postId}`);
    const article = page.getByRole("main").getByRole("article");
    await expect(article).toBeVisible();
    await expect(article.getByRole("link", { name: message("status.original", host) })).toHaveAttribute("href", post.url ?? "");
    // The first collapsed button is the content warning's; the menu button of the actions is collapsed too.
    await article.getByRole("button", { expanded: false }).first().click();
    await expect(article.getByRole("img")).toHaveCount(2);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
