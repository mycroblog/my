// SPDX-License-Identifier: AGPL-3.0-only
// The server settings screen in every width × locale project (ADR-0027): an administrator names the server once,
// in any language, and the tab title and the sidebar call the server by that name instead of its domain at once, and
// changes the registration mode with the keyboard, each step passing axe without scrolling sideways. `vite preview` has no API, so each test plays the server of
// a signed-in administrator with page.route; names change with the locale, so the structure is checked by role,
// by the names in the project's locale catalog and by what the server sends.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** The signed-in administrator's account, as GET /accounts/verify_credentials answers, with manage_settings. */
const administrator = {
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
  permissions: ["manage_settings"],
};

// The longest name the server takes, in one word, to show that the sidebar wraps it rather than scroll.
const longName = `Birds${"-and-more".repeat(9)}-of-the-${"x".repeat(5)}`;

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** The instance settings of the form, at their defaults, as GET /admin/settings lists them; `stored` replaces values. */
function instanceSettings(stored: Readonly<Record<string, string>>) {
  const items = [
    { key: "registration_mode", kind: "enum", default: "approval", choices: ["open", "approval", "invite", "closed"] },
    { key: "default_locale", kind: "locale", default: "en" },
    { key: "local_timeline_access", kind: "enum", default: "everyone", choices: ["off", "logged_in", "everyone"] },
    { key: "federated_timeline_access", kind: "enum", default: "everyone", choices: ["off", "logged_in", "everyone"] },
    { key: "require_media_description", kind: "bool", default: "false" },
    { key: "remote_media_policy", kind: "enum", default: "none", choices: ["none", "avatars", "all"] },
    { key: "remote_media_cache_ttl", kind: "int", default: "86400", minimum: 60, maximum: 31536000 },
    { key: "authorized_fetch", kind: "bool", default: "false" },
    { key: "domain_actions_visibility", kind: "enum", default: "none", choices: ["none", "users", "all"] },
    { key: "domain_actions_rationale_visibility", kind: "enum", default: "none", choices: ["none", "users", "all"] },
    { key: "search_indexing", kind: "enum", default: "deny", choices: ["allow", "deny"] },
    { key: "ai_usage", kind: "enum", default: "deny", choices: ["allow", "deny_training", "deny"] },
    { key: "indexable_default", kind: "bool", default: "false" },
    { key: "security_contact", kind: "uri", default: "" },
    { key: "link_cards_enabled", kind: "bool", default: "true" },
    { key: "finance_visibility", kind: "enum", default: "private", choices: ["private", "totals", "breakdown"] },
    { key: "finance_viewer_item_names", kind: "bool", default: "false" },
    { key: "finance_viewer_item_memos", kind: "bool", default: "false" },
    { key: "finance_viewer_income", kind: "enum", default: "totals", choices: ["totals", "plans"] },
    { key: "income_visibility", kind: "enum", default: "private", choices: ["private", "totals", "plans"] },
    { key: "federation_mode", kind: "enum", default: "both", choices: ["off", "inbound", "outbound", "both"] },
  ];
  return {
    items: items.map((item) => ({ ...item, value: stored[item.key] ?? item.default, stored: item.key in stored })),
  };
}

/**
 * Plays the API of a signed-in administrator; GET /instance names the server by its name, the same in every
 * locale. Returns the changes of the name (`name <name>`) and of the settings (`<key>=<value>`).
 */
async function administratorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let name: string | undefined = "my example";
  const settings: Record<string, string> = {};
  const changes: string[] = [];
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
        name: name ?? "my.example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "approval" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        federation: { mode: settings["federation_mode"] ?? "both" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, administrator);
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
    if (key === "GET /admin/instance_texts") {
      return json(route, {
        items: name === undefined ? [] : [{ key: "name", body: name, revised_at: "2026-10-16T00:00:00Z" }],
      });
    }
    if (key === "GET /admin/settings") return json(route, instanceSettings(settings));
    if (key === "GET /admin/instance/brand") return json(route, {});
    if (key === "PATCH /admin/settings") {
      const { values } = (request.postDataJSON() ?? {}) as { values: Record<string, string> };
      for (const [setting, value] of Object.entries(values)) {
        changes.push(`${setting}=${value}`);
        settings[setting] = value;
      }
      return json(route, instanceSettings(settings));
    }
    if (key === "PUT /admin/instance_texts/name") {
      const { body } = (request.postDataJSON() ?? {}) as { body: string };
      changes.push(`name ${body}`);
      name = body === "" ? undefined : body;
      return json(route, { key: "name", body, revised_at: "2026-10-17T00:00:00Z" });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return changes;
}

test.describe("page /admin/settings", () => {
  test.beforeEach(async ({ page }) => {
    await administratorServer(page);
    await page.goto("/admin/settings");
    await expect(page.getByRole("main").getByRole("textbox").first()).toHaveValue("my example");
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test.describe("server settings", () => {
  test("names the server, and the tab title and the sidebar call it by the new name", async ({
    page,
    makeAxeBuilder,
  }) => {
    const changes = await administratorServer(page);
    await page.goto("/admin/settings");
    const main = page.getByRole("main");
    const field = main.getByRole("textbox").first();
    await expect(field).toHaveValue("my example");
    await expect(page).toHaveTitle(/my example/);
    const section = main.getByRole("region", { name: message("admin.settings.name.title"), exact: true });
    // One field: the name is the same in every language.
    await expect(section).toMatchAriaSnapshot(`
      - region:
        - heading [level=2]
        - paragraph: /my\\.example/
        - text: /.+/
        - textbox: my example
        - button
        - status
    `);

    await field.fill(longName);
    await section.getByRole("button").click();
    await expect(section.getByRole("status")).not.toBeEmpty();
    expect(changes).toEqual([`name ${longName}`]);
    await expect(page).toHaveTitle(new RegExp(longName));
    if ((page.viewportSize()?.width ?? 0) >= 1024) {
      // The sidebar shows the server's name above the list from lg on.
      await expect(page.getByRole("navigation").getByText(longName)).toBeVisible();
    }
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("server policies", () => {
  test("takes invites only, with the keyboard", async ({ page, makeAxeBuilder }) => {
    const changes = await administratorServer(page);
    await page.goto("/admin/settings");
    const main = page.getByRole("main");
    await expect(main.getByRole("textbox").first()).toHaveValue("my example");
    const registration = main.getByRole("radiogroup", {
      name: message("admin.settings.registration.mode"),
      exact: true,
    });
    // The radio group holds the saved mode (approval); Down Arrow selects the next one, invite, in every direction.
    await registration.getByRole("radio", { checked: true }).focus();
    await page.keyboard.press("ArrowDown");
    await expect(registration.getByRole("radio").nth(2)).toBeChecked();
    const save = main.getByRole("button", { name: message("admin.settings.policies.save"), exact: true });
    await save.focus();
    await page.keyboard.press("Enter");
    await expect(main.getByRole("status").last()).not.toBeEmpty();
    expect(changes).toEqual(["registration_mode=invite"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("narrows the federation direction only after confirming what stops (ADR-0076)", async ({
    page,
    makeAxeBuilder,
  }) => {
    const changes = await administratorServer(page);
    await page.goto("/admin/settings");
    const main = page.getByRole("main");
    await expect(main.getByRole("textbox").first()).toHaveValue("my example");
    const direction = main.getByRole("radiogroup", { name: message("admin.settings.federation.mode"), exact: true });
    await expect(
      direction.getByRole("radio", { name: message("admin.settings.federation.mode.both"), exact: true }),
    ).toBeChecked();
    await direction.getByText(message("admin.settings.federation.mode.inbound"), { exact: true }).click();
    const save = main.getByRole("button", { name: message("admin.settings.policies.save"), exact: true });
    await save.click();

    // The dialog lists what stops and keeps the focus on the way out.
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("listitem")).toHaveCount(2);
    await expect(
      dialog.getByRole("button", { name: message("admin.settings.federation.narrow.cancel"), exact: true }),
    ).toBeFocused();
    expect(changes).toEqual([]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await dialog
      .getByRole("button", { name: message("admin.settings.federation.narrow.confirm"), exact: true })
      .click();
    await expect(dialog).toBeHidden();
    await expect(main.getByRole("status").last()).not.toBeEmpty();
    expect(changes).toEqual(["federation_mode=inbound"]);
  });
});
