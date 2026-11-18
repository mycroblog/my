// SPDX-License-Identifier: AGPL-3.0-only

// Apps, the settings' entry of the apps that can use an account, on the real server (#3206, ADR-0090 §5, §17): a
// reader allows an app at the consent screen, the app trades its code for a token, and the settings list it among
// the apps the reader authorized; revoked there, it is gone at once and after a reload, and its token is refused. And
// a reader registers an app of their own, issues a token of it for their account, and the token answers at the API
// as them. Each reader is a fresh person, who alone authorizes and registers. What each screen shows and asks is a
// component test on the fake (app-features, settings/apps/*.dom.test.tsx).

import type { Page } from "@playwright/test";
import { apps as scenarioApps } from "../src/scenario";
import { Apps, Consent } from "./screens";
import { expect, secrets, test } from "./test";

const tidepool = scenarioApps.find((a) => a.key === "tidepool");
if (tidepool === undefined) throw new Error("the scenario has no app tidepool");

/** Asks the client API, as the bearer of `token`, whose account the token is of. */
const whoIs = (page: Page, token: string) =>
  page.request.get("/api/v1/accounts/verify_credentials", { headers: { Authorization: `Bearer ${token}` } });

test("a reader allows an app at the consent screen and finds it among the apps they authorized; revoked there, it is gone at once and after a reload, and its token is refused", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const consent = new Consent(page, tidepool);
  await consent.open(secrets().apps["tidepool"]?.clientId ?? "", "apps-state");
  await expect(consent.heading).toBeVisible();
  const code = (await consent.allow()).get("code") ?? "";
  // The app's side: its code becomes a token, which works for the reader's account.
  const token = await consent.exchange(code);
  const allowed = await whoIs(page, token);
  expect(allowed.status()).toBe(200);

  const apps = new Apps(page);
  await apps.goTo();
  const origin = `An app registered by @${tidepool.owner}`;
  await expect(apps.authorizedApp(tidepool.name, origin)).toBeVisible();
  const app = await apps.openAuthorized(tidepool.name, origin);
  await expect(app.about).toContainText([origin, "Authorized"]);

  await app.revoke();

  // Back on the entry, without the app, whose token the server now refuses.
  await expect(apps.heading).toBeVisible();
  await expect(apps.authorized).toBeVisible();
  await expect(apps.authorizedApp(tidepool.name, origin)).toHaveCount(0);
  const revoked = await whoIs(page, token);
  expect(revoked.status()).toBe(401);

  await page.reload();
  await expect(apps.heading).toBeVisible();
  await expect(apps.authorized).toBeVisible();
  await expect(apps.authorizedApp(tidepool.name, origin)).toHaveCount(0);
});

test("a reader registers an app of their own, which shows its client ID and its secret once, issues a token of it for their account, and the token answers at the API as them", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const apps = new Apps(page);
  await apps.goTo();

  const made = await apps.register({ name: "E2E bot", redirectUri: "https://bot.test/callback", scopes: "read" });
  await expect(apps.clientId).toHaveText(made.clientId);
  await expect(apps.secret).toHaveText(made.secret ?? "");
  await apps.finish();
  await expect(apps.ownApp("E2E bot")).toBeVisible();

  const app = await apps.openOwn("E2E bot");
  await expect(app.clientId).toHaveText(made.clientId);
  const issued = await app.issueToken();
  await expect(app.accessToken).toHaveText(issued.accessToken);
  await expect(app.refreshToken).toHaveText(issued.refreshToken);
  expect(issued.scope).toBe("read");
  const answer = await whoIs(page, issued.accessToken);
  expect(answer.status()).toBe(200);
  expect(((await answer.json()) as { username: string }).username).toBe(reader.username);

  // The token is not shown again: back on the app's screen, it is nowhere.
  await app.finish();
  await expect(app.accessToken).toHaveCount(0);
});
