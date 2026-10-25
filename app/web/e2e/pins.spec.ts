// SPDX-License-Identifier: AGPL-3.0-only
// Pinned posts in every width × locale project (FR-403, ADR-0027): the viewer's profile starts its posts tab
// with the pinned posts, and the viewer pins a post from its menu with the keyboard. `vite preview` has no
// API, so the spec answers the token refresh and the client API with `page.route`, as a signed-in user's
// server would. Names come from the project's catalog, so the spec finds the same elements in every locale.
import type { Page } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Account = components["schemas"]["Account"];
type Status = components["schemas"]["Status"];
/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

const alice: Account = {
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

function post(n: number): Status {
  const id = `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`;
  return {
    id,
    uri: `https://my.example/ap/statuses/${id}`,
    url: `https://my.example/@alice/status/${id}`,
    account: alice,
    content: `<p>Fresh bread number ${String(n)}.</p>`,
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    language: "en",
    created_at: new Date(Date.now() - n * 60 * 60_000).toISOString(),
    media_attachments: [],
    mentions: [],
    tags: [],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
    favourited: false,
    boosted: false,
    bookmarked: false,
  };
}

const posts = [post(1), post(2), post(3)];

/** Whether an accessible name starts with the client message `key`. */
function startsWithMessage(key: string): RegExp {
  return new RegExp(`^${message(key).replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`)}`);
}

/** Answers the session refresh and Alice's profile with her posts, of which post 3 is pinned; pins change them. */
async function signedInServer(page: Page) {
  let pins = [posts[2]?.id ?? ""];
  const json = (body: unknown) => ({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  const withPin = (status: Status): Status => ({ ...status, pinned: pins.includes(status.id) });
  await page.route("**/oauth/token/refresh", (route) => route.fulfill(json({ access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 })));
  await page.route("**/api/v1/accounts/verify_credentials", (route) => route.fulfill(json(alice)));
  await page.route("**/api/v1/notifications/marker", (route) => route.fulfill(json({ unread_count: 0 })));
  await page.route("**/api/v1/accounts/lookup*", (route) => route.fulfill(json(alice)));
  await page.route(`**/api/v1/accounts/${alice.id}/statuses*`, (route) => {
    const pinned = new URL(route.request().url()).searchParams.get("pinned") === "true";
    const items = pinned ? pins.flatMap((id) => posts.filter((p) => p.id === id)) : posts;
    return route.fulfill(json({ items: items.map(withPin) }));
  });
  await page.route("**/api/v1/statuses/*/pin", (route) => {
    const id = new URL(route.request().url()).pathname.split("/").at(-2) ?? "";
    pins = route.request().method() === "POST" ? [id, ...pins.filter((p) => p !== id)] : pins.filter((p) => p !== id);
    const found = posts.find((p) => p.id === id);
    return found === undefined ? route.fulfill({ status: 404 }) : route.fulfill(json(withPin(found)));
  });
}

// Without motion the menu and the toast do not fade, so axe never samples them halfway.
test.use({ reducedMotion: "reduce" });

test.describe("the viewer's profile with a pinned post", () => {
  test.beforeEach(async ({ page }) => {
    await signedInServer(page);
    await page.goto("/@alice");
    await expect(page.getByRole("feed").getByRole("article")).toHaveCount(4);
  });

  test("starts with the pinned post, named as pinned, passes axe and does not scroll sideways", async ({ page, makeAxeBuilder }) => {
    const articles = page.getByRole("feed").getByRole("article");
    await expect(articles.first()).toHaveAccessibleName(startsWithMessage("status.pin.label"));
    await expect(articles.first()).toHaveAccessibleDescription("Fresh bread number 3.");
    await expect(articles.nth(1)).toHaveAccessibleDescription("Fresh bread number 1.");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("pins another post from its menu with the keyboard, which then comes first", async ({ page, makeAxeBuilder }) => {
    const articles = page.getByRole("feed").getByRole("article");
    const more = articles.nth(1).getByRole("button", { name: message("status.action.more") });
    await more.focus();
    await page.keyboard.press("Enter");
    const menu = page.getByRole("menu");
    await expect(menu).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    const pin = menu.getByRole("menuitem", { name: message("status.pin.action") });
    // The first item has the focus; the arrow keys move it down to the pin.
    for (let step = 0; step < 5 && !(await pin.evaluate((element) => element === document.activeElement)); step += 1) {
      await page.keyboard.press("ArrowDown");
    }
    await expect(pin).toBeFocused();
    await page.keyboard.press("Enter");

    await expect(page.getByRole("region", { name: message("shell.toast.region") }).getByText(message("status.pin.done"))).toBeVisible();
    await expect(articles).toHaveCount(5);
    await expect(articles.first()).toHaveAccessibleDescription("Fresh bread number 1.");
    await expect(articles.first()).toHaveAccessibleName(startsWithMessage("status.pin.label"));
    await expectNoHorizontalScroll(page);
  });
});
