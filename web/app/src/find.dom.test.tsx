// SPDX-License-Identifier: AGPL-3.0-only

// Finding people: the search finds an account by its name and the reader follows it from the results (FR-307); and
// finding people suggests accounts, which the reader follows from there (FR-308). The reader is Sam, whom Mio does not
// follow, so that the row offers Follow and not Follow back.

import { expect, test } from "bun:test";
import { waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";

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

/** Follows the account of `row` with its Follow button, and waits until the row says so. */
async function follow(app: TestApp, row: HTMLElement) {
  const asked = app.fake.script("POST /accounts/{id}/follow");
  await app.user.click(within(row).getByRole("button", { name: "Follow" }));
  expect(await within(row).findByText("Following")).toBeDefined();
  expect(asked.seen).toHaveLength(1);
}

test("the search finds an account by its name, and the reader follows it from the results", async () => {
  const app = await renderApp("/search", { viewer: "sam" });

  await app.user.type(main().getByRole("searchbox", { name: "Search" }), mio);

  await follow(app, await rowOf("People", mio));
});

test("finding people suggests an account, and the reader follows it from there", async () => {
  const app = await renderApp("/people", { viewer: "sam" });

  await follow(app, await rowOf("Find people", mio));
});
