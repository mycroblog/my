// SPDX-License-Identifier: AGPL-3.0-only
// The closure of the service in every width × locale project (FR-808, FR-809, ADR-0088 §2): while it is announced, the notice
// at the top of the sign-in page and the information page shows the dates, the operators' message and a link to the
// export that the keyboard reaches; once the server has closed, signing in gives the closed screen alone, where the
// archives of the data can be asked for. `vite preview` has no API, so each test plays the server with page.route.
// Texts come from the project's catalog.
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

const user = {
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
  permissions: [],
};

/** The operators' message, long enough in one word to show that the notice wraps it rather than scroll sideways. */
const operatorMessage = `Thank you for the years${"-and-more".repeat(8)}`;

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Plays the server with the closure `state`; `signedIn` makes the OAuth refresh and the account answer. */
async function server(page: Page, state: "announced" | "closed", { signedIn }: { signedIn: boolean }) {
  const locale = test.info().project.use.locale ?? "en";
  await page.route("**/oauth/token/refresh", (route) =>
    signedIn
      ? json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow" })
      : json(route, { error: "invalid_grant" }, 400),
  );
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
        registrations: { mode: "closed" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        federation: { mode: "both" },
        rules: [],
        closure: { state, closes_at: "2026-12-01T00:00:00Z", export_until: "2027-01-15T00:00:00Z", message: operatorMessage },
      });
    }
    if (request.headers()["authorization"] !== "Bearer e2e-token") return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    if (key === "GET /accounts/verify_credentials") return json(route, user);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /exports") return json(route, { items: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

test.describe("the announced closure", () => {
  for (const path of ["/login", "/about"]) {
    test(`shows its notice on ${path}, and the keyboard reaches the export`, async ({ page, makeAxeBuilder }) => {
      await server(page, "announced", { signedIn: false });
      await page.goto(path);
      const notice = page.getByRole("region", { name: message("closure.label"), exact: true });
      await expect(notice).toBeVisible();
      await expect(notice).toContainText(operatorMessage);
      await expect(notice.getByRole("link", { name: message("closure.announced.export"), exact: true })).toHaveAttribute("href", "/settings/account");
      // The link is in the tab order (after the skip link, if the page has one).
      for (let tabs = 0; tabs < 5; tabs += 1) {
        await page.keyboard.press("Tab");
        if (await notice.getByRole("link").evaluate((link) => link === document.activeElement)) break;
      }
      await expect(notice.getByRole("link")).toBeFocused();
      await expectNoAxeViolations(makeAxeBuilder);
      await expectNoHorizontalScroll(page);
    });
  }
});

test.describe("the closed server", () => {
  test("tells a visitor to sign in to take the data out", async ({ page, makeAxeBuilder }) => {
    await server(page, "closed", { signedIn: false });
    await page.goto("/login");
    const notice = page.getByRole("region", { name: message("closure.label"), exact: true });
    await expect(notice).toBeVisible();
    await expect(notice.getByRole("link")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("gives a signed-in user the closed screen alone, with the archives of the data", async ({ page, makeAxeBuilder }) => {
    await server(page, "closed", { signedIn: true });
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1, name: message("closure.closed.title"), exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: message("settings.account.export.request"), exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: message("closure.closed.sign_out"), exact: true })).toBeVisible();
    await expect(page.getByRole("navigation")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
