// SPDX-License-Identifier: AGPL-3.0-only
// The service closure screen in every width × locale project (FR-808, FR-809, ADR-0088, NFR-MOD-7): an administrator
// announces the closure and is warned of a short notice, sees the check of what closing ends before the
// confirmation, and closes the server only after typing its domain in an alert dialog; the progress of the
// deliveries then shows. Each step passes axe without scrolling sideways. `vite preview` has no API, so each test
// plays the server with page.route; names change with the locale, so the elements are found by role and by the
// names in the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];

/** The client message `key` in the project's locale, with each `{name}` of `values` replaced. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return Object.entries(values).reduce((out, [name, value]) => out.replace(`{${name}}`, value), text);
}

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

const account = {
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
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["manage_service_closure"],
};

const check: Schemas["AdminServiceClosureCheck"] = {
  local_accounts: 12,
  delivery_inboxes: 340,
  estimated_deletes: 4080,
  live_supporter_subscriptions: 3,
  pending_migrations: 2,
  retention_holds: 1,
};

/** The operators' message, long enough in one word to show that the screen wraps it rather than scroll sideways. */
const operatorMessage = `Thank you for the years${"-and-more".repeat(8)}`;

/** Plays the API of an administrator, with the closure `initial`; each write moves it like the server. */
async function serveApi(page: Page, initial: Schemas["AdminServiceClosure"]) {
  let closure = initial;
  const sent: string[] = [];
  const locale = test.info().project.use.locale ?? "en";
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow admin" }));
  await page.route("**/api/v1/**", (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
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
    if (key === "GET /accounts/verify_credentials") return json(route, account);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /admin/counts") return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    if (key === "GET /admin/closure") return json(route, closure);
    if (key === "GET /admin/closure/check") return json(route, check);
    if (key === "PUT /admin/closure/announcement") {
      const body = request.postDataJSON() as Schemas["AnnounceServiceClosureRequest"];
      sent.push("announce");
      closure = {
        state: "announced",
        closes_at: body.closes_at,
        export_until: body.export_until ?? body.closes_at,
        ...(body.message === undefined ? {} : { message: body.message }),
        announced_at: "2026-10-25T00:00:00Z",
        short_notice: true,
      };
      return json(route, closure);
    }
    if (key === "POST /admin/closure/close") {
      const body = request.postDataJSON() as Schemas["CloseServiceRequest"];
      sent.push(`close ${body.confirm_domain}`);
      closure = {
        ...closure,
        state: "closed",
        progress: { accounts: 12, fanouts_pending: 3, deliveries: { pending: 300, succeeded: 3000, permanent_failure: 20, skipped: 5, dead: 2 } },
      };
      return json(route, closure);
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("the service closure screen", () => {
  test("announces the closure and warns of a short notice", async ({ page, makeAxeBuilder }) => {
    const sent = await serveApi(page, { state: "none", short_notice: false });
    await page.goto("/admin/closure");
    const form = page.getByRole("main").getByRole("region", { name: message("admin.closure.announce.title"), exact: true });
    await expect(form).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Without a closing date, the form says what is missing.
    await form.getByRole("button", { name: message("admin.closure.announce"), exact: true }).click();
    await expect(form.getByText(message("admin.closure.closes_at.required"), { exact: true })).toBeVisible();

    const soon = new Date(Date.now() + 5 * 86_400_000);
    const pad = (n: number) => String(n).padStart(2, "0");
    await form.getByLabel(message("admin.closure.closes_at"), { exact: false }).first().fill(`${String(soon.getFullYear())}-${pad(soon.getMonth() + 1)}-${pad(soon.getDate())}T12:00`);
    await expect(form.getByText(message("admin.closure.short_notice", { days: "30" }), { exact: true })).toBeVisible();
    await form.getByRole("textbox", { name: message("admin.closure.message"), exact: true }).fill(operatorMessage);
    await expectNoAxeViolations(makeAxeBuilder);
    await form.getByRole("button", { name: message("admin.closure.announce"), exact: true }).click();

    await expect(page.getByRole("status").getByText(message("admin.closure.announced"), { exact: true })).toBeVisible();
    expect(sent).toEqual(["announce"]);
    await expect(page.getByRole("main").getByRole("region", { name: message("admin.closure.current.title"), exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("shows the check, then closes only after the domain is typed", async ({ page, makeAxeBuilder }) => {
    const sent = await serveApi(page, {
      state: "announced",
      closes_at: "2026-10-01T00:00:00Z",
      export_until: "2026-11-01T00:00:00Z",
      message: operatorMessage,
      announced_at: "2026-08-01T00:00:00Z",
      short_notice: false,
    });
    await page.goto("/admin/closure");
    const section = page.getByRole("main").getByRole("region", { name: message("admin.closure.close.title"), exact: true });
    await expect(section.getByText(message("admin.closure.check.migrations.warning"), { exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await section.getByRole("button", { name: message("admin.closure.close"), exact: true }).click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    const field = dialog.getByRole("textbox", { name: message("admin.closure.close.domain"), exact: true });
    await expect(field).toBeFocused();

    // A wrong domain keeps the dialog open and sends nothing.
    await field.fill("other.example");
    await dialog.getByRole("button", { name: message("admin.closure.close.confirm"), exact: true }).click();
    await expect(dialog).toBeVisible();
    expect(sent).toEqual([]);

    await field.fill("MY.example");
    await dialog.getByRole("button", { name: message("admin.closure.close.confirm"), exact: true }).click();
    await expect(page.getByRole("status").getByText(message("admin.closure.closed"), { exact: true })).toBeVisible();
    expect(sent).toEqual(["close MY.example"]);
    const progress = page.getByRole("main").getByRole("region", { name: message("admin.closure.progress.title"), exact: true });
    await expect(progress.getByText(message("admin.closure.progress.dead"), { exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
