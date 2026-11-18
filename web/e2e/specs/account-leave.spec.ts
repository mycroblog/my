// SPDX-License-Identifier: AGPL-3.0-only

// Leaving an account from the settings, on the real server (#3203; ADR-0089 §2, §3, §11): a person moves to another
// account of theirs, which the server refuses until that account names this one among its aliases; once it does, the
// move asks for the password and takes the followers there, and the person cancels it. And a person deactivates their
// account, and the app leaves for the start, signed out. Every person is fresh. What each screen asks, refuses and
// sends is a component test on the fake (web/app, settings-account-leave.dom.test.tsx).

import { AccountData, Follows, Frame, Landing, SignIn } from "./screens";
import { expect, test } from "./test";

test("a move is refused until the other account names this one an alias; then it moves the followers there, and is cancelled", async ({
  page,
  browser,
  signIn,
  fresh,
}) => {
  const from = await fresh.person();
  const to = await fresh.person();
  const follower = await fresh.person({ follows: [from] });
  await signIn(from);
  const data = new AccountData(page);
  await data.openMove();

  await data.move(to.username, from.password);

  await expect(data.target).toHaveAccessibleDescription(/ does not list this account as an alias yet\. /);
  await expect(data.moved(to.username)).toHaveCount(0);

  // The other account names this one as an alias, in a browser of its own.
  const theirs = await browser.newContext();
  try {
    const other = await theirs.newPage();
    const signInThere = new SignIn(other);
    await signInThere.open();
    await signInThere.withPassword(to.email, to.password);
    await expect(new Frame(other).heading("Home")).toBeVisible();
    const there = new AccountData(other);
    await there.openAliases();
    await there.addAlias(from.username);
  } finally {
    await theirs.close();
  }

  await data.move(to.username, from.password);

  await expect(data.moved(to.username)).toBeVisible();
  // The server moves the followers in a job of its own: the other account's followers, read again until it is done.
  const followers = new Follows(page, to);
  await expect(async () => {
    await page.goto(`/@${to.username}/followers`);
    await expect(followers.row(follower)).toBeVisible();
  }).toPass();

  await data.openMove();
  await data.cancelMove();
  await expect(data.section("The move is cancelled")).toBeVisible();
});

test("a person deactivates their account with their password, and the app leaves for the start, signed out", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const data = new AccountData(page);
  await data.openDeactivate();

  await data.deactivate(reader.password);

  await expect(new Landing(page).signIn).toBeVisible();
});
