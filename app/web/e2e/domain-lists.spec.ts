// SPDX-License-Identifier: AGPL-3.0-only
// The allowlist of federation and the blocklist subscriptions beside the domain actions (ADR-0159, NFR-MOD-7) in
// every width × locale project: the allowlist with a domain in non-Latin letters, a domain allowed and another removed
// through its alert dialog; the list of subscriptions, the page of one whose drafts are accepted together after the
// confirmation of a suspension; and the switch of the federation scope to the allowlist, which shows the follows it
// ends before it can be confirmed. Each page passes axe without scrolling sideways. `vite preview` has no API, so each
// test plays the server of a signed-in operator with page.route; names change with the locale, so the structure is
// checked by role and the identifiers the server sends (domains, keys).
import type { Page, Route } from "@playwright/test";
import {
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  recordRowHeaders,
  recordRows,
  recordTable,
  test,
} from "./fixtures";
import { preferencesOf } from "./preferences.ts";

/** The signed-in operator's account, as GET /accounts/verify_credentials answers, with manage_domains and manage_settings. */
const operator = {
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
  feed_enabled: true,
  feed_follows_indexable: true,
  follows_visibility: "public",
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["manage_domains", "manage_settings"],
};

// A long domain in non-Latin letters and its A-labels, to show that both forms wrap at 320 px.
const unicodeDomain = "とても長いサブドメインの名前.例え.jp";
const asciiDomain = new URL(`https://${unicodeDomain}`).hostname;
const subscriptionId = "0192f0c4-7a8e-7000-8000-0000000000e1";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

type Allow = { domain: string; comment: string; created_at: string };
type Draft = { key: string; change: string; domain: string; kind: string; held: string; public_comment?: string };

/** Plays the admin API of a signed-in operator; returns the changes it was asked for. */
async function operatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  let allows: Allow[] = [
    { domain: asciiDomain, comment: "A partner server with a long name.", created_at: "2026-10-10T08:00:00Z" },
    { domain: "friend.example", comment: "", created_at: "2026-10-11T08:00:00Z" },
  ];
  let drafts: Draft[] = [
    { key: `issue:suspend:${asciiDomain}`, change: "issue", domain: asciiDomain, kind: "suspend", held: "draft_mode" },
    { key: "issue:limit:noisy.example", change: "issue", domain: "noisy.example", kind: "limit", held: "draft_mode" },
  ];
  let scope = "open";
  const requests: string[] = [];
  const subscription = {
    id: subscriptionId,
    url: "https://lists.example/blocklist.csv",
    format: "mastodon_csv",
    title: "Shared blocklist",
    mode: "draft",
    suspend_as: "suspend",
    silence_as: "limit",
    apply_reject_media: true,
    apply_reject_reports: true,
    public_comment_from_list: false,
    consecutive_failures: 0,
    last_fetch: {
      id: "0192f0c4-7a8e-7000-8000-0000000000f1",
      outcome: "applied",
      http_status: 200,
      entry_count: 2,
      fetched_at: "2026-10-16T04:00:00Z",
    },
    created_at: "2026-10-15T08:00:00Z",
    updated_at: "2026-10-15T08:00:00Z",
  };
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
        registrations: { mode: "open" },
        federation: { mode: "both" },
        full_text_search: { available: false, visibilities: [] },
        translation: { browser: false, providers: [] },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, operator);
    if (key === "GET /preferences") return json(route, preferencesOf({ locale }));
    if (key === "GET /admin/domain_allows") return json(route, { items: allows });
    if (key === "POST /admin/domain_allows") {
      const body = request.postDataJSON() as { domain: string; comment?: string };
      requests.push(`${key} ${body.domain}`);
      const allow = { domain: body.domain, comment: body.comment ?? "", created_at: "2026-10-17T00:00:00Z" };
      allows = [...allows, allow];
      return json(route, allow, 201);
    }
    const removal = /^DELETE \/admin\/domain_allows\/([^/]+)$/.exec(key);
    if (removal) {
      const domain = decodeURIComponent(removal[1] ?? "");
      requests.push(`DELETE /admin/domain_allows/${domain}`);
      allows = allows.filter((allow) => allow.domain !== domain);
      return route.fulfill({ status: 204 });
    }
    if (key === "GET /admin/domain_list_subscriptions") return json(route, { items: [subscription] });
    if (key === `GET /admin/domain_list_subscriptions/${subscriptionId}`) return json(route, subscription);
    if (key === `GET /admin/domain_list_subscriptions/${subscriptionId}/fetches`)
      return json(route, { items: [subscription.last_fetch] });
    if (key === `GET /admin/domain_list_subscriptions/${subscriptionId}/drafts`) return json(route, { items: drafts });
    if (key === `POST /admin/domain_list_subscriptions/${subscriptionId}/drafts/accept`) {
      const { keys } = request.postDataJSON() as { keys: string[] };
      requests.push(`${key} ${keys.join(" ")}`);
      const done = drafts.filter((draft) => keys.includes(draft.key));
      drafts = drafts.filter((draft) => !keys.includes(draft.key));
      return json(route, { done, missing: [] });
    }
    if (key === "GET /admin/settings") {
      return json(route, {
        items: [
          {
            key: "federation_mode",
            kind: "enum",
            value: "both",
            default: "both",
            choices: ["off", "inbound", "outbound", "both"],
          },
          { key: "federation_scope", kind: "enum", value: scope, default: "open", choices: ["open", "allowlist"] },
        ],
      });
    }
    if (key === "GET /admin/settings/federation_scope/impact") {
      return json(route, {
        scope,
        follows: 1234,
        local_accounts: 56,
        domains: 78,
        top_domains: [
          { domain: asciiDomain, follows: 900 },
          { domain: "big.example", follows: 300 },
        ],
      });
    }
    if (key === "PATCH /admin/settings") {
      const { values } = request.postDataJSON() as { values: Record<string, string> };
      requests.push(`${key} ${JSON.stringify(values)}`);
      scope = values["federation_scope"] ?? scope;
      return json(route, {
        items: [
          {
            key: "federation_mode",
            kind: "enum",
            value: "both",
            default: "both",
            choices: ["off", "inbound", "outbound", "both"],
          },
          { key: "federation_scope", kind: "enum", value: scope, default: "open", choices: ["open", "allowlist"] },
        ],
      });
    }
    if (key.startsWith("GET /admin/audit_log")) return json(route, { items: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return requests;
}

for (const path of [
  "/moderation/domain-actions/allowed",
  "/moderation/domain-actions/subscriptions",
  "/moderation/domain-actions/subscriptions/new",
  `/moderation/domain-actions/subscriptions/${subscriptionId}`,
]) {
  test.describe(`page ${path}`, () => {
    test.beforeEach(async ({ page }) => {
      await operatorServer(page);
      await page.goto(path);
      await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
      await expect(page.getByRole("main").getByRole("link").first()).toBeVisible();
    });

    test("has no axe violations", async ({ makeAxeBuilder }) => {
      await expectNoAxeViolations(makeAxeBuilder);
    });

    test("does not scroll horizontally", async ({ page }) => {
      await expectNoHorizontalScroll(page);
    });
  });
}

test.describe("the allowlist", () => {
  test("allows a domain and removes another through its alert dialog", async ({ page, makeAxeBuilder }) => {
    const requests = await operatorServer(page);
    await page.goto("/moderation/domain-actions/allowed");
    const main = page.getByRole("main");
    const list = recordTable(main);
    await expect(recordRows(list)).toHaveCount(2);
    await expect(recordRowHeaders(list).first()).toContainText(unicodeDomain);

    // The form is the last section: its first field is the domain.
    const form = main.locator("form");
    await form.getByRole("textbox").first().fill("partner.example");
    await form.locator('button[type="submit"]').click();
    await expect(recordRows(list)).toHaveCount(3);
    await expect(main.getByRole("status").filter({ hasText: "partner.example" })).toBeVisible();
    await expect(form.getByRole("textbox").first()).toHaveValue("");

    // Removing asks first; the least destructive button has the focus.
    await recordRows(list).first().getByRole("button").click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(unicodeDomain);
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button").last().click();
    await expect(dialog).toBeHidden();
    await expect(recordRows(list)).toHaveCount(2);
    expect(requests).toEqual([
      "POST /admin/domain_allows partner.example",
      `DELETE /admin/domain_allows/${asciiDomain}`,
    ]);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("a blocklist subscription", () => {
  test("opens from the list and accepts its drafts together after confirming the suspension", async ({
    page,
    makeAxeBuilder,
  }) => {
    const requests = await operatorServer(page);
    await page.goto("/moderation/domain-actions/subscriptions");
    const main = page.getByRole("main");
    await recordRowHeaders(recordTable(main)).getByRole("link").click();
    await expect(page).toHaveURL(new RegExp(`/moderation/domain-actions/subscriptions/${subscriptionId}$`));
    await expect(main.getByRole("heading", { level: 1 })).toHaveText("Shared blocklist");

    // The first region holds the drafts; its first button accepts them all, and a suspension among them asks first.
    const drafts = main.getByRole("region").first();
    await expect(recordRows(recordTable(drafts))).toHaveCount(2);
    await drafts.getByRole("button").first().click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    expect(requests).toEqual([]);
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button").last().click();
    await expect(dialog).toBeHidden();
    await expect(drafts.getByRole("table")).toHaveCount(0);
    await expect(drafts.getByRole("heading", { level: 2 })).toBeFocused();
    expect(requests).toEqual([
      `POST /admin/domain_list_subscriptions/${subscriptionId}/drafts/accept issue:suspend:${asciiDomain} issue:limit:noisy.example`,
    ]);
    await expectNoHorizontalScroll(page);
  });
});

test.describe("the federation scope", () => {
  test("shows the follows the switch to the allowlist ends before it saves", async ({ page, makeAxeBuilder }) => {
    const requests = await operatorServer(page);
    await page.goto("/admin/settings/federation");
    const main = page.getByRole("main");
    // The scope is the second radio group of the page; its last choice is the allowlist.
    const scope = main.getByRole("radiogroup").nth(1);
    await scope.getByRole("radio").last().focus();
    await page.keyboard.press("Space");
    await main.locator('form button[type="submit"]').click();

    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(unicodeDomain);
    await expect(dialog.getByRole("button").first()).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(requests).toEqual([]);
    await dialog.getByRole("button").last().click();
    await expect(dialog).toBeHidden();
    await expect.poll(() => requests).toEqual(['PATCH /admin/settings {"federation_scope":"allowlist"}']);
  });
});
