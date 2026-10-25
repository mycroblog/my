// SPDX-License-Identifier: AGPL-3.0-only
// The poll of a post in every width × locale project (FR-112, ADR-0027): the options of a poll the viewer has not
// voted in, the vote, and the results after it, each axe-clean and without horizontal scrolling. `vite preview` has
// no API, so the spec answers the token refresh and the client API with `page.route`. Names come from the project's
// catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Status = components["schemas"]["Status"];
type Poll = components["schemas"]["Poll"];
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

/** A poll of another server with options long enough to wrap at 320 px, that ends in a day. */
const poll: Poll = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e1",
  expires_at: new Date(Date.now() + 24 * 60 * 60_000).toISOString(),
  expired: false,
  multiple: false,
  votes_count: 7,
  voters_count: 7,
  options: [
    { title: "The trail along the river, past the old mill and the bridge", votes_count: 4 },
    { title: "The ridge", votes_count: 3 },
    { title: "Neither", votes_count: 0 },
  ],
  voted: false,
  own_votes: [],
};

const postId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const post: Status = {
  id: postId,
  uri: "https://remote.example/users/carol/statuses/1",
  url: "https://remote.example/@carol/1",
  account: carol,
  content: "<p>Which walk on Sunday?</p>",
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
  favourited: false,
  boosted: false,
  bookmarked: false,
  poll,
};

/** Answers the client API as Alice's session. A vote answers the post with it counted. */
async function serveApi(page: Page): Promise<void> {
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({ status: statusCode, contentType: statusCode < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace("/api/v1", "");
    if (path === "/accounts/verify_credentials") return json(route, alice);
    if (path === "/instance") {
      return json(route, { domain: "my.example", name: "my example", version: "0.0.0-e2e", source_url: "https://github.com/mycroblog/my", registrations: { mode: "open" }, federation: { mode: "both" }, limits: { max_post_characters: 500 }, timelines: { local: "everyone", federated: "everyone" }, rules: [] });
    }
    if (path === "/preferences") {
      return json(route, { locale: test.info().project.use.locale ?? "en", time_zone: "UTC", default_visibility: "public", default_post_language: "en", require_media_description: false, timeline_languages: [] });
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [] });
    if (path === "/custom_emojis") return json(route, { items: [] });
    if (path === `/statuses/${postId}/context`) return json(route, { ancestors: [], descendants: [] });
    if (path === `/statuses/${postId}`) return json(route, post);
    if (path === `/statuses/${postId}/poll/votes` && request.method() === "POST") {
      const { choices } = request.postDataJSON() as { choices: number[] };
      const voted: Poll = {
        ...poll,
        votes_count: poll.votes_count + choices.length,
        voters_count: (poll.voters_count ?? 0) + 1,
        options: poll.options.map((option, index) => (choices.includes(index) ? { ...option, votes_count: (option.votes_count ?? 0) + 1 } : option)),
        voted: true,
        own_votes: choices,
      };
      return json(route, { ...post, poll: voted });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

/** The poll of the post. */
function pollRegion(page: Page) {
  return page.getByRole("main").getByRole("region", { name: message("status.poll.label"), exact: true });
}

test.describe("the poll of a post", () => {
  test.beforeEach(async ({ page }) => {
    await serveApi(page);
    await page.goto(`/@carol@remote.example/status/${postId}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    // Signed in: the poll is a form.
    await expect(pollRegion(page).getByRole("radiogroup")).toBeVisible();
  });

  test("offers its options, axe-clean and without horizontal scrolling", async ({ page, makeAxeBuilder }) => {
    await expect(pollRegion(page).getByRole("radiogroup")).toMatchAriaSnapshot(`
      - radiogroup:
        - radio [checked=false]
        - radio [checked=false]
        - radio [checked=false]
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("takes a vote and shows its results, axe-clean and without horizontal scrolling", async ({ page, makeAxeBuilder }) => {
    const region = pollRegion(page);
    // By keyboard: the radio's input is visually hidden under its drawn circle.
    const ridge = region.getByRole("radio", { name: "The ridge" });
    await ridge.focus();
    await page.keyboard.press("Space");
    await expect(ridge).toBeChecked();
    await region.getByRole("button", { name: message("status.poll.vote"), exact: true }).click();
    const results = region.getByRole("list", { name: message("status.poll.results"), exact: true });
    await expect(results).toBeVisible();
    await expect(results.getByRole("listitem").nth(1)).toContainText(message("status.poll.own_vote"));
    await expect(region.getByRole("button", { name: message("status.poll.vote"), exact: true })).toBeHidden();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
