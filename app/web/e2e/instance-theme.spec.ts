// SPDX-License-Identifier: AGPL-3.0-only
// The instance theme (ADR-0059 §7) against a real `my serve` (e2e/server.ts): with a theme saved by `my instance
// theme set`, the SPA shell and a server-rendered page load its stylesheet, and the token overrides reach
// Tailwind's utilities (an unlayered :root rule beats the theme layer) in the light and the dark theme, as does the
// custom CSS. The quality gates check the default theme, so this stays small: one project, one sample theme, which
// keeps AA (the server checks it) so that the other specs running at the same time pass with it.
import type { Page } from "@playwright/test";
import { expect, test } from "./fixtures";
import { ownsServer, runMy } from "./server-cli.ts";
import { serverURL } from "./server-origin.ts";

test.use({ baseURL: serverURL });
// One worker runs every test of the file: beforeAll and afterAll run once per worker, so with the tests spread over
// several (fullyParallel), one worker's afterAll removed the theme while another's test still expected it.
test.describe.configure({ mode: "serial" });

const sample = {
  tokens: [
    { token: "color-surface", light: "#fdfdf6", dark: "#05070d" },
    { token: "radius-md", value: "3px" },
  ],
  // A custom property that nothing reads: it shows the CSS applies without changing what other specs see.
  custom_css: ":root { --e2e-custom-css: applied; }",
};

/** The one project that runs the spec; the others neither run it nor change the server's theme. */
const runs = (browserName: string, projectName: string) =>
  ownsServer && browserName === "chromium" && projectName === "en-1440";

test.beforeEach(({ browserName }, testInfo) => {
  test.skip(!ownsServer, "the theme is saved with bin/my, which needs the database of the server e2e/server.ts starts");
  test.skip(!runs(browserName, testInfo.project.name), "the theme does not depend on the viewport or the locale");
});

test.beforeAll(({ browserName }, testInfo) => {
  if (runs(browserName, testInfo.project.name)) runMy(["instance", "theme", "set"], JSON.stringify(sample));
});

test.afterAll(({ browserName }, testInfo) => {
  if (runs(browserName, testInfo.project.name))
    runMy(["instance", "theme", "set"], JSON.stringify({ tokens: [], custom_css: "" }));
});

/** The page's background color, the radius of an element with the `rounded-md` utility, and the custom CSS's property. */
function themed(page: Page) {
  return page.evaluate(() => {
    // The page's CSP requires Trusted Types, so the probe element is built without HTML strings.
    const probe = document.createElement("div");
    probe.className = "rounded-md";
    document.body.append(probe);
    const radius = getComputedStyle(probe).borderTopLeftRadius;
    probe.remove();
    return {
      background: getComputedStyle(document.body).backgroundColor,
      radius,
      custom: getComputedStyle(document.documentElement).getPropertyValue("--e2e-custom-css").trim(),
    };
  });
}

// The server pages' stylesheet has no utilities of its own (server-pages.css, source(none)), so their probe
// checks the base styles only.
for (const { path, radius } of [
  { path: "/", radius: "3px" },
  { path: "/auth/sign_in", radius: "0px" },
]) {
  test(`${path} loads the theme's stylesheet and its overrides apply`, async ({ page }) => {
    const stylesheet = page.waitForResponse((response) => new URL(response.url()).pathname.startsWith("/theme/"));
    await page.goto(path);
    const response = await stylesheet;
    expect(response.headers()["content-type"]).toBe("text/css; charset=utf-8");
    expect(response.headers()["x-content-type-options"]).toBe("nosniff");

    await page.emulateMedia({ colorScheme: "light" });
    await expect.poll(() => themed(page)).toEqual({ background: "rgb(253, 253, 246)", radius, custom: "applied" });
    await page.emulateMedia({ colorScheme: "dark" });
    await expect.poll(() => themed(page)).toEqual({ background: "rgb(5, 7, 13)", radius, custom: "applied" });
  });
}

test("the shell's theme-color follows the theme's surface", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator('meta[name="theme-color"][media="(prefers-color-scheme: light)"]')).toHaveAttribute(
    "content",
    "#fdfdf6",
  );
  await expect(page.locator('meta[name="theme-color"][media="(prefers-color-scheme: dark)"]')).toHaveAttribute(
    "content",
    "#05070d",
  );
});
