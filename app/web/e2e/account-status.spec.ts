// SPDX-License-Identifier: AGPL-3.0-only
// The account status page in every width × locale project (ADR-0027, FR-715, FR-707): the moderation actions
// on the user's own account, in effect and past, and appealing one by keyboard through its alert dialog, each
// step passing axe without scrolling sideways. `vite preview` has no API, so each test plays the server of a
// signed-in user with page.route; names change with the locale, so the structure is checked by role and by
// what the server sends (the kinds, the explanations, the appeal).
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

/** The signed-in user's account, as GET /accounts/verify_credentials answers. */
const viewer = {
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
  permissions: [],
};

// The server's own names of the kinds are long; with a long URL in an explanation, they show that the page
// wraps at 320 px.
const suspendLabel = "Suspended: no use of the app, and the profile and posts are hidden";
const limitLabel = "Limited: posts shown to followers only and kept off the public timelines";
const longReason = `Repeated spam in replies, such as https://spam.example/${"very-long-path-".repeat(6)}end.`;
const arabicReason = "صور غير مناسبة";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

type Action = Record<string, unknown> & { id: string; state: string };

/** Plays the client API of a signed-in user with two actions on the account; returns the appeals sent. */
async function userServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let actions: Action[] = [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000007a1",
      kind: "suspend",
      kind_label: suspendLabel,
      reason_category: "spam",
      reason_category_label: "Spam",
      public_reason: longReason,
      status_ids: [],
      state: "active",
      issued_at: "2026-10-16T09:30:00Z",
      expires_at: "2026-11-15T09:30:00Z",
      appealable: true,
      appeal_deadline: "2027-04-14T09:30:00Z",
    },
    {
      id: "0192f0c4-7a8e-7000-8000-0000000007a2",
      kind: "limit",
      kind_label: limitLabel,
      reason_category: "other",
      reason_category_label: "Other",
      public_reason: arabicReason,
      status_ids: [],
      state: "lifted",
      issued_at: "2026-08-01T00:00:00Z",
      lifted_at: "2026-08-05T12:00:00Z",
      appealable: false,
      appeal_deadline: "2027-01-28T00:00:00Z",
      appeal: {
        id: "0192f0c4-7a8e-7000-8000-0000000007b2",
        text: "These were not spam.",
        state: "overturned",
        submitted_at: "2026-08-02T00:00:00Z",
        decided_at: "2026-08-05T12:00:00Z",
        decision_reason: "We agree, and lifted it.",
      },
    },
  ];
  const appeals: string[] = [];
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow" }),
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
    if (key === "GET /accounts/verify_credentials") return json(route, viewer);
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
    if (key === "GET /moderation/actions") {
      const active = url.searchParams.get("active") === "true";
      return json(route, { items: active ? actions.filter((action) => action.state === "active") : actions });
    }
    const appeal = /^POST \/moderation\/actions\/([^/]+)\/appeal$/.exec(key);
    if (appeal) {
      const [, id] = appeal;
      const body = (request.postDataJSON() ?? {}) as { text?: string };
      appeals.push(`${id} ${body.text ?? ""}`);
      actions = actions.map((action) =>
        action.id === id
          ? {
              ...action,
              appealable: false,
              appeal: {
                id: "0192f0c4-7a8e-7000-8000-0000000007b9",
                text: body.text,
                state: "pending",
                submitted_at: "2026-10-17T00:00:00Z",
              },
            }
          : action,
      );
      return json(
        route,
        actions.find((action) => action.id === id),
      );
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return appeals;
}

test.describe("page /settings/account-status", () => {
  test.beforeEach(async ({ page }) => {
    await userServer(page);
    await page.goto("/settings/account-status");
    await expect(page.getByRole("main").getByRole("article").first()).toBeVisible();
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("account status", () => {
  test("lists the actions on the account, and appeals the one in effect by keyboard after confirming", async ({
    page,
    makeAxeBuilder,
  }) => {
    const appeals = await userServer(page);
    await page.goto("/settings/account-status");
    const main = page.getByRole("main");
    const suspended = main.getByRole("article", { name: suspendLabel });
    const limited = main.getByRole("article", { name: limitLabel });
    await expect(suspended).toBeVisible();

    await expect(suspended).toMatchAriaSnapshot(`
      - article /^Suspended/:
        - heading /^Suspended/ [level=3]
        - text: In effect
        - paragraph
        - term
        - definition: Spam
        - term
        - definition:
          - time
        - term
        - definition:
          - time
        - term
        - definition: ${longReason}
        - region:
          - heading [level=4]
          - paragraph
          - button
    `);
    // The appealed action shows its appeal, and no form.
    await expect(limited.getByText("These were not spam.")).toHaveAttribute("dir", "auto");
    await expect(limited.getByText(arabicReason)).toHaveAttribute("dir", "auto");
    await expect(limited.getByRole("button")).toHaveCount(0);

    // Tab to the appeal button and open the dialog with Enter: it opens on the text, and nothing is sent yet.
    const open = suspended.getByRole("button");
    await open.focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    const text = dialog.getByRole("textbox");
    await expect(text).toBeFocused();
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
    // While the dialog fades in, its text is not yet at full contrast.
    await page.waitForFunction(() => document.getAnimations().every((animation) => animation.playState !== "running"));
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(appeals).toEqual([]);

    await page.keyboard.type("I did not post spam.");
    await page.keyboard.press("Tab");
    await page.keyboard.press("Tab");
    await expect(dialog.getByRole("button").last()).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(dialog).toBeHidden();

    // The appeal is under review in place of the form, the result stays on the page, and focus is on the appeal.
    await expect(suspended.getByText("I did not post spam.")).toBeVisible();
    await expect(suspended.getByRole("button")).toHaveCount(0);
    await expect(suspended.getByRole("heading", { level: 4 })).toBeFocused();
    await expect(main.getByRole("status").filter({ hasText: suspendLabel })).toBeVisible();
    expect(appeals).toEqual(["0192f0c4-7a8e-7000-8000-0000000007a1 I did not post spam."]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  // The error under the text goes when the text loses the focus; a pointer press on the button must still send
  // (web-client/16-look-states-and-wording.md G23).
  test("sends the appeal with a pointer after an empty attempt", async ({ page }) => {
    const appeals = await userServer(page);
    await page.goto("/settings/account-status");
    const suspended = page.getByRole("main").getByRole("article", { name: suspendLabel });
    await suspended.getByRole("button").click();
    const dialog = page.getByRole("alertdialog");
    const text = dialog.getByRole("textbox");
    const send = dialog.getByRole("button").last();

    await send.click();
    await expect(text).toHaveAttribute("aria-invalid", "true");
    await expect(text).toBeFocused();
    expect(appeals).toEqual([]);

    await text.fill("I did not post spam.");
    await send.click();
    await expect(dialog).toBeHidden();
    expect(appeals).toEqual(["0192f0c4-7a8e-7000-8000-0000000007a1 I did not post spam."]);
  });
});
