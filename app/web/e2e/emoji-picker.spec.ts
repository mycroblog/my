// SPDX-License-Identifier: AGPL-3.0-only
// The emoji picker of the new-post form in every width × locale project (ADR-0091, ADR-0051 §4, ADR-0027): opened
// from the top end corner of the text, it passes axe without scrolling sideways (also in the 320 px sheet), its grid
// is reached with Tab from the search field and moves with the arrow keys in the writing direction, and the emoji
// picked with Enter goes into the text. `vite preview` has no API, so the spec answers the client API and the token
// refresh as a signed-in session (as compose.spec.ts does). Names come from the project's catalog and, for the
// Unicode emoji, from the CLDR names of features/emoji/unicode.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, servedImage, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

/** The CLDR short name of the Unicode emoji `emoji` as the project's locale shows it (ja, or else en). */
function emojiName(emoji: string): string {
  const locale = test.info().project.use.locale === "ja" ? "ja" : "en";
  const file = fileURLToPath(new URL(`../src/features/emoji/unicode/${locale}.gen.json`, import.meta.url));
  const names = JSON.parse(readFileSync(file, "utf8")) as { emoji: Record<string, [string, string[]]> };
  const name = names.emoji[emoji]?.[0];
  if (name === undefined) throw new Error(`no name of ${emoji} in ${locale}`);
  return name;
}

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

// Plain squares stand in for the emoji images, served on the page's origin, where the CSP takes images from.
const orange = servedImage(
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==",
);
const blue = servedImage(
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGPQ6LnznxLMMGrAqAGjBgwXAwBfb48fVw/D1AAAAABJRU5ErkJggg==",
);

/** The emoji the server offers: two without a category, and a category with a long name. */
const emojis = [
  { shortcode: "wave", url: orange, static_url: orange, aliases: ["hello"] },
  { shortcode: "thanks", url: blue, static_url: blue, aliases: [] },
  {
    shortcode: "blobcat",
    url: blue,
    static_url: orange,
    category: "Blobs and other round creatures of the fediverse",
    aliases: [],
  },
  {
    shortcode: "blobfox",
    url: orange,
    static_url: orange,
    category: "Blobs and other round creatures of the fediverse",
    aliases: ["fox"],
  },
];

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
};

/** Answers the client API as a signed-in session that offers `emojis`. */
async function serveApi(page: Page): Promise<void> {
  const pageOrigin = new URL(test.info().project.use.baseURL ?? page.url()).origin;
  const cors = {
    "Access-Control-Allow-Origin": pageOrigin,
    "Access-Control-Allow-Credentials": "true",
    "Access-Control-Allow-Headers": "Authorization, Content-Type, Accept-Language, Idempotency-Key, X-My-Token-Refresh",
    "Access-Control-Allow-Methods": "GET, POST, PATCH, PUT, DELETE",
  };
  const json = (route: Route, body: unknown, status = 200) =>
    route.fulfill({
      status,
      contentType: status < 400 ? "application/json" : "application/problem+json",
      headers: cors,
      body: JSON.stringify(body),
    });
  await page.route(`${pageOrigin}/**`, (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (!path.startsWith("/api/") && !path.startsWith("/oauth/")) return route.fallback();
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (path === "/oauth/token/refresh")
      return json(route, {
        access_token: "e2e-token",
        token_type: "Bearer",
        expires_in: 3600,
        scope: "read write follow",
      });
    if (path === "/api/v1/accounts/verify_credentials") {
      return json(route, {
        ...account,
        discoverable: true,
        indexable: true,
        ai_opt_out: false,
        source: { note: "", fields: [] },
        posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
        permissions: [],
      });
    }
    if (path === "/api/v1/instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0-e2e",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (path === "/api/v1/preferences") {
      return json(route, {
        locale: test.info().project.use.locale ?? "en",
        time_zone: "UTC",
        time_zone_chosen: true,
        default_visibility: "public",
        default_quote_policy: "public",
        default_post_language: "en",
        require_media_description: false,
        email_moderator_notifications: false,
        timeline_auto_insert: true,
        timeline_languages: [],
      });
    }
    if (path === "/api/v1/custom_emojis") return json(route, { items: emojis });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

/** Opens the form from the navigation's button and returns it. */
async function openForm(page: Page, hasTouch: boolean) {
  const open = page.getByRole("button", { name: message("compose.open") });
  if (hasTouch) await open.tap();
  else await open.click();
  const form = page.getByRole("form", { name: message("compose.title") });
  await expect(form.getByRole("textbox", { name: message("compose.text.label") })).toBeVisible();
  return form;
}

/** Opens the picker from the button in the corner of the text, and returns it once its search field has focus. */
async function openPicker(page: Page, form: Awaited<ReturnType<typeof openForm>>) {
  await form.getByRole("button", { name: message("emoji.picker.button") }).click();
  const picker = page.getByRole("dialog", { name: message("emoji.picker.title") });
  await expect(picker.getByRole("searchbox", { name: message("emoji.picker.search") })).toBeFocused();
  return picker;
}

/** Presses Tab until `target` has focus: past the skin tone button and the section buttons, which a search disables. */
async function tabTo(page: Page, target: ReturnType<Page["getByRole"]>) {
  for (let presses = 0; presses < 4; presses++) {
    await page.keyboard.press("Tab");
    if (await target.evaluate((element) => element === document.activeElement)) return;
  }
  await expect(target).toBeFocused();
}

test.describe("the emoji picker of the new-post form", () => {
  test("has no axe violations and does not scroll horizontally while open", async ({
    page,
    makeAxeBuilder,
    hasTouch,
  }) => {
    await serveApi(page);
    await page.goto("/");
    const form = await openForm(page, hasTouch);
    const picker = await openPicker(page, form);
    await expect(picker.getByRole("grid", { name: message("emoji.picker.grid") })).toBeVisible();
    await expect(picker.getByRole("toolbar", { name: message("emoji.picker.categories") })).toBeVisible();

    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The skin tone menu too.
    await picker.getByRole("button", { name: message("emoji.picker.skin_tone") }).click();
    await expect(page.getByRole("menuitemradio", { name: message("emoji.picker.skin_tone.medium") })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("stays inside a 320 px sheet, over it, without scrolling sideways", async ({
    page,
    makeAxeBuilder,
    hasTouch,
  }) => {
    await serveApi(page);
    await page.goto("/");
    await page.setViewportSize({ width: 320, height: 568 });
    const form = await openForm(page, hasTouch);
    const picker = await openPicker(page, form);

    await expect(picker).toBeInViewport({ ratio: 1 });
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("picks an emoji with the keyboard alone and puts it into the text", async ({ page, hasTouch }) => {
    await serveApi(page);
    await page.goto("/");
    const form = await openForm(page, hasTouch);
    const text = form.getByRole("textbox", { name: message("compose.text.label") });
    await text.fill("Hi");
    await form.getByRole("button", { name: message("emoji.picker.button") }).focus();
    await page.keyboard.press("Enter");

    const picker = page.getByRole("dialog", { name: message("emoji.picker.title") });
    const search = picker.getByRole("searchbox", { name: message("emoji.picker.search") });
    await expect(search).toBeFocused();
    await search.fill("blob");
    await tabTo(page, picker.getByRole("gridcell", { name: ":blobcat:" }));
    // The next emoji is at the end of the line in the writing direction.
    const rtl = (await page.evaluate(() => document.documentElement.dir)) === "rtl";
    await page.keyboard.press(rtl ? "ArrowLeft" : "ArrowRight");
    await expect(picker.getByRole("gridcell", { name: ":blobfox:" })).toBeFocused();
    await page.keyboard.press("Enter");

    await expect(picker).toBeHidden();
    await expect(text).toHaveValue("Hi :blobfox: ");
    await expect(text).toBeFocused();
  });

  test("finds a custom emoji by another name and a Unicode emoji by its name in the locale", async ({
    page,
    hasTouch,
  }) => {
    await serveApi(page);
    await page.goto("/");
    const form = await openForm(page, hasTouch);
    const text = form.getByRole("textbox", { name: message("compose.text.label") });
    const picker = await openPicker(page, form);
    const search = picker.getByRole("searchbox", { name: message("emoji.picker.search") });
    await search.fill("fox");

    await expect(picker.getByRole("gridcell", { name: ":blobfox:" })).toBeVisible();
    await expect(picker.getByRole("gridcell", { name: ":wave:" })).toHaveCount(0);
    await expectNoHorizontalScroll(page);

    const name = emojiName("😀");
    await search.fill(name);
    await picker.getByRole("gridcell", { name, exact: true }).first().click();
    await expect(picker).toBeHidden();
    // A Unicode emoji goes in as it is, without spaces.
    await expect(text).toHaveValue("😀");
  });
});
