// SPDX-License-Identifier: AGPL-3.0-only
// The loading state of a list in every width × locale project (web-client/16-look-states-and-wording.md G15): while the first page is
// on its way, the region is aria-busy, a skeleton fills it without a spinner, and one visually hidden text
// names what is loading. `vite preview` has no client API, so the spec answers it with `page.route` and
// holds the list's response back until it has looked at the page. Names change with the locale, so the
// snapshots match any text.
import type { Page, Route } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

const json = (body: unknown) => ({ status: 200, contentType: "application/json", body: JSON.stringify(body) });

const bob = {
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000002",
  display_name: "Bob",
  note: "",
  badges: [],
  fields: [],
  emojis: [],
  url: "https://my.example/@bob",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  followers_count: 1,
  following_count: 0,
};

/** Holds the responses of `pattern` back until `release` is called, then answers them with `body`. */
async function hold(page: Page, pattern: string, body: unknown): Promise<{ release: () => void }> {
  let release = () => {};
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(pattern, async (route: Route) => {
    await released;
    await route.fulfill(json(body));
  });
  return { release };
}

/** The one busy region of the page's main landmark. */
function busyRegion(page: Page) {
  return page.getByRole("main").locator("[aria-busy='true']");
}

/** The region is named once, by its only text, and shows no spinner (Spinner is a progressbar). */
async function expectLoadingRegion(page: Page): Promise<void> {
  const region = busyRegion(page);
  await expect(region).toHaveCount(1);
  await expect(region).toMatchAriaSnapshot(`
    - /children: equal
    - text: /.+/
  `);
  await expect(page.getByRole("main").getByRole("progressbar")).toHaveCount(0);
}

test.describe("a timeline's first page", () => {
  test("is a busy skeleton named once, which passes axe and does not scroll sideways", async ({
    page,
    makeAxeBuilder,
  }) => {
    const { release } = await hold(page, "**/api/v1/timelines/local*", { items: [] });
    await page.goto("/local");

    await expectLoadingRegion(page);
    // The page's heading is there while the first page loads, and the skeleton under it.
    await expect(page.getByRole("main")).toMatchAriaSnapshot(`
      - heading [level=1]
      - text: /.+/
    `);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    release();
    await expect(busyRegion(page)).toHaveCount(0);
    await expect(page.getByRole("main").getByRole("heading", { level: 2 })).toBeVisible();
  });
});

test.describe("a follow list's first page", () => {
  test("is a busy skeleton of rows named once, under the page heading", async ({ page, makeAxeBuilder }) => {
    await page.route("**/api/v1/accounts/lookup*", (route) => route.fulfill(json(bob)));
    const { release } = await hold(page, `**/api/v1/accounts/${bob.id}/followers*`, { accounts: [] });
    await page.goto("/@bob/followers");

    await expectLoadingRegion(page);
    // The heading names the account by the handle in the address until its name is known.
    await expect(page.getByRole("heading", { level: 1 })).toContainText("@bob");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    release();
    await expect(page.getByRole("heading", { level: 1 })).toContainText("Bob");
    await expect(busyRegion(page)).toHaveCount(0);
  });
});
