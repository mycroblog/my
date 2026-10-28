// SPDX-License-Identifier: AGPL-3.0-only
// Sheet stories. At the 320 px container width the sheet sits on the bottom edge; at 640 and 1024 px
// it is a centered dialog. The play function tests focus in, Esc and focus return.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { DialogTrigger, Sheet, type SheetProps } from "./Dialog";

const meta = {
  title: "Overlays/Sheet",
  component: Sheet,
  decorators: [
    (Story) => (
      <div className="min-h-136">
        <Story />
      </div>
    ),
  ],
  args: {
    title: "Who can see this post?",
    closeLabel: "Close",
    children: ({ close }) => (
      <ul className="flex flex-col">
        {["Public", "Local only", "Unlisted", "Followers only", "Mentioned people only"].map((label) => (
          <li key={label}>
            <Button variant="ghost" className="w-full justify-start" onPress={close}>
              {label}
            </Button>
          </li>
        ))}
      </ul>
    ),
  },
  render: (args) => (
    <DialogTrigger>
      <Button>Visibility</Button>
      <Sheet {...args} />
    </DialogTrigger>
  ),
} satisfies Meta<typeof Sheet>;

export default meta;
type Story = StoryObj<typeof meta>;

export const FocusTrapAndReturn: Story = {
  play: async ({ canvas, userEvent }) => {
    const trigger = canvas.getByRole("button", { name: "Visibility" });
    await userEvent.click(trigger);
    const sheet = await canvas.findByRole("dialog", { name: "Who can see this post?" });
    await waitFor(() => expect(sheet).toContainElement(document.activeElement as HTMLElement));
    for (let i = 0; i < 7; i++) {
      await userEvent.tab();
      await expect(sheet).toContainElement(document.activeElement as HTMLElement);
    }
    // Esc closes the innermost layer first: the tooltip of the ×, when the × has the keyboard focus (G12), then the
    // sheet. The modal hides the tooltip's portal from assistive technology; the × keeps its own name.
    if (canvas.getByRole("button", { name: "Close" }) === document.activeElement) {
      await expect(await canvas.findByRole("tooltip", { hidden: true })).toHaveTextContent("Close");
      await userEvent.keyboard("{Escape}");
      await waitFor(() => expect(canvas.queryByRole("tooltip", { hidden: true })).not.toBeInTheDocument());
      await expect(sheet).toBeInTheDocument();
    }
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};

function renderOpen(args: SheetProps) {
  return (
    <DialogTrigger defaultOpen>
      <Button>Visibility</Button>
      <Sheet {...args} />
    </DialogTrigger>
  );
}

export const Open: Story = { render: renderOpen };

/** Taller content than fits: the list scrolls, the title stays. */
export const LongContent: Story = {
  args: {
    title: "Languages",
    children: ({ close }) => (
      <ul className="flex flex-col">
        {Array.from({ length: 20 }, (_, i) => `Language ${String(i + 1)}`).map((label) => (
          <li key={label}>
            <Button variant="ghost" className="w-full justify-start" onPress={close}>
              {label}
            </Button>
          </li>
        ))}
      </ul>
    ),
  },
  render: renderOpen,
};
