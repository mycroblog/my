// SPDX-License-Identifier: AGPL-3.0-only
// The search of posts on a server that has one (FR-309, ADR-0149), against the fake search of the test build: the demo
// server (`node e2e/server.ts demo`, the demo-chromium project of playwright.config.ts), whose accounts allow search by
// default. `search = fake` exists only in that build, so the specs of the main server cannot have it: e2e/search.spec.ts
// checks that a server without a search shows nothing of it. A real browser composes with an IME through the Chrome
// DevTools Protocol, which tests of the components cannot do.
import type { Page } from "@playwright/test";
import { readCatalog } from "./catalog.ts";
import { message, persona, signIn } from "./demo.ts";
import { expect, expectNoAxeViolations, test } from "./fixtures";
import { demoServerURL } from "./server-origin.ts";

test.use({ baseURL: demoServerURL });

/** Signs in as sam, whose app is in English, and opens the tab of the search of posts; returns its field. */
async function openPostSearch(page: Page) {
  const sam = persona("sam");
  await signIn(page, sam.email, sam.password);
  await page.goto("/search");
  await page.getByRole("tab", { name: message("search.tabs.posts") }).click();
  const field = page.getByRole("searchbox", { name: message("search.posts.label") });
  await expect(field).toBeVisible();
  return field;
}

test("finds a post of the demo scenario by a word, and says when nothing is found", async ({
  page,
  makeAxeBuilder,
}) => {
  const field = await openPostSearch(page);
  const feed = page.getByRole("feed", { name: message("search.posts.results") });

  await field.fill("sourdough");
  // The jobs that fill the index run after the seed; sending the search again reads it again.
  await expect(async () => {
    await field.press("Enter");
    await expect(feed.getByRole("article").first()).toContainText("sourdough", { timeout: 2000 });
  }).toPass({ timeout: 30_000, intervals: [1000, 2000, 5000] });
  await expect(page).toHaveURL(/[?&]type=posts/);
  await expect(page).toHaveURL(/[?&]q=sourdough/);
  await expectNoAxeViolations(makeAxeBuilder);

  await field.fill("nothing-contains-this-word");
  await field.press("Enter");
  await expect(page.getByRole("heading", { level: 3, name: message("search.posts.empty.heading") })).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
});

test("does not search during an IME composition", async ({ page }) => {
  const field = await openPostSearch(page);
  const searches: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname === "/api/v1/statuses/search") searches.push(url.searchParams.get("q") ?? "");
  });
  const cdp = await page.context().newCDPSession(page);

  await field.focus();
  await cdp.send("Input.imeSetComposition", { text: "さんぽ", selectionStart: 3, selectionEnd: 3 });
  await expect(field).toHaveValue("さんぽ");
  await cdp.send("Input.insertText", { text: "散歩" });
  await expect(field).toHaveValue("散歩");
  expect(searches).toEqual([]);

  const answered = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/v1/statuses/search");
  await field.press("Enter");
  await answered;
  expect(searches).toEqual(["散歩"]);
});

test("offers the administrator only what the common rule allows", async ({ page }) => {
  const hana = persona("hana");
  await signIn(page, hana.email, hana.password);
  await page.goto("/admin/settings");

  // The only administrator's app is in Japanese.
  const ja = readCatalog("ja");
  const group = page.getByRole("radiogroup", {
    name: ja["admin.settings.search.visibilities"]?.defaultMessage ?? "",
  });
  await expect(group.getByRole("radio")).toHaveCount(3);
  await expect(
    group.getByRole("radio", { name: ja["admin.settings.search.public"]?.defaultMessage ?? "" }),
  ).toBeChecked();
});
