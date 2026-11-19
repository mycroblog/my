// SPDX-License-Identifier: AGPL-3.0-only

// Accounts as moderators handle them, on the real server (#3260, FR-703, FR-707, FR-714, FR-721): a moderator finds a
// person through Moderation's Accounts, keeps them from posting for a week with a reason, and sees the action among
// the account's actions; the person appeals it from their settings; the moderator finds the appeal in the queue of
// appeals, reads what the person wrote, and overturns it, which lifts the action. A person whose registration waits
// for a moderator is let in once a moderator approves it from Registrations. What each screen shows in every state is
// the stories' on the fake (web/app, accounts.stories.tsx).

import { randomUUID } from "node:crypto";
import type { Browser, Page } from "@playwright/test";
import { AccountStatus, Frame, Moderation, Settings, SignIn, SignUp, StaffAccounts, VerifyEmail } from "./screens";
import { expect, mailedLink, test, type FreshPerson } from "./test";

/** Signs in as `p` on `page`, a browser of its own, and waits for the home screen. */
async function signInOn(page: Page, p: FreshPerson): Promise<void> {
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(p.email, p.password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
}

/** A browser of its own for `p`, from another address, signed in; closed by `close`. */
async function browserOf(browser: Browser, baseURL: string | undefined, p: FreshPerson, ip: string) {
  const context = await browser.newContext({
    ...(baseURL === undefined ? {} : { baseURL }),
    extraHTTPHeaders: { "X-Forwarded-For": ip },
  });
  const page = await context.newPage();
  await signInOn(page, p);
  return { page, close: () => context.close() };
}

test("a moderator keeps a person from posting from the person's account screen; the person appeals, and the moderator overturns the appeal, which lifts the action", async ({
  page,
  browser,
  baseURL,
  signIn,
  fresh,
}) => {
  const moderator = await fresh.person({ admin: true });
  const person = await fresh.person();
  const handle = `@${person.username}`;
  const reason = `Posted the same link in every thread, ${randomUUID().slice(0, 8)}.`;

  // The moderator finds the person through Moderation's Accounts and takes the action on their Review page.
  await signIn(moderator);
  const moderation = new Moderation(page);
  const staff = new StaffAccounts(page);
  await moderation.place.click();
  await staff.entry("Accounts").click();
  await staff.search(person.username);
  await staff.openAccount(handle);
  await staff.takeAction({ kind: "No posting", lasts: "7 days", reason });
  await expect(staff.said("No posting is in effect now.")).toBeVisible();
  await expect(staff.action("No posting")).toContainText("In effect");

  // The person, in a browser of their own, appeals it from their settings.
  const theirs = await browserOf(browser, baseURL, person, "198.19.252.1");
  const appeal = `I posted the link once, in one thread. ${randomUUID().slice(0, 8)}`;
  try {
    const settings = new Settings(theirs.page);
    const status = new AccountStatus(theirs.page);
    await new Frame(theirs.page).goTo("Settings");
    await status.open(settings.menu);
    await status.seeStatus();
    await status.openAction(reason);
    await status.sendAppeal(appeal);
  } finally {
    await theirs.close();
  }

  // The moderator finds the appeal in the queue, reads it, and overturns it.
  await page.goto("/moderation");
  await staff.entry("Appeals").click();
  await expect(staff.appeal(handle)).toBeVisible();
  await staff.openAppeal(handle);
  await expect(page.getByRole("main").getByText(appeal, { exact: true })).toBeVisible();
  await staff.decideAppeal("Overturn", "Sorry, we looked again: it was one post.");
  await expect(staff.said("The action is overturned and lifted.")).toBeVisible();

  // The action is lifted on the account's actions.
  await page.goto("/moderation/accounts");
  await staff.search(person.username);
  await staff.openAccount(handle);
  await staff.tab("Actions");
  await expect(staff.action("No posting")).toContainText("Lifted");
});

test("a person whose registration waits for a moderator is let in once a moderator approves it from Registrations", async ({
  page,
  browser,
  baseURL,
  fresh,
}) => {
  const moderator = await fresh.person({ admin: true });
  const who = fresh.newcomer({ approval: true });
  const signUp = new SignUp(page);
  await signUp.open();
  await signUp.as(who);
  await expect(signUp.waitsForApproval).toBeVisible();
  const verify = new VerifyEmail(page);
  await verify.open(await mailedLink(who.email, "/auth/verify_email"));
  await verify.confirm();
  await expect(verify.confirmed).toBeVisible();

  const theirs = await browserOf(browser, baseURL, moderator, "198.19.252.2");
  try {
    const staff = new StaffAccounts(theirs.page);
    await new Moderation(theirs.page).place.click();
    await staff.entry("Registrations").click();
    const handle = `@${who.username}`;
    await expect(staff.registration(handle)).toContainText(who.email);
    await staff.approve(handle);
    await expect(staff.said("1 registration approved.")).toBeVisible();
    await expect(staff.registration(handle)).toContainText("Approved");
  } finally {
    await theirs.close();
  }

  await new SignIn(page).open();
  await expect(new Frame(page).heading("Home")).toBeVisible();
});
