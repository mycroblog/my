// SPDX-License-Identifier: AGPL-3.0-only
// The webhooks screen in every width × locale project (ADR-0060, ADR-0027): an administrator sets up a webhook and
// sees its secret once, sees it listed, disables and enables it, and deletes it after its dialog, each step passing
// axe without scrolling sideways. `vite preview` has no API, so each test plays the server of a signed-in
// administrator with page.route; names change with the locale, so the structure is checked by role, by the names in
// the project's locale catalog and by what the server sends.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];
/** The client message `key` in the project's locale, with `{name}` replaced by `name`. */
function message(key: string, name = ""): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text.replace("{name}", name);
}

/** The signed-in administrator's account, as GET /accounts/verify_credentials answers, with manage_webhooks. */
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
  permissions: ["manage_webhooks", "manage_reports"],
};

const secret = "whsec_c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmU=";
const description = "Moderation chat";
/** The webhook in sentences: its description, isolated (useWebhooks.webhookName). */
const name = `⁨${description}⁩`;

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Plays the API of a signed-in administrator with no webhook at first. Returns what was sent. */
async function administratorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const sent: string[] = [];
  let webhooks: Schemas["AdminWebhook"][] = [];
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow admin" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance") return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "approval" }, federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, administrator);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /admin/counts") return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    if (key === "GET /admin/webhooks") return json(route, { items: webhooks });
    if (key === "POST /admin/webhooks") {
      const body = request.postDataJSON() as Schemas["CreateWebhookRequest"];
      sent.push(`create ${body.url} ${body.events.join(",")}`);
      const webhook: Schemas["AdminWebhook"] = {
        id: "0192f0c4-7a8e-7000-8000-0000000000b1",
        url: body.url,
        ...(body.description === undefined ? {} : { description: body.description }),
        events: body.events,
        state: "enabled",
        state_reason: "admin",
        state_changed_at: "2026-10-20T00:00:00Z",
        created_at: "2026-10-20T00:00:00Z",
        updated_at: "2026-10-20T00:00:00Z",
      };
      webhooks = [webhook];
      return json(route, { webhook, secret });
    }
    const state = /^POST \/admin\/webhooks\/([^/]+)\/(enable|disable)$/.exec(key);
    if (state) {
      sent.push(state[2] ?? "");
      webhooks = webhooks.map((webhook) => ({ ...webhook, state: state[2] === "enable" ? "enabled" : "disabled" }));
      return json(route, webhooks[0]);
    }
    if (/^DELETE \/admin\/webhooks\/[^/]+$/.test(key)) {
      sent.push("delete");
      webhooks = [];
      return route.fulfill({ status: 204 });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("webhooks", () => {
  test("sets up a webhook, shows its secret once, disables, enables and deletes it", async ({ page, makeAxeBuilder }) => {
    const sent = await administratorServer(page);
    await page.goto("/admin/webhooks");
    const main = page.getByRole("main");
    const form = main.getByRole("region", { name: message("admin.webhooks.new.title"), exact: true });
    await expect(form).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await form.getByRole("textbox", { name: message("admin.webhooks.url"), exact: true }).fill("https://hooks.example/in");
    await form.getByRole("textbox", { name: message("admin.webhooks.description"), exact: true }).fill(description);
    // The checkbox is visually hidden behind its box; its label is what a pointer presses.
    const events = form.getByRole("group", { name: message("admin.webhooks.events"), exact: true });
    await events.getByText(message("admin.webhooks.event.report_created"), { exact: true }).click();
    await expect(events.getByRole("checkbox", { name: message("admin.webhooks.event.report_created"), exact: true })).toBeChecked();
    await form.getByRole("button", { name: message("admin.webhooks.submit"), exact: true }).click();

    await expect(form.getByText(secret, { exact: true })).toBeVisible();
    expect(sent).toEqual(["create https://hooks.example/in report.created"]);
    const list = main.getByRole("region", { name: message("admin.webhooks.list.title"), exact: true });
    await expect(list.getByRole("rowheader", { name: description })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await list.getByRole("button", { name: message("admin.webhooks.disable_webhook", name), exact: true }).click();
    await expect(list.getByRole("button", { name: message("admin.webhooks.enable_webhook", name), exact: true })).toBeVisible();
    await list.getByRole("button", { name: message("admin.webhooks.enable_webhook", name), exact: true }).click();
    await expect(list.getByRole("button", { name: message("admin.webhooks.disable_webhook", name), exact: true })).toBeVisible();

    await list.getByRole("button", { name: message("admin.webhooks.delete_webhook", name), exact: true }).click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("admin.webhooks.delete"), exact: true }).click();
    await expect(list.getByRole("heading", { level: 3, name: message("admin.webhooks.empty"), exact: true })).toBeVisible();
    expect(sent).toEqual(["create https://hooks.example/in report.created", "disable", "enable", "delete"]);
  });
});
