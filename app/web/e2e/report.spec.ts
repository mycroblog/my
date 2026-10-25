// SPDX-License-Identifier: AGPL-3.0-only
// Reporting an account from a post's menu in every width × locale project (FR-701, ADR-0027): the whole
// dialog with the keyboard only, the focus on each step's heading, and axe, horizontal scroll and the ARIA
// structure on each step. `vite preview` has no API, so the spec answers the token refresh and the client API
// with `page.route`, as a signed-in user's server would. Names come from the project's catalog, so the spec
// finds the same elements in every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Status = components["schemas"]["Status"];
type CreateReportRequest = components["schemas"]["CreateReportRequest"];
/** The client message `key` in the project's locale (only messages without arguments). */
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
const bob: Status["account"] = { ...alice, id: "0192f0c4-7a8e-7000-8000-000000000002", username: "bob", acct: "bob", display_name: "Bob", url: "https://my.example/@bob" };

function status(id: string, content: string, overrides: Partial<Status> = {}): Status {
  return {
    id,
    uri: `https://my.example/ap/statuses/${id}`,
    url: `https://my.example/@bob/status/${id}`,
    account: bob,
    content,
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
    ...overrides,
  };
}

const post = status("0192f0c4-7a8e-7000-8000-0000000000c1", "<p>Buy now: https://shop.example/a/very/long/path/that/must/wrap/somewhere/before/it/scrolls/the/page</p>");
const older = status("0192f0c4-7a8e-7000-8000-0000000000c0", "<p>Another offer</p>", { spoiler_text: "Ads", created_at: new Date(Date.now() - 2 * 60 * 60_000).toISOString() });
const rules = [
  { id: "0192f0c4-7a8e-7000-8000-0000000007a1", text: "Be kind to each other" },
  { id: "0192f0c4-7a8e-7000-8000-0000000007a2", text: "No advertising", hint: "Posts that sell something without being asked." },
];

/** Answers the client API as Alice's session, and records the reports she files. */
async function serveApi(page: Page): Promise<CreateReportRequest[]> {
  const reports: CreateReportRequest[] = [];
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({ status: statusCode, contentType: statusCode < 400 ? "application/json" : "application/problem+json", body: JSON.stringify(body) });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace("/api/v1", "");
    if (path === "/accounts/verify_credentials") return json(route, alice);
    if (path === "/instance") {
      return json(route, { domain: "my.example", name: "my example", version: "0.0.0-e2e", source_url: "https://github.com/mycroblog/my", registrations: { mode: "open" }, federation: { mode: "both" }, limits: { max_post_characters: 500, require_media_description: false }, timelines: { local: "everyone", federated: "everyone" }, rules });
    }
    if (path === "/preferences") {
      // The user's locale is the project's: the app displays itself in the signed-in user's locale.
      return json(route, { locale: test.info().project.use.locale ?? "en", time_zone: "UTC", default_visibility: "public", default_post_language: "en", require_media_description: false, timeline_languages: [] });
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [] });
    if (path === `/statuses/${post.id}/context`) return json(route, { ancestors: [], descendants: [] });
    if (path === `/statuses/${post.id}`) return json(route, post);
    if (path === `/accounts/${bob.id}/statuses`) return json(route, { items: [post, older] });
    if (path === "/reports" && request.method() === "POST") {
      const body = request.postDataJSON() as CreateReportRequest;
      reports.push(body);
      return json(route, {
        id: "0192f0c4-7a8e-7000-8000-0000000007f1",
        category: body.category,
        comment: body.comment ?? "",
        forwarded: false,
        filed_at: new Date().toISOString(),
        target_account: bob,
        status_ids: body.status_ids ?? [],
        rule_ids: body.rule_ids ?? [],
      });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return reports;
}

// Without motion the dialog does not fade in (motion-safe:), so axe never samples its text half transparent:
// the first step's heading has the focus as soon as the dialog mounts.
test.use({ reducedMotion: "reduce" });

/** Checks that the step's heading has the focus, and the page on that step. */
async function expectStep(page: Page, key: string, makeAxeBuilder: Parameters<typeof expectNoAxeViolations>[0]) {
  await expect(page.getByRole("dialog").getByRole("heading", { name: message(key) })).toBeFocused();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
}

test("reports a post with the keyboard only, the focus on each step's heading", async ({ page, makeAxeBuilder }) => {
  const reports = await serveApi(page);
  await page.goto(`/@bob/status/${post.id}`);
  await expect(page.getByRole("heading", { level: 1, name: message("status.title") })).toBeVisible();

  const more = page.getByRole("main").getByRole("article").getByRole("button", { name: message("status.action.more") });
  await more.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menuitem", { name: message("report.open") })).toBeVisible();
  await page.keyboard.press("ArrowDown");
  await expect(page.getByRole("menuitem", { name: message("report.open") })).toBeFocused();
  await page.keyboard.press("Enter");

  const dialog = page.getByRole("dialog");
  await expectStep(page, "report.category.heading", makeAxeBuilder);
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("radio", { name: message("report.category.spam") })).toBeFocused();
  // Down in every direction: the radio group's arrows follow the reading order.
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  await expect(dialog.getByRole("radio", { name: message("report.category.violation") })).toBeChecked();
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: message("report.next") })).toBeFocused();
  await page.keyboard.press("Enter");

  await expectStep(page, "report.rules.heading", makeAxeBuilder);
  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("checkbox", { name: "No advertising" })).toBeFocused();
  await page.keyboard.press("Space");
  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: message("report.next") })).toBeFocused();
  await page.keyboard.press("Enter");

  await expectStep(page, "report.posts.heading", makeAxeBuilder);
  const choices = dialog.getByRole("group", { name: message("report.posts.heading") }).getByRole("checkbox");
  await expect(choices).toHaveCount(2);
  await expect(choices.first()).toBeChecked();
  // The post describes its checkbox; a post behind a content warning is described by the warning.
  await expect(choices.first()).toHaveAccessibleDescription(/^Buy now:/);
  await expect(choices.nth(1)).toHaveAccessibleDescription(/Ads/);
  await expect(dialog).toMatchAriaSnapshot(`
    - dialog:
      - heading [level=2]
      - button
      - form:
        - paragraph
        - heading [level=3]
        - group:
          - paragraph
          - list:
            - listitem:
              - checkbox [checked]
              - paragraph
            - listitem:
              - checkbox
              - paragraph
        - button
        - button
  `);
  // The two posts, Back, then Next.
  for (let i = 0; i < 4; i++) await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: message("report.next") })).toBeFocused();
  await page.keyboard.press("Enter");

  await expectStep(page, "report.comment.heading", makeAxeBuilder);
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("textbox", { name: message("report.comment.label") })).toBeFocused();
  await page.keyboard.type("Sells things in every reply");
  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: message("report.submit") })).toBeFocused();
  await page.keyboard.press("Enter");

  await expectStep(page, "report.sent.heading", makeAxeBuilder);
  expect(reports).toEqual([
    { account_id: bob.id, category: "violation", rule_ids: [rules[1]?.id], status_ids: [post.id], comment: "Sells things in every reply", forward: false },
  ]);
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: message("report.done") })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(dialog).toBeHidden();
  await expect(more).toBeFocused();
});
