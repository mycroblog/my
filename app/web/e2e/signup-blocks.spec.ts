// SPDX-License-Identifier: AGPL-3.0-only
// The sign-up blocks screen in every width × locale project (ADR-0027, FR-719): the blocks in effect with an
// email domain in non-Latin letters shown in both of its forms and a long hash that wraps, adding a block on an
// email domain with the keyboard, importing a list with the keyboard and reading the lines it skipped, and
// lifting a block, each passing axe without scrolling sideways. `vite
// preview` has no API, so each test plays the server of a signed-in moderator with page.route; names change
// with the locale, so the structure is checked by role and the values the server sends.
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
    if (key === "GET /admin/counts")
      return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    if (key === "GET /admin/signup_blocks") {
      const active = url.searchParams.get("active") === "true";
      return json(route, { items: blocks.filter((block) => !active || block.state === "active") });
    }
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

test.describe("page /moderation/signup-blocks", () => {
  test.beforeEach(async ({ page }) => {
    await moderatorServer(page);
    await page.goto("/moderation/signup-blocks");
    await expect(page.getByRole("main").getByRole("table", { name: /./ })).toBeVisible();
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("sign-up blocks", () => {
  test("lists a block in both forms of its domain, adds a block on an email domain with the keyboard, and lifts a block", async ({
    page,
    makeAxeBuilder,
  }) => {
    const requests = await moderatorServer(page);
    await page.goto("/moderation/signup-blocks");
    const main = page.getByRole("main");
    const list = main.getByRole("table").last();
    await expect(list.getByRole("rowheader")).toHaveCount(2);

    // The first row names the domain in both forms and keeps its details: the source and the comment.
    const first = list.getByRole("row").nth(1);
    await expect(first.getByRole("rowheader")).toHaveText(
      new RegExp(`${unicodeDomain.replaceAll(".", "\\.")}.*${asciiDomain.replaceAll(".", "\\.")}`),
    );
    await expect(first).toContainText(longComment);
    await expect(first.getByRole("button")).toHaveCount(1);
    await expect(list.getByRole("row").nth(2).getByRole("rowheader")).toHaveText(hash);
    await expect(list.getByRole("row").nth(2).getByRole("button")).toHaveCount(1);

    // The form with the keyboard: the kind is the email domain already; type the domain, then Tab to the first
    // effect of its radio group, which none is chosen in yet, and choose it with Space.
    const domain = main.getByRole("textbox").first();
    await domain.focus();
    await page.keyboard.type("spam.example");
    await page.keyboard.press("Tab");
    await page.keyboard.press("Space");
    const effect = main.getByRole("radiogroup").nth(1).getByRole("radio").first();
    await expect(effect).toBeChecked();
    await main.locator('form button[type="submit"]').first().focus();
    await page.keyboard.press("Enter");
    await expect(main.getByRole("status").filter({ hasText: "spam.example" })).toBeVisible();
    await expect(list.getByRole("rowheader", { name: "spam.example" })).toBeVisible();
    await expect(domain).toHaveValue("");
    expect(requests).toEqual(["POST /admin/signup_blocks email_domain spam.example require_approval"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await list.getByRole("row").filter({ hasText: unicodeDomain }).getByRole("button").click();
    await expect(list.getByRole("rowheader")).toHaveCount(2);
    // Focus moves to the button of the row that took the lifted one's place.
    await expect(list.getByRole("row").filter({ hasText: hash }).getByRole("button")).toBeFocused();
    expect(requests).toEqual([
      "POST /admin/signup_blocks email_domain spam.example require_approval",
      "POST /admin/signup_blocks/0192f0c4-7a8e-7000-8000-0000000001a2/lift",
    ]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("an import of a list of sign-up blocks", () => {
  test("adds a list with the keyboard and shows the lines it skipped and why", async ({ page, makeAxeBuilder }) => {
    const requests = await moderatorServer(page);
    await page.goto("/moderation/signup-blocks");
    const main = page.getByRole("main");
    await expect(main.getByRole("table").getByRole("rowheader")).toHaveCount(2);
    // The import is the second section; its kind is the email domain already.
    const section = main.getByRole("region").nth(1);
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
    await expect(main.getByRole("table").last().getByRole("rowheader", { name: "mailinator.example" })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
