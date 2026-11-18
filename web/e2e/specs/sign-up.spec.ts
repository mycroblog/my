// SPDX-License-Identifier: AGPL-3.0-only

// A person without an account signs up on the server's registration screen. The demo server approves registrations and
// counts on mail (src/mail.ts): a person who signs up waits on the screen that says so, the app sends them back to it,
// and it lets them in once a moderator approves and they confirm their email address; a person who signs up from an
// invite's link waits for no moderator, and is let in once they confirm theirs. The mail that asks them to confirm
// their email address leads to the screens that confirm it.

import { Frame, SignIn, SignUp, VerifyEmail } from "./screens";
import { expect, mailedLink, test } from "./test";

test("a person signs up on a server that approves registrations, and is let in once a moderator approves and they confirm their email address", async ({
  page,
  fresh,
}) => {
  const who = fresh.newcomer();
  const screen = new SignUp(page);
  await screen.open();
  await expect(screen.heading).toBeVisible();
  await screen.as(who, { reason: "I bake bread, and would like to write about it here." });
  await expect(screen.notReady).toBeVisible();
  await expect(screen.waitsForApproval).toBeVisible();

  // The app does not open for them while the registration waits: it sends them back to the same screen.
  const signIn = new SignIn(page);
  await signIn.open();
  await expect(screen.waitsForApproval).toBeVisible();

  await fresh.approve(who);
  const verify = new VerifyEmail(page);
  await verify.open(await mailedLink(who.email, "/auth/verify_email"));
  await verify.confirm();
  await expect(verify.confirmed).toBeVisible();
  await signIn.open();
  await expect(new Frame(page).heading("Home")).toBeVisible();
});

test("a person signs up from an invite's link and is let in once they confirm their email address, with no wait for a moderator", async ({
  page,
  fresh,
}) => {
  const who = fresh.newcomer();
  const invite = await fresh.invite();
  const screen = new SignUp(page);
  await screen.openInvite(invite.url);
  await expect(screen.inviteCode).toHaveValue(invite.code);
  await screen.as(who);
  const verify = new VerifyEmail(page);
  await verify.open(await mailedLink(who.email, "/auth/verify_email"));
  await verify.confirm();
  await expect(verify.confirmed).toBeVisible();

  // In the app, with no wait for a moderator.
  await new SignIn(page).open();
  await expect(new Frame(page).heading("Home")).toBeVisible();
});

test("a person who signed up confirms their email address from the link of the mail", async ({ page, fresh }) => {
  const who = fresh.newcomer();
  const signUp = new SignUp(page);
  await signUp.open();
  await signUp.as(who);
  await expect(signUp.notReady).toBeVisible();
  await expect.poll(() => fresh.emailConfirmed(who)).toBe(false);

  const screen = new VerifyEmail(page);
  await screen.open(await mailedLink(who.email, "/auth/verify_email"));
  await expect(screen.heading).toBeVisible();
  await screen.confirm();
  await expect(screen.confirmed).toBeVisible();
  await expect(screen.outcome).toHaveText("Thank you. Your email address is confirmed.");
  await expect.poll(() => fresh.emailConfirmed(who)).toBe(true);
});
