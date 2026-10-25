// SPDX-License-Identifier: AGPL-3.0-only
// Editing the viewer's own post and its edit history in every width × locale project (FR-109, ADR-0027): the
// edit dialog and the history page have no axe violations and do not scroll sideways, the history reads as
// a list of revisions named by their dates, and an edit saved from the dialog shows on the post with the
// mark that leads to the history. `vite preview` has no API, so the spec answers the token refresh and the
// client API with `page.route`, as a signed-in user's server would.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Status = components["schemas"]["Status"];
type StatusEdit = components["schemas"]["StatusEdit"];
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

const postId = "0192f0c4-7a8e-7000-8000-0000000000e1";
const hourAgo = (hours: number) => new Date(Date.now() - hours * 60 * 60_000).toISOString();

const original: StatusEdit = { content: "<p>The walk starts at nine.</p>", spoiler_text: "", sensitive: false, created_at: hourAgo(3), media_attachments: [], emojis: [] };
const latest: StatusEdit = {
  content: "<p>The walk starts at ten, at the bridge: https://example.com/a/very/long/path/that/must/wrap/somewhere/before/it/scrolls/the/page/sideways</p>",
  spoiler_text: "Plans",
  sensitive: false,
  created_at: hourAgo(2),
  media_attachments: [],
  emojis: [],
};
const revisions: StatusEdit[] = [original, latest];

/** Alice's post, edited once, as the server shows it now. */
const post: Status = {
  id: postId,
  uri: `https://my.example/ap/statuses/${postId}`,
  url: `https://my.example/@alice/status/${postId}`,
  account: alice,
  content: latest.content,
  spoiler_text: latest.spoiler_text,
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: original.created_at,
  edited_at: latest.created_at,
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
};

/** Answers the client API as Alice's session, with her edited post; an edit answers the post with the new text. */
async function serveApi(page: Page): Promise<void> {
  let current = post;
  const json = (route: Route, body: unknown) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace("/api/v1", "");
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
    if (path === `/statuses/${postId}/context`) return json(route, { ancestors: [], descendants: [] });
    if (path === `/statuses/${postId}/history`) return json(route, { items: revisions });
    if (path === `/statuses/${postId}/source`) return json(route, { id: postId, text: "The walk starts at ten, at the bridge.", spoiler_text: "Plans", language: "en" });
    if (path === `/statuses/${postId}`) {
      if (request.method() === "PUT") {
        const body = request.postDataJSON() as { text: string; spoiler_text?: string };
        current = { ...current, content: `<p>${body.text}</p>`, spoiler_text: body.spoiler_text ?? "", edited_at: new Date().toISOString() };
      }
      return json(route, current);
    }
    return route.fulfill({ status: 404, contentType: "application/problem+json", body: JSON.stringify({ type: "about:blank", title: "Not Found", status: 404 }) });
  });
}

test("the history of an edited post lists its revisions, newest first", async ({ page, makeAxeBuilder }) => {
  await serveApi(page);
  await page.goto(`/@alice/status/${postId}`);
  const article = page.getByRole("main").getByRole("article");
  await article.getByRole("link", { name: message("status.edited") }).click();

  await expect(page.getByRole("heading", { level: 1, name: message("status.history.title") })).toBeFocused();
  const list = page.getByRole("list", { name: message("status.history.list") });
  await expect(list.getByRole("article")).toHaveCount(2);
  await expect(page.getByRole("main")).toMatchAriaSnapshot(`
    - heading [level=1]
    - paragraph:
      - link
    - list:
      - listitem:
        - article:
          - heading [level=2]:
            - time
          - paragraph
          - button [expanded=false]
      - listitem:
        - article:
          - heading [level=2]:
            - time
          - paragraph
          - paragraph
  `);
  await expect(list.getByRole("article").first().getByText(message("status.history.current"))).toBeVisible();
  await expect(list.getByRole("article").last().getByText(message("status.history.original"))).toBeVisible();
  // The current revision's text is behind its content warning; opened, its long link wraps.
  await list.getByRole("article").first().getByRole("button", { expanded: false }).click();
  await expect(list.getByText(/at the bridge/)).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

test("the viewer edits their post in a dialog, and the post shows the edit", async ({ page, makeAxeBuilder }) => {
  await serveApi(page);
  await page.goto(`/@alice/status/${postId}`);
  const article = page.getByRole("main").getByRole("article");
  await article.getByRole("button", { name: message("status.action.more") }).click();
  await page.getByRole("menuitem", { name: message("status.action.edit") }).click();

  const dialog = page.getByRole("dialog", { name: message("status.edit.title") });
  const text = dialog.getByRole("textbox", { name: message("compose.text.label") });
  await expect(text).toHaveValue("The walk starts at ten, at the bridge.");
  await expect(text).toBeFocused();
  await expect(dialog.getByRole("textbox", { name: message("compose.warning.label") })).toHaveValue("Plans");
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);

  await text.fill("The walk starts at eleven.");
  await dialog.getByRole("button", { name: message("compose.warning.toggle") }).click();
  await dialog.getByRole("button", { name: message("status.edit.save") }).click();

  await expect(dialog).toBeHidden();
  await expect(page.getByRole("region", { name: message("shell.toast.region") }).getByText(message("status.edit.done"))).toBeVisible();
  await expect(article.getByText("The walk starts at eleven.")).toBeVisible();
  await expect(article.getByRole("link", { name: message("status.edited") })).toBeVisible();
});
