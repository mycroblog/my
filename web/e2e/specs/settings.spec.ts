// SPDX-License-Identifier: AGPL-3.0-only

// The settings on the real server (#3198, #3199): a reader signs out from them, which ends the session on the server
// (its token is refused from then on, a reload finds no session, and the reader's other tab leaves with them), and the
// display language a reader chose there is
// the account's, so that it is theirs again when they sign in after signing out. Each reader is a fresh person: the
// scenario's people keep their sessions and their language. What each control of the settings saves is a component
// test on the fake (app-features, settings/display.dom.test.tsx).

import type { Request } from "@playwright/test";
import { Frame, Landing, Settings, SignIn } from "./screens";
import { expect, test } from "./test";

/** Whether `request` is one of the client API that carries the session's access token. */
const withToken = (request: Request) =>
  new URL(request.url()).pathname.startsWith("/api/v1/") && request.headers()["authorization"] !== undefined;

test("a reader signs out from the settings, lands signed out at the server's address, and stays signed out; their other tab follows", async ({
  page,
  context,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  const asked = page.waitForRequest(withToken);
  await signIn(reader);
  const token = (await asked).headers()["authorization"] ?? "";
  // The server takes the token while the session lasts.
  const taken = await page.request.get("/api/v1/preferences", { headers: { Authorization: token } });
  expect(taken.status()).toBe(200);
  // The same session in a second tab of the browser, on a screen only a signed-in reader has.
  const other = await context.newPage();
  await other.goto("/notifications");
  await expect(new Frame(other).heading("Notifications")).toBeVisible();

  await new Frame(page).goTo("Settings");
  const settings = new Settings(page);
  await expect(settings.heading).toBeVisible();
  await settings.signOut();

  const landing = new Landing(page);
  await expect(landing.signIn).toBeVisible();
  await expect(page).toHaveURL("/");
  const refused = await page.request.get("/api/v1/preferences", { headers: { Authorization: token } });
  expect(refused.status()).toBe(401);
  // The other tab heard of it: it left for the server's address by itself, signed out too.
  await expect(new Landing(other).signIn).toBeVisible();
  await expect(other).toHaveURL("/");

  await page.reload();
  await expect(landing.signIn).toBeVisible();
  await expect(page).toHaveURL("/");
});

test("a reader's display language, chosen in the settings, is theirs again when they sign in after signing out", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const settings = new Settings(page);
  await settings.goTo();
  const display = await settings.openDisplay();
  await expect(display.language).toHaveValue("");

  await display.chooseLanguage("日本語");

  // The app reads in Japanese from now on, with no reload.
  const japanese = new Settings(page, "ja");
  await expect(new Frame(page).heading("表示と投稿")).toBeVisible();
  await japanese.goTo();
  await japanese.signOut();
  await expect(new Landing(page).signIn).toBeVisible();

  const signInScreen = new SignIn(page);
  await signInScreen.open();
  await signInScreen.withPassword(reader.email, reader.password);

  await expect(new Frame(page).heading("ホーム")).toBeVisible();
  await japanese.goTo();
  const kept = await japanese.openDisplay();
  await expect(kept.language).toHaveValue("ja");
});
