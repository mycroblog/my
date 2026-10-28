// SPDX-License-Identifier: AGPL-3.0-only
// The report queue and a report's page in every width × locale project (ADR-0027, FR-710, NFR-MOD-4/7): the open
// reports on an account with a long name and a reporter on a long domain, a report's page with its post, the
// ban's alert dialog, and an action on the target that resolves the report and takes it off the queue, each
// passing axe without scrolling sideways. `vite preview` has no API, so each test plays the server of a signed-in
// moderator with page.route; names change with the locale, so the structure is checked by role and by what the
// server sends (names, handles, texts).
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

type Schemas = components["schemas"];
type Report = Schemas["AdminReportDetail"];

/** The signed-in moderator's account, as GET /accounts/verify_credentials answers, with the permissions of the role. */
const moderator = {
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
  permissions: ["manage_reports", "manage_accounts", "view_audit"],
};

/** The reported account: a long display name, local. */
const target: Schemas["Account"] = {
  ...moderator,
  id: "0192f0c4-7a8e-7000-8000-000000000021",
  username: "maximiliana",
  acct: "maximiliana",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000021",
  display_name: "Maximiliana Theodora Wolfharte",
  url: "https://my.example/@maximiliana",
};

/** The reporter: the instance actor of a server on a long domain. */
const longDomain = "the-longest-community-server-name-in-the-fediverse.social.example.com";
const reporter: Schemas["Account"] = {
  ...target,
  id: "0192f0c4-7a8e-7000-8000-000000000031",
  username: "instance.actor",
  acct: `instance.actor@${longDomain}`,
  domain: longDomain,
  uri: `https://${longDomain}/actor`,
  display_name: "",
  url: `https://${longDomain}/actor`,
};

const postId = "0192f0c4-7a8e-7000-8000-0000000000b1";
const post: Schemas["Status"] = {
  id: postId,
  uri: `https://my.example/ap/statuses/${postId}`,
  url: `https://my.example/@maximiliana/status/${postId}`,
  account: target,
  content: "<p>Buy cheap followers at https://spam.example/now — limited offer!</p>",
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: "2026-10-16T08:00:00Z",
  media_attachments: [],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
};

const comment =
  "This account posts the same advertisement in reply to everyone who mentions the server, many times a day.";

function report(id: string, overrides: Partial<Report> = {}): Report {
  return {
    id,
    category: "spam",
    comment,
    forwarded: false,
    filed_at: "2026-10-16T09:00:00Z",
    state: "open",
    state_changed_at: "2026-10-16T09:00:00Z",
    reporter_account_id: reporter.id,
    reporter_account: reporter,
    uri: `https://${longDomain}/flags/1`,
    target_account_id: target.id,
    target_account: target,
    status_ids: [postId],
    rule_ids: [],
    statuses: [{ status: post }],
    notes: [
      {
        id: "0192f0c4-7a8e-7000-8000-0000000e0001",
        author_account_id: moderator.id,
        body: "Same as last week.",
        created_at: "2026-10-16T10:00:00Z",
      },
    ],
    target_history: { reports_count: 3, actions_count: 1 },
    ...overrides,
  };
}

const reportId = "0192f0c4-7a8e-7000-8000-0000000c0002";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the API of a signed-in moderator; returns the account actions taken. */
async function moderatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let reports: Report[] = [
    report(reportId),
    report("0192f0c4-7a8e-7000-8000-0000000c0001", { filed_at: "2026-10-15T09:00:00Z", comment: "" }),
  ];
  const actions: string[] = [];
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
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance")
      return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials" || key === `GET /accounts/${moderator.id}`)
      return json(route, moderator);
    if (key === "GET /preferences") {
      return json(route, {
        locale,
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "und",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (key === "GET /admin/reports") {
      const state = url.searchParams.get("state");
      return json(route, { items: reports.filter((item) => state === null || item.state === state) });
    }
    const one = /^GET \/admin\/reports\/([^/]+)$/.exec(key);
    if (one) {
      const found = reports.find((item) => item.id === one[1]);
      return found === undefined
        ? json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404)
        : json(route, found);
    }
    if (key === `GET /admin/accounts/${target.id}/actions`) {
      return json(route, {
        items: [
          {
            id: "0192f0c4-7a8e-7000-8000-0000000d0000",
            account_id: target.id,
            kind: "warn",
            reason_category: "spam",
            issuer: "moderator",
            notify_target: true,
            state: "active",
            issued_at: "2026-10-09T00:00:00Z",
          },
        ],
      });
    }
    if (key === `POST /admin/accounts/${target.id}/actions`) {
      const body = request.postDataJSON() as Schemas["CreateAccountActionRequest"];
      actions.push(`${body.kind} ${body.report_id ?? ""}`);
      reports = reports.map((item) =>
        item.id === body.report_id
          ? { ...item, state: "resolved", outcome: "actioned", state_changed_at: "2026-10-17T00:00:00Z" }
          : item,
      );
      return json(route, {
        id: "0192f0c4-7a8e-7000-8000-0000000d0001",
        account_id: target.id,
        kind: body.kind,
        reason_category: body.reason_category,
        issuer: "moderator",
        notify_target: body.notify_target,
        state: "active",
        issued_at: "2026-10-17T00:00:00Z",
        report_id: body.report_id,
      });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return actions;
}

/** The rows of the queue: the table's body rows, each headed by its report's link. */
function rows(page: Page) {
  return page
    .getByRole("main")
    .getByRole("row")
    .filter({ has: page.getByRole("rowheader") });
}

test.describe("page /moderation/reports", () => {
  test.beforeEach(async ({ page }) => {
    await moderatorServer(page);
    await page.goto("/moderation/reports");
    await expect(rows(page)).toHaveCount(2);
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("page /moderation/reports/{id}", () => {
  test.beforeEach(async ({ page }) => {
    await moderatorServer(page);
    await page.goto(`/moderation/reports/${reportId}`);
    await expect(page.getByRole("main").getByRole("article")).toBeVisible();
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("reports", () => {
  test("opens a report from the queue, asks before a ban, and resolves it with an action that takes it off the queue", async ({
    page,
    makeAxeBuilder,
  }) => {
    const actions = await moderatorServer(page);
    await page.goto("/moderation/reports");
    await expect(rows(page)).toHaveCount(2);

    await expect(rows(page).first()).toMatchAriaSnapshot(`
      - row:
        - rowheader:
          - link /@maximiliana/
          - paragraph: ${comment}
        - cell
        - cell /@instance\\.actor@${longDomain.replaceAll(".", "\\.")}/
        - cell
        - cell:
          - time
    `);

    // By keyboard: the row's link opens the report's page, whose heading takes focus.
    await rows(page).first().getByRole("link").focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/moderation/reports/${reportId}$`));
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toBeFocused();
    await expect(main.getByRole("article")).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The ban, by keyboard: the radio's input is visually hidden under its drawn circle.
    // The decision of the report's page: its last region is the handling.
    const handling = main.locator("[data-triage-decision]").getByRole("region").last();
    await handling.locator('input[type="radio"][value="ban"]').focus();
    await page.keyboard.press("Space");
    await expect(handling.locator('input[type="radio"][value="ban"]')).toBeChecked();
    await handling.locator('form button[type="submit"]').click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("button").first()).toBeFocused();
    await expect(dialog).toMatchAriaSnapshot(`
      - alertdialog /@maximiliana/:
        - heading /@maximiliana/ [level=2]
        - paragraph
        - button
        - button
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    expect(actions).toEqual([]);

    // A limit takes effect at once and resolves the report.
    await handling.locator('input[type="radio"][value="limit"]').focus();
    await page.keyboard.press("Space");
    await handling.locator('form button[type="submit"]').click();
    await expect(handling.getByRole("status").filter({ hasText: /\S/ })).toBeVisible();
    await expect(handling.getByRole("heading", { level: 2 })).toBeFocused();
    expect(actions).toEqual([`limit ${reportId}`]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Back on the queue, the report is gone from the open ones.
    await main.getByRole("link").first().click();
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first().getByRole("link")).toHaveAttribute(
      "href",
      "/moderation/reports/0192f0c4-7a8e-7000-8000-0000000c0001",
    );
  });
});
