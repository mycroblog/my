// SPDX-License-Identifier: AGPL-3.0-only
// The PWA (ADR-0028) against a real `my serve` (e2e/server.ts), which serves the HTML shell, the Web App
// Manifest and /sw.js as in production: the app is installable, the Service Worker answers the build from
// its precache and starts the app offline, and it leaves the client API and the server's own pages to the
// network. The worker behaves the same at every width and in every locale, so one project runs it.
import type { Page } from "@playwright/test";
import { expect, test } from "./fixtures";
import { serverURL } from "./server-origin.ts";

test.use({ baseURL: serverURL, serviceWorkers: "allow" });

test.beforeEach(({ browserName }, testInfo) => {
  test.skip(browserName !== "chromium", "Service Workers are observable in Chromium only");
  test.skip(testInfo.project.name !== "en-1440", "the worker does not depend on the viewport or the locale");
});

/**
 * Opens `path` and waits until the Service Worker controls the page (it claims the page when it activates).
 * The page's CSP requires Trusted Types, so the wait is a promise in the page rather than waitForFunction,
 * whose polling compiles a function from a string.
 */
async function openControlled(page: Page, path = "/"): Promise<void> {
  await page.goto(path);
  await page.evaluate(async () => {
    await navigator.serviceWorker.ready;
    if (navigator.serviceWorker.controller !== null) return;
    await new Promise((resolve) => {
      navigator.serviceWorker.addEventListener("controllerchange", resolve, { once: true });
    });
  });
}

/** Every URL in Cache Storage. */
function cachedURLs(page: Page): Promise<string[]> {
  return page.evaluate(async () => {
    const urls: string[] = [];
    for (const name of await caches.keys()) {
      const cache = await caches.open(name);
      urls.push(...(await cache.keys()).map((request) => request.url));
    }
    return urls;
  });
}

test("meets Chrome's install criteria", async ({ page }) => {
  await openControlled(page);
  const cdp = await page.context().newCDPSession(page);
  // The installability check behind Chrome's install prompt and Lighthouse's former PWA audit.
  const { installabilityErrors } = await cdp.send("Page.getInstallabilityErrors");
  expect(installabilityErrors).toEqual([]);
  // iOS takes the home screen icon from the page, not from the manifest.
  await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveAttribute("href", "/apple-touch-icon.png");
});

test("answers the build from the precache and never answers or caches the client API", async ({ page }) => {
  await openControlled(page);
  const script = page.waitForResponse((response) => new URL(response.url()).pathname.startsWith("/assets/"));
  await page.reload();
  expect((await script).fromServiceWorker()).toBe(true);

  const api = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/instance");
  await page.evaluate(async () => {
    await fetch("/api/v1/instance", { headers: { Authorization: "Bearer e2e-not-a-token" } });
    await fetch("/api/v1/instance");
  });
  expect((await api).fromServiceWorker()).toBe(false);

  const urls = await cachedURLs(page);
  expect(urls.some((url) => new URL(url).pathname.startsWith("/assets/"))).toBe(true);
  expect(urls.filter((url) => new URL(url).pathname.startsWith("/api"))).toEqual([]);
});

test("lets the server decide every page online: its status and its link preview tags", async ({ page }) => {
  await openControlled(page);

  const profile = await page.goto("/@alice");
  expect(profile?.status()).toBe(200);
  expect(profile?.fromServiceWorker()).toBe(true);
  // The canonical URL is https:// even on this plain-HTTP test server.
  await expect(page.locator('meta[property="og:url"]')).toHaveAttribute("content", /^https:\/\/[^/]+\/@alice$/);

  const missing = await page.goto("/@nobody-here");
  expect(missing?.status()).toBe(404);
  await expect(page.locator('meta[property="og:url"]')).toHaveCount(0);

  // No page is stored: only the generic shell at /home, from the precache, is ever replayed.
  const pages = (await cachedURLs(page))
    .map((url) => new URL(url).pathname)
    .filter((path) => !path.startsWith("/assets/"));
  expect(pages.filter((path) => path.startsWith("/@"))).toEqual([]);
});

test("leaves the server-rendered pages to the network", async ({ page }) => {
  await openControlled(page);
  const signIn = await page.goto("/auth/sign_in");
  expect(signIn?.fromServiceWorker()).toBe(false);
});

test("starts the app from the offline shell, but not the server's own pages", async ({ page, context }) => {
  await openControlled(page);
  await context.setOffline(true);

  await page.goto("/local");
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByRole("navigation")).toBeVisible();
  // The page says why it cannot load instead of a skeleton that never ends, under the offline bar
  // (web-client/16-look-states-and-wording.md G17).
  await expect(page.getByRole("status").filter({ hasText: "You are offline." })).toBeVisible();
  await expect(
    page.getByRole("main").getByText("You are offline, so the server cannot be reached.", { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole("main").getByRole("progressbar")).toHaveCount(0);
  // GET /instance cannot answer, and the navigation still offers the federated timeline the server offered online.
  await expect(page.getByRole("navigation").locator('a[href="/public"]')).toBeVisible();

  await expect(page.goto("/auth/sign_in")).rejects.toThrow();
  await context.setOffline(false);
});

test("opens another page offline with its heading under the offline bar, which goes back online", async ({
  page,
  context,
}) => {
  await openControlled(page, "/local");
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
  await context.setOffline(true);

  // Signed out (no session here): the navigation has the pages usable without one.
  await page.getByRole("navigation").locator('a[href="/search"]').click();
  await expect(page.getByRole("main").getByRole("heading", { level: 1, name: "Search" })).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "You are offline." })).toBeVisible();

  await context.setOffline(false);
  await expect(page.getByRole("status").filter({ hasText: "You are offline." })).toHaveCount(0);
});
