// SPDX-License-Identifier: AGPL-3.0-only

// Invites, the settings' entry of an account's own invites, on the real server (#3207, ADR-0106 §14, §17, §18): an
// account that may create invites (on the demo server, an administrator: `invite_creators` is `permitted`) creates
// one in the settings and reads its link; a person without an account opens the link, signs up with the code the
// form holds, and is let in; the creator then reads the invite as used up, and the same link signs nobody else up.
// And an account that may not create invites finds no entry in the settings' menu, and reads why at its address
// (#3116). The demo server is open to everyone, so what an invite adds to a server that needs one is the Go side's
// to test. What each screen shows and asks is a component test on the fake (app-features,
// settings/invites/*.dom.test.tsx).

import { Frame, Invites, Settings, SignIn, SignUp, VerifyEmail } from "./screens";
import { expect, mailedLink, test } from "./test";

test("an administrator creates an invite in the settings, a person signs up with its link, and the invite then reads as used up and takes nobody else", async ({
  page,
  browser,
  signIn,
  fresh,
}) => {
  const creator = await fresh.person({ admin: true });
  await signIn(creator);
  const settings = new Settings(page);
  await settings.goTo();
  const invites = new Invites(page);
  await invites.menuRow.click();
  await expect(invites.heading).toBeVisible();

  const made = await invites.createOne();
  await expect(invites.created).toBeVisible();
  await expect(invites.link).toHaveText(made.url);
  await invites.finish();
  await expect(invites.row(made.code)).toHaveAccessibleName(
    new RegExp(`^${made.code} Usable until .+ Used 0 of 1 time$`),
  );

  // A person without an account, in a browser of their own, follows the link.
  const who = fresh.newcomer();
  const theirs = await browser.newContext();
  try {
    const visitor = await theirs.newPage();
    const signUp = new SignUp(visitor);
    await signUp.openInvite(made.url);
    await expect(signUp.heading).toBeVisible();
    await expect(signUp.inviteCode).toHaveValue(made.code);
    await signUp.as(who);
    const verify = new VerifyEmail(visitor);
    await verify.open(await mailedLink(who.email, "/auth/verify_email"));
    await verify.confirm();
    await expect(verify.confirmed).toBeVisible();
    await new SignIn(visitor).open();
    await expect(new Frame(visitor).heading("Home")).toBeVisible();
  } finally {
    await theirs.close();
  }

  // Back in the creator's settings, read again from the server.
  await invites.goTo();
  await expect(invites.row(made.code)).toHaveAccessibleName(`${made.code} Used up Used 1 of 1 time`);
  await invites.row(made.code).click();
  await expect(invites.title(made.code)).toBeVisible();
  await expect(invites.status).toHaveText("Used up Used 1 of 1 time");
  await expect(invites.link).toHaveCount(0);

  // The link takes nobody else.
  const late = fresh.newcomer();
  const others = await browser.newContext();
  try {
    const visitor = await others.newPage();
    const signUp = new SignUp(visitor);
    await signUp.openInvite(made.url);
    await expect(signUp.inviteCode).toHaveValue(made.code);
    await signUp.as(late);
    await expect(signUp.inviteNotValid).toBeVisible();
    await expect(signUp.inviteCode).toHaveAttribute("aria-invalid", "true");
    await expect(signUp.heading).toBeVisible();
    await expect(signUp.notReady).toBeHidden();
  } finally {
    await others.close();
  }
});

test("a person who may not create invites finds no Invites in the settings' menu, and reads why at its address", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const settings = new Settings(page);
  await settings.goTo();
  const invites = new Invites(page);
  // The menu is drawn whole once it knows what the reader may do: its last row is there, and Invites is not.
  await expect(settings.menu.getByRole("link", { name: "Sign out…", exact: true })).toBeVisible();
  await expect(invites.menuRow).toHaveCount(0);

  await invites.goTo();
  await expect(invites.cannotCreate).toContainText(
    "Only some accounts can create invites on this server. Ask its administrators to let you create them.",
  );
  await expect(invites.create).toHaveCount(0);
});
