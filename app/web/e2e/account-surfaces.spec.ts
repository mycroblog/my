// SPDX-License-Identifier: AGPL-3.0-only
// In every width × locale project: the signed-in browsers of the settings (ADR-0090), the Web Push prompt of the
// notifications page (ADR-0086), and the small layout of link cards (FR-007). `vite preview` has no API, so each test
// plays a signed-in user's server with page.route. Names come from the project's catalog.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, servedImage, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

const account = {
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
};

const otherBrowser = "0192f0c4-7a8e-7000-8000-0000000000b2";
const sessions = [
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000b1",
    current: true,
    ip: "203.0.113.1",
    user_agent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0 Safari/537.36",
    signed_in_at: "2026-10-01T00:00:00Z",
    last_used_at: "2026-11-01T00:00:00Z",
  },
  {
    id: otherBrowser,
    current: false,
    ip: "2001:db8:85a3:8d3:1319:8a2e:370:7348",
    user_agent: "Mozilla/5.0 (Android 14; Mobile; rv:130.0) Gecko/130.0 Firefox/130.0",
    signed_in_at: "2026-09-01T00:00:00Z",
  },
];

const postId = "0192f0c4-7a8e-7000-8000-0000000000c1";
const post = {
  id: postId,
  uri: `https://my.example/ap/statuses/${postId}`,
  url: `https://my.example/@alice/status/${postId}`,
  account,
  content: '<p><a href="https://cards.example/page">https://cards.example/page</a></p>',
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: "2026-11-01T00:00:00Z",
  media_attachments: [],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
  card: {
    url: "https://cards.example/page",
    title: "A page whose title is long enough to need more than a single line in a small card on a narrow screen",
    description: "What the page says about itself, which is also long enough to be cut short in a small card.",
    site_name: "cards.example",
    image: {
      url: servedImage(
        `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 9"><rect width="16" height="9" fill="teal"/></svg>')}`,
      ),
      width: 1600,
      height: 900,
    },
  },
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: status < 400 ? "application/json" : "application/problem+json",
    body: JSON.stringify(body),
  });
}

/** Plays Alice's server. Returns the changes it was asked for. */
async function serveApi(page: Page, options: { pushServerKey?: string; linkCardSize?: "large" | "small" } = {}) {
  const changes: string[] = [];
  let shown = sessions;
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/**", (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace("/api/v1", "");
    const key = `${request.method()} ${path}`;
    if (key === "GET /accounts/verify_credentials") return json(route, { ...account, permissions: [] });
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0-e2e",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        full_text_search: { available: false, visibilities: [] },
        translation: { browser: false, providers: [] },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
        ...(options.pushServerKey === undefined ? {} : { push_server_key: options.pushServerKey }),
      });
    }
    if (key === "GET /preferences") {
      return json(route, {
        locale: test.info().project.use.locale ?? "en",
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "en",
        require_media_description: false,
        timeline_languages: [],
        link_card_size: options.linkCardSize ?? "large",
      });
    }
    if (key === "GET /preferences/notifications") return json(route, { rows: [] });
    if (key === "GET /notifications/marker") return json(route, { unread_count: 0 });
    if (key === "GET /notifications") return json(route, { items: [] });
    if (key === "GET /sessions") return json(route, { items: shown });
    if (key === `DELETE /sessions/${otherBrowser}`) {
      changes.push(key);
      shown = shown.filter((session) => session.id !== otherBrowser);
      return route.fulfill({ status: 204 });
    }
    if (key === `GET /statuses/${postId}`) return json(route, post);
    if (key === `GET /statuses/${postId}/context`) return json(route, { ancestors: [], descendants: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return changes;
}

test.describe("the signed-in browsers", () => {
  test("are listed, pass axe, and one of them signs out", async ({ page, makeAxeBuilder }) => {
    const changes = await serveApi(page);
    await page.goto("/settings/sessions");
    const main = page.getByRole("main");
    const list = main.getByRole("list").filter({ hasText: "203.0.113.1" });
    await expect(list.getByRole("listitem")).toHaveCount(2);
    await expect(list.getByText(message("settings.sessions.current"), { exact: true })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The current browser has no button: signing out is how it ends.
    const revoke = list.getByRole("button", { name: message("settings.sessions.revoke") });
    await expect(revoke).toHaveCount(1);
    await revoke.click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("settings.sessions.revoke") }).click();
    await expect(list.getByRole("listitem")).toHaveCount(1);
    expect(changes).toEqual([`DELETE /sessions/${otherBrowser}`]);
  });
});

/**
 * Makes the page see a permission it has not asked for yet. The headless shell reports Notification.permission "denied"
 * whatever grantPermissions says (e2e/web-push.ts), and a site that blocked notifications gets no prompt.
 */
async function askablePermission(page: Page) {
  await page.addInitScript(() => {
    Object.defineProperty(Notification, "permission", { get: () => "default" });
  });
}

test.describe("the Web Push prompt", () => {
  test("offers Web Push above the notifications until it is closed, also after a reload", async ({
    page,
    makeAxeBuilder,
  }) => {
    await askablePermission(page);
    await serveApi(page, {
      pushServerKey: "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U",
    });
    await page.goto("/notifications");
    const main = page.getByRole("main");
    const title = main.getByRole("heading", { name: message("settings.push_prompt.title") });
    await expect(title).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await main.getByRole("button", { name: message("settings.push_prompt.close") }).click();
    await expect(title).toHaveCount(0);
    await expect(page.getByRole("tab", { selected: true })).toBeFocused();

    await page.reload();
    await expect(page.getByRole("tab", { selected: true })).toBeVisible();
    await expect(title).toHaveCount(0);
  });

  test("does not show when the server sends no Web Push", async ({ page }) => {
    await askablePermission(page);
    await serveApi(page);
    await page.goto("/notifications");
    await expect(page.getByRole("tab", { selected: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: message("settings.push_prompt.title") })).toHaveCount(0);
  });
});

test.describe("a link card", () => {
  /** The size of the card's image once it has loaded. */
  async function imageBox(page: Page) {
    const image = page.getByRole("main").getByRole("article").locator("img").first();
    await expect(image).toBeVisible();
    return image.boundingBox();
  }

  test("has a square image in the small layout, passes axe, and does not scroll sideways", async ({
    page,
    makeAxeBuilder,
  }) => {
    await serveApi(page, { linkCardSize: "small" });
    await page.goto(`/@alice/status/${postId}`);
    // The preference arrives after the post: the image is square once it has.
    await expect
      .poll(async () => {
        const box = await imageBox(page);
        return box === null ? null : Math.round(box.width) === Math.round(box.height);
      })
      .toBe(true);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("has a wide image in the large layout, the default", async ({ page }) => {
    await serveApi(page);
    await page.goto(`/@alice/status/${postId}`);
    const box = await imageBox(page);
    expect(box).not.toBeNull();
    if (box !== null) expect(box.width).toBeGreaterThan(box.height);
  });
});
