// SPDX-License-Identifier: AGPL-3.0-only
// The landing page (docs/architecture/web-client/screens/landing.md) in every width × locale project: what the root
// address shows people who are not signed in, against `vite preview`, whose client API the spec plays with page.route
// (no token refresh, so no session).
// Names change with the locale, so elements are found by role, structure, addresses, and the instance's own text,
// which is in English in every project.
import type { Page } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

type Instance = components["schemas"]["Instance"];
type InstanceOverview = components["schemas"]["InstanceOverview"];
type Status = components["schemas"]["Status"];

const account: Status["account"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000002",
  display_name: "Bob with a rather long display name that has to wrap",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@bob",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};

function status(n: number, content: string): Status {
  return {
    id: `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`,
    uri: `https://my.example/ap/statuses/${String(n)}`,
    account,
    content,
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
  };
}

/** The counts of a small server and the administrator who runs it. */
const overview: InstanceOverview = {
  counts: {
    members: 120,
    daily_posts: [3, 5, 2, 8, 6, 4, 7],
    started_at: "2026-03-14T00:00:00Z",
    counted_at: "2026-10-28T00:00:00Z",
  },
  operators: [
    {
      ...account,
      id: "0192f0c4-7a8e-7000-8000-000000000007",
      username: "olive",
      acct: "olive",
      display_name: "Olive",
      url: "https://my.example/@olive",
      badges: [{ role_id: "0192f0c4-7a8e-7000-8000-0000000000b1", label: "Admin", color: "red" }],
    },
  ],
};

/** Plays the instance, its overview and its local timeline for a visitor without a session. */
async function serveInstance(page: Page, origin: string, overrides: Partial<Instance> = {}): Promise<void> {
  const instance: Instance = {
    domain: "my.example",
    name: "my example",
    short_description: "A small server for gardeners and everyone who likes to grow things.",
    description: "We grow vegetables, herbs and flowers.\n\nEveryone is welcome, whatever the size of their garden.",
    version: "0.0.0-e2e",
    source_url: "https://github.com/mycroblog/my",
    registrations: { mode: "approval" },
    limits: { max_post_characters: 500, require_media_description: false },
    timelines: { local: "everyone", federated: "everyone" },
    federation: { mode: "both" },
    sends_email: true,
    search_indexing: "deny",
    ai_usage: "deny",
    finance_visibility: "private",
    income_visibility: "private",
    supporters_enabled: false,
    supporter_list_visibility: "off",
    voice_enabled: false,
    voice_room_starters: "permitted",
    rules: [
      { id: "0192f0c4-7a8e-7000-8000-0000000000e1", text: "Be kind", hint: "No insults, no harassment." },
      { id: "0192f0c4-7a8e-7000-8000-0000000000e2", text: "Mark sensitive media" },
    ],
    terms_url: `${origin}/about/terms`,
    privacy_policy_url: `${origin}/about/privacy`,
    ...overrides,
  };
  await page.route("**/api/v1/instance", (route) => route.fulfill({ json: instance }));
  await page.route("**/api/v1/instance/overview", (route) => route.fulfill({ json: overview }));
  await page.route("**/api/v1/timelines/local*", (route) =>
    route.fulfill({
      json: {
        items: [
          status(1, "<p>The tomatoes are ripe!</p>"),
          status(2, `<p>${"averyveryverylongwordwithoutanyspaces".repeat(4)}</p>`),
        ],
      },
    }),
  );
}

test.describe("the landing page", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await serveInstance(page, new URL(baseURL ?? "").origin);
    await page.goto("/");
    // The two most recent posts are in the first section.
    await expect(page.getByRole("main").getByRole("article")).toHaveCount(2);
  });

  test("shows the server's calling card, then three sections under a progress line, then the footer", async ({
    page,
  }) => {
    await expect(page).toHaveURL(/\/$/);
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toHaveText("my example");
    await expect(main.locator('a[href$="/auth/sign_up"]')).toBeVisible();
    await expect(main).toMatchAriaSnapshot(`
      - main:
        - button
        - heading "my example" [level=1]
        - paragraph: my.example
        - paragraph: A small server for gardeners and everyone who likes to grow things.
        - list:
          - listitem
          - listitem
          - listitem:
            - link
        - link
        - link
        - navigation:
          - list:
            - listitem:
              - link
            - listitem:
              - link
            - listitem:
              - link
        - region:
          - heading [level=2]
          - paragraph: We grow vegetables, herbs and flowers.
          - paragraph: Everyone is welcome, whatever the size of their garden.
          - heading [level=3]
          - list:
            - listitem:
              - link
          - heading [level=3]
          - feed:
            - article
            - article
          - paragraph:
            - link
        - region:
          - heading [level=2]
          - list:
            - listitem:
              - paragraph: Be kind
              - paragraph: No insults, no harassment.
            - listitem:
              - paragraph: Mark sensitive media
        - region:
          - heading [level=2]
          - list:
            - listitem
            - listitem
            - listitem
          - link
          - heading [level=3]
          - list
    `);
  });

  test("moves from the calling card to the steps of joining, past the rules", async ({ page }) => {
    const main = page.getByRole("main");
    // The calling card's link comes first; the sticky progress line of the sections links the steps of joining too.
    await main.locator('a[href="#landing-join"]').first().click();
    await expect(main.locator("#landing-join")).toBeFocused();
    await expect(main.locator("#landing-join")).toBeInViewport();
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });

  test("leads to the information page", async ({ page }) => {
    await page.getByRole("contentinfo").locator('a[href="/about"]').click();
    await expect(page).toHaveURL(/\/about$/);
  });

  test("leads to the local timeline", async ({ page }) => {
    await page.getByRole("main").locator('a[href="/local"]').first().click();
    await expect(page).toHaveURL(/\/local$/);
  });
});

test("the landing page of a server that takes no new accounts and has no local timeline", async ({
  page,
  baseURL,
  makeAxeBuilder,
}) => {
  await serveInstance(page, new URL(baseURL ?? "").origin, {
    registrations: { mode: "closed" },
    timelines: { local: "off", federated: "off" },
    rules: [],
  });
  await page.goto("/");
  const main = page.getByRole("main");
  await expect(main.getByRole("navigation").first()).toBeVisible();
  await expect(main.locator('a[href$="/auth/sign_up"]')).toHaveCount(0);
  await expect(main.getByRole("feed")).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});
