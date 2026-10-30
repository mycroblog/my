// SPDX-License-Identifier: AGPL-3.0-only
// The terms of service and the privacy policy (#489) in every width × locale project: the registration page
// links to the web client's pages of the documents that e2e/server.ts writes (against the real `my serve`),
// the app links to them, and an administrator writes them on the terms and policies screen (against
// `vite preview`, whose API each test plays with page.route). Names change with the locale, so elements are found
// by role, structure, addresses, and the documents' own text, which is in English in every project.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { serverURL } from "./server-origin.ts";

test.describe("the registration page on the server", () => {
  test.use({ baseURL: serverURL });

  test("the registration page asks to agree to both and links to them", async ({ page }) => {
    await page.goto("/auth/sign_up");
    const main = page.getByRole("main");
    await expect(main.locator("#sign-up-agreement")).toBeVisible();
    // A required checkbox agrees to them, described by the notice above the links.
    const agreement = main.locator('input[name="agreement"]');
    await expect(agreement).toHaveAttribute("required", "");
    await expect(agreement).toHaveAttribute("aria-describedby", "sign-up-agreement");
    await expect(main.locator('.agreement a[href="/about/terms"]')).toBeVisible();
    await expect(main.locator('.agreement a[href="/about/privacy"]')).toBeVisible();
    // They open in a new tab, so the form keeps what was typed.
    await expect(main.locator(".agreement a")).toHaveCount(2);
    for (const link of await main.locator(".agreement a").all()) await expect(link).toHaveAttribute("target", "_blank");
    // Every server page links to them in its footer too, in the same tab.
    const footer = page.getByRole("contentinfo").getByRole("navigation");
    await expect(footer.getByRole("link")).toHaveCount(2);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("the pages of the documents on the server", () => {
  test.use({ baseURL: serverURL });

  test("the terms of service are an information page with a table of contents, without axe violations or scrolling sideways", async ({
    page,
    makeAxeBuilder,
  }) => {
    await page.goto("/about/terms");
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(main.getByRole("heading", { level: 2, name: "Accounts" })).toBeVisible();
    await expect(main.getByText("No spam")).toBeVisible();
    // The table of contents is a collapsed disclosure on a narrow screen, so it is attached but not always visible.
    await expect(main.locator('nav[aria-label] a[href^="#"]', { hasText: "Accounts" }).first()).toBeAttached();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("the privacy policy is one too, and the commerce disclosures, which the server does not have, are a not-found page", async ({
    page,
  }) => {
    await page.goto("/about/privacy");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    const missing = await page.goto("/about/commerce");
    expect(missing?.status()).toBeLessThan(500);
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    // The not-found state has its own heading, and no table of contents.
    await expect(page.getByRole("main").getByRole("heading", { level: 2 })).toHaveCount(1);
    await expect(page.getByRole("main").locator("nav")).toHaveCount(0);
  });
});

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** The administrator's account, as GET /accounts/verify_credentials answers, with manage_settings. */
const administrator = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
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

/**
 * Plays the server of a signed-in administrator, with terms of service and no privacy policy; the
 * texts change with what the screen saves. Returns the saves.
 */
async function administratorServer(page: Page, origin: string) {
  const locale = test.info().project.use.locale ?? "en";
  let texts = [{ key: "terms", body: "# Accounts\n\nBe kind.", revised_at: "2026-10-12T08:00:00Z" }];
  const saves: string[] = [];
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
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
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
        sends_email: true,
        search_indexing: "deny",
        ai_usage: "deny",
        rules: [],
        ...(texts.some((text) => text.key === "terms") && { terms_url: `${origin}/about/terms` }),
        ...(texts.some((text) => text.key === "privacy_policy") && { privacy_policy_url: `${origin}/about/privacy` }),
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
    if (key === "GET /notifications/marker") return json(route, {});
    if (key === "GET /admin/instance_texts") return json(route, { items: texts });
    const put = /^PUT \/admin\/instance_texts\/([^/]+)$/.exec(key);
    if (put) {
      const { body } = request.postDataJSON() as { body: string };
      const text = { key: put[1] ?? "", body, revised_at: "2026-10-17T00:00:00Z" };
      saves.push(`${text.key} ${body}`);
      texts = [...texts.filter((item) => item.key !== text.key), ...(body === "" ? [] : [text])];
      return json(route, text);
    }
    if (key === "GET /streaming") return route.fulfill({ status: 200, contentType: "text/event-stream", body: "" });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return saves;
}

test.describe("the terms and policies screen", () => {
  test("writes the privacy policy with the keyboard, without axe violations or scrolling sideways", async ({
    page,
    baseURL,
    makeAxeBuilder,
  }) => {
    const saves = await administratorServer(page, new URL(baseURL ?? "").origin);
    await page.goto("/admin/documents");
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(main.getByRole("textbox").first()).toHaveValue("# Accounts\n\nBe kind.");
    await expect(main).toMatchAriaSnapshot(`
      - main:
        - heading [level=1]
        - region:
          - heading [level=2]
          - term
          - definition
          - paragraph:
            - link
          - textbox
          - button
        - region:
          - heading [level=2]
          - term
          - definition
          - textbox
          - button
        - region:
          - heading [level=2]
          - paragraph
          - term
          - definition
          - textbox
          - button
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const privacy = main.getByRole("region").nth(1);
    const field = privacy.getByRole("textbox");
    await field.focus();
    await page.keyboard.type("# What we keep\n\nYour email address.");
    await page.keyboard.press("Tab");
    // The character count is between the field and the button only for screen readers; Tab reaches the button.
    await expect(privacy.getByRole("button")).toBeFocused();
    await page.keyboard.press("Enter");
    // The form's StatusMessage shows its result in a paragraph.
    await expect(privacy.locator('[role="status"] p')).toBeVisible();
    expect(saves).toEqual(["privacy_policy # What we keep\n\nYour email address."]);
    // The page of the new policy is linked once it is saved.
    await expect(privacy.getByRole("link")).toHaveAttribute("href", "/about/privacy");
    expect(await privacy.getByRole("link").getAttribute("target")).toBeNull();
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("the footer of the app's signed-out pages links to the documents", async ({ page, baseURL }) => {
    await administratorServer(page, new URL(baseURL ?? "").origin);
    await page.route("**/oauth/token/refresh", (route) => json(route, { error: "invalid_grant" }, 400));
    await page.goto("/search");
    const links = page.getByRole("contentinfo").getByRole("navigation");
    await expect(links.getByRole("link")).toHaveCount(1);
    await expect(links.getByRole("link")).toHaveAttribute("href", "/about/terms");
    await expectNoHorizontalScroll(page);
  });
});
