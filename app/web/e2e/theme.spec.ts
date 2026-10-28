// SPDX-License-Identifier: AGPL-3.0-only
// The color theme in every width × locale project (app/ui/README.md, "Light and dark"): a theme chosen on this
// browser reaches <html data-theme> before the first paint, whatever the OS says, and the shell passes axe in
// both themes. The theme is chosen in the menu's sheet on phones (ADR-0039); e2e/settings.spec.ts covers the
// preferences page.
import type { Page } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

/** The localStorage key of the theme (lib/storage's themeKey), which public/theme.js reads. */
const themeKey = "my:display:theme";

/**
 * Stores `theme` as lib/storage does before the page's scripts run, and records whether <html data-theme> was set
 * before <body> existed: then it was there before anything could paint.
 */
async function startWithStoredTheme(page: Page, theme: "light" | "dark") {
  await page.addInitScript(
    ([key, value]) => {
      localStorage.setItem(key, JSON.stringify({ v: 1, value }));
      const observer = new MutationObserver(() => {
        if (!document.documentElement.hasAttribute("data-theme")) return;
        document.documentElement.dataset["themeBeforeBody"] = String(document.querySelector("body") === null);
        observer.disconnect();
      });
      // An init script runs before the parser has made <html>, so the observer watches the whole document.
      observer.observe(document, { subtree: true, attributes: true, attributeFilter: ["data-theme"] });
    },
    [themeKey, theme] as const,
  );
}

/** The page's background color, which the surface token gives the body. */
function background(page: Page) {
  return page.locator("body").evaluate((body) => getComputedStyle(body).backgroundColor);
}

for (const [stored, os] of [
  ["dark", "light"],
  ["light", "dark"],
] as const) {
  test(`a stored ${stored} theme applies before the first paint on a ${os} OS, and passes axe`, async ({
    page,
    makeAxeBuilder,
  }) => {
    await page.emulateMedia({ colorScheme: os });
    await page.goto("/home");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    const osBackground = await background(page);

    await startWithStoredTheme(page, stored);
    await page.goto("/home");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();

    const html = page.locator("html");
    await expect(html).toHaveAttribute("data-theme", stored);
    await expect(html).toHaveAttribute("data-theme-before-body", "true");
    expect(await background(page)).not.toBe(osBackground);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
}

test.describe("the menu's sheet", () => {
  test.beforeEach(async ({ page }) => {
    test.skip(
      (page.viewportSize()?.width ?? 0) >= 768,
      "the rail from md has no menu; the preferences page has the theme",
    );
    // A signed-in user's session, whose items (profile, bookmarks and settings) the sheet lists; vite preview has no API.
    const json = (body: unknown) => ({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
    await page.route("**/oauth/token/refresh", (route) =>
      route.fulfill(json({ access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 })),
    );
    await page.route("**/api/v1/timelines/home*", (route) => route.fulfill(json({ items: [] })));
    await page.emulateMedia({ colorScheme: "light" });
    await page.goto("/home");
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
  });

  test("switches the theme at once with the arrow keys, and it stays after a reload", async ({
    page,
    makeAxeBuilder,
  }) => {
    await page.getByRole("navigation").getByRole("button").click();
    const sheet = page.getByRole("dialog");
    // Under the sheet's links, the theme's radio group in a row: the OS setting (checked), light and dark.
    await expect(sheet).toMatchAriaSnapshot(`
      - dialog:
        - heading [level=2]
        - button
        - list:
          - listitem:
            - link
          - listitem:
            - link
        - radiogroup:
          - radio [checked]
          - radio
          - radio
    `);
    const radios = sheet.getByRole("radio");
    await radios.first().focus();
    // The APG radio group: the arrow keys move and check (Down is the next one in both directions). Light, then dark.
    await page.keyboard.press("ArrowDown");
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    await page.keyboard.press("ArrowDown");
    await expect(radios.nth(2)).toBeChecked();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await expect(sheet).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await page.reload();
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  });
});
