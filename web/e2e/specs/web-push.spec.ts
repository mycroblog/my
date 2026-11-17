// SPDX-License-Identifier: AGPL-3.0-only

// Web Push (FR-206, ADR-0015 §12): a fresh person turns it on from the prompt over their notifications; another person
// mentions them through the API; the fake push service of the demo server verifies the server's VAPID token and
// decrypts the message, which, handed to the app's real Service Worker over CDP, shows the mention's notification, whose
// click opens its own page, /notifications/{id}, which goes on to the mention. The browser's side is src/web-push.ts.
// Notifications show in Chromium's new headless mode alone.

import { deliverPush, installPushShim, receivedPushes, shownNotifications } from "../src/web-push";
import { Frame, Notifications } from "./screens";
import { expect, test } from "./test";

test.use({ channel: "chromium" });

test("a person turns Web Push on from the prompt, and a mention arrives as a notification that opens it", async ({
  page,
  context,
  baseURL,
  signIn,
  fresh,
}) => {
  if (baseURL === undefined) throw new Error("e2e: no base URL");
  const origin = new URL(baseURL).origin;
  const shim = await installPushShim(context, origin);
  const reader = await fresh.person();
  const writer = await fresh.person();

  await signIn(reader);
  await new Frame(page).goTo("Notifications");
  const notifications = new Notifications(page);
  await notifications.turnOnPush();
  await expect(notifications.pushPrompt).toBeHidden();
  const endpoint = shim.subscription()?.endpoint;
  if (endpoint === undefined) throw new Error("the app did not subscribe through the shim");

  const mention = await fresh.post(writer, { text: `@${reader.username} are you coming on Sunday?` });
  let pushed: Awaited<ReturnType<typeof receivedPushes>> = [];
  await expect
    .poll(async () => {
      pushed = await receivedPushes(endpoint);
      return pushed.some((p) => p.payload.type === "mention");
    })
    .toBe(true);
  const message = pushed.find((p) => p.payload.type === "mention");
  if (message === undefined) throw new Error("no mention was pushed");

  await deliverPush(page, origin, message.plaintext);
  // A click opens the notification's own page, which goes on to the mention.
  const opens = `${origin}/notifications/${message.payload.notification_id}`;
  await expect.poll(() => shownNotifications(page)).toContainEqual({ title: message.payload.title, url: opens });
  await page.goto(opens);
  await expect(page).toHaveURL(`${origin}/@${writer.username}/status/${mention.id}`);
});

test("a person who chose Not now is not offered Web Push again in this browser, after a reload", async ({
  page,
  context,
  baseURL,
  signIn,
  fresh,
}) => {
  if (baseURL === undefined) throw new Error("e2e: no base URL");
  const shim = await installPushShim(context, new URL(baseURL).origin);
  const reader = await fresh.person();
  const writer = await fresh.person();
  const mention = await fresh.post(writer, { text: `@${reader.username} the bakery opens at eight` });

  await signIn(reader);
  await new Frame(page).goTo("Notifications");
  const notifications = new Notifications(page);
  await notifications.closePush();
  await expect(notifications.pushPrompt).toBeHidden();

  // The prompt shows with the notifications when it shows at all: once the mention shows, the screen has decided.
  await page.reload();
  await expect(notifications.mention(writer.displayName, mention.text)).toBeVisible();
  await expect(notifications.pushPrompt).toBeHidden();
  expect(shim.subscription()).toBeUndefined();
});
