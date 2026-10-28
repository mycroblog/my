// SPDX-License-Identifier: AGPL-3.0-only
// Saving the profile against a real `my serve` on SQLite (e2e/server.ts), in every width × locale project:
// the user opens their profile, changes their display name in the settings, and opens the profile again
// from the navigation. Every read of the API is slowed down once the profile is saved, as on a real
// network, and the profile page must show the new name in its heading and as the author of the user's
// post from its first render, not the cached old one first (#505). The page is checked with axe, for
// horizontal scrolling and against an ARIA snapshot.
import { randomBytes } from "node:crypto";
import type { Page } from "@playwright/test";
import {
  axeBuilder,
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  openFromNavigation,
  test,
} from "./fixtures";
import { ownsServer, runMy } from "./server-cli.ts";
import { serverURL } from "./server-origin.ts";
import { readCatalog } from "./catalog.ts";

// Without motion the colors do not transition, so axe never samples a color in the middle of a change.
test.use({ baseURL: serverURL, reducedMotion: "reduce" });

/** How much longer every read of the API takes once the profile is saved. */
const slowReads = 2000;

/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

const pageHeading = (page: Page) => page.getByRole("main").getByRole("heading", { level: 1 });

/**
 * Signs in through the server's sign-in screen (OAuth 2.0 with PKCE, ADR-0030), sets the user's display
 * language to the project's locale (PATCH /api/v1/preferences with the app's access token) and reloads the
 * app in it.
 */
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
  const locale = test.info().project.use.locale ?? "en";
  const response = await page.request.patch(`${serverURL}/api/v1/preferences`, {
    headers: { Authorization: authorization },
    data: { locale },
  });
  expect(response.ok(), `PATCH /api/v1/preferences: ${String(response.status())}`).toBe(true);
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", locale);
  await expect(pageHeading(page)).toHaveText(message("home.title"));
}

/** Posts `text` from the new-post sheet, opened by the navigation's button. */
async function post(page: Page, text: string): Promise<void> {
  await page.getByRole("button", { name: message("compose.open") }).click();
  const form = page.getByRole("form", { name: message("compose.title") });
  await form.getByRole("textbox", { name: message("compose.text.label") }).fill(text);
  await form.getByRole("button", { name: message("compose.submit") }).click();
  await expectToast(page, message("compose.posted"));
}

/** Asserts that a toast says `text`; an earlier toast may still say it too. */
async function expectToast(page: Page, text: string): Promise<void> {
  await expect(
    page
      .getByRole("region", { name: message("shell.toast.region") })
      .getByText(text)
      .last(),
  ).toBeVisible();
}

test("a saved display name is on the profile page from its first render", async ({ page }, testInfo) => {
  test.skip(
    !ownsServer,
    "the test creates its user with bin/my, which needs the database of the server e2e/server.ts starts",
  );
  test.setTimeout(90_000);
  const touch = testInfo.project.use.hasTouch ?? false;
  // A user new to the test, so that a retry and the other projects never meet its posts.
  const username = `e2e_profile_${randomBytes(4).toString("hex")}`;
  const email = `${username}@example.com`;
  const password = `e2e-password-${username}`;
  runMy(["admin", "create", "-username", username, "-email", email], `${password}\n`);
  await signIn(page, email, password);
  const text = `Profile save ${username}`;
  await post(page, text);
  const article = page.getByRole("feed").getByRole("article").filter({ hasText: text });

  // The profile, without a display name: its heading is the username.
  await openFromNavigation(page, "/profile", touch);
  await expect(pageHeading(page)).toHaveText(username);
  await expect(article).toContainText(username);

  const name = `Display ${username}`;
  await openFromNavigation(page, "/settings", touch);
  await page
    .getByRole("main")
    .getByRole("link", { name: message("settings.profile.title") })
    .first()
    .click();
  await expect(pageHeading(page)).toHaveText(message("settings.profile.title"));
  await page.getByRole("textbox", { name: message("settings.profile.display_name") }).fill(name);
  await page.getByRole("button", { name: message("settings.profile.save") }).click();
  await expectToast(page, message("settings.profile.saved"));

  await page.route("**/api/v1/**", async (route) => {
    if (route.request().method() === "GET" && !route.request().url().includes("/streaming"))
      await new Promise((resolve) => setTimeout(resolve, slowReads));
    await route.fallback();
  });
  // Every heading the page shows from here on, sampled on every frame.
  await page.evaluate(() => {
    const seen: string[] = [];
    (window as unknown as { headings: string[] }).headings = seen;
    const sample = () => {
      const heading = document.querySelector("main h1")?.textContent ?? "";
      if (seen.at(-1) !== heading) seen.push(heading);
      requestAnimationFrame(sample);
    };
    sample();
  });
  await openFromNavigation(page, "/profile", touch);

  // Well before any slowed read could answer, the page has the new name.
  await expect(pageHeading(page)).toHaveText(name, { timeout: slowReads / 2 });
  await expect(article).toContainText(name, { timeout: slowReads / 2 });
  const headings = await page.evaluate(() => (window as unknown as { headings: string[] }).headings);
  expect(headings, "the profile's headings since it was opened").not.toContain(username);
  // The heading has the name and the handle under it, and the post has its author's name.
  await expect(page.getByRole("main")).toMatchAriaSnapshot(`
    - heading "${name}" [level=1]
    - paragraph: "@${username}"
  `);
  await expectNoAxeViolations(() => axeBuilder(page));
  await expectNoHorizontalScroll(page);
});
