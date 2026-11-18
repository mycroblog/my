// SPDX-License-Identifier: AGPL-3.0-only

// A person signs in through the scenario's login provider, "Example SSO", from the server's sign-in screen: the fake
// OpenID Connect provider beside the demo server (src/server/demo.ts) signs in whoever its page names. A person it
// names for the first time creates their account on the server and reaches the app; the same person signs in again
// as that account; a person who denies the sign-in at the provider is told so on the sign-in screen, signed out.

import { Frame, LoginProvider, ownProfileHeading, SignIn } from "./screens";
import { expect, test } from "./test";

// The provider's certificate is signed by the development CA (scripts/mailpit-cert.sh), which the browser does not
// trust: only the specs of this file, which open the provider's page, pass over the certificate.
test.use({ ignoreHTTPSErrors: true });

test("a person signs up through the external provider and reaches the app, and signs in through it again as the same account", async ({
  page,
  browser,
  fresh,
}) => {
  const who = fresh.providerIdentity();
  await new SignIn(page).open();
  const provider = new LoginProvider(page);
  await provider.start();
  await expect(provider.heading).toBeVisible();
  await provider.signInAs(who);

  // The server knows nobody of that subject: it proposes an account from what the provider said.
  await expect(provider.createHeading).toBeVisible();
  await expect(provider.username).toHaveValue(who.username);
  await expect(provider.email).toHaveValue(who.email);
  await expect(provider.emailConfirmedByProvider).toBeVisible();
  await provider.createAccount();
  await expect(provider.signedIn).toBeVisible();
  await provider.toApp();
  const frame = new Frame(page);
  await expect(frame.heading("Home")).toBeVisible();
  await frame.goTo("Profile");
  await expect(ownProfileHeading(page, who.username)).toBeVisible();

  // In another browser, the provider names them by their subject alone, and the server signs that account in.
  const again = await browser.newPage();
  try {
    await new SignIn(again).open();
    const providerAgain = new LoginProvider(again);
    await providerAgain.start();
    await expect(providerAgain.heading).toBeVisible();
    await providerAgain.signInAs(who, { subjectOnly: true });
    const frameAgain = new Frame(again);
    await expect(frameAgain.heading("Home")).toBeVisible();
    await frameAgain.goTo("Profile");
    await expect(ownProfileHeading(again, who.username)).toBeVisible();
  } finally {
    await again.close();
  }
});

test("a sign-in the person denies at the external provider says so on the sign-in screen and signs nobody in", async ({
  page,
}) => {
  const signIn = new SignIn(page);
  await signIn.open();
  const provider = new LoginProvider(page);
  await provider.start();
  await expect(provider.heading).toBeVisible();
  await provider.deny();
  await expect(signIn.error).toHaveText(
    "The sign-in with the other service did not complete. Try again, or sign in another way.",
  );
  await expect(signIn.heading).toBeVisible();

  // Nobody is signed in: the app still sends them to the sign-in screen.
  await signIn.open();
  await expect(signIn.heading).toBeVisible();
});
