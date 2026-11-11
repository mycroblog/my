// SPDX-License-Identifier: AGPL-3.0-only
// The tips in every width × locale project (ADR-0162 §17, §19, ADR-0110): a visitor reads on the information page who
// may tip and the way to sign in; a user chooses an amount, reads the final confirmation and goes to the provider's
// checkout; back from it, the user reads only the thanks. Each step passes axe without scrolling sideways. `vite
// preview` has no API, so the test plays the server with page.route; names change with the locale, so the elements are
// found by role and by the names in the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { preferencesOf } from "./preferences.ts";

type Schemas = components["schemas"];

/** The client message `key` in the project's locale, with each `{name}` of `values` replaced. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return Object.entries(values).reduce((out, [name, value]) => out.replace(`{${name}}`, value), text);
}

const checkoutUrl = "https://checkout.payments.example/c/tip";
const tipId = "0192f0c4-7a8e-7000-8000-0000000000f1";

const viewer = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  discoverable: true,
  indexable: true,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: [],
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the API: the program enabled and the tips taken (signed in when `signedIn`), with the tip in `tipState`. */
async function serveApi(
  page: Page,
  { signedIn, tipState }: { signedIn: boolean; tipState?: Schemas["SupporterTipState"] },
) {
  const locale = test.info().project.use.locale ?? "en";
  const sent: unknown[] = [];
  await page.route("**/oauth/token/refresh", (route) =>
    signedIn
      ? json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow" })
      : json(route, { error: "invalid_grant" }, 400),
  );
  await page.route(`${checkoutUrl}*`, (route) =>
    route.fulfill({ status: 200, contentType: "text/html", body: "<!doctype html><title>Checkout</title>" }),
  );
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
        login_providers_only: false,
        registrations: { mode: "approval" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        federation: { mode: "both" },
        sends_email: true,
        search_indexing: "deny",
        ai_usage: "deny",
        link_cards: true,
        link_cards_ignore_robots_txt: false,
        finance_visibility: "private",
        income_visibility: "private",
        supporters_enabled: true,
        tips_enabled: true,
        supporter_list_visibility: "off",
        voice_enabled: false,
        voice_room_starters: "permitted",
        full_text_search: { available: false, visibilities: [] },
        translation: { browser: false, providers: [] },
        single_user: false,
        rules: [],
      };
      return json(route, instance);
    }
    if (key === "GET /accounts/verify_credentials") return json(route, viewer);
    if (key === "GET /preferences") return json(route, preferencesOf({ locale }));
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /instance/supporter_plans") return json(route, { items: [] });
    if (key === "GET /supporter/subscription") return json(route, {});
    if (key === "GET /instance/tip_amounts") {
      return json(route, {
        amounts: [
          { amount: "300", currency: "JPY" },
          { amount: "500", currency: "JPY" },
          { amount: "1000", currency: "JPY" },
        ],
      } satisfies Schemas["TipAmounts"]);
    }
    if (key === "POST /supporter/tips") {
      sent.push(request.postDataJSON());
      return json(route, { tip_id: tipId, redirect_url: checkoutUrl } satisfies Schemas["SupporterTipCheckout"]);
    }
    if (key === `GET /supporter/tips/${tipId}`) {
      return json(route, {
        id: tipId,
        state: tipState ?? "open",
        amount: { amount: "500", currency: "JPY" },
        created_at: "2026-10-22T00:00:00Z",
        ...(tipState === "paid" && { paid_at: "2026-10-22T00:01:00Z" }),
      } satisfies Schemas["SupporterTip"]);
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("the tips", () => {
  test("tell a visitor on the about page who may tip and the way to sign in", async ({ page, makeAxeBuilder }) => {
    await serveApi(page, { signedIn: false });
    await page.goto("/about");
    const section = page.getByRole("main").getByRole("region", { name: message("tip.about.title"), exact: true });
    await expect(section.getByText(message("tip.entry.eligibility"), { exact: true })).toBeVisible();
    await expect(section.getByRole("link", { name: message("tip.entry.sign_in"), exact: true })).toHaveAttribute(
      "href",
      "/login?return_to=%2Fsettings%2Fsupporter%2Ftip",
    );
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("chooses an amount, reads the final confirmation and goes to the checkout", async ({ page, makeAxeBuilder }) => {
    const sent = await serveApi(page, { signedIn: true });
    await page.goto("/settings/supporter/tip");
    const main = page.getByRole("main");
    const amounts = main.getByRole("radiogroup", { name: message("tip.form.amount"), exact: true });
    await expect(amounts).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The amounts' names change with the locale's format.
    await amounts.getByRole("radio").nth(2).check();
    await main.getByRole("button", { name: message("tip.form.submit"), exact: true }).click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog.getByText(message("tip.confirm.return.value"), { exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await dialog.getByRole("button", { name: message("tip.confirm.go"), exact: true }).click();
    await page.waitForURL(`${checkoutUrl}*`);
    expect(sent).toEqual([{ amount: { amount: "1000", currency: "JPY" } }]);
  });

  test("shows only the thanks back from the checkout of a paid tip", async ({ page, makeAxeBuilder }) => {
    await serveApi(page, { signedIn: true, tipState: "paid" });
    await page.goto(`/settings/supporter/tip?tip=${tipId}&checkout=success`);
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { name: message("tip.thanks.title"), exact: true })).toBeVisible();
    await expect(main.getByRole("radiogroup")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
