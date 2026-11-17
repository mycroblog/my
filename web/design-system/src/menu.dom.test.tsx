// SPDX-License-Identifier: AGPL-3.0-only

// What a menu is to assistive technology, at rest and opened by a press: its roles, names and states. Its keys and
// the focus they move are in menu.jsdom.test.tsx.

import { expect, jest, test } from "bun:test";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { IconButton } from "./icon-button";
import {
  FormPopup,
  MenuButton,
  MenuGroup,
  MenuHead,
  MenuItem,
  MenuPanel,
  MenuSheet,
  MenuSurface,
  useHeaded,
} from "./menu";

const noop = () => undefined;

test("a menu is named by its label, and a group of its items by its caption", () => {
  render(
    <MenuPanel label="More">
      <MenuGroup>
        <MenuItem id="link" icon="link">
          Copy link
        </MenuItem>
      </MenuGroup>
      <MenuGroup title="@mika">
        <MenuItem id="mute" icon="mute">
          Mute
        </MenuItem>
        <MenuItem id="block" icon="block">
          Block…
        </MenuItem>
      </MenuGroup>
    </MenuPanel>,
  );
  const menu = screen.getByRole("menu", { name: "More" });

  expect(within(menu).getAllByRole("menuitem")).toHaveLength(3);
  const group = within(menu).getByRole("group", { name: "@mika" });
  expect(within(group).getAllByRole("menuitem")).toEqual([
    screen.getByRole("menuitem", { name: "Mute" }),
    screen.getByRole("menuitem", { name: "Block…" }),
  ]);
  expect(group.contains(screen.getByRole("menuitem", { name: "Copy link" }))).toBe(false);
});

test("an item's line under its label is read with it as its name", () => {
  render(
    <MenuPanel label="Boost">
      <MenuGroup>
        <MenuItem id="quote" icon="quote" about="Waits for @sam's approval">
          Quote
        </MenuItem>
      </MenuGroup>
    </MenuPanel>,
  );

  expect(screen.getByRole("menuitem", { name: /^Quote\s*Waits for @sam's approval$/ })).not.toBeNull();
});

test("an item that cannot be done now is disabled, and its name says why", () => {
  render(
    <MenuPanel label="More">
      <MenuGroup>
        <MenuItem id="lists" icon="list" disabled about="You can add only accounts you follow">
          Add to list
        </MenuItem>
        <MenuItem id="mute" icon="mute">
          Mute
        </MenuItem>
      </MenuGroup>
    </MenuPanel>,
  );

  const off = screen.getByRole("menuitem", { name: /^Add to list\s*You can add only accounts you follow$/ });
  expect(off.getAttribute("aria-disabled")).toBe("true");
  expect(screen.getByRole("menuitem", { name: "Mute" }).hasAttribute("aria-disabled")).toBe(false);
});

test("the values of one setting are radio items, the chosen one checked", () => {
  render(
    <MenuPanel label="Who can see it">
      <MenuGroup chosen="followers" onChoose={noop}>
        <MenuItem id="public" icon="public">
          Public
        </MenuItem>
        <MenuItem id="followers" icon="followers">
          Followers only
        </MenuItem>
      </MenuGroup>
    </MenuPanel>,
  );

  expect(screen.getByRole("menuitemradio", { name: "Followers only", checked: true })).not.toBeNull();
  expect(screen.getByRole("menuitemradio", { name: "Public", checked: false })).not.toBeNull();
});

test("the toggles of a group are checkbox items, each one on checked", () => {
  render(
    <MenuPanel label="Add to list">
      <MenuGroup checked={["friends"]} onCheck={noop}>
        <MenuItem id="photographers" icon="included">
          Photographers
        </MenuItem>
        <MenuItem id="friends" icon="included">
          Friends
        </MenuItem>
      </MenuGroup>
    </MenuPanel>,
  );

  expect(screen.getByRole("menuitemcheckbox", { name: "Friends", checked: true })).not.toBeNull();
  expect(screen.getByRole("menuitemcheckbox", { name: "Photographers", checked: false })).not.toBeNull();
});

test("an item that leads to a place is a link to it, current where the reader is, its badge read in its name", () => {
  render(
    <MenuPanel label="Menu">
      <MenuGroup>
        <MenuItem id="lists" icon="list" href="/lists">
          Lists
        </MenuItem>
        <MenuItem id="requests" icon="follow" href="/follow-requests" current badge={<span>3 waiting</span>}>
          Follow requests
        </MenuItem>
      </MenuGroup>
    </MenuPanel>,
  );

  const here = screen.getByRole("menuitem", { name: /^Follow requests\s*3 waiting$/, current: "page" });
  expect(here.getAttribute("href")).toBe("/follow-requests");
  const other = screen.getByRole("menuitem", { name: "Lists" });
  expect(other.getAttribute("href")).toBe("/lists");
  expect(other.hasAttribute("aria-current")).toBe(false);
});

test("an item that opens a submenu reads what the submenu holds now in its name", () => {
  render(
    <MenuPanel label="Who can see and reply">
      <MenuGroup>
        <MenuItem id="visibility" icon="public" opens="Public">
          Who can see it
        </MenuItem>
      </MenuGroup>
    </MenuPanel>,
  );

  expect(screen.getByRole("menuitem", { name: /^Who can see it\s*Public$/ })).not.toBeNull();
});

// WAI-ARIA's menu pattern: an item that opens a submenu has aria-haspopup. This one says nothing of it.
test("an item that opens a submenu says that it has one", () => {
  render(
    <MenuPanel label="Who can see and reply">
      <MenuGroup>
        <MenuItem id="visibility" icon="public" opens="Public">
          Who can see it
        </MenuItem>
      </MenuGroup>
    </MenuPanel>,
  );

  expect(screen.getByRole("menuitem", { name: /^Who can see it/ }).getAttribute("aria-haspopup")).toMatch(
    /^(menu|true)$/,
  );
});

test("a menu's head is its heading, with a back and a close that tell their presses", async () => {
  const user = userEvent.setup();
  const pressed: string[] = [];
  render(
    <MenuHead
      title="Add to list"
      back={{ label: "Back", onPress: () => pressed.push("back") }}
      close={{ label: "Close", onPress: () => pressed.push("close") }}
    />,
  );

  expect(screen.getByRole("heading", { name: "Add to list" })).not.toBeNull();
  await user.click(screen.getByRole("button", { name: "Back" }));
  await user.click(screen.getByRole("button", { name: "Close" }));
  expect(pressed).toEqual(["back", "close"]);
});

test("a menu's head has no back outside a submenu and no close outside a sheet", () => {
  render(<MenuHead title="More" />);

  expect(screen.getByRole("heading", { name: "More" })).not.toBeNull();
  expect(screen.queryByRole("button")).toBeNull();
});

const items = (
  <MenuGroup>
    <MenuItem id="link" icon="link">
      Copy link
    </MenuItem>
  </MenuGroup>
);

test("a menu button is a collapsed button that has a popup, and a press opens the menu its label names", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(
    <MenuButton icon="more" label="More" onOpenChange={(open) => changes.push(open)}>
      {items}
    </MenuButton>,
  );
  const button = screen.getByRole("button", { name: "More", expanded: false });
  expect(button.getAttribute("aria-haspopup")).toMatch(/^(menu|true)$/);
  expect(button.hasAttribute("aria-pressed")).toBe(false);
  expect(screen.queryByRole("menu")).toBeNull();

  await user.click(button);

  const menu = await screen.findByRole("menu", { name: "More" });
  expect(within(menu).getByRole("menuitem", { name: "Copy link" })).not.toBeNull();
  expect(button.getAttribute("aria-expanded")).toBe("true");
  expect(button.getAttribute("aria-controls")).toBe(menu.id);
  expect(changes).toEqual([true]);
});

test("a menu button reads its count in its name, and its on state as pressed", () => {
  render(
    <MenuButton icon="boost" label="Boost" count={12} pressed>
      {items}
    </MenuButton>,
  );

  expect(screen.getByRole("button", { name: "Boost, 12", pressed: true, expanded: false })).not.toBeNull();
});

test("a menu button with no count, or a count of zero, is named by its label alone", () => {
  render(
    <MenuButton icon="boost" label="Boost" count={0}>
      {items}
    </MenuButton>,
  );

  expect(screen.getByRole("button", { name: "Boost" })).not.toBeNull();
});

test("a menu button says the keys that open its menu", () => {
  render(
    <MenuButton icon="more" label="More" aria-keyshortcuts="m">
      {items}
    </MenuButton>,
  );

  expect(screen.getByRole("button", { name: "More" }).getAttribute("aria-keyshortcuts")).toBe("m");
});

test("a menu is named by its own label where its button's name says what it holds now", async () => {
  const user = userEvent.setup();
  render(
    <MenuButton icon="public" label="Public" menuLabel="Who can see it">
      {items}
    </MenuButton>,
  );

  await user.click(screen.getByRole("button", { name: "Public" }));

  expect(await screen.findByRole("menu", { name: "Who can see it" })).not.toBeNull();
});

test("a menu its owner holds open is open, and Escape asks the owner to close it", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(
    <MenuButton icon="more" label="More" open onOpenChange={(open) => changes.push(open)}>
      {items}
    </MenuButton>,
  );
  await screen.findByRole("menu", { name: "More" });

  await user.keyboard("{Escape}");

  expect(changes).toEqual([false]);
});

test("Escape closes a menu a press opened, and the focus is back on its button", async () => {
  const user = userEvent.setup();
  render(
    <MenuButton icon="more" label="More">
      {items}
    </MenuButton>,
  );
  await user.click(screen.getByRole("button", { name: "More" }));
  await screen.findByRole("menu", { name: "More" });

  await user.keyboard("{Escape}");

  expect(screen.queryByRole("menu")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "More", expanded: false }));
});

test("a press on a disabled item does nothing: no action, and the menu stays", async () => {
  const user = userEvent.setup();
  const acted: string[] = [];
  render(
    <MenuButton icon="more" label="More">
      <MenuGroup>
        <MenuItem
          id="lists"
          icon="list"
          disabled
          about="You can add only accounts you follow"
          onAction={() => acted.push("lists")}
        >
          Add to list
        </MenuItem>
        <MenuItem id="mute" icon="mute" onAction={() => acted.push("mute")}>
          Mute
        </MenuItem>
      </MenuGroup>
    </MenuButton>,
  );
  await user.click(screen.getByRole("button", { name: "More" }));
  await screen.findByRole("menu", { name: "More" });

  await user.click(screen.getByRole("menuitem", { name: /^Add to list/ }));
  expect(acted).toEqual([]);
  expect(screen.queryByRole("menu")).not.toBeNull();

  await user.click(screen.getByRole("menuitem", { name: "Mute" }));
  expect(acted).toEqual(["mute"]);
  expect(screen.queryByRole("menu")).toBeNull();
});

test("on a phone a menu is a sheet headed by its name, and its close closes it with the focus back on its button", async () => {
  const user = userEvent.setup();
  render(
    <MenuSheet sheet>
      <MenuButton icon="more" label="More">
        {items}
      </MenuButton>
    </MenuSheet>,
  );
  await user.click(screen.getByRole("button", { name: "More" }));
  const menu = await screen.findByRole("menu", { name: "More" });

  const heading = screen.getByRole("heading", { name: "More" });
  expect(menu.contains(heading)).toBe(false);
  // The sheet has no landmark of its own: its close is the one beside its title.
  const head = heading.parentElement;
  if (head === null) throw new Error("the sheet's title has no head");
  await user.click(within(head).getByRole("button", { name: "Close" }));

  expect(screen.queryByRole("menu")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "More", expanded: false }));
});

test("from a tablet a menu is a popover with no head", async () => {
  const user = userEvent.setup();
  render(
    <MenuSheet sheet={false}>
      <MenuButton icon="more" label="More">
        {items}
      </MenuButton>
    </MenuSheet>,
  );
  await user.click(screen.getByRole("button", { name: "More" }));
  await screen.findByRole("menu", { name: "More" });

  expect(screen.queryByRole("heading")).toBeNull();
  expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
});

/** A menu entered into its submenu, in a sheet or a popover, and how often its back was pressed. */
function submenu(sheet: boolean) {
  const backs: number[] = [];
  render(
    <MenuSheet sheet={sheet}>
      <MenuButton icon="more" label="More" sub={{ title: "Add to list", onBack: () => backs.push(1) }}>
        <MenuGroup checked={["friends"]} stay onCheck={noop}>
          <MenuItem id="friends" icon="included">
            Friends
          </MenuItem>
        </MenuGroup>
      </MenuButton>
    </MenuSheet>,
  );
  return backs;
}

for (const sheet of [false, true])
  test(`a submenu in a ${sheet ? "sheet" : "popover"} takes its menu's name, headed by its name after a back`, async () => {
    const user = userEvent.setup();
    const backs = submenu(sheet);
    await user.click(screen.getByRole("button", { name: "More" }));

    expect(await screen.findByRole("menu", { name: "Add to list" })).not.toBeNull();
    expect(screen.getByRole("heading", { name: "Add to list" })).not.toBeNull();
    expect(screen.queryByRole("heading", { name: "More" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Back" }));

    expect(backs).toHaveLength(1);
  });

/** A Like button whose own press likes, and whose long press opens the menu of the reactions; how often it liked. */
function likeButton() {
  const likes: number[] = [];
  render(
    <MenuSurface
      label="Reactions"
      longPress
      trigger={<IconButton icon="favourite" label="Like" onPress={() => likes.push(1)} />}
    >
      {items}
    </MenuSurface>,
  );
  return likes;
}

test("a trigger whose long press opens its menu runs its own action on a press, and says how its menu opens", async () => {
  const user = userEvent.setup();
  const likes = likeButton();
  const button = screen.getByRole("button", { name: "Like", expanded: false });
  // React Aria's words, in en.
  expect(screen.getByRole("button", { description: "Long press or press Alt + ArrowDown to open menu" })).toBe(button);

  await user.click(button);
  await user.keyboard("{Enter}");

  expect(likes).toHaveLength(2);
  expect(screen.queryByRole("menu")).toBeNull();
});

test("Alt and ArrowDown open the menu of a trigger that a long press opens, named by its own label", async () => {
  const user = userEvent.setup();
  const likes = likeButton();
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Like" }));

  await user.keyboard("{Alt>}{ArrowDown}{/Alt}");

  expect(await screen.findByRole("menu", { name: "Reactions" })).not.toBeNull();
  expect(likes).toEqual([]);
});

test("a long press opens the menu, and its release neither closes it nor runs the trigger's own action", () => {
  jest.useFakeTimers();
  try {
    const likes = likeButton();
    const button = screen.getByRole("button", { name: "Like" });

    fireEvent.pointerDown(button, { pointerType: "touch", pointerId: 1, button: 0 });
    expect(screen.queryByRole("menu")).toBeNull();
    // The test's clock passes the time a press takes to be a long one.
    act(() => {
      jest.advanceTimersByTime(1000);
    });
    fireEvent.pointerUp(button, { pointerType: "touch", pointerId: 1, button: 0 });

    expect(screen.getByRole("menu", { name: "Reactions" })).not.toBeNull();
    expect(likes).toEqual([]);
  } finally {
    jest.useRealTimers();
  }
});

/** Says whether the form it is in is headed by a sheet. */
function Headed() {
  return <output>{useHeaded() ? "headed" : "bare"}</output>;
}

/** A schedule button whose form popup the test's owner opens and closes, and the changes it was asked for. */
function SchedulePopup({ sheet, changes }: { sheet: boolean; changes: boolean[] }) {
  const [open, setOpen] = useState(false);
  return (
    <MenuSheet sheet={sheet}>
      <FormPopup
        open={open}
        onOpenChange={(next) => {
          changes.push(next);
          setOpen(next);
        }}
        trigger={<IconButton icon="schedule" label="Schedule" />}
        label="Schedule the post"
      >
        <label>
          When
          <input type="text" />
        </label>
        <Headed />
      </FormPopup>
    </MenuSheet>
  );
}

test("a form popup is a dialog named by its label, opened by its trigger, and Escape closes it with the focus back", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<SchedulePopup sheet={false} changes={changes} />);
  const trigger = screen.getByRole("button", { name: "Schedule", expanded: false });

  await user.click(trigger);

  const dialog = await screen.findByRole("dialog", { name: "Schedule the post" });
  expect(within(dialog).getByRole("textbox", { name: "When" })).not.toBeNull();
  expect(dialog.contains(document.activeElement)).toBe(true);
  expect(within(dialog).getByRole("status").textContent).toBe("bare");
  expect(screen.queryByRole("heading")).toBeNull();

  await user.keyboard("{Escape}");

  expect(changes).toEqual([true, false]);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Schedule", expanded: false }));
});

test("on a phone a form popup is a sheet headed by its name, which its form reads, and its close closes it", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<SchedulePopup sheet changes={changes} />);

  await user.click(screen.getByRole("button", { name: "Schedule" }));

  const dialog = await screen.findByRole("dialog", { name: "Schedule the post" });
  expect(screen.getByRole("heading", { name: "Schedule the post" })).not.toBeNull();
  expect(within(dialog).getByRole("status").textContent).toBe("headed");
  expect(dialog.contains(document.activeElement)).toBe(true);

  await user.click(screen.getByRole("button", { name: "Close" }));

  expect(changes).toEqual([true, false]);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Schedule", expanded: false }));
});
