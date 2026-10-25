// SPDX-License-Identifier: AGPL-3.0-only
// Granting and revoking the roles of a local account on its admin page in every width × locale project (ADR-0027,
// FR-720): a viewer whose highest role is a custom one with manage_roles sees admin above it without a button and
// with the reason, grants moderator by keyboard after its alert dialog, and focus moves to the region's heading,
// each step passing axe without scrolling sideways. `vite preview` has no API, so the test plays the server with
// page.route; names change with the locale, so the structure is checked by role and by the role keys.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

type Schemas = components["schemas"];

/** The signed-in viewer's account, as GET /accounts/verify_credentials answers, with the permissions of its role. */
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
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["manage_reports", "manage_accounts", "manage_roles", "view_audit"],
};

/** A local account with a long display name. */
const local: Schemas["Account"] = {
  ...viewer,
  id: "0192f0c4-7a8e-7000-8000-000000000021",
  username: "maximiliana",
  acct: "maximiliana",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000021",
  display_name: "Maximiliana Theodora Wolfharte",
  url: "https://my.example/@maximiliana",
};

const admin: Schemas["AdminRole"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000a01",
  key: "admin",
  position: 1000,
  permissions: ["manage_reports", "manage_accounts", "manage_domains", "manage_rules", "manage_settings", "manage_roles", "manage_invites", "view_audit", "manage_jobs", "manage_login_recovery", "manage_custom_emojis", "manage_webhooks", "manage_announcements", "manage_finance", "manage_supporters"],
};
const staff: Schemas["AdminRole"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000a03",
  key: "community-staff-lead",
  position: 500,
  permissions: ["manage_reports", "manage_accounts", "manage_roles", "view_audit"],
};
const moderator: Schemas["AdminRole"] = { id: "0192f0c4-7a8e-7000-8000-000000000a02", key: "moderator", position: 100, permissions: ["manage_reports", "manage_accounts", "view_audit"] };
const roles = [admin, staff, moderator];

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Plays the API of the signed-in viewer; returns the role changes the app sent. */
async function rolesServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let held: Schemas["AdminRole"][] = [];
  const changes: string[] = [];
  const detail = (): Schemas["AdminAccountDetail"] => ({
    id: local.id,
    account: local,
    local: true,
    state: "active",
    restrictions: [],
    registration_state: "approved",
    registration: { submitted_at: "2026-10-01T00:00:00Z", state_changed_at: "2026-10-01T01:00:00Z" },
    email_verified: true,
    roles: held.map((role) => role.key),
    actions: [],
    reports: [],
  });
  const heldList = () => ({ items: roles.filter((role) => held.includes(role)) });
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow admin" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (request.headers()["authorization"] !== "Bearer e2e-token" && path !== "/instance") return json(route, { type: "about:blank", title: "Unauthorized", status: 401 }, 401);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, { domain: "my.example", name: "my example", version: "0.0.0", source_url: "https://github.com/mycroblog/my", registrations: { mode: "open" }, federation: { mode: "both" }, limits: { max_post_characters: 500, require_media_description: false }, timelines: { local: "everyone", federated: "everyone" }, rules: [] });
    }
    if (key === "GET /accounts/verify_credentials" || key === `GET /accounts/${viewer.id}`) return json(route, viewer);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /admin/counts") return json(route, {});
    if (key === `GET /admin/accounts/${local.id}`) return json(route, detail());
    if (key === `GET /admin/accounts/${local.id}/actions`) return json(route, { items: [] });
    if (key === "GET /admin/roles") return json(route, { items: roles });
    if (key === `GET /admin/accounts/${viewer.id}/roles`) return json(route, { items: [staff] });
    if (key === `GET /admin/accounts/${local.id}/roles`) return json(route, heldList());
    if (key === `PUT /admin/accounts/${local.id}/roles/moderator`) {
      changes.push(key);
      held = [moderator];
      return json(route, heldList());
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return changes;
}

/** The roles region: the one whose rows are headed by the role keys. */
function rolesRegion(page: Page) {
  return page.getByRole("main").getByRole("region").filter({ has: page.getByRole("heading", { level: 3, name: "moderator", exact: true }) });
}

test.describe("roles", () => {
  test("grants a role by keyboard after its confirmation, with a role above the viewer's highest one explained", async ({ page, makeAxeBuilder }) => {
    const changes = await rolesServer(page);
    await page.goto(`/moderation/accounts/${local.id}`);
    const region = rolesRegion(page);
    await expect(region.getByRole("listitem")).toHaveCount(3);
    await expect(region).toMatchAriaSnapshot(`
      - region:
        - heading [level=2]
        - paragraph
        - list:
          - listitem:
            - heading "admin" [level=3]
            - term
            - definition
            - term
            - definition: /manage_reports/
            - paragraph
          - listitem:
            - heading "community-staff-lead" [level=3]
            - button
            - term
            - definition
            - term
            - definition: /manage_roles/
          - listitem:
            - heading "moderator" [level=3]
            - button
            - term
            - definition
            - term
            - definition: /view_audit/
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // By keyboard: the moderator's grant opens its confirmation, with focus on cancel.
    const row = region.getByRole("listitem").filter({ has: page.getByRole("heading", { name: "moderator", exact: true }) });
    await row.getByRole("button").focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toMatchAriaSnapshot(`
      - alertdialog /moderator.*@maximiliana|@maximiliana.*moderator/:
        - heading [level=2]
        - paragraph
        - button
        - button
    `);
    // The dialog fades in over the page: axe measures contrast once every transition has finished.
    await expect(page.locator("[data-entering]")).toHaveCount(0);
    await page.evaluate(() => Promise.all(document.getAnimations().map((animation) => animation.finished)));
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(changes).toEqual([]);
    await page.keyboard.press("Tab");
    await page.keyboard.press("Enter");

    await expect(dialog).toBeHidden();
    expect(changes).toEqual([`PUT /admin/accounts/${local.id}/roles/moderator`]);
    await expect(region.getByRole("heading", { level: 2 })).toBeFocused();
    await expect(region.getByRole("status").filter({ hasText: /\S/ })).toBeVisible();
    await expect(page.getByRole("main").getByRole("region").first().getByText("moderator", { exact: true })).toBeVisible();
    await expect(page.locator("[data-exiting]")).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
