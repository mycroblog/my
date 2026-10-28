// SPDX-License-Identifier: AGPL-3.0-only
// The domain actions screen in every width × locale project (ADR-0027, NFR-MOD-7): the actions in effect with a
// domain in non-Latin letters shown in both of its forms, a suspension taken through its alert dialog, and
// lifting an action, each passing axe without scrolling sideways. `vite preview` has no API, so each test plays
// the server of a signed-in operator with page.route; names change with the locale, so the structure is checked
// by role and the identifiers the server sends (domains). The import of Mastodon's domain block CSV is previewed,
// a row cleared by keyboard, and the rest imported.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

/** The signed-in operator's account, as GET /accounts/verify_credentials answers, with manage_domains. */
const operator = {
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
  permissions: ["manage_domains"],
};

// A long domain in non-Latin letters and its A-labels, to show that both forms wrap at 320 px.
const unicodeDomain = "とても長いサブドメインの名前.例え.jp";
const asciiDomain = new URL(`https://${unicodeDomain}`).hostname;
const longComment =
  "Spam from every account of this server, reported many times by people here, with no answer from its moderators.";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

type Action = Record<string, unknown> & { id: string; domain: string; state: string };

/** Plays the admin API of a signed-in operator; returns the actions taken and lifted. */
async function operatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let actions: Action[] = [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000a2",
      domain: asciiDomain,
      kind: "limit",
      public_comment: longComment,
      private_note: "See the reports.",
      obfuscate: true,
      state: "active",
      issued_at: "2026-10-12T08:00:00Z",
      expires_at: "2026-11-11T08:00:00Z",
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
    if (key === "GET /accounts/verify_credentials") return json(route, operator);
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
    if (key === "GET /admin/domain_actions") {
      const active = url.searchParams.get("active") === "true";
      return json(route, { items: actions.filter((action) => !active || action.state === "active") });
    }
    if (key === "POST /admin/domain_actions") {
      const body = request.postDataJSON() as { domain: string; kind: string; obfuscate?: boolean };
      requests.push(`${key} ${body.kind} ${body.domain}`);
      const action: Action = {
        id: "0192f0c4-7a8e-7000-8000-0000000000a3",
        domain: new URL(`https://${body.domain}`).hostname,
        kind: body.kind,
        obfuscate: body.obfuscate ?? false,
        state: "active",
        issued_at: "2026-10-17T00:00:00Z",
      };
      actions = [action, ...actions];
      return json(route, action);
    }
    if (key === "POST /admin/domain_actions/import/preview") {
      requests.push(key);
      return json(route, {
        rows: [
          {
            line: 2,
            written_domain: "spam.example",
            status: "importable",
            issue: ["limit", "reject_media"],
            block: {
              domain: "spam.example",
              severity: "silence",
              reject_media: true,
              reject_reports: false,
              obfuscate: false,
            },
          },
          {
            line: 3,
            written_domain: asciiDomain,
            status: "importable",
            issue: ["reject_reports"],
            block: {
              domain: asciiDomain,
              severity: "noop",
              reject_media: false,
              reject_reports: true,
              obfuscate: false,
              public_comment: longComment,
            },
          },
          { line: 4, written_domain: "not a domain", status: "invalid", issue: [] },
        ],
      });
    }
    if (key === "POST /admin/domain_actions/import") {
      const body = request.postDataJSON() as { blocks: { domain: string }[] };
      requests.push(`${key} ${body.blocks.map((block) => block.domain).join(" ")}`);
      const issued = body.blocks.map((block, i) => ({
        id: `0192f0c4-7a8e-7000-8000-0000000000b${String(i)}`,
        domain: block.domain,
        kind: "reject_reports",
        obfuscate: false,
        state: "active",
        issued_at: "2026-10-17T00:00:00Z",
      }));
      actions = [...issued, ...actions];
      return json(route, { issued, skipped: [] });
    }
    const lift = /^POST \/admin\/domain_actions\/([^/]+)\/lift$/.exec(key);
    if (lift) {
      requests.push(key);
      const id = lift[1];
      actions = actions.map((action) =>
        action.id === id ? { ...action, state: "lifted", lifted_at: "2026-10-17T01:00:00Z" } : action,
      );
      return json(
        route,
        actions.find((action) => action.id === id),
      );
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return requests;
}

test.describe("page /moderation/domain-actions", () => {
  test.beforeEach(async ({ page }) => {
    await operatorServer(page);
    await page.goto("/moderation/domain-actions");
    await expect(page.getByRole("main").getByRole("table")).toBeVisible();
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("domain actions", () => {
  test("lists an action in both forms of its domain, suspends a domain after confirming, and lifts an action", async ({
    page,
    makeAxeBuilder,
  }) => {
    const requests = await operatorServer(page);
    await page.goto("/moderation/domain-actions");
    const main = page.getByRole("main");
    const list = main.getByRole("table");
    await expect(list.getByRole("rowheader")).toHaveCount(1);

    // The row names the domain in both forms and keeps its details: the comment and the private note.
    const action = list.getByRole("row").nth(1);
    await expect(action.getByRole("rowheader")).toHaveText(
      new RegExp(`${unicodeDomain.replaceAll(".", "\\.")}.*${asciiDomain.replaceAll(".", "\\.")}`),
    );
    await expect(action).toContainText(longComment);
    await expect(action).toContainText("See the reports.");
    await expect(action.getByRole("button")).toHaveCount(1);

    // The form: a domain, then the last kind (suspend), whose confirmation sends nothing yet.
    await main.getByRole("textbox").first().fill("spam.example");
    // By keyboard: the radio's input is visually hidden under its drawn circle.
    await main.getByRole("radio").last().focus();
    await page.keyboard.press("Space");
    await expect(main.getByRole("radio").last()).toBeChecked();
    await main.locator('form button[type="submit"]').first().click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("button").first()).toBeFocused();
    await expect(dialog).toMatchAriaSnapshot(`
      - alertdialog /spam\\.example/:
        - heading /spam\\.example/ [level=2]
        - paragraph
        - button
        - button
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(requests).toEqual([]);

    await dialog.getByRole("button").last().click();
    await expect(dialog).toBeHidden();
    await expect(main.getByRole("status").filter({ hasText: "spam.example" })).toBeVisible();
    await expect(list.getByRole("rowheader", { name: "spam.example" })).toBeVisible();
    expect(requests).toEqual(["POST /admin/domain_actions suspend spam.example"]);

    await list.getByRole("row").filter({ hasText: unicodeDomain }).getByRole("button").click();
    await expect(list.getByRole("rowheader")).toHaveCount(1);
    await expect(list.getByRole("button")).toBeFocused();
    expect(requests).toEqual([
      "POST /admin/domain_actions suspend spam.example",
      "POST /admin/domain_actions/0192f0c4-7a8e-7000-8000-0000000000a2/lift",
    ]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("Mastodon's domain block CSV", () => {
  test("previews a CSV, clears a row by keyboard, and imports the rest with focus on the import's heading", async ({
    page,
    makeAxeBuilder,
  }) => {
    const requests = await operatorServer(page);
    await page.goto("/moderation/domain-actions");
    const main = page.getByRole("main");
    await expect(main.getByRole("table").getByRole("rowheader")).toHaveCount(1);

    // The CSV's text area is the last text box of the page; the preview is the submit button of the last form.
    await main.getByRole("textbox").last().fill("#domain,#severity\nspam.example,silence\n");
    await main.locator('form button[type="submit"]').last().click();
    // The preview's rows are the list of checkboxes; once the import replaces the preview, the list of the domain
    // actions above it is the last list of the page, so `.last()` would find that one.
    const rows = main.getByRole("list").filter({ has: page.getByRole("checkbox") });
    // Each row that takes actions is a checkbox named by its domain, chosen at first; the invalid row is text.
    await expect(rows.getByRole("listitem")).toHaveCount(3);
    await expect(rows.getByRole("checkbox", { name: "spam.example" })).toBeChecked();
    await expect(rows.getByRole("checkbox", { name: new RegExp(asciiDomain.replaceAll(".", "\\.")) })).toBeChecked();
    await expect(rows.getByRole("listitem").last()).toContainText("not a domain");
    expect(requests).toEqual(["POST /admin/domain_actions/import/preview"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // By keyboard: the checkbox's input is visually hidden under its drawn box.
    await rows.getByRole("checkbox").first().focus();
    await page.keyboard.press("Space");
    await expect(rows.getByRole("checkbox").first()).not.toBeChecked();
    // The import is the last button of the page; no row suspends, so it asks nothing.
    await main.getByRole("button").last().click();
    await expect(rows).toBeHidden();
    expect(requests).toEqual([
      "POST /admin/domain_actions/import/preview",
      `POST /admin/domain_actions/import ${asciiDomain}`,
    ]);
    await expect(main.getByRole("heading", { level: 3 }).last()).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
