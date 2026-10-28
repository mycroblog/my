// SPDX-License-Identifier: AGPL-3.0-only
// Menu stories. The play functions test the menu button pattern with the keyboard: Enter opens the menu
// on its first item, arrows move, Esc closes and returns focus to the trigger, and the submenu keys
// follow the reading direction (Right opens a submenu in LTR, Left in RTL).
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor } from "storybook/test";
import { Button } from "./Button";
import { SubmenuIcon } from "./Icons";
import { Menu, MenuItem, MenuSeparator, MenuTrigger, SubmenuTrigger } from "./Menu";

/** The keys that move into and out of a submenu in the page's reading direction. */
function submenuKeys() {
  return document.documentElement.dir === "rtl"
    ? { open: "{ArrowLeft}", close: "{ArrowRight}" }
    : { open: "{ArrowRight}", close: "{ArrowLeft}" };
}

const onAction = fn();

const meta = {
  title: "Overlays/Menu",
  component: Menu,
  decorators: [
    (Story) => (
      <div className="min-h-80">
        <Story />
      </div>
    ),
  ],
  args: { "aria-label": "Post options", onAction },
  render: (args) => (
    <MenuTrigger>
      <Button>Post options</Button>
      <Menu {...args}>
        <MenuItem id="copy">Copy link</MenuItem>
        <MenuItem id="bookmark">Bookmark</MenuItem>
        <SubmenuTrigger>
          <MenuItem id="share" textValue="Share">
            <span className="flex-1">Share</span>
            <SubmenuIcon />
          </MenuItem>
          <Menu aria-label="Share">
            <MenuItem id="share-email">Email</MenuItem>
            <MenuItem id="share-link">Copy embed code</MenuItem>
          </Menu>
        </SubmenuTrigger>
        <MenuItem id="pin" isDisabled>
          Pin to profile
        </MenuItem>
        <MenuSeparator />
        <MenuItem id="delete" intent="danger">
          Delete
        </MenuItem>
      </Menu>
    </MenuTrigger>
  ),
} satisfies Meta<typeof Menu>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Keyboard: Story = {
  play: async ({ canvas, userEvent }) => {
    onAction.mockClear();
    const trigger = canvas.getByRole("button", { name: "Post options" });
    await userEvent.tab();
    await expect(trigger).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    const menu = await canvas.findByRole("menu", { name: "Post options" });
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Copy link" })).toHaveFocus());

    await userEvent.keyboard("{ArrowDown}");
    await expect(canvas.getByRole("menuitem", { name: "Bookmark" })).toHaveFocus();
    // The disabled item is skipped.
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    await expect(canvas.getByRole("menuitem", { name: "Delete" })).toHaveFocus();
    await userEvent.keyboard("{Home}");
    await expect(canvas.getByRole("menuitem", { name: "Copy link" })).toHaveFocus();

    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(menu).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());

    // Selecting an item runs its action, closes the menu and returns focus.
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Copy link" })).toHaveFocus());
    await userEvent.keyboard("{Enter}");
    await expect(onAction).toHaveBeenCalledOnce();
    await expect(onAction.mock.calls[0]?.[0]).toBe("copy");
    await waitFor(() => expect(canvas.queryByRole("menu")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};

/** In ar-XB the submenu opens with Left and closes with Right; in en the other way round. */
export const SubmenuDirection: Story = {
  play: async ({ canvas, userEvent }) => {
    const keys = submenuKeys();
    await userEvent.tab();
    await userEvent.keyboard("{Enter}");
    const share = await canvas.findByRole("menuitem", { name: "Share" });
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Copy link" })).toHaveFocus());
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    await waitFor(() => expect(share).toHaveFocus());

    // The key towards the inline start does nothing on a submenu trigger.
    await userEvent.keyboard(keys.close);
    await expect(canvas.queryByRole("menu", { name: "Share" })).not.toBeInTheDocument();

    await userEvent.keyboard(keys.open);
    await canvas.findByRole("menu", { name: "Share" });
    await waitFor(() => expect(canvas.getByRole("menuitem", { name: "Email" })).toHaveFocus());

    await userEvent.keyboard(keys.close);
    await waitFor(() => expect(canvas.queryByRole("menu", { name: "Share" })).not.toBeInTheDocument());
    await waitFor(() => expect(share).toHaveFocus());
  },
};

export const Open: Story = {
  render: (args) => (
    <MenuTrigger defaultOpen>
      <Button>Post options</Button>
      <Menu {...args}>
        <MenuItem id="copy">Copy link</MenuItem>
        <MenuItem id="bookmark">Bookmark</MenuItem>
        <MenuItem id="pin" isDisabled>
          Pin to profile
        </MenuItem>
        <MenuSeparator />
        <MenuItem id="delete" intent="danger">
          Delete
        </MenuItem>
      </Menu>
    </MenuTrigger>
  ),
};
