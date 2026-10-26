// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { AccountItem } from "./AccountItem";
import { AsidePanel } from "./AppShell";
import { Box } from "./Box";
import { Stack } from "./Stack";
import { VoiceRoomCard } from "./VoiceRoomCard";

const meta = {
  title: "Display/VoiceRoomCard",
  component: VoiceRoomCard,
  args: {
    name: "Friday night radio",
    href: "/voice/0192f0c4-7a8e-7000-8000-0000000000f1",
    host: <AccountItem name="Alice Example" handle="@alice" layout="inline" />,
    participants: "12 people",
    description: "Talking about the week's music.\nRequests welcome.",
    tags: ["#music", "#radio"],
    tagsLabel: "Hashtags",
  },
} satisfies Meta<typeof VoiceRoomCard>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The list of a page that the cards are items of. */
const inList: NonNullable<Story["decorators"]> = [
  (Story) => (
    <Box width="prose">
      <Stack as="ul" gap="3">
        <Story />
      </Stack>
    </Box>
  ),
];

/** A room of the list page: the name links to the room, then the host, the count, the description, and the hashtags. */
export const Default: Story = {
  decorators: inList,
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { level: 2, name: "Friday night radio" })).toBeVisible();
    await expect(canvas.getByRole("link", { name: "Friday night radio" })).toHaveAttribute("href", "/voice/0192f0c4-7a8e-7000-8000-0000000000f1");
    await expect(canvas.getByRole("list", { name: "Hashtags" })).toBeVisible();
  },
};

/** Without a description and hashtags. */
export const NameOnly: Story = { decorators: inList, args: { description: "", tags: [] } };

/** A long name and text in a right-to-left script wrap inside the card. */
export const LongRightToLeft: Story = {
  decorators: inList,
  args: {
    name: "غرفة الحديث الأسبوعية عن الموسيقى والكتب والأفلام والسفر وكل ما يخطر على البال",
    description: "نتحدث عن موسيقى الأسبوع",
    tags: ["#موسيقى"],
  },
};

/** Rows of the third column's panel, which is a card already: no card of their own, no description, no hashtags. */
export const InAPanel: Story = {
  decorators: [
    (Story) => (
      <Box width="sm">
        <AsidePanel heading="Voice chat">
          <Stack as="ul" gap="3">
            <Story />
          </Stack>
        </AsidePanel>
      </Box>
    ),
  ],
  args: { surface: "plain", headingLevel: 3 },
  play: async ({ canvas }) => {
    await expect(canvas.queryByText("Requests welcome.", { exact: false })).toBeNull();
    await expect(canvas.queryByRole("list", { name: "Hashtags" })).toBeNull();
  },
};
