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
  args: { text: "👍", count: "3", label: "👍 3, react with this emoji", isSelected: false, onChange: fn() },
} satisfies Meta<typeof ReactionChip>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A Unicode emoji the viewer has not reacted with: a toggle named by the emoji and the count it shows. */
export const Unicode: Story = {
  play: async ({ canvas, args }) => {
    const chip = canvas.getByRole("button", { name: "👍 3, react with this emoji" });
    await expect(chip).toHaveAttribute("aria-pressed", "false");
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
    await expect(canvas.getByRole("button", { name: "👍 3, react with this emoji" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  },
};

/** A custom emoji: its image is read as its `:shortcode:`. */
export const Custom: Story = {
  args: { image: blobcat, text: ":blobcat:", label: ":blobcat: 3, react with this emoji" },
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
export const LargeCount: Story = { args: { count: "12K", label: "👍 12K, react with this emoji", isSelected: true } };

const row = (dir: "ltr" | "rtl", width?: number) => (
  <div dir={dir} style={{ width }}>
    <ReactionRow label="Reactions" listLabel="Who reacted" onOpenList={fn()}>
      {["👍", "❤️", "😂", "🎉", "🙏", "👀", "🔥", "✨"].map((emoji, i) => (
        <ReactionChip
          key={emoji}
          text={emoji}
          count={String(i * 7 + 1)}
          label={`${emoji} ${i * 7 + 1}, react with this emoji`}
          isSelected={i % 3 === 0}
          onChange={fn()}
        />
      ))}
      <ReactionChip
        image={blobcat}
        text=":blobcat:"
        count="12K"
        label=":blobcat: 12K"
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
