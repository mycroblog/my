// SPDX-License-Identifier: AGPL-3.0-only

// A person without an account signs up on the server's registration screen. The demo server is open to everyone and
// counts on mail (src/mail.ts): a person who signs up waits on the screen that asks them to confirm their email
// address, the app sends them back to it, and it lets them in once they confirm the address from the link of the mail.
// The sign-ups from one domain wait for a moderator as on a server that approves registrations (the scenario's
// `approvalDomain`): such a person is let in only once a moderator approves, whether or not the address is confirmed.
// A person who signs up from an invite's link finds its code in the form.

import { Frame, SignIn, SignUp, VerifyEmail } from "./screens";
import { expect, mailedLink, test } from "./test";

test("a person signs up on a server open to everyone, confirms their email address from the mail's link, and is let in", async ({
  page,
  fresh,
}) => {
  const who = fresh.newcomer();
  const screen = new SignUp(page);
  await screen.open();
  await expect(screen.heading).toBeVisible();
  await screen.as(who);
  await expect(screen.notReady).toBeVisible();
  await expect(screen.asksToConfirm(who.email)).toBeVisible();
  await expect(screen.waitsForApproval).toBeHidden();
  await expect.poll(() => fresh.emailConfirmed(who)).toBe(false);

  // The app does not open for them before they confirm the address: it sends them back to the same screen.
  const signIn = new SignIn(page);
  await signIn.open();
  await expect(screen.asksToConfirm(who.email)).toBeVisible();

  const verify = new VerifyEmail(page);
  await verify.open(await mailedLink(who.email, "/auth/verify_email"));
  await expect(verify.heading).toBeVisible();
  await verify.confirm();
  await expect(verify.confirmed).toBeVisible();
  await expect(verify.outcome).toHaveText("Thank you. Your email address is confirmed.");
  await expect.poll(() => fresh.emailConfirmed(who)).toBe(true);

  await signIn.open();
  await expect(new Frame(page).heading("Home")).toBeVisible();
});

test("a person whose registration waits for a moderator is kept out, though their email address is confirmed, until a moderator approves it", async ({
  page,
  fresh,
}) => {
  const who = fresh.newcomer({ approval: true });
  const screen = new SignUp(page);
  await screen.open();
  await screen.as(who);
  await expect(screen.notReady).toBeVisible();
  await expect(screen.waitsForApproval).toBeVisible();

  // Confirming the address does not open the app while the registration waits: it sends them back to the same screen.
  const verify = new VerifyEmail(page);
  await verify.open(await mailedLink(who.email, "/auth/verify_email"));
  await verify.confirm();
  await expect(verify.confirmed).toBeVisible();
  const signIn = new SignIn(page);
  await signIn.open();
  await expect(screen.waitsForApproval).toBeVisible();
  await expect(screen.asksToConfirm(who.email)).toBeHidden();

  await fresh.approve(who);
  await signIn.open();
  await expect(new Frame(page).heading("Home")).toBeVisible();
});

test("a person signs up from an invite's link, whose code the form holds, and is let in once they confirm their email address", async ({
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

  await new SignIn(page).open();
  await expect(new Frame(page).heading("Home")).toBeVisible();
});
