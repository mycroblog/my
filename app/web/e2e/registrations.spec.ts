// SPDX-License-Identifier: AGPL-3.0-only
// The registrations screen in every width × locale project (ADR-0027, NFR-MOD-7): the sign-ups awaiting
// approval with their applicants' reasons, approving one through its alert dialog with a note and rejecting
// another, each passing axe without scrolling sideways. `vite preview` has no API, so each test plays the server
// of a signed-in moderator with page.route; names change with the locale, so the structure is checked by role
// and by what the server sends (usernames, reasons).
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

/** The signed-in moderator's account, as GET /accounts/verify_credentials answers, with manage_accounts. */
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
  permissions: ["manage_accounts"],
};

// A long reason in one word and one in Arabic, to show that reasons wrap at 320 px and keep their own direction.
const longReason = `I would like to post about birds. My blog is at https://birds.example/${"very-long-path-".repeat(6)}end.`;
const arabicReason = "أريد الانضمام إلى هذا المجتمع";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

type Registration = Record<string, unknown> & { user_id: string; username: string };

/** Plays the admin API of a signed-in moderator; returns the decisions taken. */
async function moderatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let pending: Registration[] = [
    {
      user_id: "0192f0c4-7a8e-7000-8000-0000000000b1",
      account_id: "0192f0c4-7a8e-7000-8000-0000000000c1",
      username: "bob",
      state: "pending",
      since: "2026-10-16T09:00:00Z",
      email: "bob.with.a.long.address@birds.example",
      email_verified: true,
      reason: longReason,
      submitted_at: "2026-10-16T09:00:00Z",
    },
    {
      user_id: "0192f0c4-7a8e-7000-8000-0000000000b2",
      account_id: "0192f0c4-7a8e-7000-8000-0000000000c2",
      username: "carol",
      state: "pending",
      since: "2026-10-16T10:00:00Z",
      email: "carol@example.com",
      email_verified: false,
      reason: arabicReason,
      invite_id: "0192f0c4-7a8e-7000-8000-0000000000e1",
      submitted_at: "2026-10-16T10:00:00Z",
    },
  ];
  const decisions: string[] = [];
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
        timeline_languages: [],
      });
    }
    if (key === "GET /admin/registrations") return json(route, { items: pending });
    const decide = /^POST \/admin\/registrations\/([^/]+)\/(approve|reject)$/.exec(key);
    if (decide) {
      const [, userId, decision] = decide;
      const body = (request.postDataJSON() ?? {}) as { note?: string };
      decisions.push(`${decision} ${userId} ${body.note ?? ""}`.trim());
      const registration = pending.find((item) => item.user_id === userId);
      pending = pending.filter((item) => item.user_id !== userId);
      return json(route, {
        ...registration,
        state: decision === "approve" ? "approved" : "rejected",
        since: "2026-10-17T00:00:00Z",
      });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return decisions;
}

test.describe("page /moderation/registrations", () => {
  test.beforeEach(async ({ page }) => {
    await moderatorServer(page);
    await page.goto("/moderation/registrations");
    await expect(page.getByRole("main").getByRole("table")).toBeVisible();
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("registrations", () => {
  test("lists the registrations with their reasons, approves one with a note after confirming, and rejects the other", async ({
    page,
    makeAxeBuilder,
  }) => {
    const decisions = await moderatorServer(page);
    await page.goto("/moderation/registrations");
    const main = page.getByRole("main");
    const list = main.getByRole("table");
    await expect(list.getByRole("rowheader")).toHaveCount(2);

    // The first row: the applicant, the facts of the sign-up, the reason, and a button for each decision.
    const bob = list.getByRole("row").nth(1);
    await expect(bob.getByRole("rowheader")).toContainText("@bob");
    await expect(bob).toContainText("bob.with.a.long.address@birds.example");
    await expect(bob).toContainText(longReason);
    await expect(bob.locator("time")).toHaveCount(1);
    await expect(bob.getByRole("button", { name: /@bob/ })).toHaveCount(2);
    await expect(list.getByText(arabicReason)).toHaveAttribute("dir", "auto");

    // Approve bob: the dialog opens on the note, and nothing is sent before confirming.
    await list.getByRole("row").nth(1).getByRole("button").first().click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    const note = dialog.getByRole("textbox");
    await expect(note).toBeFocused();
    await expect(dialog).toMatchAriaSnapshot(`
      - alertdialog /@bob/:
        - heading /@bob/ [level=2]
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

    await note.fill("Known from the birding club.");
    await dialog.getByRole("button").last().click();
    await expect(dialog).toBeHidden();
    await expect(list.getByRole("rowheader")).toHaveCount(1);
    await expect(main.getByRole("status").filter({ hasText: "@bob" })).toBeVisible();
    // Focus moves from the gone row to the next registration's first button.
    await expect(list.getByRole("row").nth(1).getByRole("button").first()).toBeFocused();
    expect(decisions).toEqual(["approve 0192f0c4-7a8e-7000-8000-0000000000b1 Known from the birding club."]);

    // Reject carol by keyboard: Tab to her reject button, Enter, then confirm without a note.
    await page.keyboard.press("Tab");
    await expect(list.getByRole("row").nth(1).getByRole("button").last()).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button").last().click();
    await expect(dialog).toBeHidden();
    await expect(list).toBeHidden();
    await expect(main.getByRole("status").filter({ hasText: "@carol" })).toBeVisible();
    await expect(main.getByRole("heading", { level: 1 })).toBeFocused();
    expect(decisions).toEqual([
      "approve 0192f0c4-7a8e-7000-8000-0000000000b1 Known from the birding club.",
      "reject 0192f0c4-7a8e-7000-8000-0000000000b2",
    ]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
