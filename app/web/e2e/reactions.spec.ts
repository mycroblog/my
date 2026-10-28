// SPDX-License-Identifier: AGPL-3.0-only
// The emoji reactions of a post in every width × locale project (FR-116, ADR-0052 §5, ADR-0027): the row of
// reactions under the post, the picker of reactions, reacting in place of the favourite, and the dialog of the
// accounts that reacted, each axe-clean and without horizontal scrolling. `vite preview` has no API, so the spec
// answers the token refresh and the client API with `page.route`. Names come from the project's catalog and match exactly:
// "React" is a substring of every "React with {emoji}" toggle.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Status = components["schemas"]["Status"];
/** The client message `key` in the project's locale, with `{emoji}` replaced by `emoji`. */
function message(key: string, emoji = ""): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text.replace("{emoji}", emoji);
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
const bob: Status["account"] = {
  ...alice,
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  display_name: "Bob",
  url: "https://my.example/@bob",
};

/** A square image, in place of the custom emoji's. */
const emojiImage = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==",
  "base64",
);

/** A custom emoji of this server. */
const blobcat = {
  name: "blobcat",
  shortcode: "blobcat",
  emoji: {
    shortcode: "blobcat",
    url: "https://my.example/media/blobcat.png",
    static_url: "https://my.example/media/blobcat.png",
  },
};

const postId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const post: Status = {
  id: postId,
  uri: `https://my.example/ap/statuses/${postId}`,
  url: `https://my.example/@bob/status/${postId}`,
  account: bob,
  content: "<p>The trail along the river is open again.</p>",
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
  favourites_count: 1,
  reactions: [
    { name: "👍", count: 12, me: false },
    { ...blobcat, count: 3, me: false },
    {
      name: "blobfox@a-rather-long-community-server-name.example",
      shortcode: "blobfox",
      domain: "a-rather-long-community-server-name.example",
      emoji: {
        shortcode: "blobfox",
        url: "https://my.example/media/blobfox.png",
        static_url: "https://my.example/media/blobfox.png",
      },
      count: 1,
      me: false,
    },
  ],
  favourited: true,
  boosted: false,
  bookmarked: false,
};

/** Answers the client API as Alice's session. A reaction answers the post with Alice's reaction in place of her favourite. */
async function serveApi(page: Page): Promise<void> {
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({
      status: statusCode,
      contentType: statusCode < 400 ? "application/json" : "application/problem+json",
      body: JSON.stringify(body),
    });
  await page.route("**/media/*.png", (route) =>
    route.fulfill({ status: 200, contentType: "image/png", body: emojiImage }),
  );
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace("/api/v1", "");
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
    if (path === "/custom_emojis") return json(route, { items: [blobcat.emoji] });
    if (path === `/statuses/${postId}/context`) return json(route, { ancestors: [], descendants: [] });
    if (path === `/statuses/${postId}`) return json(route, post);
    if (path === `/statuses/${postId}/reaction` && request.method() === "PUT") {
      const { emoji } = request.postDataJSON() as { emoji: string };
      return json(route, {
        ...post,
        favourited: false,
        favourites_count: 0,
        reactions: [...post.reactions, { name: emoji, count: 1, me: true }],
      });
    }
    if (path === `/statuses/${postId}/reactions`) {
      return json(route, {
        items: [
          { account: bob, emoji: { name: "👍" }, created_at: post.created_at },
          { account: alice, emoji: blobcat, created_at: post.created_at },
        ],
      });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

/** The article of the post, which holds its reactions and actions. */
function article(page: Page) {
  return page.getByRole("main").getByRole("article").filter({ hasText: "The trail along the river is open again." });
}

/** Opens the picker of reactions of the post, and returns it once its search field has focus. */
async function openPicker(page: Page) {
  await article(page)
    .getByRole("button", { name: message("reaction.picker.button"), exact: true })
    .click();
  const picker = page.getByRole("dialog", { name: message("reaction.picker.title") });
  await expect(picker.getByRole("searchbox", { name: message("emoji.picker.search") })).toBeFocused();
  return picker;
}

test.describe("the reactions of a post", () => {
  test.beforeEach(async ({ page }) => {
    await serveApi(page);
    await page.goto(`/@bob/status/${postId}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    // Signed in: the reactions are toggles.
    await expect(
      article(page).getByRole("button", { name: message("reaction.toggle", "👍"), exact: true }),
    ).toBeVisible();
  });

  test("are a row of toggles and counts, axe-clean and without horizontal scrolling", async ({
    page,
    makeAxeBuilder,
  }) => {
    const reactions = article(page).getByRole("group", { name: message("reaction.list.label"), exact: true });
    await expect(reactions).toMatchAriaSnapshot(`
      - group:
        - button [pressed=false]
        - button [pressed=false]
        - img
        - button
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("offer the post form's emoji picker, axe-clean and without horizontal scrolling", async ({
    page,
    makeAxeBuilder,
  }) => {
    const picker = await openPicker(page);
    await expect(picker.getByRole("grid", { name: message("reaction.picker.grid") })).toBeVisible();
    await expect(picker.getByRole("toolbar", { name: message("emoji.picker.categories") })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The skin tone menu too.
    await picker.getByRole("button", { name: message("emoji.picker.skin_tone") }).click();
    await expect(page.getByRole("menuitemradio", { name: message("emoji.picker.skin_tone.medium") })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("keep the picker inside a 320 px viewport without scrolling sideways", async ({ page, makeAxeBuilder }) => {
    await page.setViewportSize({ width: 320, height: 568 });
    const picker = await openPicker(page);
    await expect(picker).toBeInViewport({ ratio: 1 });
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("take an emoji picked in the picker in place of the favourite", async ({ page }) => {
    const post = article(page);
    const picker = await openPicker(page);
    // A section's button moves focus to its first emoji, the jack-o-lantern of the activities.
    await picker.getByRole("button", { name: message("emoji.picker.group.activities"), exact: true }).click();
    await page.keyboard.press("Enter");
    await expect(picker).toBeHidden();
    await expect(post.getByRole("button", { name: message("reaction.toggle", "🎃"), exact: true })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await expect(post.getByRole("button", { name: message("status.action.favourite"), exact: true })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  test("list the accounts that reacted in a dialog", async ({ page, makeAxeBuilder }) => {
    await article(page)
      .getByRole("button", { name: message("reaction.list.open"), exact: true })
      .click();
    const dialog = page.getByRole("dialog", { name: message("reaction.list.title") });
    await expect(dialog.getByRole("link", { name: /Bob/ })).toBeVisible();
    await expect(dialog.getByRole("link", { name: /Alice/ })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
