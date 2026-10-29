// SPDX-License-Identifier: AGPL-3.0-only
// A new post is on the viewer's profile from the moment the profile opens again, from the navigation or back in
// the history (ADR-0045, web-client/README.md S1, S7), against a real `my serve` on SQLite (e2e/server.ts) in every
// width × locale project. The post marks stale the lists that no page shows; the profile's route fetches its
// marked list again and the router waits for it (defaultStaleReloadMode "blocking"), so the profile never
// renders the posts from before. Every GET of the client API takes 800 ms more, as over a real network, so
// that a stale list replaced only when a refetch ended would be seen.
// The user is new to the test (a random name), created with `my admin create` (e2e/server-cli.ts).
import { randomBytes } from "node:crypto";
import type { Page } from "@playwright/test";
import {
  axeBuilder,
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  localeDir,
  openFromNavigation,
  test,
  newPostButton,
} from "./fixtures";
import { ownsServer, runMy } from "./server-cli.ts";
import { serverURL } from "./server-origin.ts";
import { readCatalog } from "./catalog.ts";

test.use({ baseURL: serverURL });

/** The project's locale. */
function projectLocale(): string {
  return test.info().project.use.locale ?? "en";
}

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = projectLocale();
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** How much longer every GET of the client API takes, in ms. */
const latency = 800;

const pageHeading = (page: Page) => page.getByRole("main").getByRole("heading", { level: 1 });

/** Signs in through the server's screen, and makes the project's locale the user's display language. */
async function signIn(page: Page, email: string, password: string): Promise<void> {
  const authorized = page.waitForRequest(
    (request) =>
      new URL(request.url()).pathname.startsWith("/api/") &&
      request.headers()["authorization"]?.startsWith("Bearer ") === true,
  );
  // /login is not a page: it opens the server's sign-in screen at once.
  await page.goto("/login");
  await page.locator('input[type="email"][autocomplete="username"]').fill(email);
  await page.locator('input[type="password"][autocomplete="current-password"]').fill(password);
  await page.locator('main button[type="submit"]').first().click();
  const authorization = (await authorized).headers()["authorization"] ?? "";
  await expect(page).toHaveURL(`${serverURL}/home`);
  const locale = projectLocale();
  const response = await page.request.patch(`${serverURL}/api/v1/preferences`, {
    headers: { Authorization: authorization },
    data: { locale },
  });
  expect(response.ok(), `PATCH /api/v1/preferences: ${String(response.status())}`).toBe(true);
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("dir", localeDir(locale));
  await expect(pageHeading(page)).toHaveText(message("home.title"));
}

/** Creates a user new to the test (a random name) with `my admin create`, and signs in as it. */
async function newUser(page: Page, prefix: string): Promise<string> {
  const username = `${prefix}_${randomBytes(4).toString("hex")}`;
  const email = `${username}@example.com`;
  const password = `e2e-password-${username}`;
  runMy(["admin", "create", "-username", username, "-email", email], `${password}\n`);
  await signIn(page, email, password);
  return username;
}

/** The Authorization header that the app sends with its next request to the client API. */
function nextAuthorization(page: Page): Promise<string> {
  return page
    .waitForRequest(
      (request) =>
        new URL(request.url()).pathname.startsWith("/api/") &&
        request.headers()["authorization"]?.startsWith("Bearer ") === true,
    )
    .then((request) => request.headers()["authorization"] ?? "");
}

/** Makes every GET of the client API but the stream take `latency` ms more. */
async function slowDown(page: Page): Promise<void> {
  await page.route("**/api/v1/**", async (route) => {
    if (route.request().method() === "GET" && !route.request().url().includes("/streaming")) {
      await new Promise((resolve) => setTimeout(resolve, latency));
    }
    await route.continue();
  });
}

/**
 * Posts `text` from the new-post sheet, opened by the navigation's button, and waits until it is done: until the
 * toast says `notice` (`compose.posted.first` after the account's first post, `compose.posted` after the others).
 */
async function post(page: Page, text: string, notice = "compose.posted"): Promise<void> {
  await newPostButton(page, message("compose.open")).click();
  const form = page.getByRole("form", { name: message("compose.title") });
  await form.getByRole("textbox", { name: message("compose.text.label") }).fill(text);
  // The toast of an earlier post would pass for this one's: it goes after 5 s, unless the pointer is over it.
  const posted = page.getByRole("region", { name: message("shell.toast.region") }).getByText(message(notice));
  await page.mouse.move(0, 0);
  await expect(posted).toHaveCount(0, { timeout: 15_000 });
  await form.getByRole("button", { name: message("compose.submit") }).click();
  await expect(posted).toBeVisible({ timeout: latency * 10 });
}

/**
 * Starts watching the page: from now on, every change of the DOM that shows the profile of `username` with its
 * feed of posts but without a post containing `text` counts as a stale list. Returns the count so far.
 */
async function watchForStaleProfile(page: Page, username: string, text: string): Promise<() => Promise<number>> {
  await page.evaluate(
    ([name, wanted]) => {
      const w = window as unknown as { __staleRenders: number };
      w.__staleRenders = 0;
      const check = () => {
        const main = document.querySelector("main");
        const feed = main?.querySelector('[role="feed"]');
        if (main?.querySelector("h1")?.textContent !== name || feed == null) return;
        if (!feed.textContent.includes(wanted)) w.__staleRenders += 1;
      };
      new MutationObserver(check).observe(document.body, { subtree: true, childList: true, characterData: true });
    },
    [username, text] as const,
  );
  return () => page.evaluate(() => (window as unknown as { __staleRenders: number }).__staleRenders);
}

test("a new post is on the profile from the moment it opens again", async ({ page }, info) => {
  test.skip(!ownsServer, "creates its user with bin/my against the server's database");
  test.setTimeout(120_000);
  const touch = info.project.use.hasTouch === true;
  const username = await newUser(page, "e2e_fresh");
  await slowDown(page);

  // The profile has a post, and its list is cached.
  const first = `first ${username}`;
  await post(page, first, "compose.posted.first");
  await openFromNavigation(page, "/profile", touch);
  await expect(pageHeading(page)).toHaveText(username);
  await expect(page.getByRole("feed").getByRole("article").filter({ hasText: first })).toBeVisible();

  // Another post from the home timeline, and the profile again.
  await openFromNavigation(page, "/home", touch);
  await expect(pageHeading(page)).toHaveText(message("home.title"));
  const second = `second ${username}`;
  await post(page, second);
  const staleRenders = await watchForStaleProfile(page, username, second);
  await openFromNavigation(page, "/profile", touch);
  await expect(pageHeading(page)).toHaveText(username);
  const feed = page.getByRole("feed", { name: message("profile.tab.posts") });
  await expect(feed.getByRole("article").filter({ hasText: second })).toBeVisible();
  // Give a stale list the time it would have taken to be replaced.
  await page.waitForTimeout(latency * 2);
  expect(await staleRenders(), "renders of the profile's feed without the new post").toBe(0);

  // Once more from the home timeline, and back in the history to the profile.
  await openFromNavigation(page, "/home", touch);
  await expect(pageHeading(page)).toHaveText(message("home.title"));
  const third = `third ${username}`;
  await post(page, third);
  const staleAfterBack = await watchForStaleProfile(page, username, third);
  await page.goBack();
  await expect(pageHeading(page)).toHaveText(username);
  await expect(feed.getByRole("article").filter({ hasText: third })).toBeVisible();
  await page.waitForTimeout(latency * 2);
  expect(await staleAfterBack(), "renders of the profile's feed without the post made before going back").toBe(0);

  await expect(feed.getByRole("article").first()).toMatchAriaSnapshot(`
    - article:
      - link
      - link:
        - time
      - paragraph: ${third}
  `);
  await expectNoAxeViolations(() => axeBuilder(page));
  await expectNoHorizontalScroll(page);
});

test("a change of the timeline languages is on the local timeline from the moment it opens again", async ({
  page,
}, info) => {
  test.skip(!ownsServer, "creates its user with bin/my against the server's database");
  test.setTimeout(120_000);
  const touch = info.project.use.hasTouch === true;
  const username = await newUser(page, "e2e_langs");
  await slowDown(page);

  // The local timeline has the user's post in English, and its list is cached. The post names its language: one
  // of undetermined language is on every timeline whatever the timeline languages (data-model/README.md, FR-208).
  const text = `in English ${username}`;
  const authorized = nextAuthorization(page);
  await openFromNavigation(page, "/profile", touch);
  const authorization = await authorized;
  const posted = await page.request.post(`${serverURL}/api/v1/statuses`, {
    headers: { Authorization: authorization, "Idempotency-Key": `e2e-${username}` },
    data: { text, visibility: "public", language: "en" },
  });
  expect(posted.ok(), `POST /api/v1/statuses: ${String(posted.status())}`).toBe(true);
  await openFromNavigation(page, "/local", touch);
  await expect(pageHeading(page)).toHaveText(message("local.title"));
  await expect(page.getByRole("feed").getByRole("article").filter({ hasText: text })).toBeVisible({
    timeout: latency * 10,
  });

  // The timelines show German only from now on.
  await openFromNavigation(page, "/settings", touch);
  await page.getByRole("main").locator('a[href="/settings/preferences"]').click();
  await expect(pageHeading(page)).toHaveText(message("settings.preferences.title"));
  const german = await page.evaluate(
    () => new Intl.DisplayNames([document.documentElement.lang], { type: "language" }).of("de") ?? "de",
  );
  const languages = page.getByRole("group", { name: message("settings.timeline_languages.label") });
  // The checkbox is visually hidden behind its box; its label is what a pointer presses.
  await languages.getByText(german, { exact: true }).click();
  await expect(languages.getByRole("checkbox", { name: german, exact: true })).toBeChecked();
  await expect(page.getByText(message("settings.saved")).first()).toBeVisible({ timeout: latency * 10 });

  // A render of the local timeline with the English post would be a stale list.
  await page.evaluate(
    ([heading, unwanted]) => {
      const w = window as unknown as { __staleRenders: number };
      w.__staleRenders = 0;
      new MutationObserver(() => {
        const main = document.querySelector("main");
        if (main?.querySelector("h1")?.textContent === heading && main.textContent.includes(unwanted))
          w.__staleRenders += 1;
      }).observe(document.body, { subtree: true, childList: true, characterData: true });
    },
    [message("local.title"), text] as const,
  );
  await openFromNavigation(page, "/local", touch);
  await expect(pageHeading(page)).toHaveText(message("local.title"));
  // Give a stale list the time it would have taken to be replaced.
  await page.waitForTimeout(latency * 2);
  expect(
    await page.evaluate(() => (window as unknown as { __staleRenders: number }).__staleRenders),
    "renders of the local timeline with the English post",
  ).toBe(0);
  await expect(page.getByRole("main").getByText(text)).toHaveCount(0);

  await expect(pageHeading(page)).toMatchAriaSnapshot(`- heading "${message("local.title")}" [level=1]`);
  await expectNoAxeViolations(() => axeBuilder(page));
  await expectNoHorizontalScroll(page);
});
