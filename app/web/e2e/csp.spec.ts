// SPDX-License-Identifier: AGPL-3.0-only
// The SPA under the real Content-Security-Policy of `my serve` (e2e/server.ts; ADR-0019 §9, ADR-0028):
// it loads, signs in through the server's sign-in screen and reaches the client API, and no page on the
// way reports a CSP violation. The pages of `vite preview` get the same policy from the fixture
// (e2e/fixtures.ts), so every spec fails on a sink the policy refuses; this one checks the server's own shell.
import type { Page } from "@playwright/test";
import { expect, test } from "./fixtures";
import { admin, serverURL } from "./server-origin.ts";

test.use({ baseURL: serverURL });

/** A `securitypolicyviolation` event (https://w3c.github.io/webappsec-csp/#violation-events), reduced to what identifies it. */
type Violation = { page: string; directive: string; blocked: string; source: string; sample: string };

/**
 * Collects every CSP violation that any document of the page reports, from its first script on. The
 * listener is added before the page's own scripts run (addInitScript) and reports through a binding, so
 * nothing in the page is compiled from a string, which the page's Trusted Types would block.
 */
async function collectViolations(page: Page): Promise<Violation[]> {
  const violations: Violation[] = [];
  await page.exposeFunction("__reportCSPViolation", (violation: Violation) => {
    violations.push(violation);
  });
  await page.addInitScript(() => {
    const report = (globalThis as unknown as { __reportCSPViolation: (v: Violation) => void }).__reportCSPViolation;
    document.addEventListener("securitypolicyviolation", (event) => {
      report({
        page: document.location.pathname,
        directive: event.effectiveDirective,
        blocked: event.blockedURI,
        source: `${event.sourceFile}:${String(event.lineNumber)}:${String(event.columnNumber)}`,
        sample: event.sample,
      });
    });
  });
  return violations;
}

test("the app signs in and reaches the client API with no CSP violation", async ({ page }) => {
  const violations = await collectViolations(page);
  /** Requests that went to another origin than the server's: the runtime config's apiOrigin is wrong. */
  const elsewhere: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).origin !== serverURL) elsewhere.push(request.url());
  });

  // /login is not a page: it opens the server's sign-in screen at once.
  await page.goto("/login");

  await page.locator('input[type="email"][autocomplete="username"]').fill(admin.email);
  await page.locator('input[type="password"][autocomplete="current-password"]').fill(admin.password);
  // The web client is first party, so no consent page follows: the server redirects to the app's callback,
  // which exchanges the code and opens the home timeline (/home) with the access token.
  const authorized = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname.startsWith("/api/") &&
      response.request().headers()["authorization"]?.startsWith("Bearer ") === true &&
      response.ok(),
  );
  await page.locator('main button[type="submit"]').first().click();
  await authorized;

  await expect(page).toHaveURL(`${serverURL}/home`);
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
  expect(elsewhere, "requests to another origin than the server's").toEqual([]);
  expect(violations, "CSP violations").toEqual([]);
});
