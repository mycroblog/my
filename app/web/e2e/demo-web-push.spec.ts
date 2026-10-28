// SPDX-License-Identifier: AGPL-3.0-only
// Web Push on the demo server (ADR-0118 §4, FR-206, FR-210): a person turns Web Push on in the notification settings,
// another person replies through the API, the fake push service of the demo server (`my dev push-service`) verifies
// the VAPID token and decrypts the message, and the message, handed to the real Service Worker over CDP, shows the
// reply's notification. A row turned off in the Push column sends nothing, and a subscription that the push service
// reports gone (410) is deleted by the server. The browser side is e2e/web-push.ts. It runs in the demo-chromium
// project of playwright.config.ts (E2E_DEMO=1), in Chromium's new headless mode, where notifications can be shown.
import type { BrowserContext, Page } from "@playwright/test";
import { apiAs, type DemoAPI, message, persona, signIn } from "./demo.ts";
import { expect, test } from "./fixtures";
import { demoServerURL } from "./server-origin.ts";
import {
  deliverPush,
  expireSubscription,
  type FakeMessage,
  fakeMessages,
  installPushShim,
  payloadsOf,
  type PushShim,
  shownNotifications,
} from "./web-push.ts";

// The worker must run (the config blocks Service Workers elsewhere), and the default headless shell refuses
// notifications whatever the permission says (e2e/web-push.ts).
test.use({ baseURL: demoServerURL, serviceWorkers: "allow", channel: "chromium" });

const sam = persona("sam");
const layla = persona("layla");

type Status = { id: string };

/** Opens the notification settings of `sam` in a new context with the push shim, and turns Web Push on. */
async function samWithPush(context: BrowserContext): Promise<{ page: Page; shim: PushShim; endpoint: string }> {
  const shim = await installPushShim(context, demoServerURL);
  const page = await context.newPage();
  await signIn(page, sam.email, sam.password);
  await page.goto("/settings/notifications");
  const toggle = page.getByRole("switch", { name: message("settings.push.enable") });
  const saved = page.waitForResponse(
    (r) => r.url().endsWith("/api/v1/push/subscription") && r.request().method() === "PUT" && r.status() === 200,
  );
  // React Aria hides the native input under its drawing: Space toggles it (WAI-ARIA APG, Switch pattern).
  await toggle.focus();
  await page.keyboard.press("Space");
  await saved;
  await expect(toggle).toBeChecked();
  const endpoint = shim.subscription()?.endpoint;
  if (endpoint === undefined) throw new Error("the app did not subscribe through the shim");
  return { page, shim, endpoint };
}

/** Polls the fake's messages to `endpoint` until `done` holds for them, and returns them. */
async function messagesUntil(endpoint: string, done: (messages: FakeMessage[]) => boolean): Promise<FakeMessage[]> {
  let messages: FakeMessage[] = [];
  await expect
    .poll(
      async () => {
        messages = await fakeMessages(endpoint);
        return done(messages);
      },
      { timeout: 60_000 },
    )
    .toBe(true);
  return messages;
}

/** A new post by `author` and a reply to it by `replier`, each with a text unique to this run. */
async function replyTo(author: DemoAPI, replier: DemoAPI, words: string): Promise<{ post: Status; reply: Status }> {
  const nonce = String(Date.now());
  const post = (await author.call("POST", "/statuses", { text: `${words} ${nonce}`, visibility: "public" })) as Status;
  const reply = (await replier.call("POST", "/statuses", {
    text: `@${sam.username} ${words}: a reply ${nonce}`,
    visibility: "public",
    in_reply_to_id: post.id,
  })) as Status;
  return { post, reply };
}

test("a reply reaches the Service Worker as a push notification; a row turned off sends nothing", async ({
  browser,
}) => {
  test.setTimeout(180_000);
  const context = await browser.newContext({ baseURL: demoServerURL });
  const { page, endpoint } = await samWithPush(context);

  // Favourites off in the Push column: the favourite below creates a notification in the app but sends nothing.
  const favourites = page.getByRole("checkbox", {
    name: message("settings.notifications.cell")
      .replace("{type}", message("settings.notifications.group.favourite"))
      .replace("{channel}", message("settings.notifications.channel.push")),
  });
  await expect(favourites).toBeChecked();
  const rowSaved = page.waitForResponse(
    (r) => r.url().endsWith("/api/v1/push/subscription") && r.request().method() === "PUT" && r.status() === 200,
  );
  await favourites.focus();
  await page.keyboard.press("Space");
  await rowSaved;
  await expect(favourites).not.toBeChecked();

  const [samAPI, laylaAPI] = await Promise.all([apiAs(sam), apiAs(layla)]);
  try {
    const post = (await samAPI.call("POST", "/statuses", {
      text: `Push test, favourite me ${String(Date.now())}`,
      visibility: "public",
    })) as Status;
    await laylaAPI.call("POST", `/statuses/${post.id}/favourite`);
    await replyTo(samAPI, laylaAPI, "Push test");

    const messages = await messagesUntil(endpoint, (m) => payloadsOf(m).some((p) => p.type === "mention"));
    // Every message to the subscription passed the fake's checks: the VAPID token of the server's key, and a body
    // that decrypts with the subscription's keys.
    for (const m of messages) {
      expect(m.status, m.error).toBe(201);
      expect(m.subject).toBeTruthy();
    }
    const payloads = payloadsOf(messages);
    expect(payloads.map((p) => p.type)).not.toContain("favourite");
    const reply = payloads.find((p) => p.type === "mention");
    if (reply === undefined) throw new Error("no reply was pushed");
    expect(reply.lang).toBe(sam.locale);
    expect(reply.title).toContain(layla.displayName);

    // The plaintext that the fake decrypted goes to the real Service Worker, whose `push` handler shows it.
    const plaintext = messages.find(
      (m) => m.plaintext !== undefined && payloadsOf([m])[0]?.type === "mention",
    )?.plaintext;
    await deliverPush(page, demoServerURL, plaintext ?? "");
    await expect
      .poll(() => shownNotifications(page), { timeout: 15_000 })
      .toContainEqual({ title: reply.title, tag: reply.notification_id });
  } finally {
    await Promise.all([samAPI.dispose(), laylaAPI.dispose()]);
    await context.close();
  }
});

test("the server deletes a subscription that the push service reports gone", async ({ browser }) => {
  test.setTimeout(180_000);
  const context = await browser.newContext({ baseURL: demoServerURL });
  const { page, endpoint } = await samWithPush(context);
  await expireSubscription(endpoint);

  const [samAPI, laylaAPI] = await Promise.all([apiAs(sam), apiAs(layla)]);
  try {
    await replyTo(samAPI, laylaAPI, "Gone test");
    await messagesUntil(endpoint, (m) => m.some((x) => x.status === 410));
  } finally {
    await Promise.all([samAPI.dispose(), laylaAPI.dispose()]);
  }

  // The grant has no subscription any more: the switch shows off after a reload.
  const toggle = page.getByRole("switch", { name: message("settings.push.enable") });
  await expect
    .poll(
      async () => {
        await page.reload();
        await expect(toggle).toBeVisible();
        return toggle.isChecked();
      },
      { timeout: 30_000 },
    )
    .toBe(false);
  await context.close();
});
