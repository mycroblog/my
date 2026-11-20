// SPDX-License-Identifier: AGPL-3.0-only

// Show more under the reader's lists that grow in place (#3279), on their routes and the fake API: the reader's
// invites, an account's followers, the follow requests to a locked account, and the posts of the report panel. Each
// list holds more than a page of the server's; a press of Show more adds the rest under the first page, the focus on
// the first item it added, and Show more goes once the list is read to its end. The events' list is
// events-routes.dom.test.tsx's, the finance totals' Earlier and Later finance.dom.test.tsx's.

import { expect, test } from "bun:test";
import { fakeOrigin, scenario, type FakePerson, type FakeSeed } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";

/** How many items a page of the server's holds when the request names no limit. */
const page = 20;

/** `count` people of another server, `fan0@many.example` and on, drawn as the scenario's first person is. */
const fans = (count: number): FakePerson[] =>
  Array.from({ length: count }, (_, i) => ({
    username: `fan${String(i)}@many.example`,
    email: "",
    password: "",
    displayName: `Fan ${String(i)}`,
    note: "",
    fields: [],
    locale: "en",
    avatar: (scenario.people[0] as FakePerson).avatar,
  }));

/** The scenario with `people` added. */
const withPeople = (people: readonly FakePerson[], more: Partial<FakeSeed> = {}): Partial<FakeSeed> => ({
  ...scenario,
  people: [...scenario.people, ...people],
  ...more,
});

/** Presses Show more (`more`) and waits until the list shows `count` of `items`. */
async function showMore(app: TestApp, more: HTMLElement, items: () => HTMLElement[], count: number) {
  await app.user.click(more);
  await waitFor(() => {
    if (items().length !== count) throw new Error(`test: ${String(items().length)} of ${String(count)} are shown`);
  });
}

/**
 * Waits until the focus is on `items[at]`, in it, or on the row that holds it and not the one before.
 */
const focusIsOn = (app: TestApp, items: readonly HTMLElement[], at: number) =>
  waitFor(() => {
    const focused = app.focused();
    const item = items[at];
    const before = items[at - 1];
    const onIt = focused !== null && item !== undefined && (item.contains(focused) || focused.contains(item));
    if (!onIt || (before !== undefined && focused.contains(before)))
      throw new Error(`test: the focus is on ${focused?.outerHTML.slice(0, 80) ?? "the page"}`);
  });

test("the reader's invites: Show more adds the older ones under the first page, the focus on the first it added", async () => {
  const app = await renderApp("/settings/invites", { access: { invites: "many" } });
  const list = await main().findByRole("list", { name: "Your invites" });
  const links = () => within(list).getAllByRole("link");
  expect(links()).toHaveLength(page);

  await showMore(app, main().getByRole("button", { name: "Show more" }), links, 26);

  await focusIsOn(app, within(list).getAllByRole("listitem"), page);
  expect(main().queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
});

test("an account's followers: Show more adds the next page under the first, the focus on the first it added", async () => {
  const people = fans(24);
  const app = await renderApp("/@sam/followers", {
    seed: withPeople(people, {
      follows: [...scenario.follows, ...people.map((p) => ({ follower: p.username, followee: "sam" }))],
    }),
  });
  const panel = within(await screen.findByRole("tabpanel"));
  await panel.findByRole("button", { name: "Show more" });
  const accounts = () => panel.getAllByRole("link");
  expect(accounts()).toHaveLength(page);
  const total = scenario.follows.filter((f) => f.followee === "sam").length + people.length;

  await showMore(app, panel.getByRole("button", { name: "Show more" }), accounts, total);

  await focusIsOn(app, accounts(), page);
  expect(panel.queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
});

test("the follow requests: Show more adds the older requests under the newest, the focus on the first it added", async () => {
  const people = fans(24);
  const app = await renderApp("/notifications/follow_requests", {
    viewer: "ren",
    seed: withPeople(people),
    followRequests: { from: people.map((p) => p.username) },
  });
  await main().findByRole("button", { name: "Show more" });
  const accounts = () => main().getAllByRole("link");
  expect(accounts()).toHaveLength(page);
  const total = accounts().length + (await requestsLeft(app));

  await showMore(app, main().getByRole("button", { name: "Show more" }), accounts, total);

  await focusIsOn(app, accounts(), page);
  expect(main().queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
});

/** How many follow requests to the reader the server holds after its first page. */
async function requestsLeft(app: TestApp): Promise<number> {
  const all = await app.fake.serve(new Request(`${fakeOrigin}/api/v1/follow_requests?limit=40`));
  const body = (await all.json()) as { items: unknown[] };
  return body.items.length - page;
}

test("the report panel's posts: Show more adds the account's older posts, the focus on the first it added", async () => {
  const posts = Array.from({ length: 26 }, (_, i) => ({
    key: `loaf-${String(i)}`,
    author: "sam",
    text: `Loaf number ${String(i + 1)}.`,
    visibility: "public" as const,
    language: "en",
  }));
  const app = await renderApp("/@sam", { seed: { people: scenario.people, posts } });
  const card = within(await screen.findByRole("region", { name: "Sam Rivera" }));
  await app.user.click(await card.findByRole("button", { name: "More" }));
  await app.user.click(
    within(await screen.findByRole("menu", { name: "More" })).getByRole("menuitem", { name: "Report…" }),
  );
  const panel = within(await screen.findByRole("dialog", { name: /^Report .*@sam/ }));
  await panel.findByRole("button", { name: "Show more" });
  const choices = () => panel.getAllByRole("checkbox", { name: /Loaf number/ });
  expect(choices()).toHaveLength(page);

  await showMore(app, panel.getByRole("button", { name: "Show more" }), choices, posts.length);

  await focusIsOn(app, choices(), page);
  expect(panel.queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
});
