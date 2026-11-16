// SPDX-License-Identifier: AGPL-3.0-only

// Finding people: the search finds an account by its name and the reader follows it from the results (FR-307); and
// finding people suggests an account that posted lately, which the reader follows from there (FR-308).

import { People, Search } from "./screens";
import { expect, test } from "./test";

test("the search finds an account by its name, and the reader follows it from the results", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const reader = await fresh.person();

  await signIn(reader);
  const search = new Search(page);
  await search.goTo();
  await search.find(author.displayName);
  await expect(search.account(author.displayName).follow).toBeVisible();
  await search.follow(author.displayName);
  await expect(search.account(author.displayName).following).toBeVisible();
});

test("finding people suggests an account that posted lately, and the reader follows it from there", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  await fresh.post(author, { text: "Back from the coast with a roll of film." });
  const reader = await fresh.person();

  await signIn(reader);
  const people = new People(page);
  await people.goTo();
  await expect(people.account(author.displayName).follow).toBeVisible();
  await people.follow(author.displayName);
  await expect(people.account(author.displayName).following).toBeVisible();
});
