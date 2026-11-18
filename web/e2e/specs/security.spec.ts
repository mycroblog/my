// SPDX-License-Identifier: AGPL-3.0-only

// Security, the settings' entry of how an account signs in, on the real server (#3205): two-factor authentication
// turned on there asks for a code at the next sign-in; a passkey added there signs the reader in without a password;
// a browser signed out from another one is refused on its next request; and an account linked there at the login
// provider signs the reader in through it. Each reader is a fresh person, who alone signs in a new way. What each
// screen shows and asks is a component test on the fake (app-features, settings/security/*.dom.test.tsx).

import type { Request } from "@playwright/test";
import { loginProvider } from "../src/scenario";
import { totp, totpStep } from "../src/seed/totp";
import {
  Frame,
  Landing,
  LoginProvider,
  ownProfileHeading,
  Security,
  Settings,
  SignIn,
  virtualAuthenticator,
} from "./screens";
import { expect, test, totpCode } from "./test";

/** Whether `request` is one of the client API that carries the session's access token. */
const withToken = (request: Request) =>
  new URL(request.url()).pathname.startsWith("/api/v1/") && request.headers()["authorization"] !== undefined;

/** Safari on a Mac, as it names itself: the second browser of a reader, which the list of browsers tells from the first. */
const safariOnMac =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15";

/** Signs out from the settings' menu, and waits for the landing page of a reader without a session. */
async function signOut(settings: Settings): Promise<void> {
  await settings.goTo();
  await settings.signOut();
  await expect(new Landing(settings.page).signIn).toBeVisible();
}

test("a reader turns on two-factor authentication in the settings, and the next sign-in asks for a code of their app after the password", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const security = new Security(page);
  await security.goTo();

  const setup = await security.setUpTwoFactor();
  await expect(setup.qrCode).toBeVisible();
  await expect(setup.key).toHaveText(setup.secret);
  // The code of one step, which the server then takes no second time.
  const step = totpStep();
  await setup.turnOn(totp(setup.secret, step * 30_000));

  await expect(setup.recoveryCodes).toHaveCount(10);
  await setup.finish();
  await expect(security.heading).toBeVisible();
  await expect(security.twoFactorOn).toBeVisible();

  await signOut(new Settings(page));
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(reader.email, reader.password);
  await expect(screen.twoFactorHeading).toBeVisible();
  await screen.enterCode(totpCode(setup.secret, { avoidStep: step }));
  await expect(new Frame(page).heading("Home")).toBeVisible();
});

test("a reader adds a passkey in the settings, and signs in with it, without a password", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  await virtualAuthenticator(page);
  const security = new Security(page);
  await security.goTo();
  await expect(security.passkeys).toHaveAccessibleName("Passkeys");

  await security.addPasskey("E2E laptop", reader.password);

  await expect(security.heading).toBeVisible();
  await expect(security.removePasskey("E2E laptop")).toBeVisible();
  await expect(security.passkeys).toHaveAccessibleName("Passkeys 1");

  await signOut(new Settings(page));
  // The sign-in screen asks the browser for a passkey as it opens, and the authenticator answers.
  await new SignIn(page).open();
  const frame = new Frame(page);
  await expect(frame.heading("Home")).toBeVisible();
  await frame.goTo("Profile");
  await expect(ownProfileHeading(page, reader.username)).toBeVisible();
});

test("a reader signs out their other browser from the settings, and that browser's session is refused from then on", async ({
  page,
  browser,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const other = await browser.newContext({ userAgent: safariOnMac });
  try {
    const second = await other.newPage();
    const asked = second.waitForRequest(withToken);
    const screen = new SignIn(second);
    await screen.open();
    await screen.withPassword(reader.email, reader.password);
    await expect(new Frame(second).heading("Home")).toBeVisible();
    const token = (await asked).headers()["authorization"] ?? "";
    const taken = await second.request.get("/api/v1/preferences", { headers: { Authorization: token } });
    expect(taken.status()).toBe(200);

    const security = new Security(page);
    await security.goTo();
    // The row of the other browser, by the name its User-Agent gives it; the reader's other sessions stay.
    await expect(security.signOutOf("Safari on macOS")).toBeVisible();
    await security.signOutBrowser("Safari on macOS");
    await expect(security.signOutOf("Safari on macOS")).toHaveCount(0);

    // The other browser's next request is refused, and a reload finds no session.
    const refused = await second.request.get("/api/v1/preferences", { headers: { Authorization: token } });
    expect(refused.status()).toBe(401);
    await second.reload();
    await expect(new Landing(second).signIn).toBeVisible();
  } finally {
    await other.close();
  }
});

test.describe("at the login provider", () => {
  // The provider's certificate is signed by the development CA, which the browser does not trust: only the test that
  // opens the provider's page passes over the certificate.
  test.use({ ignoreHTTPSErrors: true });

  test("a reader links an account at the login provider from the settings, confirming with their password, and then signs in through it", async ({
    page,
    signIn,
    fresh,
  }) => {
    const reader = await fresh.person();
    const who = fresh.providerIdentity();
    await signIn(reader);
    const security = new Security(page);
    await security.goTo();

    await security.link(loginProvider.name, reader.password);
    const provider = new LoginProvider(page);
    await expect(provider.heading).toBeVisible();
    await provider.signInAs(who);

    // Back on Security, which says what happened and takes it off the address.
    await expect(security.linked(loginProvider.name)).toBeVisible();
    await expect(page).toHaveURL("/settings/security");
    await expect(security.unlink(loginProvider.name)).toBeVisible();

    await signOut(new Settings(page));
    await new SignIn(page).open();
    await provider.start();
    await expect(provider.heading).toBeVisible();
    await provider.signInAs(who, { subjectOnly: true });
    const frame = new Frame(page);
    await expect(frame.heading("Home")).toBeVisible();
    await frame.goTo("Profile");
    await expect(ownProfileHeading(page, reader.username)).toBeVisible();
  });
});
