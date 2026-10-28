// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent } from "storybook/test";
import { ReactionChip, ReactionRow } from "./ReactionChip";

// A plain orange square stands in for a custom emoji image.
const orange =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==";
const blobcat = { shortcode: "blobcat", url: orange, static_url: orange };

const meta = {
  title: "Timeline/ReactionChip",
  component: ReactionChip,
  args: { text: "👍", count: "3", label: "React with 👍", isSelected: false, onChange: fn() },
} satisfies Meta<typeof ReactionChip>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A Unicode emoji the viewer has not reacted with: a toggle named by the emoji and described by the count. */
export const Unicode: Story = {
  play: async ({ canvas, args }) => {
    const chip = canvas.getByRole("button", { name: "React with 👍" });
    await expect(chip).toHaveAttribute("aria-pressed", "false");
    await expect(chip).toHaveAccessibleDescription("3");
    await userEvent.click(chip);
    await expect(args.onChange).toHaveBeenCalledWith(true);
    // Back to the resting state, so the screenshots do not catch the hover.
    await userEvent.unhover(chip);
    chip.blur();
  },
};

/** The viewer's own reaction. */
export const Selected: Story = {
  args: { isSelected: true },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "React with 👍" })).toHaveAttribute("aria-pressed", "true");
  },
};

/** A custom emoji: its image is read as its `:shortcode:`. */
export const Custom: Story = {
  args: { image: blobcat, text: ":blobcat:", label: "React with :blobcat:" },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("img", { name: ":blobcat:" })).toBeVisible();
  },
};

/** An emoji of another server: shown with its count, no button. */
export const ShownOnly: Story = {
  render: (args) => <ReactionChip {...args} image={blobcat} text=":blobcat:" onChange={null} />,
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};

/** A compact large count. */
export const LargeCount: Story = { args: { count: "12K", isSelected: true } };

const row = (dir: "ltr" | "rtl", width?: number) => (
  <div dir={dir} style={{ width }}>
    <ReactionRow label="Reactions" listLabel="Who reacted" onOpenList={fn()}>
      {["👍", "❤️", "😂", "🎉", "🙏", "👀", "🔥", "✨"].map((emoji, i) => (
        <ReactionChip
          key={emoji}
          text={emoji}
          count={String(i * 7 + 1)}
          label={`React with ${emoji}`}
          isSelected={i % 3 === 0}
          onChange={fn()}
        />
      ))}
      <ReactionChip
        image={blobcat}
        text=":blobcat:"
        count="12K"
        label="React with :blobcat:"
        isSelected={false}
        onChange={null}
      />
    </ReactionRow>
  </div>
);

/** Many reactions wrap onto more lines in a narrow column. */
export const RowWrapping: Story = {
  render: () => row("ltr", 280),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("group", { name: "Reactions" })).toBeVisible();
  },
};

/** The row in a right-to-left page. */
export const RowRtl: Story = { render: () => row("rtl", 280) };
