// SPDX-License-Identifier: AGPL-3.0-only

// Finding people as a feature: an account found by the search or suggested is followed from its row.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { Search } from "../search/search";
import { FindPeople, loadPeople } from "./people";

const mio = "Mio 澪";

/** The row of the account named `name` in the list named `list`, once the screen shows it. */
const rowOf = (list: string, name: string) =>
  waitFor(() => {
    const [row, ...more] = within(screen.getByRole("list", { name: list }))
      .getAllByRole("listitem")
      .filter((item) => within(item).queryByRole("link", { name }) !== null);
    if (row === undefined || more.length > 0) throw new Error(`test: no one row of ${list} is ${name}'s`);
    return row;
  });

/** Follows the account of `row` with its Follow button, and waits until the row says so. */
async function follow(
  { fake, user }: { fake: ReturnType<typeof fakeData>["fake"]; user: ReturnType<typeof renderFeature> },
  row: HTMLElement,
) {
  const asked = fake.script("POST /accounts/{id}/follow");
  await user.click(within(row).getByRole("button", { name: "Follow" }));
  expect(await within(row).findByText("Following")).not.toBeNull();
  expect(asked.seen).toHaveLength(1);
}

test("the search finds an account by its name, and the reader follows it from the results", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  const user = renderFeature(data, <Search q={mio} onQuery={() => undefined} />);

  await follow({ fake, user }, await rowOf("People", mio));
});

test("finding people suggests an account, and the reader follows it from there", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  await loadPeople(data);
  const user = renderFeature(data, <FindPeople />);

  await follow({ fake, user }, await rowOf("Find people", mio));
});
