// SPDX-License-Identifier: AGPL-3.0-only

// A person signs in through the server's sign-in screens, each way the scenario's people sign in, and the client
// opens their home timeline, with the posts of the people they follow.

import { Frame, homeTimeline, SignIn } from "./screens";
import { expect, person, secrets, test, totpCode } from "./test";

test("a person signs in with a password and lands on the home timeline, with a post of someone they follow", async ({
  page,
}) => {
  const yuki = person("yuki");
  const screen = new SignIn(page);
  await screen.open();
  await expect(screen.heading).toBeVisible();
  await screen.withPassword(yuki.email, yuki.password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
  await expect(homeTimeline(page).feed).toBeVisible();
  await expect(homeTimeline(page).post(person("layla").displayName, "صباح الخير").article).toBeVisible();
});

test("a wrong password keeps the person on the sign-in screen and says why", async ({ page }) => {
  const sam = person("sam");
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(sam.email, "not-the-password");
  await expect(screen.error).toHaveText("The email address or password is incorrect.");
  await expect(screen.heading).toBeVisible();
});

// A fresh person: a code the server took cannot sign in again, so a second run as the same person would be refused.
test("a person with two-factor authentication signs in with the code of their app", async ({ page, fresh }) => {
  const p = await fresh.person({ totp: true });
  if (p.totp === undefined) throw new Error("the fresh person has no two-factor authentication");
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(p.email, p.password);
  await expect(screen.twoFactorHeading).toBeVisible();
  await screen.enterCode(totpCode(p.totp.secret, { avoidStep: p.totp.step }));
  await expect(new Frame(page).heading("Home")).toBeVisible();
});

test("a person with a passkey signs in without a password", async ({ page }) => {
  const mio = person("mio");
  const screen = new SignIn(page);
  const passkey = secrets().passkeys["mio"];
  if (passkey === undefined) throw new Error("the seed made no passkey for mio");
  await screen.addPasskey(passkey);
  await screen.open();
  await expect(homeTimeline(page).post(mio.displayName, "パスワードなしでログインできる").article).toBeVisible();
});
