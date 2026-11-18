// SPDX-License-Identifier: AGPL-3.0-only

// A person who lost their password asks the server for a link from the sign-in screen, opens the link of the mail
// (src/mail.ts), sets a new password on the server's reset screen, and signs in with it.

import { ForgotPassword, Frame, PasswordReset, SignIn } from "./screens";
import { expect, mailedLink, test } from "./test";

test("a person who lost their password sets a new one from the link of the mail and signs in with it", async ({
  page,
  fresh,
}) => {
  const p = await fresh.person();
  const password = `new-${p.password}`;
  const forgot = new ForgotPassword(page);
  await forgot.open();
  await expect(forgot.heading).toBeVisible();
  await forgot.ask(p.email);
  await expect(forgot.outcome).toHaveText(
    `If an account uses ${p.email}, a link to set a new password is on its way. The link works for 60 minutes. If no email arrives, check the spam folder and the address.`,
  );

  const screen = new PasswordReset(page);
  await screen.open(await mailedLink(p.email, "/auth/reset_password"));
  await expect(screen.heading).toBeVisible();
  await screen.setPassword(password);
  await expect(screen.changed).toBeVisible();

  await screen.toSignIn();
  const signIn = new SignIn(page);
  await expect(signIn.heading).toBeVisible();
  // The password they lost signs nobody in; the new one does.
  await signIn.withPassword(p.email, p.password);
  await expect(signIn.error).toHaveText("The email address or password is incorrect.");
  await signIn.withPassword(p.email, password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
});
