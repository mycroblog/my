// SPDX-License-Identifier: AGPL-3.0-only
// The user's own invites in every width × locale project (ADR-0108 §5, ADR-0027): a user who may create invites and has
// no staff permission opens their invites from the settings, creates an invite within the caps, reads its link, and
// revokes an invite; the page passes axe without scrolling sideways and asks for no admin data. `vite preview` has no
// API, so the test plays the server with page.route; names change with the locale, so the elements are found by role,
// by address and by the names in the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];

/** The client message `key` in the project's locale, with `{name}` arguments replaced by `values`. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return Object.entries(values).reduce((out, [name, value]) => out.replaceAll(`{${name}}`, value), text);
}

/** The signed-in account, without permissions: the server lets everyone create invites. */
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
  permissions: [],
};

/** An invite of the user's with `code`, as GET /invites lists it. */
function invite(id: string, code: string, extra: Partial<Schemas["OwnInvite"]> = {}): Schemas["OwnInvite"] {
  return {
    id,
    code,
    url: `https://my.example/auth/sign_up?invite=${code}`,
    max_uses: 1,
    uses: 0,
    state: "usable",
    expires_at: "2026-10-23T00:00:00Z",
    created_at: "2026-10-16T00:00:00Z",
    ...extra,
  };
}

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the API of the user; returns the paths of the admin requests the app sent and the invites it created. */
async function serveApi(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const adminRequests: string[] = [];
  const created: Schemas["CreateOwnInviteRequest"][] = [];
  let invites: Schemas["OwnInvite"][] = [
    invite("0192f0c4-7a8e-7000-8000-0000000009a1", "ABCDEFGHJKLMNPQR", { max_uses: 10, uses: 3 }),
    invite("0192f0c4-7a8e-7000-8000-0000000009a2", "STUVWXYZ23456789", { state: "expired" }),
  ];
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow push" }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname.replace(/^\/api\/v1/, "");
    if (path.startsWith("/admin/")) adminRequests.push(path);
    const key = `${req.method()} ${path}`;
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
    if (key === "GET /custom_emojis") return json(route, { items: [] });
    if (key === "GET /invites") {
      return json(route, {
        items: invites,
        can_create: true,
        usable: invites.filter((i) => i.state === "usable").length,
        limits: {
          max_active: 5,
          max_uses: 10,
          max_expires_in: 30 * 86_400,
          default_max_uses: 1,
          default_expires_in: 7 * 86_400,
        },
      } satisfies Schemas["OwnInvitePage"]);
    }
    if (key === "POST /invites") {
      const body = req.postDataJSON() as Schemas["CreateOwnInviteRequest"];
      created.push(body);
      const made = invite("0192f0c4-7a8e-7000-8000-0000000009a3", "NEWCODE234567ABC", { max_uses: body.max_uses ?? 1 });
      invites = [made, ...invites];
      return json(route, made);
    }
    const revoke = /^POST \/invites\/([^/]+)\/revoke$/u.exec(key);
    if (revoke !== null) {
      invites = invites.map((i) => (i.id === revoke[1] ? { ...i, state: "revoked" } : i));
      return json(
        route,
        invites.find((i) => i.id === revoke[1]),
      );
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return { adminRequests, created };
}

test.describe("member invites", () => {
  test("a user who may create invites creates one and revokes one from the settings", async ({
    page,
    makeAxeBuilder,
  }) => {
    const server = await serveApi(page);
    await page.goto("/settings");
    const main = page.getByRole("main");
    await main.locator('a[href="/settings/invites"]').click();

    await expect(page).toHaveURL(/\/settings\/invites$/u);
    const form = main.getByRole("region", { name: message("settings.invites.new.title"), exact: true });
    await expect(form).toBeVisible();
    const list = main.getByRole("region", { name: message("settings.invites.list.title"), exact: true });
    await expect(list.getByRole("heading", { level: 3, name: "ABCDEFGHJKLMNPQR" })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await form.getByRole("button", { name: message("settings.invites.create"), exact: true }).click();
    await expect(form.getByRole("status")).not.toBeEmpty();
    await expect(form.getByText("https://my.example/auth/sign_up?invite=NEWCODE234567ABC")).toBeVisible();
    await expect(list.getByRole("heading", { level: 3, name: "NEWCODE234567ABC" })).toBeVisible();
    expect(server.created).toEqual([{ max_uses: 1, expires_in: 7 * 86_400 }]);

    // The accessible name ends with the date of the invite, formatted in the locale: the text before it names it.
    const revokeName = message("settings.invites.revoke_invite", { code: "ABCDEFGHJKLMNPQR" }).split("{date}")[0] ?? "";
    await list.locator(`button[aria-label^="${revokeName}"]`).click();
    await expect(list.getByRole("heading", { level: 3, name: "ABCDEFGHJKLMNPQR" })).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(server.adminRequests).toEqual([]);
  });
});
