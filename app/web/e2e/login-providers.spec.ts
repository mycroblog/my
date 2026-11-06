// SPDX-License-Identifier: AGPL-3.0-only
// The login providers screen in every width × locale project (ADR-0106, ADR-0110): an administrator opens the form
// from the list, sets up an OpenID Connect provider and lands on its page with its callback URL, sees it listed, and on
// its page disables it after the warning dialog and deletes it after its dialog, back on the list; each step passing
// axe without scrolling sideways. `vite preview` has no API, so each test plays
// the server of a signed-in administrator with page.route; names change with the locale, so the structure is checked
// by role, by the names in the project's locale catalog and by what the server sends.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, recordRowHeaders, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];
/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** The signed-in administrator's account, as GET /accounts/verify_credentials answers, with manage_login_providers. */
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
  permissions: ["manage_login_providers"],
};

const providerId = "0192f0c4-7a8e-7000-8000-0000000000c1";
const callbackUrl = `https://my.example/auth/external/${providerId}/callback`;
const displayName = "Example SSO";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the API of a signed-in administrator with no login provider at first. Returns what was sent. */
async function administratorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const sent: string[] = [];
  let providers: Schemas["AdminLoginProvider"][] = [];
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
        registrations: { mode: "approval" },
        federation: { mode: "both" },
        full_text_search: { available: false, visibilities: [] },
        translation: { browser: false, providers: [] },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
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
    if (key === "GET /admin/counts")
      return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    if (key === "GET /admin/settings/login_providers_only/impact") {
      return json(route, {
        setting_on: false,
        enabled_providers: 0,
        users_without_provider: 0,
        includes_viewer: false,
        starts_limit: false,
      });
    }
    if (key === "GET /admin/login_providers") return json(route, { items: providers });
    if (/^GET \/admin\/login_providers\/[^/]+$/.test(key)) {
      const provider = providers.find((one) => path.endsWith(`/${one.id}`));
      if (provider !== undefined) return json(route, provider);
    }
    if (key === "POST /admin/login_providers") {
      const body = request.postDataJSON() as Schemas["CreateLoginProviderRequest"];
      sent.push(
        `create ${body.kind} ${body.issuer ?? ""} ${body.client_id ?? ""}${(body.guilds ?? []).length > 0 ? ` guilds=${(body.guilds ?? []).join(",")}` : ""}`,
      );
      const provider: Schemas["AdminLoginProvider"] = {
        id: providerId,
        kind: body.kind,
        display_name: body.display_name,
        ...(body.issuer === undefined ? {} : { issuer: body.issuer }),
        client_id: body.client_id ?? "",
        scopes: body.scopes ?? [],
        allow_signup: body.allow_signup ?? false,
        allow_link: body.allow_link ?? true,
        claims: body.claims ?? [],
        organizations: body.organizations ?? [],
        guilds: body.guilds ?? [],
        state: "enabled",
        state_changed_at: "2026-10-20T00:00:00Z",
        callback_url: callbackUrl,
        // A provider with users linked to it, so the dialogs warn about them.
        linked_users: 2,
        created_at: "2026-10-20T00:00:00Z",
        revised_at: "2026-10-20T00:00:00Z",
      };
      providers = [provider];
      return json(route, provider);
    }
    const state = /^POST \/admin\/login_providers\/([^/]+)\/(enable|disable)$/.exec(key);
    if (state) {
      sent.push(state[2] ?? "");
      providers = providers.map((provider) => ({ ...provider, state: state[2] === "enable" ? "enabled" : "disabled" }));
      return json(route, providers[0]);
    }
    if (/^DELETE \/admin\/login_providers\/[^/]+$/.test(key)) {
      sent.push("delete");
      providers = [];
      return route.fulfill({ status: 204 });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("login providers", () => {
  test("sets up a provider, shows its callback URL, disables and deletes it", async ({ page, makeAxeBuilder }) => {
    const sent = await administratorServer(page);
    await page.goto("/admin/login-providers");
    const main = page.getByRole("main");
    const list = main.getByRole("region", { name: message("admin.login_providers.list.title"), exact: true });
    await expect(
      list.getByRole("heading", { level: 3, name: message("admin.login_providers.empty"), exact: true }),
    ).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The list holds no form: "Add a provider" opens its page.
    await main.getByRole("link", { name: message("admin.login_providers.new.title"), exact: true }).click();
    await expect(page).toHaveURL(/\/admin\/login-providers\/new$/);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await main
      .getByRole("textbox", { name: message("admin.login_providers.display_name"), exact: true })
      .fill(displayName);
    await main
      .getByRole("textbox", { name: message("admin.login_providers.issuer"), exact: true })
      .fill("https://sso.example");
    await main
      .getByRole("textbox", { name: message("admin.login_providers.client_id"), exact: true })
      .fill("my-client");
    await main.getByLabel(message("admin.login_providers.client_secret"), { exact: true }).fill("s3cret");
    await main.getByRole("button", { name: message("admin.login_providers.submit"), exact: true }).click();

    // Setting it up opens its page, with the callback URL to register at the provider.
    await expect(page).toHaveURL(new RegExp(`/admin/login-providers/${providerId}$`));
    await expect(main.getByRole("heading", { level: 2, name: displayName, exact: true })).toBeVisible();
    await expect(main.getByText(callbackUrl, { exact: true })).toBeVisible();
    expect(sent).toEqual(["create oidc https://sso.example my-client"]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The list shows it, and leads back to its page.
    await main.getByRole("link", { name: message("admin.login_providers.back"), exact: true }).click();
    await expect(recordRowHeaders(list).filter({ hasText: displayName })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await list.getByRole("link", { name: displayName, exact: true }).click();

    const actions = main.getByRole("region", { name: message("admin.login_providers.manage.title"), exact: true });
    await actions.getByRole("button", { name: message("admin.login_providers.disable"), exact: true }).click();
    const disable = page.getByRole("alertdialog");
    await expect(disable).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await disable.getByRole("button", { name: message("admin.login_providers.disable"), exact: true }).click();
    await expect(
      actions.getByRole("button", { name: message("admin.login_providers.enable"), exact: true }),
    ).toBeVisible();

    await actions.getByRole("button", { name: message("admin.login_providers.delete"), exact: true }).click();
    const remove = page.getByRole("alertdialog");
    await expect(remove).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await remove.getByRole("button", { name: message("admin.login_providers.delete"), exact: true }).click();

    // Deleting goes back to the list.
    await expect(page).toHaveURL(/\/admin\/login-providers$/);
    await expect(
      list.getByRole("heading", { level: 3, name: message("admin.login_providers.empty"), exact: true }),
    ).toBeVisible();
    expect(sent).toEqual(["create oidc https://sso.example my-client", "disable", "delete"]);
  });

  test("sets up a Discord provider with the servers it admits", async ({ page, makeAxeBuilder }) => {
    const sent = await administratorServer(page);
    await page.goto("/admin/login-providers/new");
    const main = page.getByRole("main");
    const form = main.locator("form");
    await expect(form).toBeVisible();

    await form.getByText(message("admin.login_providers.kind.discord"), { exact: true }).click();
    await expect(
      form.getByRole("radio", { name: message("admin.login_providers.kind.discord"), exact: true }),
    ).toBeChecked();
    await expect(form.getByRole("textbox", { name: message("admin.login_providers.issuer"), exact: true })).toHaveCount(
      0,
    );
    await form
      .getByRole("textbox", { name: message("admin.login_providers.display_name"), exact: true })
      .fill(displayName);
    await form
      .getByRole("textbox", { name: message("admin.login_providers.client_id"), exact: true })
      .fill("my-client");
    await form.getByLabel(message("admin.login_providers.client_secret"), { exact: true }).fill("s3cret");
    await form
      .getByRole("textbox", { name: message("admin.login_providers.guilds"), exact: true })
      .fill("1234567890, 987654321");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await form.getByRole("button", { name: message("admin.login_providers.submit"), exact: true }).click();

    await expect(main.getByText(callbackUrl, { exact: true })).toBeVisible();
    expect(sent).toEqual(["create discord  my-client guilds=1234567890,987654321"]);
  });
});
