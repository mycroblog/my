// SPDX-License-Identifier: AGPL-3.0-only

// The Menu leads to the reader's lists, and a list to its timeline, where a member's new post arrives by the stream; a
// list that no longer exists says so.

import { Frame, ListIndex, ListScreen, streamOpened } from "./screens";
import { expect, person, test } from "./test";

const yuki = person("yuki");
const sam = person("sam");

test("the Menu leads to the reader's lists, and a list to its timeline, where a member's new post arrives", async ({
  page,
  signIn,
  fresh,
}) => {
  const stream = streamOpened(page);
  await signIn(yuki);
  const menu = await new Frame(page).openMenu();
  await menu.goTo("Lists");

  const lists = new ListIndex(page);
  await expect(lists.heading).toBeVisible();
  await expect(lists.list("Bread and coffee")).toHaveAccessibleName("Bread and coffee 2 members");
  await expect(lists.list("翻訳の仲間")).toHaveAccessibleName("翻訳の仲間 1 member");
  await expect(lists.list("A list with a name long enough to wrap on a phone")).toBeVisible();

  const list = await lists.open("Bread and coffee");
  await expect(list.heading).toBeVisible();
  await expect(list.feed).toBeVisible();

  // Made once the stream is open: the stream brings it, not the page read as the list opened.
  await stream;
  const { text } = await fresh.post(sam, { text: "Rolls out of the oven for the meetup." });
  // Not asserted first: another test's post by Sam may arrive with it.
  await expect(list.post(sam.displayName, text).article).toBeVisible();
});

test("a list that no longer exists says so", async ({ page, signIn }) => {
  await signIn(yuki);
  const list = new ListScreen(page, "List");
  // An ID of the server's form that no list has.
  await list.open("01890a5d-ac96-774b-bcce-b302099a8057");
  await expect(list.gone).toBeVisible();
});
