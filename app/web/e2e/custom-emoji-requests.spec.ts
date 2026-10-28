// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji requests in every width × locale project (ADR-0070 §2, §3, ADR-0027): a user whose roles have
// create_custom_emojis and no staff permission opens their custom emoji page from the settings, sees their limit,
// requests an emoji with an image, and withdraws a request; an administrator with manage_custom_emojis opens the
// requests tab from the address that their notification links to and rejects a request with a reason. Every screen
// passes axe without scrolling sideways. The user's page asks for no admin data. `vite preview` has no API, so the test
// plays the server with page.route; names change with the locale, so the elements are found by role, by address and by
// the names in the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, servedImage, test } from "./fixtures";
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

const image = servedImage(
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==",
);

/** The signed-in account, as GET /accounts/verify_credentials answers, with `permissions`. */
function account(permissions: Schemas["Permission"][]) {
  return {
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
    permissions,
  };
}

const longShortcode = "blobcat_with_a_very_long_name";

/** The uploaded image of a request, as POST /media answers once it is processed. */
const uploaded: Schemas["MediaAttachment"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000a9",
  type: "image",
  state: "ready",
  meta: { width: 16, height: 16, focus: { x: 0, y: 0 } },
  url: image,
  preview_url: image,
};

/** A request of `state`, with a long shortcode. */
function request(
  id: string,
  state: Schemas["CustomEmojiRequestState"],
  extra: Partial<Schemas["AdminCustomEmojiRequest"]> = {},
): Schemas["AdminCustomEmojiRequest"] {
  return {
    id,
    kind: "create",
    state,
    shortcode: longShortcode,
    url: image,
    static_url: image,
    created_at: "2026-10-21T00:00:00Z",
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

/** Plays the API of a viewer with `permissions`; returns the paths of the admin requests the app sent. */
async function serveApi(page: Page, permissions: Schemas["Permission"][]) {
  const locale = test.info().project.use.locale ?? "en";
  const adminRequests: string[] = [];
  let requests: Schemas["AdminCustomEmojiRequest"][] = [request("0192f0c4-7a8e-7000-8000-0000000000e1", "pending")];
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, {
      access_token: "e2e-token",
      token_type: "Bearer",
      expires_in: 3600,
      scope: "read write follow push admin",
    }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
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
    if (key === "GET /accounts/verify_credentials") return json(route, account(permissions));
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
    if (key === "GET /custom_emojis/mine")
      return json(route, { items: [], used: requests.filter((r) => r.state === "pending").length, limit: 5 });
    if (key === "GET /custom_emoji_requests") return json(route, { items: requests });
    // The image is processed at once.
    if (key === "POST /media") return json(route, uploaded, 202);
    if (key === "POST /custom_emoji_requests") {
      const body = req.postDataJSON() as Schemas["NewCustomEmojiRequest"];
      const created = request("0192f0c4-7a8e-7000-8000-0000000000e2", "pending", {
        shortcode: body.shortcode ?? longShortcode,
      });
      requests = [created, ...requests];
      return json(route, created);
    }
    const withdraw = /^POST \/custom_emoji_requests\/([^/]+)\/withdraw$/u.exec(key);
    if (withdraw !== null) {
      requests = requests.map((r) =>
        r.id === withdraw[1] ? { ...r, state: "withdrawn", decided_at: "2026-10-22T00:00:00Z" } : r,
      );
      return json(
        route,
        requests.find((r) => r.id === withdraw[1]),
      );
    }
    if (key === "GET /admin/custom_emojis") return json(route, { items: [] });
    if (key === "GET /admin/custom_emoji_requests") {
      const state = url.searchParams.get("state") ?? "pending";
      return json(route, {
        items: requests
          .filter((r) => r.state === state)
          .map((r) => ({
            ...r,
            requester: { ...account([]), id: "0192f0c4-7a8e-7000-8000-000000000002", username: "bob", acct: "bob" },
          })),
      });
    }
    const reject = /^POST \/admin\/custom_emoji_requests\/([^/]+)\/reject$/u.exec(key);
    if (reject !== null) {
      const body = req.postDataJSON() as Schemas["RejectCustomEmojiRequestRequest"];
      requests = requests.map((r) =>
        r.id === reject[1]
          ? {
              ...r,
              state: "rejected",
              decided_at: "2026-10-22T00:00:00Z",
              ...(body.reason === undefined ? {} : { reason: body.reason }),
            }
          : r,
      );
      return json(
        route,
        requests.find((r) => r.id === reject[1]),
      );
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return adminRequests;
}

test.describe("custom emoji requests", () => {
  test("a holder of create_custom_emojis requests an emoji and withdraws a request", async ({
    page,
    makeAxeBuilder,
  }) => {
    const adminRequests = await serveApi(page, ["create_custom_emojis"]);
    await page.goto("/settings");
    const main = page.getByRole("main");
    await main.locator('a[href="/settings/custom-emojis"]').click();

    await expect(page).toHaveURL(/\/settings\/custom-emojis$/u);
    const form = main.getByRole("region", { name: message("custom_emojis.new.title"), exact: true });
    await expect(form).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await form
      .locator('input[type="file"]')
      .setInputFiles({ name: "blobfox.png", mimeType: "image/png", buffer: Buffer.from("iVBORw0KGgo=", "base64") });
    await form.getByRole("textbox", { name: message("custom_emojis.shortcode") }).fill("blobfox");
    await form.getByRole("button", { name: message("custom_emojis.submit"), exact: true }).click();
    await expect(form.getByRole("status")).not.toBeEmpty();

    const requests = main.getByRole("region", { name: message("custom_emojis.requests.title"), exact: true });
    await expect(requests.getByRole("heading", { level: 3, name: ":blobfox:" })).toBeVisible();
    await requests
      .getByRole("button", { name: message("custom_emojis.withdraw_request", { emoji: "⁦:blobfox:⁩" }) })
      .click();
    await expect(requests.getByText(message("custom_emoji_requests.state.withdrawn"), { exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(adminRequests).toEqual([]);
  });

  test("an administrator rejects a request with a reason from the requests tab", async ({ page, makeAxeBuilder }) => {
    await serveApi(page, ["manage_custom_emojis"]);
    await page.goto("/admin/custom-emojis?tab=requests");
    const main = page.getByRole("main");
    const region = main.getByRole("region", { name: message("admin.custom_emoji_requests.title"), exact: true });
    await expect(region.getByRole("rowheader", { name: `:${longShortcode}:` })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const name = `⁦:${longShortcode}:⁩`;
    await region
      .getByRole("button", { name: message("admin.custom_emoji_requests.reject_request", { emoji: name }) })
      .click();
    const dialog = page.getByRole("alertdialog");
    await dialog
      .getByRole("textbox", { name: message("admin.custom_emoji_requests.reason") })
      .fill("هذا الاسم طويل جدًا");
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("admin.custom_emoji_requests.reject"), exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(region.getByRole("rowheader", { name: `:${longShortcode}:` })).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
