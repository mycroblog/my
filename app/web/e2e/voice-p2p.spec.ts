// SPDX-License-Identifier: AGPL-3.0-only
// A voice chat call between two real browsers (ADR-0100, ADR-0101 §4, §5): against the server of e2e/server.ts with
// `voice = p2p` (`node e2e/server.ts voice`, no STUN or TURN: both browsers are on this machine, so the host
// candidates connect), in two browser contexts of Chromium with its fake microphone (the voice-p2p-chromium project of
// playwright.config.ts). The host starts a room, the guest joins as a listener, and the host turns the microphone
// on: both RTCPeerConnections connect and the guest receives the host's audio. The guest raises a hand, the host lets
// the guest speak, and the host receives the guest's audio. The host leaves and ends the room for everyone.
//
// The app shows the call's state ("Connected", a microphone that is off) but not whether audio arrives, so the spec records the
// RTCPeerConnections each page makes (an init script that wraps the constructor, nothing in the app) and reads the
// packets of their inbound audio from getStats().
import { randomBytes } from "node:crypto";
import type { BrowserContext, Page } from "@playwright/test";
import { expect, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { ownsServer, runMy } from "./server-cli.ts";
import { voiceServerURL } from "./server-origin.ts";

declare global {
  interface Window {
    /** The RTCPeerConnections the page made, recorded by `recordPeerConnections`. */
    e2ePeerConnections?: RTCPeerConnection[];
  }
}

test.use({ baseURL: voiceServerURL });

/** The client message `key` (the project is in English), with its `{name}` arguments replaced. */
function message(key: string, values: Record<string, string> = {}): string {
  const text = readCatalog("en")[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key}`);
  return text.replaceAll(/\{(\w+(?:, number)?)\}/g, (_, name: string) => values[name] ?? `{${name}}`);
}

type User = { username: string; email: string; password: string };

/** A new local user on the voice chat server, made with `my admin create`. */
function createUser(role: string): User {
  const username = `e2e_voice_${role}_${randomBytes(4).toString("hex")}`;
  const user = { username, email: `${username}@example.com`, password: `e2e-password-${username}` };
  runMy(["admin", "create", "-username", user.username, "-email", user.email], `${user.password}\n`, "voice");
  return user;
}

/** Records every RTCPeerConnection the context's pages make in `window.e2ePeerConnections`. */
async function recordPeerConnections(context: BrowserContext): Promise<void> {
  await context.addInitScript(() => {
    const Native = window.RTCPeerConnection;
    const made: RTCPeerConnection[] = [];
    window.e2ePeerConnections = made;
    window.RTCPeerConnection = class extends Native {
      constructor(configuration?: RTCConfiguration) {
        super(configuration);
        made.push(this);
      }
    };
  });
}

/** For each connected RTCPeerConnection of the page, the audio packets it has received. */
function receivedAudio(page: Page): Promise<number[]> {
  return page.evaluate(async () => {
    const counts: number[] = [];
    for (const pc of window.e2ePeerConnections ?? []) {
      if (pc.connectionState !== "connected") continue;
      let packets = 0;
      for (const report of (await pc.getStats()).values() as IterableIterator<{
        type: string;
        kind?: string;
        packetsReceived?: number;
      }>) {
        if (report.type === "inbound-rtp" && report.kind === "audio") packets += report.packetsReceived ?? 0;
      }
      counts.push(packets);
    }
    return counts;
  });
}

/** How many of the page's RTCPeerConnections are not closed. */
function openConnections(page: Page): Promise<number> {
  return page.evaluate(() => (window.e2ePeerConnections ?? []).filter((pc) => pc.connectionState !== "closed").length);
}

/**
 * Waits until a connected RTCPeerConnection of the page receives audio, and that it keeps arriving. A new speaker's
 * pair connects again (a new connection, ICE and DTLS on localhost), which takes a few seconds.
 */
async function expectReceivesAudio(page: Page): Promise<void> {
  let first = 0;
  await expect
    .poll(
      async () => {
        first = Math.max(0, ...(await receivedAudio(page)));
        return first;
      },
      { timeout: 20_000 },
    )
    .toBeGreaterThan(0);
  await expect.poll(async () => Math.max(0, ...(await receivedAudio(page)))).toBeGreaterThan(first);
}

/** Signs in to the app through the server's sign-in screen (OAuth 2.0 with PKCE) and waits for the home page. */
async function signIn(page: Page, user: User): Promise<void> {
  await page.goto("/login");
  await page.getByRole("main").getByRole("button").click();
  await page.locator('input[type="email"][autocomplete="username"]').fill(user.email);
  await page.locator('input[type="password"][autocomplete="current-password"]').fill(user.password);
  await page.locator('main button[type="submit"]').first().click();
  await expect(page).toHaveURL(`${voiceServerURL}/home`);
  await expect(page.getByRole("main").getByRole("heading", { level: 1, name: message("home.title") })).toBeVisible();
}

const dock = (page: Page) => page.getByRole("toolbar", { name: message("voice.dock.label") });
/** The speakers (`count` of them) or the listeners on the room's stage. */
const speakers = (page: Page, count: number) =>
  page.getByRole("list", { name: message("voice.stage.speakers", { "count, number": String(count) }) });
const listeners = (page: Page, count: number) =>
  page.getByRole("list", { name: message("voice.stage.listeners", { "count, number": String(count) }) });
/** The call's state in the room's head ("Connected"). */
const callState = (page: Page) => page.getByRole("main").getByText(message("voice.call.live"), { exact: true });

/** Turns the microphone of a speaker on and waits until the stage no longer shows it off. */
async function turnMicrophoneOn(page: Page, name: string, speakerCount: number): Promise<void> {
  const microphone = dock(page).getByRole("button", { name: message("voice.call.microphone") });
  await expect(microphone).toBeEnabled();
  if ((await microphone.getAttribute("aria-pressed")) !== "true") await microphone.click();
  await expect(microphone).toHaveAttribute("aria-pressed", "true");
  await expect(speakers(page, speakerCount).getByRole("listitem").filter({ hasText: name })).not.toContainText(
    message("voice.participant.mic_off"),
  );
}

test.describe("a voice chat call between two browsers", () => {
  test.skip(
    !ownsServer,
    "the spec creates users with bin/my, which needs the database of the server e2e/server.ts starts",
  );

  test("the host speaks to a listener, lets the listener speak, and ends the room", async ({
    browser,
    page: hostPage,
  }) => {
    test.setTimeout(90_000);
    const host = createUser("host");
    const guest = createUser("guest");
    const guestContext = await browser.newContext({ permissions: ["microphone"] });
    await recordPeerConnections(hostPage.context());
    await recordPeerConnections(guestContext);
    const guestPage = await guestContext.newPage();
    // The call's warnings (signals that could not be sent or taken, a refused microphone) go to the test's output.
    for (const [who, page] of [
      ["host", hostPage],
      ["guest", guestPage],
    ] as const) {
      page.on("console", (entry) => {
        if (entry.text().startsWith("voice:") || entry.text().includes("Permissions policy"))
          console.log(`[${who}] ${entry.type()}: ${entry.text()}`);
      });
    }

    try {
      const name = `E2E call ${randomBytes(3).toString("hex")}`;
      await test.step("the host starts a room", async () => {
        await signIn(hostPage, host);
        await hostPage.goto("/voice");
        await hostPage.getByRole("button", { name: message("voice.start") }).click();
        const dialog = hostPage.getByRole("dialog", { name: message("voice.form.new_title") });
        await dialog.getByRole("textbox", { name: message("voice.form.name") }).fill(name);
        await dialog.getByRole("button", { name: message("voice.form.submit_new") }).click();
        await expect(hostPage.getByRole("heading", { level: 1, name })).toBeVisible();
        await expect(hostPage).toHaveURL(/\/voice\/[^/]+$/);
        // Starting the room joins the host: the call starts without a join button.
        await expect(callState(hostPage)).toBeVisible();
        await expect(hostPage.getByRole("button", { name: message("voice.call.join_here") })).toHaveCount(0);
      });

      await test.step("the guest joins as a listener", async () => {
        await signIn(guestPage, guest);
        await guestPage.goto(hostPage.url());
        await expect(guestPage.getByRole("heading", { level: 1, name })).toBeVisible();
        await guestPage.getByRole("button", { name: message("voice.room.join") }).click();
        await expect(callState(guestPage)).toBeVisible();
        await expect(dock(guestPage).getByRole("button", { name: message("voice.call.raise_hand") })).toBeVisible();
        for (const page of [guestPage, hostPage]) {
          await expect(speakers(page, 1).getByRole("listitem")).toHaveCount(1);
          await expect(listeners(page, 1).getByRole("listitem")).toHaveCount(1);
        }
      });

      await test.step("the host speaks and the guest hears it", async () => {
        await turnMicrophoneOn(hostPage, host.username, 1);
        await expect(speakers(guestPage, 1).getByRole("listitem").filter({ hasText: host.username })).not.toContainText(
          message("voice.participant.mic_off"),
        );
        await expectReceivesAudio(guestPage);
      });

      await test.step("the guest raises a hand, the host lets the guest speak, and the host hears the guest", async () => {
        await dock(guestPage)
          .getByRole("button", { name: message("voice.call.raise_hand") })
          .click();
        const guestAvatar = listeners(hostPage, 1).getByRole("listitem").filter({ hasText: guest.username });
        await expect(guestAvatar).toContainText(message("voice.participant.hand_raised"));
        // The host opens the guest's menu from the avatar.
        await guestAvatar.getByRole("button").click();
        await hostPage.getByRole("menuitem", { name: message("voice.moderate.grant") }).click();
        await expect(guestAvatar).not.toContainText(message("voice.participant.hand_raised"));
        // The grant lets the guest speak; the guest then becomes a speaker.
        await dock(guestPage)
          .getByRole("button", { name: message("voice.call.speak") })
          .click();
        await expect(speakers(hostPage, 2).getByRole("listitem").filter({ hasText: guest.username })).toContainText(
          message("voice.role.speaker"),
        );
        await turnMicrophoneOn(guestPage, guest.username, 2);
        await expectReceivesAudio(hostPage);
        await expectReceivesAudio(guestPage);
      });

      await test.step("the host leaves and ends the room", async () => {
        await dock(hostPage)
          .getByRole("button", { name: message("voice.call.leave") })
          .click();
        const dialog = hostPage.getByRole("alertdialog", { name: message("voice.call.host_leave.title") });
        // The radio's input is visually hidden too: the user presses its label.
        await dialog.getByText(message("voice.call.host_leave.end"), { exact: true }).click();
        await expect(dialog.getByRole("radio", { name: message("voice.call.host_leave.end") })).toBeChecked();
        await dialog.getByRole("button", { name: message("voice.call.leave") }).click();
        // The guest's call ends with the reason, and neither page is in a call any more.
        const ended = message("voice.call.ended", { reason: message("voice.end_reason.host") });
        await expect(guestPage.getByRole("alert").filter({ hasText: ended })).toBeVisible();
        await expect(dock(hostPage)).toHaveCount(0);
        await expect(dock(guestPage)).toHaveCount(0);
        await expect.poll(() => openConnections(hostPage)).toBe(0);
        await expect.poll(() => openConnections(guestPage)).toBe(0);
        // Both pages, still open on the room, say that it ended, and neither offers to join it.
        for (const page of [hostPage, guestPage]) {
          await expect(page.getByRole("heading", { level: 1, name: message("voice.room.ended") })).toBeVisible();
          await expect(page.getByRole("button", { name: message("voice.room.join") })).toHaveCount(0);
          await expect(page.getByRole("button", { name: message("voice.call.join_here") })).toHaveCount(0);
        }
      });
    } finally {
      await guestContext.close();
    }
  });
});
