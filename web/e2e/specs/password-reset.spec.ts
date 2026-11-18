// SPDX-License-Identifier: AGPL-3.0-only

// A person who lost their password sets a new one on the server's reset screen and signs in with it. The link is the
// one an administrator issues for the person, which leads to the same screen as the one the server mails.

import { Frame, PasswordReset, SignIn } from "./screens";
import { expect, test } from "./test";

test("a person sets a new password from a reset link and signs in with it", async ({ page, fresh }) => {
  const p = await fresh.person();
  const password = `new-${p.password}`;
  const screen = new PasswordReset(page);
  await screen.open(await fresh.passwordResetLink(p));
  await expect(screen.heading).toBeVisible();
  await screen.setPassword(password);
  await expect(screen.changed).toBeVisible();

  await screen.toSignIn();
  const signIn = new SignIn(page);
  await expect(signIn.heading).toBeVisible();
  await signIn.withPassword(p.email, password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
});
