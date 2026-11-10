// SPDX-License-Identifier: AGPL-3.0-only
// The notification requests (ADR-0158 §7) in every width × locale project: the row at the top of the notifications,
// the list of requests, and the page of one request. The built client runs on `vite preview`, which has no API: the
// spec answers the OAuth refresh and the client API with `page.route`, as a signed-in user's server would. Names
// change with the locale, so elements are found by role and position.
import type { Page } from "@playwright/test";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

const alice = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};

/** The senders of the held notifications: a long name, and a right-to-left one. */
const senders = [
  {
    ...alice,
    id: "0192f0c4-7a8e-7000-8000-000000000011",
    username: "bartholomew",
    acct: "bartholomew@remote.example",
    display_name: "Bartholomew Montgomery-Fairweather of the Long Display Names",
    url: "https://remote.example/@bartholomew",
    created_at: "2026-11-01T00:00:00Z",
  },
  {
    ...alice,
    id: "0192f0c4-7a8e-7000-8000-000000000012",
    username: "layla",
    acct: "layla@remote.example",
    display_name: "ليلى",
    url: "https://remote.example/@layla",
  },
];

/** A mention held in the request of `sender`. */
function held(n: number, sender: (typeof senders)[number]) {
  const id = String(n).padStart(2, "0");
  const postId = `0192f0c4-7a8e-7000-8000-0000000004${id}`;
  return {
    id: `0192f0c4-7a8e-7000-8000-0000000003${id}`,
    type: "mention",
    created_at: "2026-10-16T09:00:00Z",
    account: sender,
    status: {
      id: postId,
      uri: `https://remote.example/users/${sender.username}/statuses/${postId}`,
      url: `https://remote.example/@${sender.username}/${postId}`,
      account: sender,
      content: "<p>@alice https://example.com/a/very/long/path/that/must/wrap/somewhere/before/it/scrolls/sideways</p>",
      spoiler_text: "",
      sensitive: false,
      visibility: "direct",
      language: "en",
      created_at: "2026-10-16T09:00:00Z",
      media_attachments: [],
      mentions: [],
      tags: [],
      emojis: [],
      replies_count: 0,
      boosts_count: 0,
      favourites_count: 0,
      reactions: [],
      filtered: [],
    },
  };
}

/** Answers the session refresh, the notifications, and the requests; returns the answers to the requests it got. */
async function signedInServer(page: Page) {
  const heldBy = new Map(
    senders.map((sender, index) => [sender.id, [held(index * 2, sender), held(index * 2 + 1, sender)]]),
  );
  const answers: string[] = [];
  const json = (body: unknown, status = 200) => ({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
  const notFound = () => ({
    status: 404,
    contentType: "application/problem+json",
    body: JSON.stringify({ title: "Not Found", status: 404 }),
  });
  const requestOf = (id: string) => {
    const items = heldBy.get(id);
    const account = senders.find((sender) => sender.id === id);
    return items === undefined || account === undefined
      ? undefined
      : { id, account, notifications_count: items.length, last_notification: items[0] };
  };
  await page.route("**/oauth/token/refresh", (route) =>
    route.fulfill(json({ access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 })),
  );
  await page.route("**/api/v1/accounts/verify_credentials", (route) => route.fulfill(json(alice)));
  await page.route("**/api/v1/notifications/marker", (route) => route.fulfill(json({ unread_count: 0 })));
  await page.route(
    (url) => url.pathname === "/api/v1/notifications",
    (route) => route.fulfill(json({ items: [] })),
  );
  await page.route(
    (url) => url.pathname.startsWith("/api/v1/notifications/requests"),
    (route) => {
      const path = new URL(route.request().url()).pathname.replace("/api/v1/notifications/requests", "");
      const [, id, action] = path.split("/");
      if (path === "/summary") {
        return route.fulfill(
          json({
            accounts_count: heldBy.size,
            notifications_count: [...heldBy.values()].reduce((sum, items) => sum + items.length, 0),
          }),
        );
      }
      if (path === "") return route.fulfill(json({ items: [...heldBy.keys()].flatMap((key) => requestOf(key) ?? []) }));
      if (id === undefined) return route.fulfill(notFound());
      if (route.request().method() === "POST") {
        answers.push(`${action} ${id}`);
        if (!heldBy.delete(id)) return route.fulfill(notFound());
        return route.fulfill({ status: 204 });
      }
      if (action === "notifications") {
        const items = heldBy.get(id);
        return route.fulfill(items === undefined ? notFound() : json({ items }));
      }
      const request = requestOf(id);
      return route.fulfill(request === undefined ? notFound() : json(request));
    },
  );
  return { answers };
}

test.describe("notification requests", () => {
  let server: Awaited<ReturnType<typeof signedInServer>>;
  test.beforeEach(async ({ page }) => {
    server = await signedInServer(page);
  });

  test("the row on the notifications page leads to the list, which passes axe and does not scroll sideways", async ({
    page,
    makeAxeBuilder,
  }) => {
    await page.goto("/notifications");
    const row = page.getByRole("main").locator('a[href="/notifications/requests"]');
    await expect(row).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await row.click();
    const list = page.getByRole("main").getByRole("list");
    await expect(list.getByRole("listitem")).toHaveCount(2);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("accepting a request from the list takes it off, without a reload", async ({ page }) => {
    await page.goto("/notifications/requests");
    const list = page.getByRole("main").getByRole("list");
    await expect(list.getByRole("listitem")).toHaveCount(2);

    await list.getByRole("listitem").first().getByRole("button").first().click();

    await expect(list.getByRole("listitem")).toHaveCount(1);
    expect(server.answers).toEqual([`accept ${senders[0]?.id ?? ""}`]);
    // The focus moved to the first button of the row that took its place.
    await expect(list.getByRole("listitem").first().getByRole("button").first()).toBeFocused();
  });

  test("the page of a request lists its notifications, passes axe, and does not scroll sideways", async ({
    page,
    makeAxeBuilder,
  }) => {
    await page.goto(`/notifications/requests/${senders[1]?.id ?? ""}`);
    await expect(page.getByRole("feed").getByRole("article")).toHaveCount(2);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});
