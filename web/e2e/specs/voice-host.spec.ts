// SPDX-License-Identifier: AGPL-3.0-only

// The host's side of a voice chat room on the real server (ADR-0100 §3, §5, §6, §9; FR-923, FR-925, FR-926, FR-929):
// the host opens a room with its start post from the form, a second person joins it and raises a hand, the host lets
// them speak, makes them a co-host, hands the room over to them and leaves; the room goes on under its new host, and
// the start post, with its draft that holds the room's name, is on the host's profile.

import { Frame, Profile, SignIn, VoiceRoom, VoiceRoomOpen } from "./screens";
import type { Page } from "@playwright/test";
import type { Person } from "../src/scenario";
import { expect, test } from "./test";

// Chromium's fake microphone, taken without asking: the host joins the room they open as a speaker.
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

test("a host opens a room with its start post, lets a guest speak, makes them co-host, hands it over and leaves", async ({
  page,
  browser,
  baseURL,
  fresh,
}) => {
  // Both may open rooms (the server's default lets the roles with `start_voice_rooms`, the admin's among them), so the
  // guest may take the room over.
  const host = await fresh.person({ admin: true });
  const guest = await fresh.person({ admin: true });
  const name = `Host check ${guest.username.slice(-6)}`;

  await signInOn(page, host);
  const form = new VoiceRoomOpen(page);
  await form.open();
  await form.name.fill(name);
  await expect(form.startPost).toBeChecked();
  // The draft of the start post holds the room's name as it is written.
  await expect(form.postText).toHaveValue(new RegExp(name));
  await form.submit.click();
  // The room's page takes the form's place: its controls, then its address, which is not the form's.
  await expect(page.getByRole("toolbar", { name: "Room controls", exact: true })).toBeVisible();
  await expect(page).toHaveURL(/\/voice\/(?!new$)[^/]+$/);
  const id = new URL(page.url()).pathname.split("/").at(-1) ?? "";
  const hosting = new VoiceRoom(page, id);
  await expect(hosting.leave).toBeEnabled();

  // The guest, in a browser of their own, joins and raises a hand.
  const other = await browser.newContext({
    ...(baseURL === undefined ? {} : { baseURL }),
    permissions: ["microphone"],
    extraHTTPHeaders: { "X-Forwarded-For": "198.19.250.2" },
  });
  const guestPage = await other.newPage();
  await signInOn(guestPage, guest);
  const room = new VoiceRoom(guestPage, id);
  await room.open();
  await room.join.click();
  await room.raiseHand.click();

  // The host lets the raised hand speak: the hand goes down.
  const tile = hosting.tile(guest.displayName);
  await expect(tile).toHaveAccessibleName(/Hand raised/);
  await tile.click();
  await hosting.menuItem(guest.displayName, "Let them speak").click();
  await expect(tile).not.toHaveAccessibleName(/Hand raised/);
  // The guest may speak now: Speak in place of Raise hand.
  await expect(room.speak).toBeVisible();

  // The host makes them a co-host.
  await tile.click();
  await hosting.menuItem(guest.displayName, "Make co-host").click();
  await expect(tile).toHaveAccessibleName(/Co-host/);

  // The host hands the room over, asked first, and stays in it as a participant.
  await tile.click();
  await hosting.menuItem(guest.displayName, "Make host…").click();
  await hosting.dialogButton("Make host").click();
  await expect(hosting.participant(guest.displayName)).toHaveAccessibleName(/, Host/);
  await expect(hosting.participant(host.displayName)).not.toHaveAccessibleName(/, Host/);

  // No longer the host, the host leaves with no choice to make; the room goes on under the guest.
  await hosting.leave.click();
  await expect(hosting.join).toBeVisible();
  await expect(room.participant(guest.displayName)).toHaveAccessibleName(/, Host/);
  await expect(room.participant(host.displayName)).toHaveCount(0);

  // The start post went out with the room, on the host's profile.
  const profile = new Profile(page, host);
  await profile.open();
  // Its text as the draft wrote it, then the room's link on a line of its own.
  const started = new RegExp(`^I opened the voice chat room “${name}”\\.(?:\\s.*)?$`, "s");
  await expect(page.getByRole("main").getByText(started)).toBeVisible();
  await other.close();
});
