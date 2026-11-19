// SPDX-License-Identifier: AGPL-3.0-only

// A voice chat room's page for the listener, on the fake API with the calls on fakes: Join takes the focus to the
// room's controls and the page shows no call bar of its own, which the other screens show without a microphone for a
// listener; a room that ends while the reader is elsewhere is told by a toast once, and its page then says it ended,
// with no Join and no lists, the column beside the timeline listing it no more; the list says the voice chat is off
// while it is; and a visitor reads the summary of a room the server shows, with the sign-in that leads into it.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp } from "./test/app";
import { callsOnFakes } from "./test/call";
import { fakeApp } from "./test/server";

/** The bars of the call the shell shows, each a group named by its room. */
const bars = (room: string) => screen.queryAllByRole("group", { name: room });

test("Join takes the focus to the room's controls; the room's page shows no call bar, the others a listener's, without a microphone", async () => {
  const calls = callsOnFakes();
  const on = fakeApp();
  const id = on.fake.voice.roomId(0);
  const app = await renderApp(`/voice/${id}`, { on });
  const room = "Morning coffee chat";

  await app.user.click(await main().findByRole("button", { name: "Join" }));
  await calls.clock.advance(0);
  const controls = await screen.findByRole("toolbar", { name: "Room controls" });
  await waitFor(() => {
    expect(controls.contains(app.focused())).toBe(true);
  });
  expect(bars(room)).toHaveLength(0);

  await app.navigate("/lists");
  await waitFor(() => {
    expect(bars(room).length).toBeGreaterThan(0);
  });
  for (const bar of bars(room)) {
    expect(within(bar).queryAllByRole("button", { name: /^(Mute|Unmute)$/ })).toHaveLength(0);
    expect(within(bar).getByRole("button", { name: "Leave" })).toBeDefined();
  }
});

test("a room that ends while the reader is elsewhere is told once; its page says it ended, with no Join, and the column lists it no more", async () => {
  const calls = callsOnFakes();
  const on = fakeApp();
  const id = on.fake.voice.roomId(0);
  const app = await renderApp(`/voice/${id}`, { on });
  const room = "Morning coffee chat";
  await app.user.click(await main().findByRole("button", { name: "Join" }));
  await calls.clock.advance(0);
  await screen.findByRole("toolbar", { name: "Room controls" });
  await app.navigate("/");

  app.fake.voice.send({ event: "ended", data: { reason: "host" } });
  const toast = await screen.findByText(`The host ended ${room}.`, { exact: false });
  expect(toast).toBeDefined();
  await waitFor(() => {
    expect(screen.queryAllByRole("link", { name: new RegExp(`^${room}`) })).toHaveLength(0);
  });

  await app.navigate(`/voice/${id}`);
  expect(await main().findByRole("heading", { level: 2, name: room })).toBeDefined();
  // Said for the eye, and once in the page's live region.
  expect(main().getByText(`The host ended ${room}.`, { selector: "p" })).toBeDefined();
  expect(main().queryAllByRole("button", { name: "Join" })).toHaveLength(0);
  expect(main().queryAllByRole("list", { name: "Listeners" })).toHaveLength(0);

  // The room's page said it: the toast does not come back on the next screen.
  await app.navigate("/lists");
  expect(screen.queryAllByText(`The host ended ${room}.`, { exact: false })).toHaveLength(0);
});

test("the rooms' screen says the voice chat is off while it is, and reads no room", async () => {
  const on = fakeApp({ voiceOff: true });
  const rooms = on.fake.script("GET /voice/rooms");
  await renderApp("/voice", { on });

  expect(await main().findByText("Voice chat is not available on this server.")).toBeDefined();
  expect(rooms.seen).toHaveLength(0);
});

test("the profile of a room's host shows the room where they host, which leads to it", async () => {
  const on = fakeApp();
  const id = on.fake.voice.roomId(0);
  await renderApp("/@sam", { on });

  const entry = await main().findByRole("link", { name: /^Morning coffee chat/ });
  expect(entry.getAttribute("href")).toBe(`/voice/${id}`);
});

test("a visitor reads the summary of a room the server shows, and the sign-in that leads into it", async () => {
  const on = fakeApp({ visitor: true, voiceSummaries: true });
  const id = on.fake.voice.roomId(0);
  await renderApp(`/voice/${id}`, { on });

  expect(await main().findByRole("heading", { level: 2, name: "Morning coffee chat" })).toBeDefined();
  expect(main().getByRole("button", { name: "Sign in to join" })).toBeDefined();
});
