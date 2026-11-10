// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji admin screen in every width × locale project (ADR-0110, ADR-0051): an administrator with
// manage_custom_emojis pages through the emoji by previous / next, opens one emoji's page from its row, disables it
// there and enables it again, passing axe without scrolling sideways on the list and on the emoji's page. `vite
// preview` has no API, so the test plays the server of a signed-in operator with page.route; names change with the
// locale, so they come from the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import {
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  recordRowHeaders,
  recordTable,
  servedImage,
  test,
} from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { preferencesOf } from "./preferences.ts";

/** The client message `key` in the project's locale, with `values` in its placeholders. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return Object.entries(values).reduce((out, [name, value]) => out.replaceAll(`{${name}}`, value), text);
}

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
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["manage_custom_emojis"],
};

const image = servedImage(
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==",
);

const firstId = "0192f0c4-7a8e-7000-8000-0000000000e1";
const secondId = "0192f0c4-7a8e-7000-8000-0000000000e2";
const longShortcode = "blobcat_with_a_very_long_name";

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the admin API of a signed-in operator with two local emoji, one a page; returns the changes made. */
async function operatorServer(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const actions: string[] = [];
  let disabled = false;
  const emoji = (id: string, shortcode: string) => ({
    id,
    shortcode,
    local: true,
    uri: `https://my.example/ap/emojis/${id}`,
    url: image,
    static_url: image,
    state: id === secondId && disabled ? "disabled" : "enabled",
    visible_in_picker: true,
    category: "Blobs and other round creatures of the fediverse",
    aliases: ["cat"],
    created_by: operator,
    created_by_deleted: false,
    created_at: "2026-10-20T00:00:00Z",
    updated_at: "2026-10-20T00:00:00Z",
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
    if (key === "GET /preferences") {
      return json(route, preferencesOf({ locale }));
    }
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /custom_emojis") return json(route, { items: [] });
    if (key === "GET /admin/custom_emoji_requests") return json(route, { items: [] });
    // Two pages of one emoji each, newest first.
    if (key === "GET /admin/custom_emojis") {
      return url.searchParams.get("cursor") === "next"
        ? json(route, { items: [emoji(secondId, longShortcode)] })
        : json(route, { items: [emoji(firstId, "blobfox")], next_cursor: "next" });
    }
    if (key === `GET /admin/custom_emojis/${secondId}`) return json(route, emoji(secondId, longShortcode));
    if (key === `POST /admin/custom_emojis/${secondId}/disable`) {
      actions.push(key);
      disabled = true;
      return json(route, emoji(secondId, longShortcode));
    }
    if (key === `POST /admin/custom_emojis/${secondId}/enable`) {
      actions.push(key);
      disabled = false;
      return json(route, emoji(secondId, longShortcode));
    }
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return actions;
}

test.describe("custom emoji", () => {
  test("pages through the emoji, and disables and enables one on its page", async ({ page, makeAxeBuilder }) => {
    const actions = await operatorServer(page);
    await page.goto("/admin/custom-emojis");
    const main = page.getByRole("main");
    const pages = main.getByRole("navigation", { name: message("admin.custom_emojis.pages") });
    await expect(pages.getByRole("link", { name: message("admin.custom_emojis.pages.emoji") })).toHaveAttribute(
      "aria-current",
      "page",
    );
    // The page's one primary action is a link to the form's page.
    await expect(main.getByRole("link", { name: message("admin.custom_emojis.new.title") })).toHaveAttribute(
      "href",
      "/admin/custom-emojis/new",
    );
    const table = recordTable(main);
    await expect(recordRowHeaders(table)).toContainText(":blobfox:");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await main.getByRole("button", { name: message("admin.list.next"), exact: true }).click();
    await expect(recordRowHeaders(table)).toContainText(`:${longShortcode}:`);
    await table.getByRole("link", { name: `:${longShortcode}:` }).click();

    await expect(page).toHaveURL(new RegExp(`/admin/custom-emojis/${secondId}$`, "u"));
    const state = main.getByRole("region", { name: message("admin.custom_emojis.detail.state"), exact: true });
    const name = `⁦:${longShortcode}:⁩`;
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    await state.getByRole("button", { name: message("admin.custom_emojis.disable_emoji", { emoji: name }) }).click();
    await expect(state.getByRole("status")).toHaveText(message("admin.custom_emojis.disabled_status", { emoji: name }));
    await state.getByRole("button", { name: message("admin.custom_emojis.enable_emoji", { emoji: name }) }).click();
    await expect(state.getByRole("status")).toHaveText(message("admin.custom_emojis.enabled_status", { emoji: name }));
    expect(actions).toEqual([
      `POST /admin/custom_emojis/${secondId}/disable`,
      `POST /admin/custom_emojis/${secondId}/enable`,
    ]);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
