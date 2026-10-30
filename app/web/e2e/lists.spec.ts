// SPDX-License-Identifier: AGPL-3.0-only
// The user's lists in every width × locale project (ADR-0122; web-client/screens/lists.md, list-timeline.md,
// profile.md): opened from the navigation (the rail from md, the menu's sheet below it), a list is created, gets a
// followed account, shows its timeline, is renamed and deleted, axe-clean and without horizontal scroll at every step;
// a profile of an account the user does not follow has no "Add to list". `vite preview` has no API, so the spec answers
// the token refresh and the client API with `page.route`, as a signed-in user's server would. Names come from the
// project's catalog, so the spec finds the same elements in every locale.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import {
  escapeRegExp,
  expect,
  expectNoAxeViolations,
  expectNoHorizontalScroll,
  openFromNavigation,
  test,
} from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Account = components["schemas"]["Account"];
type List = components["schemas"]["List"];
type Status = components["schemas"]["Status"];

/** The client message `key` in the project's locale, its arguments left as they are. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const text = readCatalog(locale)[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** A pattern of the message `key` whose arguments may be anything (a name in directional isolates). */
function pattern(key: string): RegExp {
  return new RegExp(
    `^${message(key)
      .split(/\{[a-z]+\}/u)
      .map(escapeRegExp)
      .join(".+")}$`,
    "u",
  );
}

const alice: Account = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
};
const bob: Account = {
  ...alice,
  id: "0192f0c4-7a8e-7000-8000-000000000002",
  username: "bob",
  acct: "bob",
  display_name: "Bob",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000002",
  url: "https://my.example/@bob",
};
const carol: Account = {
  ...alice,
  id: "0192f0c4-7a8e-7000-8000-000000000003",
  username: "carol",
  acct: "carol",
  display_name: "Carol",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000003",
  url: "https://my.example/@carol",
};

const bobsPost: Status = {
  id: "0192f0c4-7a8e-7000-8000-000000000101",
  uri: "https://my.example/ap/statuses/0192f0c4-7a8e-7000-8000-000000000101",
  url: "https://my.example/@bob/status/0192f0c4-7a8e-7000-8000-000000000101",
  account: bob,
  content: "<p>Fresh bread out of the oven.</p>",
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 60 * 60_000).toISOString(),
  media_attachments: [],
  mentions: [],
  tags: [],
  emojis: [],
  replies_count: 0,
  boosts_count: 0,
  favourites_count: 0,
  reactions: [],
  favourited: false,
  boosted: false,
  bookmarked: false,
};

/**
 * Answers the client API as Alice's session: she follows Bob and not Carol, and has no list at first. The lists and
 * their members change with the requests.
 */
async function serveApi(page: Page): Promise<void> {
  let lists: List[] = [];
  const members = new Map<string, Account[]>();
  const json = (route: Route, body: unknown, statusCode = 200) =>
    route.fulfill({
      status: statusCode,
      contentType: statusCode < 400 ? "application/json" : "application/problem+json",
      body: JSON.stringify(body),
    });
  const notFound = (route: Route) => json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600 }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace("/api/v1", "");
    const method = request.method();
    if (path === "/accounts/verify_credentials") return json(route, alice);
    if (path === "/instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0-e2e",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "open" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (path === "/preferences") {
      // The user's locale is the project's: the app displays itself in the signed-in user's locale.
      return json(route, {
        locale: test.info().project.use.locale ?? "en",
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "en",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (path === "/notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (path === "/timelines/home") return json(route, { items: [bobsPost] });
    if (path === "/accounts/search") return json(route, [bob, carol]);
    if (path === "/accounts/relationships") {
      return json(
        route,
        url.searchParams.getAll("id").map((id) => ({
          id,
          following: id === bob.id,
          requested: false,
          followed_by: false,
          requested_by: false,
          show_boosts: true,
          notify: false,
          blocking: false,
          muting: false,
          muting_notifications: false,
        })),
      );
    }
    if (path === "/accounts/lookup") {
      const acct = url.searchParams.get("acct");
      const found = [bob, carol].find((account) => account.acct === acct);
      return found === undefined ? notFound(route) : json(route, found);
    }
    if (/^\/accounts\/[^/]+\/statuses$/u.test(path)) return json(route, { items: [] });
    const accountLists = /^\/accounts\/([^/]+)\/lists$/u.exec(path);
    if (accountLists !== null) {
      return json(route, {
        items: lists.filter((list) => (members.get(list.id) ?? []).some((a) => a.id === accountLists[1])),
      });
    }
    if (path === "/lists" && method === "GET") return json(route, { items: lists });
    if (path === "/lists" && method === "POST") {
      const { name } = request.postDataJSON() as { name: string };
      const list: List = {
        id: `0192f0c4-7a8e-7000-8000-0000000a${String(lists.length).padStart(4, "0")}`,
        name,
        members_count: 0,
        created_at: new Date().toISOString(),
      };
      lists = [...lists, list];
      return json(route, list);
    }
    const timeline = /^\/timelines\/list\/([^/]+)$/u.exec(path);
    if (timeline !== null) {
      const ids = new Set((members.get(timeline[1] ?? "") ?? []).map((a) => a.id));
      return lists.some((list) => list.id === timeline[1])
        ? json(route, { items: ids.has(bob.id) ? [bobsPost] : [] })
        : notFound(route);
    }
    const member = /^\/lists\/([^/]+)\/accounts\/([^/]+)$/u.exec(path);
    if (member !== null) {
      const [, listId = "", accountId] = member;
      const current = members.get(listId) ?? [];
      const next =
        method === "POST"
          ? [...current.filter((a) => a.id !== accountId), ...[bob].filter((a) => a.id === accountId)]
          : current.filter((a) => a.id !== accountId);
      members.set(listId, next);
      lists = lists.map((list) => (list.id === listId ? { ...list, members_count: next.length } : list));
      return route.fulfill({ status: 204 });
    }
    const listMembers = /^\/lists\/([^/]+)\/accounts$/u.exec(path);
    if (listMembers !== null) return json(route, { accounts: members.get(listMembers[1] ?? "") ?? [] });
    const one = /^\/lists\/([^/]+)$/u.exec(path);
    if (one !== null) {
      const list = lists.find((item) => item.id === one[1]);
      if (list === undefined) return notFound(route);
      if (method === "PATCH") {
        const { name } = request.postDataJSON() as { name: string };
        lists = lists.map((item) => (item.id === list.id ? { ...item, name } : item));
        return json(route, { ...list, name });
      }
      if (method === "DELETE") {
        lists = lists.filter((item) => item.id !== list.id);
        return route.fulfill({ status: 204 });
      }
      return json(route, list);
    }
    return notFound(route);
  });
}

test.describe("the lists", () => {
  test("are created, filled with a followed account, read, renamed and deleted", async ({
    page,
    hasTouch,
    makeAxeBuilder,
  }) => {
    await serveApi(page);
    await page.goto("/home");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await openFromNavigation(page, "/lists", hasTouch);
    await expect(page.getByRole("heading", { level: 1, name: message("lists.title") })).toBeFocused();
    await expectNoAxeViolations(makeAxeBuilder);

    // Create: the button, the name, Create.
    const main = page.getByRole("main");
    await main.getByRole("button", { name: message("lists.create") }).click();
    await main.getByRole("textbox", { name: message("lists.name") }).fill("Bakers");
    await main.getByRole("button", { name: message("lists.create.submit"), exact: true }).click();
    await expect(main.getByRole("link", { name: "Bakers" })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Add Bob: Manage members, the search, Add. Carol, whom Alice does not follow, is not offered.
    const manage = main.getByRole("button", { name: new RegExp(escapeRegExp(message("lists.members.manage"))) });
    await manage.click();
    await expect(manage).toHaveAttribute("aria-expanded", "true");
    await main.getByRole("searchbox", { name: message("lists.members.search") }).fill("b");
    const results = main.getByRole("list", { name: message("lists.members.results") });
    await expect(results.getByRole("listitem")).toHaveCount(1);
    await results.getByRole("button", { name: pattern("lists.members.add_of") }).click();
    await expect(results.getByText(message("lists.members.added_mark"))).toBeVisible();
    await expect(main.getByRole("list", { name: message("lists.members.list") }).getByRole("link")).toHaveCount(1);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Read: the list's name opens its timeline, with Bob's post.
    await main.getByRole("link", { name: "Bakers" }).click();
    await expect(page.getByRole("heading", { level: 1, name: "Bakers" })).toBeFocused();
    await expect(page.getByRole("feed").getByRole("article")).toHaveCount(1);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // Rename: back to the lists, Rename, the name, Save.
    await page.getByRole("link", { name: message("lists.manage") }).click();
    await expect(page.getByRole("heading", { level: 1, name: message("lists.title") })).toBeFocused();
    await main.getByRole("button", { name: new RegExp(escapeRegExp(message("lists.rename"))) }).click();
    const field = main.getByRole("textbox", { name: message("lists.name") });
    await field.fill("Bread people");
    await main.getByRole("button", { name: message("lists.rename.submit"), exact: true }).click();
    await expect(main.getByRole("link", { name: "Bread people" })).toBeVisible();

    // Delete: Delete, then Delete in the dialog.
    await main.getByRole("button", { name: new RegExp(escapeRegExp(message("lists.delete"))) }).click();
    const dialog = page.getByRole("alertdialog");
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("lists.delete"), exact: true }).click();
    await expect(main.getByRole("heading", { level: 3, name: message("lists.empty") })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });

  test("are not offered on the profile of an account the user does not follow", async ({ page }) => {
    await serveApi(page);
    await page.goto("/@carol");
    await expect(page.getByRole("heading", { level: 1, name: "Carol" })).toBeVisible();
    await page.getByRole("button", { name: pattern("profile.more") }).click();
    await expect(page.getByRole("menuitem", { name: message("profile.menu.mute") })).toBeVisible();
    await expect(page.getByRole("menuitem", { name: message("lists.add_to_list.open") })).toHaveCount(0);
  });
});
