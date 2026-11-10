// SPDX-License-Identifier: AGPL-3.0-only
// The sign-up blocks screen in every width × locale project (ADR-0110, FR-719): the list of the blocks in effect with
// an email domain in non-Latin letters shown in both of its forms, a long hash that wraps, and no free text; a block's
// page with its comment and the lift; adding a block on an email domain with the keyboard on the page of a new block;
// and importing a list with the keyboard and reading the lines it skipped on the import page, each passing axe without
// scrolling sideways. `vite preview` has no API, so each test plays the server of a signed-in moderator with
// page.route; names change with the locale, so the structure is checked by role and the values the server sends.
import type { Page, Route } from "@playwright/test";
import {
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  recordRowHeaders,
  recordRows,
  recordTable,
  test,
} from "./fixtures";
import { preferencesOf } from "./preferences.ts";

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

// A long domain in non-Latin letters and its A-labels, to show that both forms wrap at 320 px.
const unicodeDomain = "とても長い使い捨てのメール.例え.jp";
const asciiDomain = new URL(`https://${unicodeDomain}`).hostname;
const hash = "5d41402abc4b2a76b9719d911017c592ae0b4f1c7ad8e2c6f5a1b0e9d8c7b6a5";
const longComment = "Disposable addresses used for many accounts that posted the same advertisement on the same day.";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

type Block = Record<string, unknown> & { id: string; value: string; state: string };

/** Plays the admin API of a signed-in moderator; returns the blocks added and lifted. */
async function moderatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let blocks: Block[] = [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000001a2",
      kind: "email_domain",
      value: asciiDomain,
      severity: "reject",
      comment: longComment,
      state: "active",
      created_at: "2026-10-12T08:00:00Z",
      expires_at: "2026-11-11T08:00:00Z",
    },
    {
      id: "0192f0c4-7a8e-7000-8000-0000000001a1",
      kind: "email_hash",
      value: hash,
      severity: "require_approval",
      state: "active",
      created_at: "2026-10-11T08:00:00Z",
    },
  ];
  const requests: string[] = [];
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
        full_text_search: { available: false, visibilities: [] },
        translation: { browser: false, providers: [] },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, moderator);
    if (key === "GET /preferences") {
      return json(route, preferencesOf({ locale }));
    }
    if (key === "GET /admin/counts")
      return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    if (key === "GET /admin/signup_blocks") {
      const active = url.searchParams.get("active") === "true";
      return json(route, { items: blocks.filter((block) => !active || block.state === "active") });
    }
    const one = /^GET \/admin\/signup_blocks\/([^/]+)$/.exec(key);
    const found = one && blocks.find((block) => block.id === one[1]);
    if (found) return json(route, found);
    if (key === "POST /admin/signup_blocks") {
      const body = request.postDataJSON() as { kind: string; value: string; severity: string };
      requests.push(`${key} ${body.kind} ${body.value} ${body.severity}`);
      const value = body.kind === "email_domain" ? new URL(`https://${body.value}`).hostname : body.value;
      const block: Block = {
        id: "0192f0c4-7a8e-7000-8000-0000000001a3",
        kind: body.kind,
        value,
        severity: body.severity,
        state: "active",
        created_at: "2026-10-17T00:00:00Z",
      };
      blocks = [block, ...blocks];
      return json(route, block);
    }
    if (key === "POST /admin/signup_blocks/import") {
      const body = request.postDataJSON() as { kind: string; text: string; severity: string };
      requests.push(`${key} ${body.kind} ${body.severity}`);
      // Every line is added but the one in non-Latin letters, which is blocked already, and a value that is no domain.
      const skipped: { line: number; value: string; reason: string }[] = [];
      let added = 0;
      body.text.split("\n").forEach((raw, index) => {
        const value = raw.trim();
        if (value === "" || value.startsWith("#")) return;
        if (value === unicodeDomain) skipped.push({ line: index + 1, value, reason: "already_blocked" });
        else if (!value.includes(".")) skipped.push({ line: index + 1, value, reason: "invalid" });
        else {
          added += 1;
          blocks = [
            {
              id: `0192f0c4-7a8e-7000-8000-${String(index).padStart(12, "0")}`,
              kind: body.kind,
              value,
              severity: body.severity,
              state: "active",
              created_at: "2026-10-17T00:00:00Z",
            },
            ...blocks,
          ];
        }
      });
      return json(route, { added, skipped });
    }
    const lift = /^POST \/admin\/signup_blocks\/([^/]+)\/lift$/.exec(key);
    if (lift) {
      requests.push(key);
      const id = lift[1];
      blocks = blocks.map((block) =>
        block.id === id ? { ...block, state: "lifted", lifted_at: "2026-10-17T01:00:00Z" } : block,
      );
      return json(
        route,
        blocks.find((block) => block.id === id),
      );
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return requests;
}

const blockId = "0192f0c4-7a8e-7000-8000-0000000001a2";

/** A pattern of the domain shown in both of its forms, its U-labels first. */
const bothForms = new RegExp(`${unicodeDomain.replaceAll(".", "\\.")}.*${asciiDomain.replaceAll(".", "\\.")}`);

for (const path of [
  "/moderation/signup-blocks",
  `/moderation/signup-blocks/${blockId}`,
  "/moderation/signup-blocks/new",
  "/moderation/signup-blocks/import",
]) {
  test.describe(`page ${path}`, () => {
    test.beforeEach(async ({ page }) => {
      await moderatorServer(page);
      await page.goto(path);
      await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
      await expect(page.getByRole("main").getByRole("link").first()).toBeVisible();
    });

    test("has no axe violations", async ({ makeAxeBuilder }) => {
      await expectNoAxeViolations(makeAxeBuilder);
    });

    test("does not scroll horizontally", async ({ page }) => {
      await expectNoHorizontalScroll(page);
    });
  });
}

test.describe("sign-up blocks", () => {
  test("lists a block in both forms of its domain without its comment, and lifts it on its page", async ({
    page,
    makeAxeBuilder,
  }) => {
    const requests = await moderatorServer(page);
    await page.goto("/moderation/signup-blocks");
    const main = page.getByRole("main");
    const list = recordTable(main);
    await expect(recordRowHeaders(list)).toHaveCount(2);

    // The first row names the domain in both forms; the comment is on the block's page only.
    const first = recordRows(list).first();
    await expect(recordRowHeaders(first)).toHaveText(bothForms);
    await expect(list).not.toContainText(longComment);
    await expect(list.getByRole("button")).toHaveCount(0);
    await expect(recordRowHeaders(recordRows(list).nth(1))).toHaveText(hash);

    // By keyboard: the row's link opens the block's page.
    await recordRowHeaders(first).getByRole("link").focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/moderation/signup-blocks/${blockId}$`));
    await expect(main.getByRole("heading", { level: 1 })).toHaveText(bothForms);
    await expect(main).toContainText(longComment);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The lift asks nothing first; its button goes away, and focus moves to the heading of its region.
    const state = main.getByRole("region").first();
    await state.getByRole("button").click();
    await expect(state.getByRole("button")).toHaveCount(0);
    await expect(state.getByRole("heading", { level: 2 })).toBeFocused();
    await expect(state.getByRole("status")).toContainText(unicodeDomain);
    expect(requests).toEqual([`POST /admin/signup_blocks/${blockId}/lift`]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The list of the blocks in effect keeps only the other one.
    await main.locator('a[href="/moderation/signup-blocks"]').first().click();
    await expect(recordRowHeaders(recordTable(main))).toHaveText([hash]);
  });

  test("adds a block on an email domain with the keyboard on the page of a new block", async ({
    page,
    makeAxeBuilder,
  }) => {
    const requests = await moderatorServer(page);
    await page.goto("/moderation/signup-blocks");
    const main = page.getByRole("main");
    await expect(recordRowHeaders(recordTable(main))).toHaveCount(2);

    // The link-button of the list opens the form; the list page has no form.
    await expect(main.getByRole("textbox")).toHaveCount(0);
    await main.locator('a[href="/moderation/signup-blocks/new"]').click();
    await expect(page).toHaveURL(/\/moderation\/signup-blocks\/new$/);

    // The kind is the email domain already; type the domain, then Tab to the first effect of its radio group, which
    // none is chosen in yet, and choose it with Space.
    const domain = main.getByRole("textbox").first();
    await domain.focus();
    await page.keyboard.type("spam.example");
    await page.keyboard.press("Tab");
    await page.keyboard.press("Space");
    const effect = main.getByRole("radiogroup").nth(1).getByRole("radio").first();
    await expect(effect).toBeChecked();
    await main.locator('form button[type="submit"]').focus();
    await page.keyboard.press("Enter");
    await expect(main.getByRole("status").filter({ hasText: "spam.example" })).toBeVisible();
    await expect(domain).toHaveValue("");
    expect(requests).toEqual(["POST /admin/signup_blocks email_domain spam.example require_approval"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The way back leads to the list, which shows the new block.
    await main.locator('a[href="/moderation/signup-blocks"]').first().click();
    await expect(recordRowHeaders(recordTable(main)).filter({ hasText: "spam.example" })).toBeVisible();
  });
});

test.describe("an import of a list of sign-up blocks", () => {
  test("adds a list with the keyboard and shows the lines it skipped and why", async ({ page, makeAxeBuilder }) => {
    const requests = await moderatorServer(page);
    await page.goto("/moderation/signup-blocks/import");
    const main = page.getByRole("main");
    // The import is the region of the page; its kind is the email domain already.
    const section = main.getByRole("region").first();
    const listField = section.getByRole("textbox").first();
    await listField.focus();
    await page.keyboard.insertText(
      ["# Disposable email domains", "throwaway.example", unicodeDomain, "not-a-domain", "", "mailinator.example"].join(
        "\n",
      ),
    );
    // Tab past the file button to the first effect, which none is chosen in yet, and choose it with Space.
    await page.keyboard.press("Tab");
    await expect(section.getByRole("button").first()).toBeFocused();
    await page.keyboard.press("Tab");
    await page.keyboard.press("Space");
    await expect(section.getByRole("radiogroup").nth(1).getByRole("radio").first()).toBeChecked();
    await section.locator('button[type="submit"]').focus();
    await page.keyboard.press("Enter");

    const table = section.getByRole("table");
    await expect(table).toBeVisible();
    expect(requests).toEqual(["POST /admin/signup_blocks/import email_domain require_approval"]);
    await expect(section.getByRole("status").last()).not.toBeEmpty();
    await expect(listField).toHaveValue("");
    await expect(table).toMatchAriaSnapshot(`
      - table:
        - rowgroup:
          - row:
            - columnheader
            - columnheader
            - columnheader
        - rowgroup:
          - row:
            - cell
            - cell "${unicodeDomain}"
            - cell
          - row:
            - cell
            - cell "not-a-domain"
            - cell
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The list page shows the blocks the import added.
    await main.getByRole("navigation").locator('a[href="/moderation/signup-blocks"]').click();
    await expect(recordRowHeaders(recordTable(main)).filter({ hasText: "mailinator.example" })).toBeVisible();
  });
});
