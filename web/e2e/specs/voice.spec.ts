// SPDX-License-Identifier: AGPL-3.0-only

// Two people in one voice chat room on the real server, over P2P (ADR-0101 §3, FR-924): the host, in one browser,
// speaks into Chromium's fake microphone, and the listener, in another, receives the sound. The listener moves to other
// screens of the app and the sound keeps arriving, since the call outlives every screen; leaving from the call's bar
// ends it, its connections closed.

import { connectedPeers, Frame, keepPeerConnections, openPeers, receivedAudio, SignIn, VoiceRoom } from "./screens";
import type { Page } from "@playwright/test";
import type { Person } from "../src/scenario";
import { expect, test } from "./test";

// Chromium's fake microphone (a tone), taken without asking.
test.use({
  launchOptions: { args: ["--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream"] },
  permissions: ["microphone"],
});

/** Signs in as `p` on `page` and waits for the home screen (a fresh person follows no one, so it has no feed). */
async function signInOn(page: Page, p: Person): Promise<void> {
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(p.email, p.password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
}

test("a listener hears the host of a P2P room, on every screen they move to, until they leave", async ({
  page,
  browser,
  baseURL,
  fresh,
}) => {
  const host = await fresh.person({ admin: true });
  const listener = await fresh.person();
  const { id, name } = await fresh.voiceRoom(host, { name: "Sound check" });

  // The host takes the room's place of their API session in the browser, and turns the microphone on.
  await signInOn(page, host);
  const hosting = new VoiceRoom(page, id);
  await hosting.open();
  await hosting.join.click();
  await hosting.unmute.click();
  await expect(hosting.mute).toBeVisible();

  // The listener, in a browser of their own, from an address of its own.
  const other = await browser.newContext({
    ...(baseURL === undefined ? {} : { baseURL }),
    permissions: ["microphone"],
    extraHTTPHeaders: { "X-Forwarded-For": "198.19.250.1" },
  });
  const listening = await other.newPage();
  await listening.addInitScript(keepPeerConnections);
  await signInOn(listening, listener);
  const room = new VoiceRoom(listening, id);
  await room.open();
  await room.join.click();
  // The room's page holds the call's controls (and no call bar): the call is live once they can be pressed, and the
  // sound comes once a connection to the host is up; each wait is its own, so that setting up has the time of several.
  await expect(room.speak).toBeEnabled();
  await expect(room.connecting).toHaveCount(0);
  await expect.poll(() => connectedPeers(listening), { message: "a connection to the host" }).toBeGreaterThan(0);

  /** The audio that arrived is more than `bytes`, as the browser counts it, some time later. */
  const grows = async (bytes: number) => {
    let now = 0;
    await expect
      .poll(async () => (now = await receivedAudio(listening)), { message: `more audio than ${bytes} bytes` })
      .toBeGreaterThan(bytes);
    return now;
  };
  let heard = await grows(0);

  // The call goes on across the screens: the sound keeps arriving on each.
  const frame = new Frame(listening);
  await frame.goTo("Notifications");
  await expect(frame.heading("Notifications")).toBeVisible();
  await expect(room.callBar(name)).toBeVisible();
  heard = await grows(heard);
  await frame.goTo("Home");
  await expect(frame.heading("Home")).toBeVisible();
  heard = await grows(heard);

  // Leaving from the bar ends the call: the bar goes and its connections close.
  await room.leaveFromBar(name).click();
  await expect(room.callBar(name)).toHaveCount(0);
  await expect.poll(() => openPeers(listening), { message: "the call's connections closed" }).toBe(0);
  await other.close();
});
