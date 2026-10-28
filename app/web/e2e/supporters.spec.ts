// SPDX-License-Identifier: AGPL-3.0-only
// The supporters screen in every width × locale project (ADR-0072, ADR-0027): an administrator with
// manage_supporters sees the program and its warning, the payment provider's state with how to fix its last
// reconciliation, the plans with a name in Arabic (dir="auto") and the subscriptions, reconciles now and sees why a
// second one is too soon, enables the program, creates a plan, retires one and ends a subscription after their dialogs. Each
// step passes axe without scrolling sideways. `vite preview` has no API, so the test plays the server with page.route;
// names change with the locale, so the elements are found by role and by the names in the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];
/** The client message `key` in the project's locale, with each `{name}` of `values` replaced. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return Object.entries(values).reduce((out, [name, value]) => out.replace(`{${name}}`, value), text);
}

/** The signed-in administrator, as GET /accounts/verify_credentials answers. */
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
  permissions: ["manage_supporters"],
};

function plan(id: string, overrides: Partial<Schemas["AdminSupporterPlan"]> = {}): Schemas["AdminSupporterPlan"] {
  return {
    id,
    name: "Supporter",
    description: "",
    rank: 1,
    visibility: "public",
    monthly_price: { amount: "500", currency: "JPY" },
    yearly_price: { amount: "5400", currency: "JPY" },
    annual_discount_percent: 10,
    emoji_limit: 5,
    role: { id: `role-${id}`, key: "supporter", position: 10, permissions: [] },
    prices_ready: true,
    retired: false,
    members: 1,
    created_at: "2026-10-20T00:00:00Z",
    revised_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

const subscription: Schemas["AdminSupporterSubscription"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e1",
  state: "active",
  plan_id: "0192f0c4-7a8e-7000-8000-0000000000c1",
  plan_name: "Supporter",
  interval: "month",
  price: { amount: "500", currency: "JPY" },
  period_ends_at: "2026-11-20T00:00:00Z",
  account_id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  started_at: "2026-10-20T00:00:00Z",
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the API of the administrator: the program, the plans and the subscriptions change with each write, which `sent` records. */
async function serveApi(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const sent: string[] = [];
  let enabled = false;
  let plans = [
    plan("0192f0c4-7a8e-7000-8000-0000000000c1"),
    plan("0192f0c4-7a8e-7000-8000-0000000000c2", { name: "داعم ذهبي", rank: 2, members: 0 }),
  ];
  let subscriptions = [subscription];
  let reconciles = 0;
  const paymentStatus: Schemas["PaymentStatus"] = {
    provider: "stripe",
    mode: "test",
    time_zone: "UTC",
    last_reconcile: {
      outcome: "failed",
      error_kind: "permission",
      requests: 1,
      checked_subscriptions: 0,
      corrected_subscriptions: 0,
      occurred_at: "2026-10-22T00:00:00Z",
    },
    warnings: ["no_webhook"],
  };
  const program = (): Schemas["AdminSupporterProgram"] => ({
    enabled,
    ...(enabled ? { provider: "stripe" } : {}),
    configured_provider: "stripe",
    sends_email: true,
    commerce_disclosure: false,
    live_subscriptions: subscriptions.filter((s) => s.state === "active").length,
    can_enable: true,
    warnings: ["no_commerce_disclosure"],
  });
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
      return json(route, {
        locale,
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "und",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /admin/counts")
      return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    if (key === "GET /admin/supporter_program") return json(route, program());
    if (key === "PUT /admin/supporter_program") {
      enabled = (request.postDataJSON() as Schemas["AdminSupporterProgramRequest"]).enabled;
      sent.push(`program ${String(enabled)}`);
      return json(route, program());
    }
    if (key === "GET /admin/payment_status") return json(route, paymentStatus);
    if (key === "POST /admin/payment_reconciles") {
      reconciles += 1;
      sent.push(`reconcile ${String(reconciles)}`);
      if (reconciles > 1) return json(route, { type: "about:blank", title: "Too Many Requests", status: 429 }, 429);
      return route.fulfill({ status: 204 });
    }
    if (key === "GET /admin/supporter_plans") return json(route, { items: plans });
    if (key === "POST /admin/supporter_plans") {
      const body = request.postDataJSON() as Schemas["AdminSupporterPlanCreateRequest"];
      sent.push(`create ${body.role} ${body.monthly_price.amount} ${body.monthly_price.currency}`);
      const created = plan("0192f0c4-7a8e-7000-8000-0000000000c3", {
        name: body.name,
        rank: body.rank,
        prices_ready: false,
        members: 0,
      });
      plans = [...plans, created];
      return json(route, created);
    }
    if (/^POST \/admin\/supporter_plans\/[^/]+\/retire$/.test(key)) {
      sent.push("retire");
      plans = plans.map((p) => (path.includes(p.id) ? { ...p, retired: true, retired_at: "2026-10-22T00:00:00Z" } : p));
      return route.fulfill({ status: 204 });
    }
    if (key === "GET /admin/supporter_subscriptions") return json(route, { items: subscriptions });
    if (/^POST \/admin\/supporter_subscriptions\/[^/]+\/cancel$/.test(key)) {
      sent.push("cancel");
      subscriptions = subscriptions.map((s) => ({ ...s, state: "expired" }));
      return route.fulfill({ status: 204 });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("supporters", () => {
  test("shows the payment provider's state and reconciles now once an hour", async ({ page, makeAxeBuilder }) => {
    const sent = await serveApi(page);
    await page.goto("/admin/supporters");
    const payment = page
      .getByRole("main")
      .getByRole("region", { name: message("admin.supporters.payment.title"), exact: true });
    await expect(payment.getByText(message("admin.supporters.payment.mode.test"), { exact: true })).toBeVisible();
    await expect(
      payment.getByText(message("admin.supporters.payment.error.permission"), { exact: true }),
    ).toBeVisible();
    await expect(
      payment.getByText(message("admin.supporters.payment.warning.no_webhook"), { exact: true }),
    ).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const reconcile = payment.getByRole("button", {
      name: message("admin.supporters.payment.reconcile.run"),
      exact: true,
    });
    await reconcile.click();
    await expect(payment.getByRole("status")).toHaveText(message("admin.supporters.payment.reconcile.queued"));
    await reconcile.click();
    await expect(payment.getByRole("alert")).toHaveText(message("admin.supporters.payment.reconcile.too_soon"));
    expect(sent).toEqual(["reconcile 1", "reconcile 2"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("enables the program, creates and retires a plan, and ends a subscription after their dialogs", async ({
    page,
    makeAxeBuilder,
  }) => {
    const sent = await serveApi(page);
    await page.goto("/admin/supporters");
    const main = page.getByRole("main");
    const programRegion = main.getByRole("region", { name: message("admin.supporters.program.title"), exact: true });
    await expect(
      programRegion.getByText(message("admin.supporters.warning.no_commerce_disclosure"), { exact: false }),
    ).toBeVisible();
    const plansRegion = main.getByRole("region", { name: message("admin.supporters.plans.title"), exact: true });
    await expect(plansRegion.getByText("داعم ذهبي")).toHaveAttribute("dir", "auto");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // React Aria hides the native switch under its drawing; the label is what the user presses.
    await programRegion.getByText(message("admin.supporters.program.switch"), { exact: true }).click();
    await expect(
      programRegion.getByRole("switch", { name: message("admin.supporters.program.switch"), exact: true }),
    ).toBeChecked();
    expect(sent).toContain("program true");

    const form = main.getByRole("region", { name: message("admin.supporters.new.title"), exact: true });
    await form.getByRole("textbox", { name: message("admin.supporters.plan.role"), exact: true }).fill("fan");
    await form.getByRole("textbox", { name: message("admin.supporters.plan.name"), exact: true }).fill("Fan");
    await form.getByRole("textbox", { name: message("admin.supporters.plan.amount"), exact: true }).fill("300");
    await form.getByRole("textbox", { name: message("admin.supporters.plan.rank"), exact: true }).fill("3");
    await form.getByRole("button", { name: message("admin.supporters.new.submit"), exact: true }).click();
    await expect(plansRegion.getByRole("heading", { level: 3, name: "Fan" })).toBeVisible();
    await expect(plansRegion.getByText(message("admin.supporters.plan.preparing"), { exact: true })).toBeVisible();
    expect(sent).toContain("create fan 300 JPY");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await plansRegion
      .getByRole("button", { name: message("admin.supporters.plan.retire_plan", { name: "⁨Fan⁩" }), exact: true })
      .click();
    const retireDialog = page.getByRole("alertdialog");
    await expect(retireDialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await retireDialog.getByRole("button", { name: message("admin.supporters.plan.retire"), exact: true }).click();
    await expect(
      main.getByRole("region", { name: message("admin.supporters.retired.title"), exact: true }),
    ).toBeVisible();
    expect(sent).toContain("retire");

    const subscriptionsRegion = main.getByRole("region", {
      name: message("admin.supporters.subscriptions.title"),
      exact: true,
    });
    await subscriptionsRegion
      .getByRole("button", {
        name: message("admin.supporters.subscription.cancel_of", { name: "⁦@bob⁩" }),
        exact: true,
      })
      .click();
    const cancelDialog = page.getByRole("alertdialog");
    await expect(cancelDialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await cancelDialog
      .getByRole("button", { name: message("admin.supporters.subscription.cancel"), exact: true })
      .click();
    await expect(
      subscriptionsRegion.getByRole("cell", { name: message("admin.supporters.state.expired"), exact: true }),
    ).toBeVisible();
    expect(sent).toContain("cancel");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
