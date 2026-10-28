// SPDX-License-Identifier: AGPL-3.0-only
// The appeals screens in every width × locale project (ADR-0027, FR-707, NFR-MOD-7): the appeals awaiting review
// by an account with a long name, an appeal's page next to its action, and overturning it through its alert
// dialog with an explanation, after which the action shows as lifted; each passing axe without scrolling
// sideways. `vite preview` has no API, so each test plays the server of a signed-in moderator with page.route;
// names change with the locale, so the structure is checked by role and by what the server sends.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

type Schemas = components["schemas"];
type Appeal = Schemas["AdminAppeal"];

/** The signed-in moderator's account, as GET /accounts/verify_credentials answers, with manage_accounts. */
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
  permissions: ["manage_accounts"],
};

/** The account that appealed: a long display name. */
const appellant: Schemas["Account"] = {
  ...moderator,
  id: "0192f0c4-7a8e-7000-8000-000000000021",
  username: "maximiliana",
  acct: "maximiliana",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000021",
  display_name: "Maximiliana Theodora Wolfharte",
  url: "https://my.example/@maximiliana",
};

// A long text in one word and one in Arabic, to show that texts wrap at 320 px and keep their own direction.
const longText = `I only posted the link to my own blog: https://birds.example/${"very-long-path-".repeat(6)}end.`;
const arabicText = "لم أنشر أي إعلانات";

const appealId = "0192f0c4-7a8e-7000-8000-0000000f0001";
/** Another appeal awaiting review, which stays so. */
const otherAppealId = "0192f0c4-7a8e-7000-8000-0000000f0002";

function appealOf(overrides: Partial<Appeal>): Appeal {
  return {
    id: appealId,
    account_id: appellant.id,
    text: longText,
    state: "pending",
    submitted_at: "2026-10-16T09:00:00Z",
    action: {
      id: "0192f0c4-7a8e-7000-8000-0000000d0101",
      account_id: appellant.id,
      kind: "suspend",
      reason_category: "spam",
      public_reason: "Repeated advertising.",
      private_note: "Third report this week.",
      issuer: "moderator",
      issued_by_account_id: moderator.id,
      notify_target: true,
      state: "active",
      issued_at: "2026-10-15T09:00:00Z",
    },
    ...overrides,
  };
}

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/**
 * How long the lists of appeals take once a decision is made: a slow server, so that a list shown from its cache
 * before the list comes again would show the decided appeal (#468).
 */
const slowListMs = 1500;

/** Plays the admin API of a signed-in moderator; returns the decisions taken. */
async function moderatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let appeals: Appeal[] = [
    appealOf({}),
    appealOf({ id: otherAppealId, text: arabicText, submitted_at: "2026-10-16T08:00:00Z" }),
  ];
  const decisions: string[] = [];
  let slowLists = false;
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
    if (key === "GET /accounts/verify_credentials") return json(route, moderator);
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
    if (key === `GET /accounts/${appellant.id}`) return json(route, appellant);
    if (key === "GET /admin/appeals") {
      if (slowLists) await new Promise((resolve) => setTimeout(resolve, slowListMs));
      const state = url.searchParams.get("state");
      return json(route, { items: state === null ? appeals : appeals.filter((appeal) => appeal.state === state) });
    }
    const one = /^GET \/admin\/appeals\/([^/]+)$/.exec(key);
    if (one) {
      const appeal = appeals.find((item) => item.id === one[1]);
      return appeal === undefined
        ? json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404)
        : json(route, appeal);
    }
    const decide = /^POST \/admin\/appeals\/([^/]+)\/(uphold|overturn)$/.exec(key);
    if (decide) {
      const [, id, decision] = decide;
      const body = (request.postDataJSON() ?? {}) as { public_reason?: string };
      decisions.push(`${decision} ${id} ${body.public_reason ?? ""}`.trim());
      const decidedAt = "2026-10-17T00:00:00Z";
      appeals = appeals.map((appeal) =>
        appeal.id !== id
          ? appeal
          : {
              ...appeal,
              state: decision === "uphold" ? "upheld" : "overturned",
              decided_at: decidedAt,
              decided_by_account_id: moderator.id,
              ...(body.public_reason === undefined ? {} : { decision_reason: body.public_reason }),
              action:
                decision === "overturn" ? { ...appeal.action, state: "lifted", lifted_at: decidedAt } : appeal.action,
            },
      );
      slowLists = true;
      return json(
        route,
        appeals.find((appeal) => appeal.id === id),
      );
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return decisions;
}

test.describe("page /moderation/appeals", () => {
  test.beforeEach(async ({ page }) => {
    await moderatorServer(page);
    await page.goto("/moderation/appeals");
    await expect(page.getByRole("main").getByText(appellant.display_name).first()).toBeVisible();
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("appeals", () => {
  test("opens an appeal from the list by keyboard and overturns it after confirming, which lifts its action", async ({
    page,
    makeAxeBuilder,
  }) => {
    const decisions = await moderatorServer(page);
    await page.goto("/moderation/appeals");
    const main = page.getByRole("main");
    const table = main.getByRole("table");
    const firstRow = table.getByRole("row").nth(1);
    await expect(firstRow.getByRole("cell").filter({ hasText: appellant.display_name })).toBeVisible();
    await expect(firstRow.getByRole("rowheader")).toContainText(longText);
    await expect(firstRow.getByRole("cell")).toHaveCount(3);
    await expect(firstRow.getByRole("cell").last().getByRole("time")).toBeVisible();
    await expect(table.getByText(arabicText)).toHaveAttribute("dir", "auto");

    // Open the first appeal from the keyboard.
    const first = firstRow.getByRole("link");
    await first.focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/moderation/appeals/${appealId}$`));
    const regions = main.getByRole("region");
    await expect(regions).toHaveCount(3);
    await expect(regions.first().getByText(appellant.display_name)).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await expect(regions.nth(1).getByRole("definition")).toHaveCount(7);

    // Overturn: the dialog opens on the explanation, and nothing is sent before confirming.
    const review = regions.nth(2);
    await review.getByRole("button").last().click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    const reason = dialog.getByRole("textbox");
    await expect(reason).toBeFocused();
    await expect(dialog).toMatchAriaSnapshot(`
      - alertdialog:
        - heading [level=2]
        - paragraph
        - text: /.+/
        - textbox
        - text: /.+/
        - status
        - button
        - button
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(decisions).toEqual([]);

    await reason.fill("The link was to your own blog.");
    await dialog.getByRole("button").last().click();
    await expect(dialog).toBeHidden();
    await expect(review.getByRole("status")).not.toBeEmpty();
    await expect(review.getByRole("heading", { level: 2 })).toBeFocused();
    await expect(review.getByText("The link was to your own blog.")).toBeVisible();
    await expect(review.getByRole("button")).toHaveCount(0);
    // The action's state reads as lifted, with the moment it was lifted as a new fact.
    await expect(regions.nth(1).getByRole("definition")).toHaveCount(8);
    expect(decisions).toEqual([`overturn ${appealId} The link was to your own blog.`]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Back on the list, the appeal is gone from those awaiting review as soon as the list shows, though the
    // server answers the lists slowly now: the list is not shown from a cache older than the decision.
    await main.locator('a[href="/moderation/appeals"]').click();
    await expect(page).toHaveURL(/\/moderation\/appeals$/);
    await expect(main.getByRole("table").locator(`a[href="/moderation/appeals/${otherAppealId}"]`)).toBeVisible();
    expect(await main.getByRole("table").locator(`a[href="/moderation/appeals/${appealId}"]`).count()).toBe(0);
    await expect(main.getByRole("table").locator('a[href^="/moderation/appeals/"]')).toHaveCount(1);
  });
});
