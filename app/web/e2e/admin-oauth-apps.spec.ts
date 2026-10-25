// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps admin screen in every width × locale project (ADR-0027, ADR-0090 §1): an administrator with
// manage_oauth_apps disables an app after the alert dialog and enables it again, passing axe without scrolling
// sideways. `vite preview` has no API, so the test plays the server of a signed-in operator with page.route; names
// change with the locale, so they come from the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** The whole name of a button whose message `key` holds the app's `{name}`, which comes in a directional isolate. */
function appButtonName(key: string): RegExp {
  const escape = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const [before = "", after = ""] = message(key).split("{name}");
  return new RegExp(`^${escape(before)}[\u2066-\u2068]${escape(appName)}\u2069${escape(after)}$`, "u");
}

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
  permissions: ["manage_oauth_apps"],
};

const appId = "0192f0c4-7a8e-7000-8000-0000000000a1";
const appName = "A-very-long-named-app-for-testing-that-wraps-at-320-pixels";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Plays the admin API of a signed-in operator with one user-registered app; returns the changes made. */
async function operatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let disabled = false;
  const actions: string[] = [];
  const client = () => ({
    id: appId,
    client_id: "e2e-client",
    name: appName,
    registration: "user",
    confidential: false,
    scope: "read write",
    owner_id: operator.id,
    disabled,
    created_at: "2026-10-22T00:00:00Z",
  });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow admin" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance") return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, { domain: "my.example", name: "my example", version: "0.0.0", source_url: "https://github.com/mycroblog/my", registrations: { mode: "open" }, federation: { mode: "both" }, limits: { max_post_characters: 500, require_media_description: false }, timelines: { local: "everyone", federated: "everyone" }, rules: [] });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, operator);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /admin/oauth_clients") return json(route, { items: [client()] });
    if (key === `POST /admin/oauth_clients/${appId}/disable`) {
      actions.push(key);
      disabled = true;
      return json(route, client());
    }
    if (key === `POST /admin/oauth_clients/${appId}/enable`) {
      actions.push(key);
      disabled = false;
      return json(route, client());
    }
    if (key === "GET /admin/oauth_client_host_denials") return json(route, { items: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return actions;
}

test.describe("OAuth apps", () => {
  test("disables an app after the dialog and enables it again", async ({ page, makeAxeBuilder }) => {
    const actions = await operatorServer(page);
    await page.goto("/admin/oauth-apps");
    const main = page.getByRole("main");
    const table = main.getByRole("table");
    await expect(table.getByRole("rowheader")).toContainText(appName);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await table.getByRole("button", { name: appButtonName("admin.oauth_apps.disable_app") }).click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("admin.oauth_apps.disable"), exact: true }).click();

    const enable = table.getByRole("button", { name: appButtonName("admin.oauth_apps.enable_app") });
    await expect(enable).toBeVisible();
    await expect(table.getByText(message("admin.oauth_apps.disabled"), { exact: true })).toBeVisible();
    await enable.click();
    await expect(table.getByRole("button", { name: appButtonName("admin.oauth_apps.disable_app") })).toBeVisible();
    expect(actions).toEqual([`POST /admin/oauth_clients/${appId}/disable`, `POST /admin/oauth_clients/${appId}/enable`]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
