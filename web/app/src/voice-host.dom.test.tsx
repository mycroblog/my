// SPDX-License-Identifier: AGPL-3.0-only

// A voice chat room for its host and its listeners, on the fake API with the calls on fakes: a listener a host lets
// speak gets Speak in place of Raise hand; the host's leave dialog keeps whom it chose by who they are, and says when
// they left; the call's bar of another screen leads the host to that dialog; a removal asked from a tile ends with the
// focus on the room's controls and its result said; a room the host ends shows as ended at once, with no Join; the
// form opens a room whose name the server counts by characters, not UTF-16 units, and says a refused start post under
// its text with the way on; and the settings of a room that ended say so, with nothing to try again.

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { callsOnFakes, type FakeCalls } from "./test/call";
import { fakeApp } from "./test/server";

/** The name the room's tiles and the dialogs call the participant of `username`. */
async function nameOf(app: TestApp, room: string, username: string): Promise<string> {
  const { data } = await app.data.api.GET("/voice/rooms/{id}", { params: { path: { id: room } } });
  const p = data?.participants.find((q) => q.account.username === username);
  if (p === undefined) throw new Error(`test: ${username} is not in the room`);
  return p.account.display_name || p.account.username;
}

/** Renders the room at `index` of the fake's list, and joins it as the page's Join does. */
async function joined(calls: FakeCalls, index: number) {
  const on = fakeApp();
  const id = on.fake.voice.roomId(index);
  const app = await renderApp(`/voice/${id}`, { on });
  await app.user.click(await main().findByRole("button", { name: "Join" }));
  await calls.clock.advance(0);
  await screen.findByRole("toolbar", { name: "Room controls" });
  return { app, id };
}

/** The room's controls. */
const controls = () => within(screen.getByRole("toolbar", { name: "Room controls" }));

/** The tile of the participant named `name`, a button for a host who may act on them. */
const tile = (name: string) => main().getByRole("button", { name: new RegExp(`^${name}(?:, .*)?$`) });

test("a listener a host lets speak gets Speak in place of Raise hand, on the page open", async () => {
  const calls = callsOnFakes();
  const { app } = await joined(calls, 0);
  expect(await controls().findByRole("button", { name: "Raise hand" })).toBeDefined();

  app.fake.voice.grantReader();
  await calls.clock.advance(0);
  expect(await controls().findByRole("button", { name: "Speak" })).toBeDefined();
  expect(controls().queryAllByRole("button", { name: "Raise hand" })).toHaveLength(0);
});

test("the host's leave dialog keeps whom it chose as others come and go, and says so when they leave", async () => {
  const calls = callsOnFakes();
  const { app, id } = await joined(calls, 2);
  const mio = await nameOf(app, id, "mio");
  await app.user.click(controls().getByRole("button", { name: "Leave" }));
  const dialog = within(await screen.findByRole("dialog"));
  await app.user.click(dialog.getByRole("radio", { name: new RegExp(mio) }));

  // Someone listed before the one chosen leaves: the choice stays on them.
  app.fake.voice.participantLeaves(app.fake.accountId("hana"));
  await calls.clock.advance(0);
  await waitFor(() => {
    expect(dialog.getByRole<HTMLInputElement>("radio", { name: new RegExp(mio) }).checked).toBe(true);
  });

  // The one chosen leaves: nothing is chosen, the dialog says why, and nothing is sent.
  app.fake.voice.participantLeaves(app.fake.accountId("mio"));
  await calls.clock.advance(0);
  expect(await dialog.findByText(`${mio} left the room. Choose someone else, or end the room.`)).toBeDefined();
  expect(dialog.getByRole("button", { name: "Hand over and leave" }).hasAttribute("disabled")).toBe(true);
});

test("the call's bar of another screen leads the host of others to the room's choice of who hosts next", async () => {
  const calls = callsOnFakes();
  const { app, id } = await joined(calls, 2);
  await app.navigate("/lists");
  const [bar] = await screen.findAllByRole("group", { name: /^Photo walk planning/ });
  if (bar === undefined) throw new Error("test: no bar of the call");
  await app.user.click(within(bar).getByRole("button", { name: "Leave" }));

  await app.arrived(`/voice/${id}`);
  expect(await screen.findByRole("dialog", { name: "Who hosts the room next?" })).toBeDefined();
  expect(app.fake.voice.joined()?.room).toBe(id);
});

test("a removal asked from a tile ends with the focus on the room's controls and its result said", async () => {
  const calls = callsOnFakes();
  const { app, id } = await joined(calls, 2);
  const kai = await nameOf(app, id, "kai");
  await app.user.click(tile(kai));
  await app.user.click(await screen.findByRole("menuitem", { name: "Remove from the room…" }));
  const dialog = within(await screen.findByRole("alertdialog"));
  await app.user.click(dialog.getByRole("button", { name: "Remove" }));

  expect(await screen.findByText(`You removed ${kai} from the room.`)).toBeDefined();
  await waitFor(() => {
    expect(screen.getByRole("toolbar", { name: "Room controls" }).contains(app.focused())).toBe(true);
  });
  expect(main().queryAllByRole("button", { name: new RegExp(`^${kai}(?:, .*)?$`) })).toHaveLength(0);
});

test("a room the host ends shows as ended at once, never with Join", async () => {
  const calls = callsOnFakes();
  const { app } = await joined(calls, 2);
  await app.user.click(main().getByRole("button", { name: "Manage the room" }));
  await app.user.click(await screen.findByRole("menuitem", { name: "End the room…" }));
  const dialog = within(await screen.findByRole("alertdialog"));
  const joins: number[] = [];
  const watch = new MutationObserver(() => {
    joins.push(main().queryAllByRole("button", { name: "Join" }).length);
  });
  watch.observe(screen.getByRole("main"), { childList: true, subtree: true });
  await app.user.click(dialog.getByRole("button", { name: "End the room" }));

  expect(await main().findByText("This room has ended.")).toBeDefined();
  watch.disconnect();
  expect(joins.every((n) => n === 0)).toBe(true);
});

test("the form opens a room of a name 100 characters long in emoji, and says a refused start post under its text", async () => {
  callsOnFakes();
  const app = await renderApp("/voice/new");
  const name = await main().findByRole("textbox", { name: "Name" });
  // 100 characters, 200 UTF-16 units: the server counts the characters.
  await app.user.click(name);
  await app.user.paste("🎨".repeat(100));
  const post = main().getByRole("textbox", { name: "Text of the post" });
  await app.user.clear(post);
  await app.user.click(post);
  await app.user.paste("a".repeat(501));
  await app.user.click(main().getByRole("button", { name: "Open and join" }));

  const said = await main().findByText(/To open the room without it, untick the start post\.$/);
  // It describes the post's text, which it stands under.
  expect(post.getAttribute("aria-describedby")?.split(" ")).toContain(said.closest("[id]")?.id);
  expect(main().queryAllByText("Correct what is marked, then try again.", { exact: false }).length).toBeGreaterThan(0);

  await app.user.click(main().getByRole("checkbox", { name: "Post to this server that the room is open" }));
  await app.user.click(main().getByRole("button", { name: "Open and join" }));
  await waitFor(() => {
    expect(app.path()).toMatch(/^\/voice\/(?!new$)[^/]+$/);
  });
});

test("the settings and the invitations of a room that ended say so, with nothing to try again", async () => {
  const on = fakeApp();
  const id = on.fake.voice.endedRoomId();
  const app = await renderApp(`/voice/${id}/settings`, { on });
  expect(
    await main().findByText("This room has ended, so its settings and invitations are gone with it."),
  ).toBeDefined();
  expect(main().queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
  await app.navigate(`/voice/${id}/invitations`);
  expect(
    await main().findByText("This room has ended, so its settings and invitations are gone with it."),
  ).toBeDefined();
  expect(main().queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
});
