// SPDX-License-Identifier: AGPL-3.0-only
// The header of a post card with the names that are hardest to fit, in every width × locale project
// (ADR-0027, web-client/16-look-states-and-wording.md G7, G9): a 30-character display name, an Arabic display name and a handle on a
// long domain, as authors and as a booster. Each header stays within two lines, the page does not scroll
// sideways (the per-PR 320 px projects are where that breaks first), and a local author's handle has no
// domain while a remote one's has it (web-client/13-status-detail-and-actions.md §13). The accounts are the client's shared test
// fixtures (src/test/fixtures.ts), and `vite preview` has no client API, so the spec answers the local
// timeline itself.
import type { components } from "@mycroblog/mykit";
import { longDomainAccountFixture, longNameAccountFixture, rtlNameAccountFixture } from "../src/test/fixtures.ts";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, expectPostHeadersInTwoLines, test } from "./fixtures";

type Account = components["schemas"]["Account"];
type Status = components["schemas"]["Status"];
type StatusPage = components["schemas"]["StatusPage"];

const accounts = [longNameAccountFixture, rtlNameAccountFixture, longDomainAccountFixture] as const;

function status(n: number, account: Account, overrides: Partial<Status> = {}): Status {
  return {
    id: `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`,
    uri: `https://${account.domain}/statuses/${String(n)}`,
    account,
    content: `<p>Post number ${String(n)}.</p>`,
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    language: "en",
    created_at: new Date(Date.now() - n * 7 * 60_000).toISOString(),
    media_attachments: [],
    mentions: [],
    tags: [],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
    ...overrides,
  };
}

/** A post by each account, then a boost by the right-to-left one of a post by the long-domain one. */
function timelinePage(): StatusPage {
  const [longName, rtlName, longDomain] = accounts;
  return {
    items: [
      status(1, longName),
      status(2, rtlName, { language: "ar", content: "<p>مرحبا بالعالم</p>" }),
      status(3, longDomain),
      status(4, rtlName, { content: "", boost: status(5, longDomain) }),
    ],
  };
}

test.describe("a post card's header with long and right-to-left names", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/timelines/local*", async (route) => {
      await route.fulfill({ json: timelinePage() });
    });
    await page.goto("/local");
    await expect(page.getByRole("feed").getByRole("article")).toHaveCount(4);
  });

  test("takes at most two lines", async ({ page }) => {
    await expectPostHeadersInTwoLines(page.getByRole("feed"));
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });

  test("names the author's link by the whole name and handle", async ({ page }) => {
    const feed = page.getByRole("feed");
    for (const account of accounts) {
      await expect(feed.getByRole("link", { name: `${account.display_name} @${account.acct}` }).first()).toBeVisible();
    }
  });

  test("shows a local author's handle without the domain and a remote one's with it", async ({ page }) => {
    const [longName, rtlName, longDomain] = accounts;
    await expect(page.getByRole("feed")).toMatchAriaSnapshot(`
      - feed:
        - article:
          - link "${longName.display_name} @${longName.username}"
        - article:
          - link "${rtlName.display_name} @${rtlName.username}@${rtlName.domain}"
        - article:
          - link "${longDomain.display_name} @${longDomain.username}@${longDomain.domain}"
        - article:
          - link "${longDomain.display_name} @${longDomain.username}@${longDomain.domain}"
    `);
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });
});
