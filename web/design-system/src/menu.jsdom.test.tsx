// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom does not move the focus of a React Aria menu with the arrow keys (ADR-0171 §4).

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MenuButton, MenuGroup, MenuItem } from "./menu";

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
