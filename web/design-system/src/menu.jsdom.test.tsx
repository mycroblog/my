// SPDX-License-Identifier: AGPL-3.0-only

// The keys of a menu and the focus they move. On jsdom: an opened React Aria menu takes no focus on happy-dom, so no
// key but Escape reaches an item there (ADR-0171 §4). What a menu is at rest is in menu.dom.test.tsx.

import { expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { MenuButton, MenuGroup, MenuItem, MenuSheet } from "./menu";

/** A More button whose menu holds three items, and the ids of the items that acted, in order. */
function moreMenu() {
  const acted: string[] = [];
  render(
    <MenuButton icon="more" label="More">
      <MenuGroup>
        <MenuItem id="link" icon="link" onAction={() => acted.push("link")}>
          Copy link
        </MenuItem>
        <MenuItem id="share" icon="share" onAction={() => acted.push("share")}>
          Share
        </MenuItem>
        <MenuItem id="bookmark" icon="bookmark" onAction={() => acted.push("bookmark")}>
          Bookmark
        </MenuItem>
      </MenuGroup>
    </MenuButton>,
  );
  return acted;
}

const item = (name: string) => screen.getByRole("menuitem", { name });

/** Opens the menu from the keyboard: Tab to its button, then Enter. */
async function openWithEnter(user: ReturnType<typeof userEvent.setup>) {
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "More", expanded: false }));
  await user.keyboard("{Enter}");
  await screen.findByRole("menu", { name: "More" });
}

test("Enter on a menu button opens its menu with the focus on the first item", async () => {
  const user = userEvent.setup();
  moreMenu();

  await openWithEnter(user);

  expect(document.activeElement).toBe(item("Copy link"));
});

test("End moves the focus to the last item of a menu, and the arrows to the one before and after", async () => {
  const user = userEvent.setup();
  moreMenu();
  await openWithEnter(user);

  await user.keyboard("{End}");
  expect(document.activeElement).toBe(item("Bookmark"));
  await user.keyboard("{ArrowUp}");
  expect(document.activeElement).toBe(item("Share"));
  await user.keyboard("{ArrowDown}");
  expect(document.activeElement).toBe(item("Bookmark"));
});

test("Enter on a menu item acts once and closes the menu", async () => {
  const user = userEvent.setup();
  const acted = moreMenu();
  await openWithEnter(user);

  await user.keyboard("{ArrowDown}");
  expect(document.activeElement).toBe(item("Share"));
  await user.keyboard("{Enter}");

  expect(acted).toEqual(["share"]);
  expect(screen.queryByRole("menu")).toBeNull();
});

test("Escape closes a menu and no item acts", async () => {
  const user = userEvent.setup();
  const acted = moreMenu();
  await openWithEnter(user);

  await user.keyboard("{Escape}");

  expect(screen.queryByRole("menu")).toBeNull();
  expect(acted).toEqual([]);
});

/** The focus is back on the More button, which is collapsed again: on jsdom it arrives a frame after the close. */
async function focusIsBack() {
  await waitFor(
    () => {
      // A cheap look while it waits: a query that finds nothing describes the whole page each time it fails.
      if (document.activeElement?.getAttribute("aria-expanded") !== "false") throw new Error("the focus is not back");
    },
    { interval: 5 },
  );
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "More", expanded: false }));
}

test("the focus goes back to a menu's button when Escape closes the menu", async () => {
  const user = userEvent.setup();
  moreMenu();
  await openWithEnter(user);

  await user.keyboard("{Escape}");

  await focusIsBack();
});

test("the focus goes back to a menu's button when an item acted", async () => {
  const user = userEvent.setup();
  const acted = moreMenu();
  await openWithEnter(user);

  await user.keyboard("{Enter}");

  expect(acted).toEqual(["link"]);
  await focusIsBack();
});

test("Space and ArrowDown on a menu button open its menu at the first item, ArrowUp at the last", async () => {
  const user = userEvent.setup();
  moreMenu();
  await user.tab();

  for (const [key, name] of [
    [" ", "Copy link"],
    ["{ArrowDown}", "Copy link"],
    ["{ArrowUp}", "Bookmark"],
  ] as const) {
    await user.keyboard(key);
    await screen.findByRole("menu", { name: "More" });
    expect(document.activeElement).toBe(item(name));
    await user.keyboard("{Escape}");
    await focusIsBack();
  }
});

test("Home moves the focus to the first item of a menu, and a letter to the next item that starts with it", async () => {
  const user = userEvent.setup();
  moreMenu();
  await openWithEnter(user);

  await user.keyboard("b");
  expect(document.activeElement).toBe(item("Bookmark"));
  await user.keyboard("{Home}");
  expect(document.activeElement).toBe(item("Copy link"));
});

test("Tab does not take the focus out of an open menu", async () => {
  const user = userEvent.setup();
  render(
    <>
      <MenuButton icon="more" label="More">
        <MenuGroup>
          <MenuItem id="link" icon="link">
            Copy link
          </MenuItem>
          <MenuItem id="share" icon="share">
            Share
          </MenuItem>
        </MenuGroup>
      </MenuButton>
      <button type="button">Behind</button>
    </>,
  );
  await openWithEnter(user);
  const menu = screen.getByRole("menu", { name: "More" });

  for (const shift of [false, false, true, true, true]) {
    await user.tab({ shift });
    expect(menu.contains(document.activeElement)).toBe(true);
  }
});

test("the arrows pass over a disabled item, which no key reaches", async () => {
  const user = userEvent.setup();
  const acted: string[] = [];
  render(
    <MenuButton icon="more" label="More">
      <MenuGroup>
        <MenuItem id="link" icon="link" onAction={() => acted.push("link")}>
          Copy link
        </MenuItem>
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
  await openWithEnter(user);
  expect(document.activeElement).toBe(item("Copy link"));

  await user.keyboard("{ArrowDown}");
  expect(document.activeElement).toBe(item("Mute"));
  await user.keyboard("{ArrowUp}");
  expect(document.activeElement).toBe(item("Copy link"));
  await user.keyboard("a");
  expect(document.activeElement).toBe(item("Copy link"));
  expect(acted).toEqual([]);
});

/** A visibility button whose menu holds the values of one setting, Followers only chosen, and what was chosen. */
function visibilityMenu({ stay = false }: { stay?: boolean } = {}) {
  const chose: string[] = [];
  function Visibility() {
    const [chosen, setChosen] = useState("followers");
    return (
      <MenuButton icon="public" label="More">
        <MenuGroup
          chosen={chosen}
          stay={stay}
          onChoose={(id) => {
            chose.push(id);
            setChosen(id);
          }}
        >
          <MenuItem id="public" icon="public">
            Public
          </MenuItem>
          <MenuItem id="followers" icon="followers">
            Followers only
          </MenuItem>
          <MenuItem id="direct" icon="direct">
            Direct
          </MenuItem>
        </MenuGroup>
      </MenuButton>
    );
  }
  render(<Visibility />);
  return chose;
}

const value = (name: string, checked: boolean) => screen.getByRole("menuitemradio", { name, checked });

// MenuSurface's own words: "The focus goes to its first item (its chosen one)". It goes to the first, chosen or not.
test("a menu of values opens with the focus on the chosen one", async () => {
  const user = userEvent.setup();
  visibilityMenu();

  await openWithEnter(user);

  expect(value("Followers only", true).contains(document.activeElement)).toBe(true);
});

test("Enter on another value chooses it and closes the menu, which then shows it checked", async () => {
  const user = userEvent.setup();
  const chose = visibilityMenu();
  await openWithEnter(user);

  await user.keyboard("{End}");
  expect(document.activeElement).toBe(value("Direct", false));
  await user.keyboard("{Enter}");

  expect(chose).toEqual(["direct"]);
  expect(screen.queryByRole("menu")).toBeNull();
  await focusIsBack();
  await user.keyboard("{Enter}");
  await screen.findByRole("menu", { name: "More" });
  expect(value("Direct", true)).not.toBeNull();
  expect(value("Followers only", false)).not.toBeNull();
});

test("a choice in a group that stays keeps the menu open, with the focus on the value now checked", async () => {
  const user = userEvent.setup();
  const chose = visibilityMenu({ stay: true });
  await openWithEnter(user);

  await user.keyboard("{Home}{Enter}");

  expect(chose).toEqual(["public"]);
  expect(document.activeElement).toBe(value("Public", true));
  expect(value("Followers only", false)).not.toBeNull();
});

/** A More button whose menu holds two toggles, Friends on, and what each press checked. */
function listsMenu({ stay }: { stay: boolean }) {
  const checks: [string, boolean][] = [];
  function Lists() {
    const [on, setOn] = useState<readonly string[]>(["friends"]);
    return (
      <MenuButton icon="more" label="More">
        <MenuGroup
          checked={on}
          stay={stay}
          onCheck={(id, checked) => {
            checks.push([id, checked]);
            setOn(checked ? [...on, id] : on.filter((other) => other !== id));
          }}
        >
          <MenuItem id="photographers" icon="included">
            Photographers
          </MenuItem>
          <MenuItem id="friends" icon="included">
            Friends
          </MenuItem>
        </MenuGroup>
      </MenuButton>
    );
  }
  render(<Lists />);
  return checks;
}

const toggle = (name: string, checked: boolean) => screen.getByRole("menuitemcheckbox", { name, checked });

test("Space and Enter on the toggles of a group that stays check and uncheck them, and the menu stays open", async () => {
  const user = userEvent.setup();
  const checks = listsMenu({ stay: true });
  await openWithEnter(user);
  expect(document.activeElement).toBe(toggle("Photographers", false));

  await user.keyboard(" ");
  expect(document.activeElement).toBe(toggle("Photographers", true));
  await user.keyboard("{ArrowDown}{Enter}");
  expect(document.activeElement).toBe(toggle("Friends", false));

  expect(checks).toEqual([
    ["photographers", true],
    ["friends", false],
  ]);
});

test("a toggle of a group that does not stay closes the menu", async () => {
  const user = userEvent.setup();
  const checks = listsMenu({ stay: false });
  await openWithEnter(user);

  await user.keyboard("{Enter}");

  expect(checks).toEqual([["photographers", true]]);
  expect(screen.queryByRole("menu")).toBeNull();
  await focusIsBack();
});

/** A More button whose menu has an item that opens the submenu of the reader's lists, in a sheet or a popover. */
function ProfileMenu({ sheet }: { sheet: boolean }) {
  const [panel, setPanel] = useState<"top" | "lists">("top");
  return (
    <MenuSheet sheet={sheet}>
      <MenuButton
        icon="more"
        label="More"
        sub={
          panel === "lists"
            ? {
                title: "Add to list",
                onBack: () => {
                  setPanel("top");
                },
              }
            : undefined
        }
        onOpenChange={(open) => {
          if (!open) setPanel("top");
        }}
      >
        {panel === "lists" ? (
          <MenuGroup checked={[]} stay onCheck={() => undefined}>
            <MenuItem id="photographers" icon="included">
              Photographers
            </MenuItem>
            <MenuItem id="friends" icon="included">
              Friends
            </MenuItem>
          </MenuGroup>
        ) : (
          <MenuGroup>
            <MenuItem id="mute" icon="mute">
              Mute
            </MenuItem>
            <MenuItem
              id="lists"
              icon="list"
              opens="2"
              onAction={() => {
                setPanel("lists");
              }}
            >
              Add to list
            </MenuItem>
          </MenuGroup>
        )}
      </MenuButton>
    </MenuSheet>
  );
}

/**
 * A More button whose menu opens the submenu of the reader's lists, which opens the submenu of a new list's audience:
 * each item that opens one is the last of its menu, so that the focus on it is no menu's first.
 */
function NestedMenu({ sheet }: { sheet: boolean }) {
  const [panel, setPanel] = useState<"top" | "lists" | "audience">("top");
  const back = { lists: "top", audience: "lists" } as const;
  return (
    <MenuSheet sheet={sheet}>
      <MenuButton
        icon="more"
        label="More"
        sub={
          panel === "top"
            ? undefined
            : {
                title: panel === "lists" ? "Add to list" : "Who sees a new list",
                onBack: () => {
                  setPanel(back[panel]);
                },
              }
        }
        onOpenChange={(open) => {
          if (!open) setPanel("top");
        }}
      >
        {panel === "top" && (
          <MenuGroup>
            <MenuItem id="mute" icon="mute">
              Mute
            </MenuItem>
            <MenuItem id="block" icon="block">
              Block
            </MenuItem>
            <MenuItem
              id="lists"
              icon="list"
              opens="2"
              onAction={() => {
                setPanel("lists");
              }}
            >
              Add to list
            </MenuItem>
          </MenuGroup>
        )}
        {panel === "lists" && (
          <MenuGroup>
            <MenuItem id="photographers" icon="included">
              Photographers
            </MenuItem>
            <MenuItem id="friends" icon="included">
              Friends
            </MenuItem>
            <MenuItem
              id="audience"
              icon="followers"
              opens="Only you"
              onAction={() => {
                setPanel("audience");
              }}
            >
              New list
            </MenuItem>
          </MenuGroup>
        )}
        {panel === "audience" && (
          <MenuGroup>
            <MenuItem id="private" icon="hidden">
              Only you
            </MenuItem>
            <MenuItem id="public" icon="public">
              Everyone
            </MenuItem>
          </MenuGroup>
        )}
      </MenuButton>
    </MenuSheet>
  );
}

/** The focus is on the menu item whose name starts with `name`: on jsdom it arrives a frame after its menu is back. */
async function focusIsOnItem(name: string) {
  await waitFor(
    () => {
      // A cheap look while it waits: a query that finds nothing describes the whole page each time it fails.
      const focused = document.activeElement;
      if (focused?.getAttribute("role") !== "menuitem" || !focused.textContent.startsWith(name))
        throw new Error(`the focus is not on ${name}`);
    },
    { interval: 5 },
  );
}

for (const sheet of [false, true]) {
  const where = sheet ? "sheet" : "popover";

  test(`in a ${where}, Enter on an item that opens a submenu keeps the menu open, with the focus on the submenu's first item`, async () => {
    const user = userEvent.setup();
    render(<ProfileMenu sheet={sheet} />);
    await openWithEnter(user);
    expect(document.activeElement).toBe(item("Mute"));

    await user.keyboard("{ArrowDown}{Enter}");

    await screen.findByRole("menu", { name: "Add to list" });
    expect(document.activeElement).toBe(toggle("Photographers", false));
  });

  test(`in a ${where}, a submenu's back puts its menu back with the focus on an item of it, and Escape closes it from there`, async () => {
    const user = userEvent.setup();
    render(<ProfileMenu sheet={sheet} />);
    await openWithEnter(user);
    await user.keyboard("{ArrowDown}{Enter}");
    await screen.findByRole("menu", { name: "Add to list" });

    await user.click(screen.getByRole("button", { name: "Back" }));

    const menu = await screen.findByRole("menu", { name: "More" });
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
    await waitFor(() => {
      expect(menu.contains(document.activeElement)).toBe(true);
    });
    expect(document.activeElement?.getAttribute("role")).toBe("menuitem");

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();
    await focusIsBack();
  });

  test(`in a ${where}, a submenu's back gives the focus to the item that opened the submenu`, async () => {
    const user = userEvent.setup();
    render(<ProfileMenu sheet={sheet} />);
    await openWithEnter(user);
    await user.keyboard("{ArrowDown}{Enter}");
    await screen.findByRole("menu", { name: "Add to list" });

    await user.click(screen.getByRole("button", { name: "Back" }));

    const menu = await screen.findByRole("menu", { name: "More" });
    // Not the menu's first item, Mute: the one the reader came through.
    await focusIsOnItem("Add to list");
    expect(menu.contains(document.activeElement)).toBe(true);
    await user.keyboard("{ArrowUp}");
    expect(document.activeElement).toBe(item("Mute"));
  });

  test(`in a ${where}, back from a submenu of a submenu gives the focus to the item that opened each, one menu at a time`, async () => {
    const user = userEvent.setup();
    render(<NestedMenu sheet={sheet} />);
    await openWithEnter(user);
    await user.keyboard("{End}{Enter}");
    await screen.findByRole("menu", { name: "Add to list" });
    await user.keyboard("{End}{Enter}");
    await screen.findByRole("menu", { name: "Who sees a new list" });
    expect(document.activeElement).toBe(item("Only you"));

    await user.click(screen.getByRole("button", { name: "Back" }));
    const lists = await screen.findByRole("menu", { name: "Add to list" });
    await focusIsOnItem("New list");
    expect(lists.contains(document.activeElement)).toBe(true);

    await user.click(screen.getByRole("button", { name: "Back" }));
    const top = await screen.findByRole("menu", { name: "More" });
    await focusIsOnItem("Add to list");
    expect(top.contains(document.activeElement)).toBe(true);
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
  });

  test(`in a ${where}, Escape in a submenu closes the whole menu, with the focus back on its button`, async () => {
    const user = userEvent.setup();
    render(<ProfileMenu sheet={sheet} />);
    await openWithEnter(user);
    await user.keyboard("{ArrowDown}{Enter}");
    await screen.findByRole("menu", { name: "Add to list" });

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("menu")).toBeNull();
    await focusIsBack();
  });
}
