// SPDX-License-Identifier: AGPL-3.0-only
// The L4 tasks that turn Web Push on (ADR-0118 §4), named by the `push` key of a screen's front matter (screens.ts).
// The browser of the task subscribes with the demo server's fake push service through the shim of e2e/web-push.ts,
// which the init page installs (mcp.ts) and which keeps the subscription in a state file of the task. The harness, not
// the agent, judges the task: the browser subscribed, the server holds that subscription for the signed-in person,
// and, after another person of the scenario replies to them, the fake push service received and decrypted the
// reply's notification. As a postcondition of the harness only, a new browser session on the same subscription hands
// that message to the app's Service Worker over CDP and finds the notification in registration.getNotifications().
// The agent sees neither OS notifications nor the browser's permission prompt (notifications are granted
// beforehand), and opening a notification (`notificationclick`) is not exercised: the screen's front matter notes it.
import { existsSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { Catalog } from "../e2e/catalog.ts";
import { deliverPush, fakeMessages, payloadsOf, shownNotifications } from "../e2e/web-push.ts";
import type { SetupContext } from "./screens.ts";

const root = resolve(import.meta.dirname, "../../..");

/** The state file of the push shim (mcp.ts): the browser's subscription and its application server key. */
export type PushState = { subscription: { endpoint: string }; applicationServerKey: string };

/** The browser's subscription in the state file `file`, or undefined when the app did not subscribe (or unsubscribed). */
export function readPushState(file: string): PushState | undefined {
  if (!existsSync(file)) {
    return undefined;
  }
  return JSON.parse(readFileSync(file, "utf8")) as PushState;
}

/**
 * The code (for browser_run_code_unsafe) that the dry run runs in the agent's place: it opens the notification
 * settings at `origin` and turns the switch of Web Push on, as the shortest path of the task ends, and waits until the
 * app has saved the subscription. The app is in Japanese (run.ts signs in with it).
 */
export function turnPushOnCode(origin: string): string {
  const catalog = JSON.parse(readFileSync(join(root, "locales/ja/client/settings.json"), "utf8")) as Catalog;
  const label = catalog["settings.push.enable"]?.defaultMessage;
  if (label === undefined) {
    throw new Error("no message settings.push.enable in locales/ja");
  }
  return `async (page) => {
  await page.goto(${JSON.stringify(`${origin}/settings/notifications`)});
  const toggle = page.getByRole("switch", { name: ${JSON.stringify(label)} });
  const saved = page.waitForResponse((r) => r.url().endsWith("/api/v1/push/subscription") && r.request().method() === "PUT" && r.status() === 200);
  await toggle.focus();
  await page.keyboard.press("Space");
  await saved;
}`;
}

/**
 * The code (for browser_run_code_unsafe) of the postcondition: hands the plaintext `data` that the fake decrypted to
 * the Service Worker of `origin` (deliverPush of e2e/web-push.ts), once it is active, and fails unless the worker then shows the
 * notification of tag `tag` within 15 seconds.
 */
export function postconditionCode(origin: string, data: string, tag: string): string {
  return `async (page) => {
  const deliverPush = ${deliverPush.toString()};
  const shownNotifications = ${shownNotifications.toString()};
  // The new session's worker is still installing: a message to a registration without an active worker is lost.
  await page.evaluate(async () => { await navigator.serviceWorker.ready; });
  await deliverPush(page, ${JSON.stringify(origin)}, ${JSON.stringify(data)});
  for (let i = 0; i < 30; i++) {
    if ((await shownNotifications(page)).some((n) => n.tag === ${JSON.stringify(tag)})) return;
    await page.waitForTimeout(500);
  }
  const permission = await page.evaluate(() => Notification.permission);
  const shown = JSON.stringify(await shownNotifications(page));
  throw new Error("the Service Worker showed no notification of the pushed message (permission " + permission + ", shown " + shown + ")");
}`;
}

/** The harness's verdict on a push task, and the message the postcondition hands to the Service Worker. */
export type PushJudgement = {
  passed: boolean;
  detail: string;
  /** The plaintext that the fake decrypted and the notification's tag (its ID), when the task passed. */
  delivered?: { data: string; tag: string };
};

/**
 * Judges a push task after it ended. `state` is the shim's state file, `database` the demo server's SQLite
 * database (read only), `persona` the signed-in person, and `replier` the person who replies to them. A task the
 * person did not complete fails; a reply that the fake does not receive within a minute throws, since that is a fault
 * of the server or the harness rather than of the screen.
 */
export async function judgePush(options: {
  state: string;
  database: string;
  persona: string;
  replier: string;
  api: SetupContext["api"];
}): Promise<PushJudgement> {
  const { persona, replier, api } = options;
  const state = readPushState(options.state);
  if (state === undefined) {
    return { passed: false, detail: "the browser has no push subscription: the app did not subscribe" };
  }
  const { endpoint } = state.subscription;
  const account = (await api(persona, "GET", "/accounts/verify_credentials")) as { id: string };
  const db = new DatabaseSync(options.database, { readOnly: true });
  let row: { account_id: string } | undefined;
  try {
    row = db.prepare("SELECT account_id FROM push_subscriptions WHERE endpoint = ?").get(endpoint) as
      { account_id: string } | undefined;
  } finally {
    db.close();
  }
  if (row === undefined) {
    return { passed: false, detail: "the browser subscribed, but the server holds no subscription of it" };
  }
  if (row.account_id !== account.id) {
    return { passed: false, detail: `the server holds the browser's subscription for another account than ${persona}` };
  }

  const since = Date.now();
  const nonce = String(since);
  const post = (await api(persona, "POST", "/statuses", {
    text: `L4 push check ${nonce}`,
    visibility: "public",
  })) as { id: string };
  await api(replier, "POST", "/statuses", {
    text: `@${persona} L4 push check: a reply ${nonce}`,
    visibility: "public",
    in_reply_to_id: post.id,
  });
  const deadline = since + 60_000;
  let seen = "";
  while (Date.now() < deadline) {
    const messages = (await fakeMessages(endpoint)).filter((m) => Date.parse(m.received_at) >= since - 1000);
    seen = messages.map((m) => `${String(m.status)}${m.error === undefined ? "" : ` ${m.error}`}`).join(", ");
    for (const message of messages) {
      const payload = payloadsOf([message])[0];
      if (payload?.type === "mention" && message.plaintext !== undefined) {
        return {
          passed: true,
          detail: `the server holds the browser's subscription, and the fake push service received and decrypted the reply's notification "${payload.title}"`,
          delivered: { data: message.plaintext, tag: payload.notification_id },
        };
      }
    }
    await new Promise((done) => setTimeout(done, 1000));
  }
  throw new Error(
    `the fake push service received no notification of the reply to ${persona} within a minute (messages: ${seen || "none"})`,
  );
}
