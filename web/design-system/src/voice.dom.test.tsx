// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { fireEvent, isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  HeadcountMark,
  ParticipantGrid,
  ParticipantTile,
  ReactionFlow,
  RoomControl,
  RoomControls,
  RoomLine,
  RoomTitle,
} from "./voice";

/** What has the focus, as a reader hears it: a button by its name, or nothing. */
function focused(): string {
  const at = document.activeElement;
  if (at === null || at === document.body) return "nothing";
  return `${at.getAttribute("role") ?? at.tagName.toLowerCase()}: ${at.getAttribute("aria-label") ?? at.textContent}`;
}

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

test("a participant's tile that a press does nothing on is no control: an image named as the button would be", () => {
  render(<ParticipantTile name="Mika" size="stage" role="Host" />);

  expect(screen.queryAllByRole("button")).toHaveLength(0);
  const tile = screen.getByRole("img", { name: "Mika, Host" });
  expect(tile.tabIndex).toBe(-1);
});

test("the reader's own tile says so on a line of its own, after the name, which its cut never takes", () => {
  render(<ParticipantTile name="Someone with a very long name" size="listener" self="You" role="Co-host" />);

  const tile = screen.getByRole("img", { name: "Someone with a very long name, You, Co-host" });
  const lines = [...tile.children].map((child) => child.textContent).filter((text) => text !== "");
  expect(lines).toEqual(["Someone with a very long name", "You", "Co-host"]);
});

test("a participant without a role or a state is named by their name alone, which it shows with no mark", () => {
  render(<ParticipantTile name="Sora" size="listener" />);

  const tile = screen.getByRole("img", { name: "Sora" });
  expect(tile.textContent).toBe("Sora");
  expect(tile.querySelectorAll("svg")).toHaveLength(0);
});

test("a participant the reader blocks shows, marked by the block's glyph and its words", () => {
  render(<ParticipantTile name="Haruto" size="stage" blocked="Blocked" />);

  const tile = screen.getByRole("img", { name: "Haruto, Blocked" });
  expect(tile.textContent).toBe("Haruto Blocked");
  expect(tile.querySelectorAll("svg")).toHaveLength(1);
});

test("a participant's states name the tile only while they hold, in the same order whichever are set", () => {
  const { rerender } = render(<ParticipantTile name="Ren" size="listener" role="Co-host" hand="Hand raised" />);
  expect(screen.getByRole("img", { name: "Ren, Co-host, Hand raised" })).toBeDefined();

  rerender(<ParticipantTile name="Ren" size="listener" blocked="Blocked" muted="Microphone off" />);
  expect(screen.getByRole("img", { name: "Ren, Blocked, Microphone off" })).toBeDefined();

  rerender(<ParticipantTile name="Ren" size="listener" />);
  expect(screen.getByRole("img", { name: "Ren" })).toBeDefined();
});

test("a headcount is read as its label, its glyph and figure hidden", () => {
  render(<HeadcountMark count={{ shown: "20", label: "20 in the room" }} />);

  expect(isInaccessible(screen.getByText("20 in the room"))).toBe(false);
  expect(isInaccessible(screen.getByText("20"))).toBe(true);
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
  expect(items.map((item) => within(item).getByRole("img").getAttribute("aria-label"))).toEqual(["Yui", "Kenji"]);
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

  // The stage stays heard; the reactions are not.
  expect(screen.getByRole("img", { name: "Mika" })).toBeDefined();
  const clap = screen.getByText("👏");
  expect(clap.closest('[aria-hidden="true"]')).not.toBeNull();
  fireEvent.animationEnd(clap);
  fireEvent.animationEnd(screen.getByText("🔥"));
  expect(done).toEqual(["a", "b"]);
});

test("the reactions are no stop of Tab: from the stage the focus goes on past them", async () => {
  const user = userEvent.setup();
  render(
    <>
      <ReactionFlow
        reactions={[
          { id: "a", emoji: "👏" },
          { id: "b", emoji: <img src="/party.png" alt=":party:" /> },
        ]}
        onDone={() => undefined}
      >
        <ParticipantTile name="Mika" size="stage" onPress={() => undefined} />
      </ReactionFlow>
      <button type="button">After the stage</button>
    </>,
  );

  // No reaction is heard, a custom emoji's picture neither.
  expect(screen.queryAllByRole("img")).toHaveLength(0);
  expect(isInaccessible(screen.getByText("👏"))).toBe(true);
  await user.tab();
  expect(focused()).toBe("button: Mika");
  await user.tab();
  expect(focused()).toBe("button: After the stage");
});

test("a reaction that arrives later rises too, and is done once its own animation ends", () => {
  const done: string[] = [];
  const stage = <ParticipantTile name="Mika" size="stage" />;
  const { rerender } = render(
    <ReactionFlow reactions={[{ id: "a", emoji: "👏" }]} onDone={(id) => done.push(id)}>
      {stage}
    </ReactionFlow>,
  );
  rerender(
    <ReactionFlow
      reactions={[
        { id: "a", emoji: "👏" },
        { id: "c", emoji: "❤️" },
      ]}
      onDone={(id) => done.push(id)}
    >
      {stage}
    </ReactionFlow>,
  );

  fireEvent.animationEnd(screen.getByText("❤️"));
  expect(done).toEqual(["c"]);
});

test("a room's line is one link named by its name and caption, its emoji and faces hidden", () => {
  render(
    <RoomLine
      href="/voice/1"
      icon="☕"
      name="Morning coffee chat"
      caption="Hosted by Mika"
      count={{ shown: "20", label: "20 in the room" }}
      speakers={[{ name: "Mika" }, { name: "Ren" }, { name: "Sora" }, { name: "Aoi" }]}
    />,
  );

  const link = screen.getByRole("link", { name: "Morning coffee chat Hosted by Mika 20 in the room" });
  expect(link.getAttribute("href")).toBe("/voice/1");
  // The first three faces show.
  expect(link.querySelectorAll("[data-initial]")).toHaveLength(3);
});

test("the room's controls are a toolbar of buttons the arrow keys move along, leaving last, each named by what it does now", async () => {
  const user = userEvent.setup();
  const did: string[] = [];
  render(
    <RoomControls
      label="Room controls"
      end={<RoomControl icon="leave" label="Leave" onPress={() => did.push("leave")} />}
    >
      <RoomControl icon="microphone" label="Unmute" on onPress={() => did.push("unmute")} />
      <RoomControl icon="react" label="React" onPress={() => did.push("react")} />
    </RoomControls>,
  );

  const bar = screen.getByRole("toolbar", { name: "Room controls" });
  const buttons = within(bar).getAllByRole("button");
  const [mute, react, leave] = buttons;
  if (mute === undefined || react === undefined || leave === undefined) throw new Error("three controls");
  expect(buttons.map((b) => b.textContent)).toEqual(["Unmute", "React", "Leave"]);
  // One name for what is shown and heard: no pressed state beside it.
  expect(within(bar).getByRole("button", { name: "Unmute" })).toBe(mute);
  expect(buttons.some((b) => b.hasAttribute("aria-pressed"))).toBe(false);

  await user.tab();
  expect(document.activeElement).toBe(mute);
  await user.keyboard(" ");
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(react);
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(leave);
  await user.keyboard("{Enter}");
  expect(did).toEqual(["unmute", "leave"]);
});

test("a room's title is a heading of its name and how many are in it, the emoji hidden", () => {
  render(<RoomTitle icon="☕" name="Morning coffee chat" count={{ shown: "20", label: "20 in the room" }} />);

  expect(screen.getByRole("heading", { level: 2, name: "Morning coffee chat 20 in the room" })).toBeDefined();
});
