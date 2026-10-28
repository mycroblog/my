// SPDX-License-Identifier: AGPL-3.0-only
// The app shell in every width × locale project (ADR-0027): the layout of each page, its landmarks, and
// what a navigation does to focus and the document title (web-client/README.md Y5). Names change with the
// locale, so elements are found by role and position, and texts are compared with each other. The user's
// navigation needs a session: `vite preview` has no API, so serveSignedIn answers as a signed-in user's server.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, imageVariants, servedImage, test } from "./fixtures";

/** Alice, the signed-in user. */
const alice = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Alice",
  note: "",
  badges: [],
  fields: [],
  emojis: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};

/**
 * Answers the session refresh and the client API as the server of `account` would: the account, no notifications
 * and empty timelines and lists. Everything else is a 404, GET /instance included, so the navigation has no
 * Federated (the server does not say that it serves it).
 */
async function serveSignedIn(page: Page, account: Record<string, unknown> = alice): Promise<void> {
  const json = (route: Route, body: unknown, status = 200) =>
    route.fulfill({
      status,
      contentType: status < 400 ? "application/json" : "application/problem+json",
      body: JSON.stringify(body),
    });
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/**", (route) => {
    const path = new URL(route.request().url()).pathname.replace("/api/v1", "");
    if (path === "/accounts/verify_credentials" || path === "/accounts/lookup") return json(route, account);
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path.startsWith("/timelines/") || path === "/notifications" || path.endsWith("/statuses"))
      return json(route, { items: [] });
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
}

/** Every page of the shell, in the order of the navigation. */
const pages = ["/home", "/local", "/notifications", "/search", "/profile", "/settings"] as const;

/** The pages whose h1 only says where the user is, visually hidden (web-client/16-look-states-and-wording.md G5). */
const hiddenHeadingPages: ReadonlySet<string> = new Set(["/home", "/local", "/notifications", "/search", "/settings"]);

/**
 * The timelines, whose h1 is hidden only where the rail shows (from md): the phones' tab bar does not say which
 * timeline this is (web-client/screens/home.md, local.md).
 */
const railHiddenHeadingPages: ReadonlySet<string> = new Set(["/home", "/local"]);

/** The pages that phones reach from the menu's sheet instead of the tab bar (ADR-0039). */
const menuPages: ReadonlySet<string> = new Set(["/profile", "/settings"]);

/** Whether the project's viewport has the phones' tab bar rather than the rail (Tailwind's md). */
function hasTabBar(page: Page): boolean {
  return (page.viewportSize()?.width ?? 0) < 768;
}

/** Whether the project's width puts the signed-out top bar's items behind its Menu button (below lg). */
function hasTopMenu(page: Page): boolean {
  return (page.viewportSize()?.width ?? 0) < 1024;
}

/** The items of the navigation that the project's width shows; the others are hidden (display: none). */
function shownItems(page: Page) {
  return page.getByRole("navigation").getByRole("listitem").filter({ visible: true });
}

/** The instance name in index.html's runtime config. */
const instanceName = "my example";

/** `text` as a literal in a regular expression. */
function escapeRegExp(text: string): string {
  return text.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);
}

for (const path of pages) {
  test.describe(`page ${path}`, () => {
    test.beforeEach(async ({ page }) => {
      await serveSignedIn(page);
      await page.goto(path);
      await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    });

    test("has no axe violations", async ({ makeAxeBuilder }) => {
      await expectNoAxeViolations(makeAxeBuilder);
    });

    test("does not scroll horizontally", async ({ page }) => {
      await expectNoHorizontalScroll(page);
    });

    test("hides its heading from view only when the heading just says where the user is", async ({ page }) => {
      const heading = page.getByRole("main").getByRole("heading", { level: 1 });
      expect(((await heading.textContent()) ?? "").trim()).not.toBe("");
      if (!hiddenHeadingPages.has(path)) {
        // Profile opens the user's own profile, whose heading names the account.
        await expect(heading).not.toHaveClass(/\bsr-only\b/);
        return;
      }
      if (railHiddenHeadingPages.has(path)) {
        await expect(heading).not.toHaveClass(/\bsr-only\b/);
        if (hasTabBar(page)) {
          // Below md the heading says which timeline this is.
          const box = await heading.boundingBox();
          expect(box?.height).toBeGreaterThan(1);
          return;
        }
      } else {
        await expect(heading).toHaveClass(/\bsr-only\b/);
      }
      // The navigation already says where the user is: the h1 is visually hidden (a 1 px clipped box, Tailwind's
      // sr-only or the app stylesheet's data-hidden-with-rail) and is still the page's named level-one heading.
      const box = await heading.boundingBox();
      expect(box?.width).toBeLessThanOrEqual(1);
      expect(box?.height).toBeLessThanOrEqual(1);
    });

    test("is titled after its heading and marks its navigation item as current", async ({ page }) => {
      const heading = await page.getByRole("heading", { level: 1 }).textContent();
      expect(heading).toBeTruthy();
      await expect(page).toHaveTitle(new RegExp(`${escapeRegExp(heading ?? "")}.*${escapeRegExp(instanceName)}`));
      const current = page.locator('nav a[aria-current="page"]').filter({ visible: true });
      if (hasTabBar(page) && menuPages.has(path)) {
        // On phones the page is in the menu's sheet, which marks it; no item of the tab bar is current.
        await expect(current).toHaveCount(0);
        await page.getByRole("navigation").getByRole("button").click();
        const sheet = page.getByRole("dialog");
        await expect(sheet.locator(`a[href="${path}"]`)).toHaveAttribute("aria-current", "page");
        await expect(sheet.locator('a[aria-current="page"]')).toHaveCount(1);
      } else {
        await expect(current).toHaveCount(1);
        await expect(current).toHaveAttribute("href", path);
      }
    });
  });
}

// Signed out: vite preview has no OAuth endpoints, so no session resumes.
test.describe("/login", () => {
  test("is no page: it opens the server's sign-in screen at once, to come back to return_to", async ({ page }) => {
    // vite preview has no authorization screen: answer it with an empty page, and keep the URL it was opened with.
    const authorize = new Promise<URL>((resolve) => {
      void page.route("**/oauth/authorize?*", async (route) => {
        resolve(new URL(route.request().url()));
        await route.fulfill({ status: 200, contentType: "text/html", body: "<title>Sign in</title>" });
      });
    });
    await page.goto("/login?return_to=%2Ftags%2Fcats");
    const url = await authorize;
    expect(url.searchParams.get("response_type")).toBe("code");
    expect(url.searchParams.get("code_challenge_method")).toBe("S256");
    expect(new URL(url.searchParams.get("redirect_uri") ?? "").pathname).toBe("/login/callback");
  });
});

test.describe("the navigation while signed out", () => {
  test("offers only the pages usable without a session and signing in, without axe violations", async ({
    page,
    makeAxeBuilder,
  }) => {
    await page.goto("/local");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    // Local, Search, About and Sign in (the instance answers nothing here, so no Federated and no Sign up). Below lg
    // the items are behind the Menu button, and Sign in, the call to action, stays in the bar.
    const nav = page.getByRole("navigation");
    const menu = nav.getByRole("button");
    if (hasTopMenu(page)) {
      await expect(nav).toMatchAriaSnapshot(`
        - navigation:
          - paragraph:
            - link
          - link
          - button [expanded=false]
      `);
      await expect(nav.getByRole("link").nth(1)).toHaveAttribute("href", "/login?return_to=%2Flocal");
      await menu.click();
      await expect(menu).toHaveAttribute("aria-expanded", "true");
    } else {
      await expect(menu).toBeHidden();
    }
    const items = hasTopMenu(page) ? 3 : 4;
    await expect(nav).toMatchAriaSnapshot(`
      - navigation:
        - list:
          - /children: equal
${Array.from({ length: items }, () => `          - listitem:\n            - link`).join("\n")}
    `);
    const links = shownItems(page).locator("a");
    await expect(links.nth(0)).toHaveAttribute("href", "/local");
    await expect(links.nth(1)).toHaveAttribute("href", "/search");
    await expect(links.nth(2)).toHaveAttribute("href", "/about");
    if (!hasTopMenu(page)) await expect(links.nth(3)).toHaveAttribute("href", "/login?return_to=%2Flocal");
    for (const path of ["/home", "/notifications", "/profile", "/bookmarks", "/settings"]) {
      await expect(page.getByRole("navigation").locator(`a[href="${path}"]`)).toHaveCount(0);
    }
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("keeps the top bar to one row, with its items behind the Menu button below lg", async ({ page }) => {
    await page.goto("/local");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    const nav = page.getByRole("navigation");
    const box = await nav.boundingBox();
    // One row of 44 px targets and the bar's padding: nothing wraps.
    expect(box?.height ?? 0).toBeLessThan(80);
    if (!hasTopMenu(page)) return;
    const menu = nav.getByRole("button");
    await menu.click();
    const local = nav.locator('a[href="/local"]');
    await expect(local).toBeVisible();
    // The items are not a dialog: Tab goes on into them, Escape closes them and focus returns to the button.
    await page.keyboard.press("Tab");
    await expect(local).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(menu).toHaveAttribute("aria-expanded", "false");
    await expect(menu).toBeFocused();
    await expect(local).toBeHidden();
    // Following an item closes them.
    await menu.click();
    await nav.locator('a[href="/search"]').click();
    await expect(page).toHaveURL(/\/search$/);
    await expect(menu).toHaveAttribute("aria-expanded", "false");
    await expectNoHorizontalScroll(page);
  });

  test("ends the page with one footer and no column about the server, at every width", async ({ page }) => {
    await page.goto("/local");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.getByRole("complementary")).toHaveCount(0);
    const footer = page.getByRole("contentinfo");
    await expect(footer).toHaveCount(1);
    await expect(footer.locator('a[href="/about"]')).toBeVisible();
    // The footer is the end of the page: nothing follows it, and nothing below the screen's height is blank.
    const box = await footer.boundingBox();
    const height = await page.evaluate(() => document.documentElement.scrollHeight);
    expect((box?.y ?? 0) + (box?.height ?? 0)).toBeCloseTo(height, 0);
    await expectNoHorizontalScroll(page);
  });

  test("sends a page that needs a session to sign in, which comes back to it", async ({ page }) => {
    // Through /login, which is no page, straight to the server's sign-in screen (vite preview has none: an empty one).
    const authorize = page.waitForRequest((request) => new URL(request.url()).pathname === "/oauth/authorize");
    await page.route("**/oauth/authorize?*", (route) =>
      route.fulfill({ status: 200, contentType: "text/html", body: "<title>Sign in</title>" }),
    );
    await page.goto("/notifications");
    await authorize;
  });
});

test.describe("layout", () => {
  test.beforeEach(async ({ page }) => {
    await serveSignedIn(page);
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  });

  test("exposes the skip link, the navigation, and the main landmark with one level-one heading", async ({ page }) => {
    // Phones: Home, Local, Notifications, Search and the Menu button. From md: Home, Local, Notifications, Search,
    // Profile, Bookmarks and Settings (the instance answers nothing here, so no Federated).
    const items = hasTabBar(page)
      ? ["link", "link", "link", "link", "button"]
      : ["link", "link", "link", "link", "link", "link", "link"];
    await expect(page.locator("body")).toMatchAriaSnapshot(`
      - link
      - navigation:
        - list:
          - /children: equal
${items.map((role) => `          - listitem:\n            - ${role}`).join("\n")}
      - main:
        - heading [level=1]
    `);
    await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
  });

  test("puts the navigation on the bottom edge of phones and on the inline-start edge of larger screens", async ({
    page,
  }) => {
    const viewport = page.viewportSize();
    const box = await page.getByRole("navigation").boundingBox();
    expect(viewport).not.toBeNull();
    expect(box).not.toBeNull();
    if (viewport === null || box === null) return;
    if (viewport.width < 768) {
      expect(box.y + box.height).toBeCloseTo(viewport.height, 0);
      expect(box.width).toBeCloseTo(viewport.width, 0);
    } else {
      expect(box.height).toBeCloseTo(viewport.height, 0);
      const rtl = (await page.locator("html").getAttribute("dir")) === "rtl";
      expect(rtl ? viewport.width - (box.x + box.width) : box.x).toBeCloseTo(0, 0);
    }
  });

  test("shows the icon of every navigation item at one size", async ({ page }) => {
    const icons = shownItems(page).locator("svg");
    await expect(icons).toHaveCount(hasTabBar(page) ? 5 : 7);
    for (const icon of await icons.all()) {
      expect(await icon.boundingBox()).toMatchObject({ width: 24, height: 24 });
    }
  });

  test("shows the column about the server only on wide screens", async ({ page }) => {
    const wide = (page.viewportSize()?.width ?? 0) >= 1280;
    await expect(page.getByRole("complementary")).toHaveCount(wide ? 1 : 0);
  });
});

test.describe("the sidebar of desktops", () => {
  /** A long name, which the sidebar cuts to one line; the link is still named by all of it. */
  const displayName = "Bartholomew Montgomery-Fitzwilliam the Third";
  const avatar = servedImage(
    `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><circle cx="24" cy="24" r="24" fill="#8f4f6d"/></svg>')}`,
  );

  test.beforeEach(async ({ page }) => {
    // vite preview has no API: answer the session refresh and the viewer's account as a signed-in user's server.
    const json = (body: unknown) => ({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
    await page.route("**/oauth/token/refresh", (route) =>
      route.fulfill(json({ access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 })),
    );
    await page.route("**/api/v1/accounts/verify_credentials", (route) =>
      route.fulfill(
        json({
          id: "0192f0c4-7a8e-7000-8000-000000000001",
          username: "alice",
          acct: "alice",
          domain: "my.example",
          uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
          display_name: displayName,
          note: "",
          fields: [],
          emojis: [],
          url: "https://my.example/@alice",
          locked: false,
          bot: false,
          created_at: "2026-10-01T00:00:00Z",
          avatar: imageVariants(avatar),
        }),
      ),
    );
    await page.route("**/api/v1/notifications/marker", (route) => route.fulfill(json({ unread_count: 0 })));
    await page.goto("/search");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
  });

  test("names the server above the list and the signed-in account below it, from lg only", async ({
    page,
    makeAxeBuilder,
  }) => {
    const viewport = page.viewportSize();
    const navigation = page.getByRole("navigation");
    const viewer = navigation.locator('a[href="/@alice"]');
    if ((viewport?.width ?? 0) < 1024) {
      await expect(navigation.getByText(instanceName)).toBeHidden();
      await expect(viewer).toBeHidden();
      return;
    }
    await expect(viewer).toBeVisible();
    // The list's links keep their order; the server's name, a link to the root address, comes before the list and
    // the account after it.
    await expect(navigation).toMatchAriaSnapshot(`
      - navigation:
        - paragraph:
          - link "${instanceName}"
        - list:
          - /children: equal
${Array.from({ length: 7 }, () => "          - listitem:\n            - link").join("\n")}
        - link "${displayName} @alice"
    `);
    await expect(viewer).not.toHaveAttribute("aria-current");
    await expect(navigation.locator('a[href="/"]')).toHaveText(instanceName);
    // The account is always in view, at the bottom of the sidebar, inside it.
    const box = await viewer.boundingBox();
    const nav = await navigation.boundingBox();
    expect(box).not.toBeNull();
    expect(nav).not.toBeNull();
    if (viewport === null || box === null || nav === null) return;
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
    expect(box.y + box.height).toBeGreaterThan(viewport.height - 80);
    expect(box.x).toBeGreaterThanOrEqual(nav.x);
    expect(box.x + box.width).toBeLessThanOrEqual(nav.x + nav.width);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  // The sidebar scrolls vertically (overflow-y: auto, so overflow-x is auto too): content wider than its box,
  // by a single pixel, shows a horizontal scrollbar at its bottom. Desktop widths beyond the matrix are checked
  // from one project per locale.
  test("scrolls sideways neither itself nor the page at desktop widths", async ({ page }) => {
    test.skip(page.viewportSize()?.width !== 1440, "resizes the viewport itself; runs once per locale");
    const navigation = page.getByRole("navigation");
    for (const width of [1280, 1440, 1920]) {
      await page.setViewportSize({ width, height: 900 });
      await expect(navigation.locator('a[href="/@alice"]')).toBeVisible();
      await expectNoHorizontalScroll(page);
      // The sidebar and every element in it that can scroll sideways: none is wider inside than its box. Names cut
      // to one line (overflow: hidden) are wider inside on purpose and never scroll.
      const overflowing = await navigation.evaluate((nav) =>
        [nav, ...nav.querySelectorAll<HTMLElement>("*")]
          .filter((element) => element === nav || ["auto", "scroll"].includes(getComputedStyle(element).overflowX))
          .filter((element) => element.scrollWidth > element.clientWidth)
          .map(
            (element) =>
              `${element.tagName.toLowerCase()} scrollWidth ${element.scrollWidth}px > clientWidth ${element.clientWidth}px`,
          ),
      );
      expect(overflowing, `at ${width}px`).toEqual([]);
    }
  });
});

test.describe("navigation", () => {
  test("moves focus to the new page's heading and retitles the document", async ({ page, hasTouch }) => {
    await serveSignedIn(page);
    await page.goto("/home");
    const firstHeading = await page.getByRole("heading", { level: 1 }).textContent();
    await expect(page.getByRole("heading", { level: 1 })).not.toBeFocused();

    // Local, in the tab bar on phones and in the rail from md.
    const local = page.getByRole("navigation").locator('a[href="/local"]').filter({ visible: true });
    if (hasTouch) await local.tap();
    else await local.click();

    await expect(page).toHaveURL(/\/local$/);
    const heading = page.getByRole("heading", { level: 1 });
    await expect(heading).toBeFocused();
    const text = (await heading.textContent()) ?? "";
    expect(text).not.toBe(firstHeading);
    await expect(page).toHaveTitle(new RegExp(`${escapeRegExp(text)}.*${escapeRegExp(instanceName)}`));
  });

  test("works from the keyboard, and back and forward keep focus on the page's heading", async ({ page }) => {
    await serveSignedIn(page);
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();

    // The skip link comes first and moves focus to the main landmark.
    await page.keyboard.press("Tab");
    await expect(page.getByRole("link").first()).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("main")).toBeFocused();

    await page.getByRole("navigation").locator('a[href="/search"]').focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/search$/);
    await expect(page.getByRole("heading", { level: 1 })).toBeFocused();

    await page.goBack();
    await expect(page).toHaveURL(/\/home$/);
    await expect(page.getByRole("heading", { level: 1 })).toBeFocused();
    await expect(page.getByRole("navigation").locator('a[href="/home"]')).toHaveAttribute("aria-current", "page");
  });
});

test.describe("the menu of the phones' tab bar", () => {
  test.beforeEach(async ({ page }) => {
    test.skip(!hasTabBar(page), "the rail from md has every item and no menu");
    await serveSignedIn(page);
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  });

  test("opens a sheet with the profile, the bookmarks and the settings from the keyboard, and Esc gives focus back", async ({
    page,
    makeAxeBuilder,
  }) => {
    const menu = page.getByRole("navigation").getByRole("button");
    await menu.focus();
    await page.keyboard.press("Enter");

    const sheet = page.getByRole("dialog");
    await expect(sheet).toMatchAriaSnapshot(`
      - dialog:
        - heading [level=2]
        - button
        - list:
          - listitem:
            - link
          - listitem:
            - link
          - listitem:
            - link
        - paragraph:
          - link "${instanceName}"
        - paragraph:
          - link
    `);
    await expect(sheet.getByRole("link")).toHaveText([/./, /./, /./, /./, /./]);
    await expect(sheet.locator('a[href="/"]')).toBeVisible();
    await expect(sheet.locator('a[href="/about"]')).toBeVisible();
    await expect(sheet.locator('a[href="/profile"]')).toBeVisible();
    await expect(sheet.locator('a[href="/bookmarks"]')).toBeVisible();
    await expect(sheet.locator('a[href="/settings"]')).toBeVisible();
    await expect(sheet.locator('a[aria-current="page"]')).toHaveCount(0);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await page.keyboard.press("Escape");
    await expect(sheet).toBeHidden();
    await expect(menu).toBeFocused();
  });

  test("closes when one of its links is followed, and focus moves to the new page's heading", async ({
    page,
    hasTouch,
  }) => {
    const menu = page.getByRole("navigation").getByRole("button");
    if (hasTouch) await menu.tap();
    else await menu.click();
    const settings = page.getByRole("dialog").locator('a[href="/settings"]');
    if (hasTouch) await settings.tap();
    else await settings.click();

    await expect(page).toHaveURL(/\/settings$/);
    await expect(page.getByRole("dialog")).toBeHidden();
    await expect(page.getByRole("heading", { level: 1 })).toBeFocused();
  });
});

test("an unknown address shows the not-found page inside the layout", async ({ page, makeAxeBuilder }) => {
  await page.goto("/no-such-page");
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByRole("navigation")).toBeVisible();
  // Signed out, the way out is the landing page; signed-in users get their home timeline (HomeLink).
  await expect(page.getByRole("main").getByRole("link")).toHaveAttribute("href", "/");
  await expectNoAxeViolations(makeAxeBuilder);
});
