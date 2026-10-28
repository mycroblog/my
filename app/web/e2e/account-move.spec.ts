// SPDX-License-Identifier: AGPL-3.0-only
// The aliases and the move of the account in every width × locale project (ADR-0027, FR-009, ADR-0089): the pages pass
// axe without scrolling sideways, and the account moves by keyboard through its alert dialog (which asks for the
// password and starts on it), then cancels the move. `vite preview` has no API, so each test plays the server of a
// signed-in user with page.route; names change with the locale, so the structure is checked by role and by the
// project's catalog.
import type { Page, Route } from "@playwright/test";
import { readCatalog } from "./catalog.ts";
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

/** The account to move to: its handle is long enough to show that the dialog wraps at 320 px. */
const target = {
  id: "0192f0c4-7a8e-7000-8000-000000000007",
  username: "carol",
  acct: "carol@a-rather-long-server-name-for-the-new-account.example",
  display_name: "Carol",
  note: "",
  fields: [],
  emojis: [],
  url: "https://a-rather-long-server-name-for-the-new-account.example/@carol",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  badges: [],
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Plays the client API of a signed-in user who has one alias; returns what the move calls received. */
async function userServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let moved = false;
  const moves: string[] = [];
  const cancellations: string[] = [];
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance") return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, { domain: "my.example", name: "my example", version: "0.0.0", source_url: "https://github.com/mycroblog/my", registrations: { mode: "open" }, federation: { mode: "both" }, limits: { max_post_characters: 500, require_media_description: false }, timelines: { local: "everyone", federated: "everyone" }, rules: [] });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, moved ? { ...viewer, moved: target } : viewer);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /accounts/aliases") {
      return json(route, { aliases: [{ uri: "https://old.example/users/alice", acct: "alice@old.example", created_at: "2026-10-01T09:00:00Z" }] });
    }
    if (key === "GET /accounts/password") return json(route, { has_password: true });
    // The page also loads the passkeys and the linked accounts, which decide how the move is confirmed.
    if (key === "GET /passkeys") return json(route, { items: [] });
    if (key === "GET /linked_identities") return json(route, { items: [], providers: [] });
    if (key === "GET /accounts/lookup") return json(route, target);
    if (key === "POST /accounts/move") {
      const body = (request.postDataJSON() ?? {}) as { target_account_id?: string; password?: string };
      moves.push(`${body.target_account_id ?? ""} ${body.password ?? ""}`);
      moved = true;
      return route.fulfill({ status: 204 });
    }
    if (key === "POST /accounts/move/cancel") {
      cancellations.push("cancel");
      moved = false;
      return route.fulfill({ status: 204 });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return { moves, cancellations };
}

for (const path of ["/settings/aliases", "/settings/move"]) {
  test.describe(`page ${path}`, () => {
    test.beforeEach(async ({ page }) => {
      await userServer(page);
      await page.goto(path);
      await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
      await expect(page.getByRole("main").getByRole("heading", { level: 2 }).first()).toBeVisible();
    });

    test("has no axe violations", async ({ makeAxeBuilder }) => {
      await expectNoAxeViolations(makeAxeBuilder);
    });

    test("does not scroll horizontally", async ({ page }) => {
      await expectNoHorizontalScroll(page);
    });
  });
}

test.describe("moving the account", () => {
  test("moves by keyboard through the dialog that asks for the password, then cancels the move", async ({ page, makeAxeBuilder }) => {
    const locale = test.info().project.use.locale ?? "en";
    const catalog = readCatalog(locale);
    const api = await userServer(page);
    await page.goto("/settings/move");
    const main = page.getByRole("main");

    // The field of the new account; Enter in it looks the account up and opens the dialog.
    await main.getByRole("textbox").fill("@carol@a-rather-long-server-name-for-the-new-account.example");
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    const password = dialog.locator('input[type="password"]');
    await expect(password).toBeFocused();
    // What moves and what stays are two lists, and the dialog has its two buttons.
    await expect(dialog.getByRole("list")).toHaveCount(2);
    await expect(dialog.getByRole("button")).toHaveCount(2);
    // While the dialog fades in, its text is not yet at full contrast.
    await page.waitForFunction(() => document.getAnimations().every((animation) => animation.playState !== "running"));
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(api.moves).toEqual([]);

    await expect(dialog.getByRole("link", { name: catalog["account_move.move.confirm.export"]?.defaultMessage ?? "" })).toHaveAttribute("href", "/settings/import-export");
    await page.keyboard.type("secret");
    await page.keyboard.press("Enter");
    await expect(dialog).toBeHidden();
    expect(api.moves).toEqual([`${target.id} secret`]);

    // The page turns into the moved state, whose result stays in the status.
    await expect(main.getByRole("heading", { level: 2 })).toHaveCount(1);
    await expect(main.getByRole("status")).not.toBeEmpty();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Cancelling asks first.
    await main.getByRole("button").click();
    await expect(dialog).toBeVisible();
    await page.waitForFunction(() => document.getAnimations().every((animation) => animation.playState !== "running"));
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button").last().click();
    await expect(dialog).toBeHidden();
    expect(api.cancellations).toEqual(["cancel"]);
    await expect(main.getByRole("textbox")).toBeVisible();
  });

  // The error under the password goes when the field loses the focus; a pointer press on the button must still send
  // (web-client/16-look-states-and-wording.md G23).
  test("moves with a pointer after an attempt without the password", async ({ page }) => {
    const api = await userServer(page);
    await page.goto("/settings/move");
    await page.getByRole("main").getByRole("textbox").fill("@carol@a-rather-long-server-name-for-the-new-account.example");
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("alertdialog");
    const password = dialog.locator('input[type="password"]');
    const move = dialog.getByRole("button").last();

    await move.click();
    await expect(password).toHaveAttribute("aria-invalid", "true");
    await expect(password).toBeFocused();
    expect(api.moves).toEqual([]);

    await password.fill("secret");
    await move.click();
    await expect(dialog).toBeHidden();
    expect(api.moves).toEqual([`${target.id} secret`]);
  });
});
