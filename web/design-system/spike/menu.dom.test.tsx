import { afterEach, expect, test } from "bun:test";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfirmDialog } from "../src/dialog";
import { MenuButton, MenuGroup, MenuItem } from "../src/menu";
import { Switch } from "../src/switch";

afterEach(cleanup);

test("a menu opens from its button, moves with the arrows, acts on Enter, and gives the focus back", async () => {
  const user = userEvent.setup();
  const picked: string[] = [];
  render(
    <MenuButton icon="more" label="More">
      <MenuGroup>
        <MenuItem id="link" icon="link" onAction={() => picked.push("link")}>
          Copy link
        </MenuItem>
        <MenuItem id="block" icon="block" onAction={() => picked.push("block")}>
          Block…
        </MenuItem>
      </MenuGroup>
    </MenuButton>,
  );
  const button = screen.getByRole("button", { name: "More" });
  expect(button.getAttribute("aria-expanded")).toBe("false");
  await user.click(button);
  const menu = await screen.findByRole("menu", { name: "More" });
  expect(button.getAttribute("aria-expanded")).toBe("true");
  expect(within(menu).getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["Copy link", "Block…"]);
  await user.keyboard("{ArrowDown}");
  await waitFor(() => expect(document.activeElement?.textContent).toBe("Copy link"));
  await user.keyboard("{ArrowDown}");
  await waitFor(() => expect(document.activeElement?.textContent).toBe("Block…"));
  await user.keyboard("{Enter}");
  expect(picked).toEqual(["block"]);
  expect(screen.queryByRole("menu")).toBeNull();
  await waitFor(() => expect(document.activeElement).toBe(button));
});

test("Escape closes the menu without acting", async () => {
  const user = userEvent.setup();
  const picked: string[] = [];
  render(
    <MenuButton icon="more" label="More">
      <MenuGroup>
        <MenuItem id="link" icon="link" onAction={() => picked.push("link")}>
          Copy link
        </MenuItem>
      </MenuGroup>
    </MenuButton>,
  );
  await user.click(screen.getByRole("button", { name: "More" }));
  await screen.findByRole("menu");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(picked).toEqual([]);
});

test("a switch is a named switch that Space toggles", async () => {
  const user = userEvent.setup();
  const seen: boolean[] = [];
  render(<Switch label="Show boosts" isSelected={false} onChange={(v: boolean) => seen.push(v)} />);
  const sw = screen.getByRole("switch", { name: "Show boosts" });
  sw.focus();
  await user.keyboard(" ");
  expect(seen).toEqual([true]);
});

test("a confirmation is a named dialog that holds the focus, and Escape refuses", async () => {
  const user = userEvent.setup();
  const seen: string[] = [];
  render(
    <ConfirmDialog title="Delete this post?" cancel="Cancel" confirm="Delete" onCancel={() => seen.push("cancel")} onConfirm={() => seen.push("confirm")}>
      It cannot be undone.
    </ConfirmDialog>,
  );
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
  await user.tab();
  await user.tab();
  await user.tab();
  expect(dialog.contains(document.activeElement)).toBe(true);
  await user.click(within(dialog).getByRole("button", { name: "Delete" }));
  await user.keyboard("{Escape}");
  expect(seen).toEqual(["confirm", "cancel"]);
});

test("Enter on the button opens the menu on its first item", async () => {
  const user = userEvent.setup();
  render(
    <MenuButton icon="more" label="More">
      <MenuGroup>
        <MenuItem id="link" icon="link">
          Copy link
        </MenuItem>
        <MenuItem id="block" icon="block">
          Block…
        </MenuItem>
      </MenuGroup>
    </MenuButton>,
  );
  screen.getByRole("button", { name: "More" }).focus();
  await user.keyboard("{Enter}");
  await screen.findByRole("menu");
  await waitFor(() => expect(document.activeElement?.textContent).toBe("Copy link"));
  await user.keyboard("{End}");
  await waitFor(() => expect(document.activeElement?.textContent).toBe("Block…"));
});
