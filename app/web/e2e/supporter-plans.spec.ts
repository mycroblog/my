// SPDX-License-Identifier: AGPL-3.0-only
// The user's page of the supporter plans and the supporter list in every width × locale project (ADR-0071 §4,
// ADR-0072 §6, §8, §13, ADR-0027): a user chooses a plan with a name in Arabic (dir="auto"), reads the final
// confirmation and goes to the provider's checkout; a member sees their subscription and its cancel dialog; a visitor
// reads the supporter list by plan. Each step passes axe without scrolling sideways. `vite preview` has no API, so the
// test plays the server with page.route; names change with the locale, so the elements are found by role and by the
// names in the project's locale catalog.
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

const basicId = "0192f0c4-7a8e-7000-8000-0000000000c1";
const goldId = "0192f0c4-7a8e-7000-8000-0000000000c2";
const plans: Schemas["SupporterPlan"][] = [
  { id: basicId, name: "Basic", description: "", rank: 1, monthly_price: { amount: "500", currency: "JPY" }, yearly_price: { amount: "5400", currency: "JPY" } },
  {
    id: goldId,
    name: "داعم ذهبي",
    description: "A thank-you on the about page.",
    rank: 2,
    monthly_price: { amount: "1000", currency: "JPY" },
    yearly_price: { amount: "10800", currency: "JPY" },
  },
];

const checkoutUrl = "https://checkout.payments.example/c/session";

function account(n: number, name: string): Schemas["Account"] {
  const id = `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`;
  const username = name.toLowerCase();
  return {
    id,
    username,
    acct: username,
    domain: "my.example",
    uri: `https://my.example/ap/actors/${id}`,
    display_name: name,
    note: "",
    fields: [],
    emojis: [],
    badges: [],
    url: `https://my.example/@${username}`,
    locked: false,
    bot: false,
    created_at: "2026-10-01T00:00:00Z",
  };
}

const viewer = {
  ...account(1, "Alice"),
  discoverable: true,
  indexable: true,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: [],
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Plays the API: the program enabled with the list public, the viewer's subscription `own` (signed in when `signedIn`). */
async function serveApi(page: Page, { own, signedIn }: { own: Schemas["OwnSupporterSubscription"]; signedIn: boolean }) {
  const locale = test.info().project.use.locale ?? "en";
  const sent: string[] = [];
  await page.route("**/oauth/token/refresh", (route) =>
    signedIn
      ? json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow" })
      : json(route, { error: "invalid_grant" }, 400),
  );
  await page.route(`${checkoutUrl}*`, (route) => route.fulfill({ status: 200, contentType: "text/html", body: "<!doctype html><title>Checkout</title>" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      const instance: Schemas["Instance"] = {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "approval" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        federation: { mode: "both" },
        sends_email: true,
        search_indexing: "deny",
        ai_usage: "deny",
        finance_visibility: "private",
        income_visibility: "private",
        supporters_enabled: true,
        supporter_list_visibility: "public",
        rules: [],
      };
      return json(route, instance);
    }
    if (key === "GET /accounts/verify_credentials") return json(route, viewer);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /instance/supporter_plans") return json(route, { items: plans });
    if (key === "GET /supporter/subscription") return json(route, own);
    if (key === "POST /supporter/checkout") {
      const body = request.postDataJSON() as Schemas["SupporterPlanChoice"];
      sent.push(`checkout ${body.plan_id} ${body.interval}`);
      return json(route, { subscription_id: "0192f0c4-7a8e-7000-8000-0000000000e1", redirect_url: checkoutUrl });
    }
    if (key === "GET /instance/supporters") {
      return json(route, {
        plans: plans.map((p) => ({ id: p.id, name: p.name })),
        items: [
          { plan_id: goldId, account: account(3, "Carol") },
          { plan_id: basicId, account: account(4, "Dave") },
        ],
      });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("the supporter plans", () => {
  test("chooses a plan, reads the final confirmation and goes to the checkout", async ({ page, makeAxeBuilder }) => {
    const sent = await serveApi(page, { own: {}, signedIn: true });
    await page.goto("/settings/supporter");
    const main = page.getByRole("main");
    const planGroup = main.getByRole("radiogroup", { name: message("supporter.plans.plan"), exact: true });
    await expect(planGroup.getByText("داعم ذهبي")).toHaveAttribute("dir", "auto");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // React Aria hides the native radio under its drawing; the label is what the user presses.
    await planGroup.getByText("داعم ذهبي").click();
    await main.getByRole("button", { name: message("supporter.subscribe"), exact: true }).click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(message("supporter.confirm.benefits.value"), { exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await dialog.getByRole("button", { name: message("supporter.confirm.go"), exact: true }).click();
    await page.waitForURL(`${checkoutUrl}*`);
    expect(sent).toEqual([`checkout ${goldId} month`]);
  });

  test("shows a member's subscription and the dialog that cancels it", async ({ page, makeAxeBuilder }) => {
    await serveApi(page, {
      own: {
        subscription: {
          id: "0192f0c4-7a8e-7000-8000-0000000000e1",
          state: "active",
          plan_id: goldId,
          plan_name: "داعم ذهبي",
          interval: "year",
          price: { amount: "10800", currency: "JPY" },
          period_ends_at: "2027-10-20T00:00:00Z",
          started_at: "2026-10-20T00:00:00Z",
        },
      },
      signedIn: true,
    });
    await page.goto("/settings/supporter");
    const main = page.getByRole("main");
    const region = main.getByRole("region", { name: message("supporter.subscription.title"), exact: true });
    await expect(region.getByText("داعم ذهبي")).toHaveAttribute("dir", "auto");
    await expect(main.getByRole("region", { name: message("supporter.change.title"), exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await region.getByRole("button", { name: message("supporter.cancel_subscription"), exact: true }).click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("supporter.keep"), exact: true }).click();
    await expect(dialog).toBeHidden();
  });

  test("lists the supporters who chose to be listed, by plan, to a visitor", async ({ page, makeAxeBuilder }) => {
    await serveApi(page, { own: {}, signedIn: false });
    await page.goto("/about/supporters");
    const main = page.getByRole("main");
    await expect(main.getByRole("region", { name: "داعم ذهبي", exact: true }).getByRole("link")).toContainText("Carol");
    await expect(main).toMatchAriaSnapshot(`
      - main:
        - heading [level=1]
        - paragraph
        - region "Basic":
          - heading "Basic" [level=2]
          - list:
            - listitem:
              - link /Dave @dave/
        - region "داعم ذهبي":
          - heading "داعم ذهبي" [level=2]
          - list:
            - listitem:
              - link /Carol @carol/
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
