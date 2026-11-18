// SPDX-License-Identifier: AGPL-3.0-only

// The welcome step of a new account, on the real server (#3209): a person signs up from an administrator's invite,
// confirms their email address from the mail's link, follows the server's way on to the welcome step, gives a display
// name, and arrives at the home timeline; their profile then bears the name. What the step shows, sends and refuses,
// and that an account that is not new skips it, is a component test on the fake (web/app, welcome.dom.test.tsx).

import { Frame, SignUp, VerifyEmail, Welcome } from "./screens";
import { expect, mailedLink, test } from "./test";

test("a person signs up from an invite's link, gives a display name in the welcome step, arrives at the home timeline, and their profile bears the name", async ({
  page,
  fresh,
}) => {
  const who = fresh.newcomer();
  const invite = await fresh.invite();
  const signUp = new SignUp(page);
  await signUp.openInvite(invite.url);
  await expect(signUp.inviteCode).toHaveValue(invite.code);
  await signUp.as(who);
  const verify = new VerifyEmail(page);
  await verify.open(await mailedLink(who.email, "/auth/verify_email"));
  await verify.confirm();
  await expect(verify.confirmed).toBeVisible();

  const welcome = new Welcome(page);
  await welcome.arrive();
  const name = `Newcomer ${who.username.slice(-6)}`;
  await welcome.continueAs(name);

  const frame = new Frame(page);
  await expect(frame.heading("Home")).toBeVisible();
  await frame.goTo("Profile");
  await expect(frame.heading(new RegExp(`^${name} \\(@${who.username}@[^)]+\\)$`))).toBeVisible();
});
