// SPDX-License-Identifier: AGPL-3.0-only

// The keys of the room's controls, on jsdom: on happy-dom the toolbar's ArrowLeft leaves the focus where it is.

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RoomControl, RoomControls } from "./voice";

/** What has the focus, as a reader hears it: a button by its name, or nothing. */
function focused(): string {
  const at = document.activeElement;
  if (at === null || at === document.body) return "nothing";
  return `${at.getAttribute("role") ?? at.tagName.toLowerCase()}: ${at.getAttribute("aria-label") ?? at.textContent}`;
}

test("the room's controls are one stop of Tab, the arrow keys move back along them, and Tab leaves them", async () => {
  const user = userEvent.setup();
  render(
    <>
      <RoomControls label="Room controls" end={<RoomControl icon="leave" label="Leave" onPress={() => undefined} />}>
        <RoomControl icon="hand" label="Raise hand" onPress={() => undefined} />
        <RoomControl icon="react" label="React" onPress={() => undefined} />
      </RoomControls>
      <button type="button">After the controls</button>
    </>,
  );
  expect(screen.getByRole("toolbar", { name: "Room controls" })).toBeDefined();

  await user.tab();
  expect(focused()).toBe("button: Raise hand");
  await user.keyboard("{ArrowRight}");
  await user.keyboard("{ArrowRight}");
  expect(focused()).toBe("button: Leave");
  await user.keyboard("{ArrowLeft}");
  expect(focused()).toBe("button: React");
  await user.keyboard("{ArrowLeft}");
  expect(focused()).toBe("button: Raise hand");
  await user.tab();
  expect(focused()).toBe("button: After the controls");
});

test("a control that cannot be pressed is passed by the keys and runs nothing", async () => {
  const user = userEvent.setup();
  const did: string[] = [];
  render(
    <RoomControls
      label="Room controls"
      end={<RoomControl icon="leave" label="Leave" onPress={() => did.push("leave")} />}
    >
      <RoomControl icon="microphone" label="Mute" isDisabled onPress={() => did.push("mute")} />
      <RoomControl icon="react" label="React" onPress={() => did.push("react")} />
    </RoomControls>,
  );
  const bar = screen.getByRole("toolbar", { name: "Room controls" });

  await user.click(within(bar).getByRole("button", { name: "Mute" }));
  await user.tab();
  expect(focused()).toBe("button: React");
  await user.keyboard("{ArrowLeft}");
  expect(focused()).toBe("button: React");
  await user.keyboard("{ArrowRight}");
  expect(focused()).toBe("button: Leave");
  expect(did).toEqual([]);
});
