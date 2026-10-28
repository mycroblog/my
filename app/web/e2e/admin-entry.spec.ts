// SPDX-License-Identifier: AGPL-3.0-only
// The navigation's Moderation entry with what awaits the moderators, and the notifications from the instance, in
// every width × locale project (FR-712, ADR-0027, ADR-0039, ADR-0058). From md the rail (and from lg the sidebar)
// has Moderation with the count; below md the tab bar's Menu button carries the count and its sheet has the
// entry. A moderator has no Administration entry. The counts are in the accessible names and drawn as digits.
// The notifications of a moderator and of a user whose account was acted on lead to the screens that follow them
// up. `vite preview` has no API, so each
// test plays the server of a signed-in moderator with page.route; names change with the locale, so the structure
// is checked by role, by count and by address.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

/** The signed-in moderator's account, as GET /accounts/verify_credentials answers, with the moderator's permissions. */
const moderator = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  discoverable: true,
  indexable: true,
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["manage_reports", "manage_accounts", "view_audit"],
};

const bob = {
  ...moderator,
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  display_name: "Bob",
  url: "https://my.example/@bob",
  permissions: [],
};

const reportId = "0192f0c4-7a8e-7000-8000-0000000000e9";
const at = "2026-10-16T09:00:00Z";

/** Newest first: the notifications from the instance, with a long explanation that must wrap at 320 px. */
const notifications = [
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000a5",
    type: "moderation_notice",
    created_at: at,
    moderation_action: {
      id: "0192f0c4-7a8e-7000-8000-0000000000f1",
      kind: "limit",
      kind_label: "Limit",
      reason_category: "spam",
      reason_category_label: "Spam",
      public_reason: `Repeated promotional posts, such as https://shop.example/${"a-very-long-path-".repeat(4)}end.`,
      status_ids: [],
      state: "active",
      issued_at: at,
      expires_at: "2026-10-23T09:00:00Z",
      appealable: true,
    },
  },
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000a4",
    type: "report_resolved",
    created_at: at,
    report: {
      id: "0192f0c4-7a8e-7000-8000-0000000000f2",
      category: "spam",
      comment: "",
      forwarded: false,
      filed_at: at,
      target_account: bob,
      status_ids: [],
      rule_ids: [],
      state: "resolved",
    },
  },
  { id: "0192f0c4-7a8e-7000-8000-0000000000a3", type: "admin_report", created_at: at, report_id: reportId },
  { id: "0192f0c4-7a8e-7000-8000-0000000000a2", type: "admin_sign_up", created_at: at, account: bob },
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000a1",
    type: "relationships_severed",
    created_at: at,
    relationship_severance: { kind: "domain_suspension", domain: "bad.example", followers_lost: 2, following_lost: 1 },
  },
];

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the client API of a signed-in moderator, with 2 open reports and 1 registration awaiting approval. */
async function moderatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, {
      access_token: "e2e-token",
      token_type: "Bearer",
      expires_in: 3600,
      scope: "read write follow admin",
    }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance")
      return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "approval" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, moderator);
    if (key === "GET /preferences") {
      return json(route, {
        locale,
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "und",
        require_media_description: false,
        timeline_auto_insert: false,
        timeline_languages: [],
      });
    }
    if (key === "GET /admin/counts") return json(route, { open_reports: 2, pending_registrations: 1 });
    if (key === "GET /admin/registrations") return json(route, { items: [] });
    if (key === "GET /admin/reports") return json(route, { items: [] });
    if (key === "GET /notifications") return json(route, { items: notifications });
    if (key === "GET /notifications/marker" || key === "PUT /notifications/marker")
      return json(route, { last_read_id: notifications[0]?.id, unread_count: 0 });
    if (key === "GET /timelines/home") return json(route, { items: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

/** Whether the project's viewport has the phones' tab bar rather than the rail (Tailwind's md). */
function hasTabBar(page: Page): boolean {
  return (page.viewportSize()?.width ?? 0) < 768;
}

/** The count of 3 as the project's locale writes it. */
const three = /3|٣/;

test.describe("the moderation entry", () => {
  test.beforeEach(async ({ page }) => {
    await moderatorServer(page);
  });

  test("names the pending count where the width shows the entry, has no Administration, and opens the moderation page", async ({
    page,
    makeAxeBuilder,
  }) => {
    await page.goto("/home");
    const nav = page.getByRole("navigation").first();
    const entry = nav.locator('a[href="/moderation"]');
    await expect(nav.locator('a[href="/admin"]')).toHaveCount(0);
    if (hasTabBar(page)) {
      await expect(entry).toBeHidden();
      const menu = nav.getByRole("button").last();
      await expect(menu).toHaveAccessibleName(three);
      await expect(menu.locator('[aria-hidden="true"]', { hasText: three })).toBeVisible();
      await menu.click();
      const sheet = page.getByRole("dialog");
      const link = sheet.locator('a[href="/moderation"]');
      await expect(sheet.locator('a[href="/admin"]')).toHaveCount(0);
      await expect(link).toHaveAccessibleName(three);
      await expect(link.locator('[aria-hidden="true"]', { hasText: three })).toBeVisible();
      // The sheet fades and slides in: axe measures contrast once it has settled.
      await expect(page.locator("[data-entering]")).toHaveCount(0);
      await expectNoAxeViolations(makeAxeBuilder);
      await link.click();
    } else {
      await expect(entry).toBeVisible();
      await expect(entry).toHaveAccessibleName(three);
      await expect(entry.locator('[aria-hidden="true"]', { hasText: three })).toBeVisible();
      await expectNoAxeViolations(makeAxeBuilder);
      await entry.click();
    }

    await expect(page).toHaveURL(/\/moderation$/);
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toBeFocused();
    // The overview, not a copy of the menu: a tile for each counted queue, a link to it with the number that waits
    // there, then the reports to handle first (none open here) and the way to the whole queue.
    await expect(main).toMatchAriaSnapshot(`
      - heading [level=1]
      - region:
        - heading [level=2]
        - list:
          - listitem:
            - paragraph:
              - link:
                - /url: /moderation/reports
            - paragraph: /2|٢/
          - listitem:
            - paragraph:
              - link:
                - /url: /moderation/registrations
            - paragraph: /1|١/
      - region:
        - heading [level=2]
        - heading [level=3]
        - link:
          - /url: /moderation/reports
    `);
    // On a phone the sheet fades out as the page opens.
    await expect(page.locator("[data-exiting]")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("the notifications from the instance", () => {
  test.beforeEach(async ({ page }) => {
    await moderatorServer(page);
  });

  test("say what happened, lead to the screens that follow them up, pass axe, and do not scroll sideways", async ({
    page,
    makeAxeBuilder,
  }) => {
    await page.goto("/notifications");
    const feed = page.getByRole("feed");
    await expect(feed.getByRole("article")).toHaveCount(5);
    await expect(feed).toMatchAriaSnapshot(`
      - feed:
        - article:
          - paragraph
          - time
          - paragraph
          - paragraph
          - paragraph:
            - time
          - link:
            - /url: /settings/account-status
        - article:
          - paragraph:
            - link:
              - /url: /@bob
          - time
        - article:
          - paragraph
          - time
          - link:
            - /url: /moderation/reports/${reportId}
        - article:
          - paragraph:
            - link:
              - /url: /@bob
          - time
          - link:
            - /url: /moderation/registrations
        - article:
          - paragraph
          - time
          - link:
            - /url: /@alice/followers
          - link:
            - /url: /@alice/following
    `);
    await expect(
      feed
        .getByRole("article")
        .first()
        .getByText(/Repeated promotional posts/),
    ).toHaveAttribute("dir", "auto");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // From the keyboard: the sign-up's link opens the review of registrations.
    const review = feed.getByRole("article").nth(3).locator('a[href="/moderation/registrations"]');
    await review.focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/moderation\/registrations$/);
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeFocused();
  });
});
