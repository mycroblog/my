// SPDX-License-Identifier: AGPL-3.0-only
// The color theme in every width × locale project (app/ui/README.md, "Light and dark"): a theme chosen on this
// browser reaches <html data-theme> before the first paint, whatever the OS says, and the shell passes axe in
// both themes. The theme is chosen on the preferences page only, which e2e/settings.spec.ts covers.
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
