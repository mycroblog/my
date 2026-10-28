// SPDX-License-Identifier: AGPL-3.0-only
// VoiceRoomCard stories: a room of the list page in the look of the stage (the Live pill, the count and the time open,
// the name, the stacked speakers beside the host, the description and hashtags), one with only its host, a long name
// in a right-to-left script, and the rows of the third column's panel.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { AsidePanel } from "./AppShell";
import { Box } from "./Box";
import { Link } from "./Link";
import { Stack } from "./Stack";
import { VoiceRoomCard } from "./VoiceRoomCard";

const host = (
  <Link href="/@alice" variant="standalone">
    <span dir="auto">Alice Example</span>
  </Link>
);

const meta = {
  title: "Display/VoiceRoomCard",
  component: VoiceRoomCard,
  args: {
    name: "Friday night radio",
    href: "/voice/0192f0c4-7a8e-7000-8000-0000000000f1",
    liveLabel: "Live",
    stats: "12 people · 38 min",
    speakers: [
      { id: "1", name: "Alice Example" },
      { id: "2", name: "Sota" },
      { id: "3", name: "Mio" },
      { id: "4", name: "Kenji" },
      { id: "5", name: "Yui" },
    ],
    host,
    hostLabel: "Host",
    description: "Talking about the week's music.\nRequests welcome.",
    tags: [
      { label: "#music", href: "/tags/music" },
      { label: "#radio", href: "/tags/radio" },
    ],
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

/** A room of the list page: the Live pill and the stats, the name that links to the room, the speakers and the host, the description, and the hashtags. */
export const Default: Story = {
  decorators: inList,
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Live")).toBeVisible();
    await expect(canvas.getByText("12 people · 38 min")).toBeVisible();
    await expect(canvas.getByRole("heading", { level: 2, name: "Friday night radio" })).toBeVisible();
    await expect(canvas.getByRole("link", { name: "Friday night radio" })).toHaveAttribute(
      "href",
      "/voice/0192f0c4-7a8e-7000-8000-0000000000f1",
    );
    await expect(canvas.getByRole("link", { name: "Alice Example" })).toBeVisible();
    await expect(canvas.getByRole("list", { name: "Hashtags" })).toBeVisible();
  },
};

/** The room of the user's call: the "Joined" badge beside the Live pill. */
export const Joined: Story = {
  decorators: inList,
  args: { joinedLabel: "Joined" },
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Joined")).toBeVisible();
    await expect(canvas.getByRole("link", { name: "#music" })).toHaveAttribute("href", "/tags/music");
  },
};

/** Only the host speaks, without a description and hashtags. */
export const NameOnly: Story = {
  decorators: inList,
  args: { description: "", tags: [], stats: "1 person · 2 min", speakers: [{ id: "1", name: "Alice Example" }] },
};

/** A long name and text in a right-to-left script wrap inside the card. */
export const LongRightToLeft: Story = {
  decorators: inList,
  args: {
    name: "غرفة الحديث الأسبوعية عن الموسيقى والكتب والأفلام والسفر وكل ما يخطر على البال",
    description: "نتحدث عن موسيقى الأسبوع",
    tags: [{ label: "#موسيقى", href: "/tags/موسيقى" }],
    stats: "12 people · 1 hr 5 min",
  },
};

/** Rows of the third column's panel, which is a card already: no card of their own, no description, no hashtags. */
export const InAPanel: Story = {
  decorators: [
    (Story) => (
      <Box width="sm">
        <AsidePanel heading="Voice chat">
          <Stack as="ul" gap="4">
            <Story />
          </Stack>
        </AsidePanel>
      </Box>
    ),
  ],
  args: { surface: "plain", headingLevel: 3 },
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Live")).toBeVisible();
    await expect(canvas.queryByText("Requests welcome.", { exact: false })).toBeNull();
    await expect(canvas.queryByRole("list", { name: "Hashtags" })).toBeNull();
  },
};
