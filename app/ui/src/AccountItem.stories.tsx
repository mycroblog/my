// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { AccountItem } from "./AccountItem";
import { Box } from "./Box";
import { Button } from "./Button";
import { EmojiText } from "./CustomEmoji";
import { Stack } from "./Stack";

const orange =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==";

const meta = {
  title: "Display/AccountItem",
  component: AccountItem,
  args: { name: "Alice Example", handle: "@alice@example.com" },
  decorators: [
    (Story) => (
      <Box width="sm">
        <Story />
      </Box>
    ),
  ],
} satisfies Meta<typeof AccountItem>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Plain text: the name and the handle, with the avatar decorative. */
export const Plain: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Alice Example")).toBeVisible();
    await expect(canvas.queryByRole("link")).toBeNull();
  },
};

/** A link to the profile, named by the name and the handle. */
export const AsLink: Story = {
  args: { href: "/@alice@example.com" },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "Alice Example @alice@example.com" })).toBeVisible();
  },
};

/** The whole row is the target, as in a follow list. */
export const PaddedList: Story = {
  render: (args) => (
    <Stack as="ul" divided border="y">
      {["Alice Example", "Bob", "Carol"].map((name) => (
        <Box as="li" key={name}>
          <AccountItem {...args} name={name} href="/@alice" layout="padded" />
        </Box>
      ))}
    </Stack>
  ),
};

/** A name without spaces and a long handle wrap instead of widening the row. */
export const LongWords: Story = {
  args: {
    href: "/@alice",
    name: "A".repeat(80),
    handle: "@averyveryveryverylongusername@a-very-long-domain-name.example.com",
  },
};

/** A right-to-left name runs right to left; the handle stays left to right. */
export const RightToLeft: Story = {
  args: { href: "/@alice", name: "أليس إكزامبل", handle: "@alice@example.com" },
};

/** The name with custom emoji. */
export const CustomEmojiName: Story = {
  args: {
    href: "/@alice",
    name: "Alice :blobcat:",
    nameContent: (
      <EmojiText text="Alice :blobcat:" emojis={[{ shortcode: "blobcat", url: orange, static_url: orange }]} />
    ),
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("img", { name: ":blobcat:" })).toBeVisible();
  },
};

/** An action at the end of the row. */
export const WithTrailing: Story = {
  args: { href: "/@alice", trailing: <span aria-label="Reacted with a heart">❤</span> },
};

/** Actions below the account in a narrow container, beside it from a wide one; with detail lines. */
export const WithActions: Story = {
  args: {
    details: "Requested 2 days ago",
    trailing: (
      <>
        <Button>Accept</Button>
        <Button>Reject</Button>
      </>
    ),
    trailingPlacement: "below",
  },
};

/** As wide as its text, inside running content. */
export const Inline: Story = {
  args: { href: "/@alice", layout: "inline" },
};
