// SPDX-License-Identifier: AGPL-3.0-only
// Popover stories. The play function tests that focus moves in on open, Esc closes it and focus
// returns to the trigger.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { Popover, type PopoverProps, PopoverTrigger } from "./Popover";

/** The titled form of Popover; the aria-label form is its own story. */
type TitledPopoverProps = Pick<PopoverProps, "children" | "placement"> & { title: string };

const meta = {
  title: "Overlays/Popover",
  decorators: [
    (Story) => (
      <div className="min-h-72">
        <Story />
      </div>
    ),
  ],
  args: {
    title: "Content warnings",
    children: ({ close }) => (
      <>
        <p className="text-sm text-fg-muted">
          A content warning hides the post behind a summary until a reader chooses to show it.
        </p>
        <Button variant="ghost" size="sm" className="self-start" onPress={close}>
          Got it
        </Button>
      </>
    ),
  },
  render: (args) => (
    <PopoverTrigger>
      <Button>What is this?</Button>
      <Popover {...args} />
    </PopoverTrigger>
  ),
} satisfies Meta<TitledPopoverProps>;

export default meta;
type Story = StoryObj<typeof meta>;

export const FocusAndReturn: Story = {
  play: async ({ canvas, userEvent }) => {
    const trigger = canvas.getByRole("button", { name: "What is this?" });
    await userEvent.click(trigger);
    const dialog = await canvas.findByRole("dialog", { name: "Content warnings" });
    await waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());

    await userEvent.keyboard("{Enter}");
    await userEvent.click(await canvas.findByRole("button", { name: "Got it" }));
    await waitFor(() => expect(canvas.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};

export const Open: Story = {
  render: (args) => (
    <PopoverTrigger defaultOpen>
      <Button>What is this?</Button>
      <Popover {...args} />
    </PopoverTrigger>
  ),
};

/** Without a visible title, the popover is named by aria-label. */
export const LabelledByAriaLabel: Story = {
  render: () => (
    <PopoverTrigger defaultOpen>
      <Button>Add emoji</Button>
      <Popover aria-label="Emoji">
        <p className="text-2xl">😀 🎉 🙏 ❤️</p>
      </Popover>
    </PopoverTrigger>
  ),
};
