// SPDX-License-Identifier: AGPL-3.0-only
// The instance's information page (FR-718, #625) in every width × locale project: what everyone may read about the
// server at /about, against `vite preview`, whose client API the spec plays with page.route (no token refresh, so no
// session). Names change with the locale, so elements are found by role, structure, addresses, and the instance's
// own text, which is in English in every project.
import type { Page } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

type Instance = components["schemas"]["Instance"];
type DomainAction = components["schemas"]["InstanceDomainAction"];

const domainActions: DomainAction[] = [
  {
    domain: "spam.example",
    digest: "1f6c8a7bfe4b3c0d2e9a8b7c6d5e4f30112233445566778899aabbccddeeff00",
    kind: "suspend",
    comment: "Spam, harassment and a rather long explanation that has to wrap on the narrowest screens",
  },
  {
    // An obfuscated domain long enough to have to wrap.
    domain: "a-very-long-subdomain-name.*************************************.example",
    digest: "2a7d9b8c0f5c4d1e3f0b9c8d7e6f504122334455667788990011aabbccddeeff",
    kind: "limit",
  },
];

/** The instance's status page, on another site (FR-810). */
const statusPageUrl = "https://status.my.example/";

/** Plays the instance and its moderated domains (or the status that the visibility setting answers) for a visitor without a session. */
async function serveInstance(page: Page, origin: string, domains: { status: 200 } | { status: 401 | 404 }): Promise<void> {
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
    status_page_url: statusPageUrl,
  };
  await page.route("**/api/v1/instance", (route) => route.fulfill({ json: instance }));
  await page.route("**/api/v1/instance/domain_actions*", (route) =>
    domains.status === 200
      ? route.fulfill({ json: { items: domainActions } })
      : route.fulfill({
          status: domains.status,
          contentType: "application/problem+json",
          json: { type: domains.status === 401 ? "/api/v1/problems/unauthenticated" : "about:blank", title: "Error", status: domains.status },
        }),
  );
}

test.describe("the information page", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await serveInstance(page, new URL(baseURL ?? "").origin, { status: 200 });
    await page.goto("/about");
    await expect(page.getByRole("main").locator("li", { hasText: "spam.example" })).toBeVisible();
  });

  test("tells everyone about the server, its rules and documents, the servers it moderates, and its source code", async ({ page }) => {
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toContainText("my example");
    await expect(main).toMatchAriaSnapshot(`
      - main:
        - heading [level=1]
        - paragraph: A small server for gardeners and everyone who likes to grow things.
        - region:
          - heading [level=2]
          - paragraph: We grow vegetables, herbs and flowers.
          - paragraph: Everyone is welcome, whatever the size of their garden.
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
          - navigation:
            - list:
              - listitem:
                - link
              - listitem:
                - link
        - region:
          - heading [level=2]
          - paragraph
          - paragraph
          - list:
            - listitem:
              - paragraph: /spam\\.example/
              - paragraph: /Spam, harassment/
            - listitem:
              - paragraph: /a-very-long-subdomain-name/
        - region:
          - heading [level=2]
        - region:
          - heading [level=2]
          - paragraph
          - paragraph:
            - link
        - region:
          - heading [level=2]
          - paragraph
          - paragraph:
            - link
        - region:
          - heading [level=2]
          - paragraph
          - list:
            - listitem:
              - link
            - listitem:
              - link
            - listitem:
              - link
            - listitem:
              - link
    `);
  });

  test("links to the server's status page, which opens in a new tab (FR-810)", async ({ page }) => {
    const link = page.getByRole("main").locator(`a[href="${statusPageUrl}"]`);
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute("target", "_blank");
    await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  test("links to the license notices of what the web app bundles, which open as plain text", async ({ page }) => {
    await page.getByRole("main").locator('a[href="/licenses/LICENSE-Unicode.txt"]').click();
    await expect(page).toHaveURL(/\/licenses\/LICENSE-Unicode\.txt$/u);
    await expect(page.locator("body")).toContainText("UNICODE LICENSE V3");
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test("the information page of a server that lists the servers it moderates to nobody", async ({ page, baseURL, makeAxeBuilder }) => {
  await serveInstance(page, new URL(baseURL ?? "").origin, { status: 404 });
  await page.goto("/about");
  const main = page.getByRole("main");
  await expect(main.getByRole("region").getByRole("navigation")).toBeVisible();
  // The description, the rules, the documents, the statement to crawlers, the status page, the source code, and the licenses.
  await expect(main.getByRole("region")).toHaveCount(7);
  await expect(main.locator("li", { hasText: "spam.example" })).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

test("the information page of a server that lists the servers it moderates to its users only", async ({ page, baseURL, makeAxeBuilder }) => {
  await serveInstance(page, new URL(baseURL ?? "").origin, { status: 401 });
  await page.goto("/about");
  const main = page.getByRole("main");
  await expect(main.locator('a[href="/login?return_to=%2Fabout"]')).toBeVisible();
  await expect(main.locator("li", { hasText: "spam.example" })).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});
