// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ParticipantGrid, ParticipantTile, ReactionFlow, RoomControl, RoomControls, RoomLine } from "./voice";

test("a participant's tile is a button named by the name first, then their role, the block's mark and their states", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(
    <ParticipantTile
      name="Mika"
      size="stage"
      role="Host"
      blocked="Blocked"
      speaking="Speaking"
      muted="Microphone off"
      hand="Hand raised"
      onPress={() => presses++}
    />,
  );

  const tile = screen.getByRole("button", { name: "Mika, Host, Blocked, Speaking, Microphone off, Hand raised" });
  await user.click(tile);
  await user.keyboard("{Enter}");
  expect(presses).toBe(2);
});

test("a participant without a role or a state is named by their name alone, which it shows with no mark", () => {
  render(<ParticipantTile name="Sora" size="listener" />);

  const tile = screen.getByRole("button", { name: "Sora" });
  expect(tile.textContent).toBe("Sora");
  expect(tile.querySelectorAll("svg")).toHaveLength(0);
});

test("a participant the reader blocks shows, marked by the block's glyph and its words", () => {
  render(<ParticipantTile name="Haruto" size="stage" blocked="Blocked" />);

  const tile = screen.getByRole("button", { name: "Haruto, Blocked" });
  expect(tile.textContent).toBe("Haruto Blocked");
  expect(tile.querySelectorAll("svg")).toHaveLength(1);
});

test("the tiles of a kind are a list named by its label, an item a tile", () => {
  render(
    <ParticipantGrid label="Listeners" size="listener">
      <ParticipantTile name="Yui" size="listener" />
      <ParticipantTile name="Kenji" size="listener" />
    </ParticipantGrid>,
  );

  const list = screen.getByRole("list", { name: "Listeners" });
  const items = within(list).getAllByRole("listitem");
  expect(items.map((item) => within(item).getByRole("button").getAttribute("aria-label"))).toEqual(["Yui", "Kenji"]);
});

test("reactions rise over the stage hidden from assistive technology, and each is done once it has risen", () => {
  const done: string[] = [];
  render(
    <ReactionFlow
      reactions={[
        { id: "a", emoji: "👏" },
        { id: "b", emoji: "🔥" },
      ]}
      onDone={(id) => done.push(id)}
    >
      <ParticipantTile name="Mika" size="stage" />
    </ReactionFlow>,
  );

  // The stage stays pressable and heard; the reactions are not.
  expect(screen.getByRole("button", { name: "Mika" })).toBeDefined();
  const clap = screen.getByText("👏");
  expect(clap.closest('[aria-hidden="true"]')).not.toBeNull();
  fireEvent.animationEnd(clap);
  fireEvent.animationEnd(screen.getByText("🔥"));
  expect(done).toEqual(["a", "b"]);
});

test("a room's line is one link named by its name and caption, its emoji and faces hidden", () => {
  render(
    <RoomLine
      href="/voice/1"
      icon="☕"
      name="Morning coffee chat"
      caption="Hosted by Mika · 20 in the room"
      speakers={[{ name: "Mika" }, { name: "Ren" }, { name: "Sora" }, { name: "Aoi" }]}
    />,
  );

  const link = screen.getByRole("link", { name: "Morning coffee chat Hosted by Mika · 20 in the room" });
  expect(link.getAttribute("href")).toBe("/voice/1");
  // The first three faces show.
  expect(link.querySelectorAll("[data-initial]")).toHaveLength(3);
});

test("the room's controls are a toolbar the arrow keys move along, leaving last, and a toggle says it is on", async () => {
  const user = userEvent.setup();
  const did: string[] = [];
  render(
    <RoomControls
      label="Room controls"
      end={<RoomControl icon="leave" label="Leave" onPress={() => did.push("leave")} />}
    >
      <RoomControl icon="microphone" label="Mute" pressed onPress={() => did.push("mute")} />
      <RoomControl icon="react" label="React" onPress={() => did.push("react")} />
    </RoomControls>,
  );

  const bar = screen.getByRole("toolbar", { name: "Room controls" });
  const buttons = within(bar).getAllByRole("button");
  const [mute, react, leave] = buttons;
  if (mute === undefined || react === undefined || leave === undefined) throw new Error("three controls");
  expect(buttons.map((b) => b.textContent)).toEqual(["Mute", "React", "Leave"]);
  expect(within(bar).getByRole("button", { name: "Mute", pressed: true })).toBe(mute);
  expect(react.hasAttribute("aria-pressed")).toBe(false);

  await user.tab();
  expect(document.activeElement).toBe(mute);
  await user.keyboard(" ");
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(react);
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(leave);
  await user.keyboard("{Enter}");
  expect(did).toEqual(["mute", "leave"]);
});
