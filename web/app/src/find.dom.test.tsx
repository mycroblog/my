// SPDX-License-Identifier: AGPL-3.0-only

// Finding people at their addresses: what is typed in the search is in the address and finds an account by its name
// (FR-307), and finding people suggests accounts (FR-308). Following from a row is the features' (people/people.dom.test.tsx).
// The reader is Sam, or QuickDeals, who follows no one yet.

import { expect, test } from "bun:test";
import { waitFor, within } from "@testing-library/react";
import { main, renderApp } from "./test/app";

const mio = "Mio 澪";

/** The row of the account named `name` in the list named `list`, once the screen shows it. */
const rowOf = (list: string, name: string) =>
  waitFor(() => {
    const [row, ...more] = within(main().getByRole("list", { name: list }))
      .getAllByRole("listitem")
      .filter((item) => within(item).queryByRole("link", { name }) !== null);
    if (row === undefined || more.length > 0) throw new Error(`test: no one row of ${list} is ${name}'s`);
    return row;
  });

test("what a reader types in the search is in the address, and finds the account by its name", async () => {
  const app = await renderApp("/search", { viewer: "sam" });

  await app.user.type(main().getByRole("searchbox", { name: "Search" }), mio);

  await rowOf("People", mio);
  expect(new URL(app.path(), "http://app.test").searchParams.get("q")).toBe(mio);
});

test("the address /people suggests an account", async () => {
  await renderApp("/people", { viewer: "sam" });

  await rowOf("Find people", mio);
});

test("Find more people, on the home of a reader who follows no one, opens the screen of all the suggested people", async () => {
  const app = await renderApp("/", { viewer: "quickdeals" });

  await app.user.click(await main().findByRole("link", { name: "Find more people" }));

  await rowOf("Find people", mio);
  expect(app.path()).toBe("/people");
  expect(main().getByRole("heading", { level: 1, name: "Find people" })).toBeDefined();
});
