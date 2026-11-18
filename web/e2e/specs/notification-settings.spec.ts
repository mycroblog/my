// SPDX-License-Identifier: AGPL-3.0-only

// The notification settings on the real server (#3201): what a reader changes at /settings/notifications is the
// account's, so that a reload finds it. One test per kind of write: a cell of the tables (the email of a kind where
// the server sends email, the app otherwise), a choice of the policy for notifications from people the reader may not
// know, and Web Push on this device, turned on and off through the fake push service of the demo server
// (src/web-push.ts). Each reader is a fresh person. What each control sends and says is a component test on the fake
// (app-features, settings/notifications-settings*.dom.test.tsx).

import { installPushShim } from "../src/web-push";
import { NotificationSettings } from "./screens";
import { expect, test } from "./test";

test("a kind turned on and off in the settings is kept across reloads: its email on a server that sends email, the app otherwise", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const settings = new NotificationSettings(page);
  await settings.goTo();
  // The instance says whether the server sends email: the tables have the Email column only then.
  const instance = (await (await page.request.get("/api/v1/instance")).json()) as { sends_email?: boolean };
  const channel = instance.sends_email === true ? "Email" : "In the app";
  await expect(settings.column("From other people", "Email")).toHaveCount(channel === "Email" ? 1 : 0);
  const mentions = settings.cell("From other people", "Mentions", channel);
  // A new account's mentions arrive in the app and not by email.
  const starts = channel === "In the app";
  await expect(mentions).toBeChecked({ checked: starts });

  await settings.setCell("From other people", "Mentions", channel, !starts);
  await page.reload();
  await expect(mentions).toBeChecked({ checked: !starts });

  await settings.setCell("From other people", "Mentions", channel, starts);
  await page.reload();
  await expect(mentions).toBeChecked({ checked: starts });
  // The kinds beside it stayed as they were.
  await expect(settings.cell("From other people", "New followers", "In the app")).toBeChecked();
});

test("a choice of the policy for notifications from people the reader may not know is kept across a reload", async ({
  page,
  signIn,
  fresh,
}) => {
  const reader = await fresh.person();
  await signIn(reader);
  const settings = new NotificationSettings(page);
  await settings.goTo();
  const newAccounts = settings.policy("New accounts");
  await expect(newAccounts).toHaveValue(NotificationSettings.valueOf("Accept"));

  await settings.choose("New accounts", "Hold back");
  await page.reload();

  await expect(newAccounts).toHaveValue(NotificationSettings.valueOf("Hold back"));
  // The other conditions are their own.
  await expect(settings.policy("People you do not follow")).toHaveValue(NotificationSettings.valueOf("Accept"));
});

test.describe("Web Push on this device", () => {
  // Notifications show in Chromium's new headless mode alone (src/web-push.ts).
  test.use({ channel: "chromium" });

  test("turned on in the settings it registers this device's subscription, turned off it deletes it, and a reload finds each", async ({
    page,
    context,
    baseURL,
    signIn,
    fresh,
  }) => {
    if (baseURL === undefined) throw new Error("e2e: no base URL");
    const shim = await installPushShim(context, new URL(baseURL).origin);
    const reader = await fresh.person();
    await signIn(reader);
    const settings = new NotificationSettings(page);
    await settings.goTo();
    await expect(settings.push).toBeEnabled();
    await expect(settings.push).not.toBeChecked();
    await expect(settings.column("From other people", "Push")).toHaveCount(0);

    await settings.setPush(true);

    await expect(settings.push).toBeChecked();
    expect(shim.subscription()?.endpoint).toBeDefined();
    // The subscription is for every kind: each cell of Push is on.
    await expect(settings.cell("From other people", "All", "Push")).toBeChecked();
    await page.reload();
    await expect(settings.push).toBeChecked();
    await expect(settings.cell("From other people", "Mentions", "Push")).toBeChecked();

    await settings.setPush(false);

    await expect(settings.push).not.toBeChecked();
    await page.reload();
    await expect(settings.table("From other people")).toBeVisible();
    await expect(settings.push).toBeEnabled();
    await expect(settings.push).not.toBeChecked();
    await expect(settings.column("From other people", "Push")).toHaveCount(0);
  });
});
