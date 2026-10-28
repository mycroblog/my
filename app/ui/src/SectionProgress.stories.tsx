// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Section } from "./Section";
import { SectionProgress } from "./SectionProgress";
import { Text } from "./Text";

const parts = [
  { id: "story-place", title: "What kind of place is this?" },
  { id: "story-rules", title: "What do we agree to?" },
  { id: "story-join", title: "How do I join?" },
];

const meta = {
  title: "Layout/SectionProgress",
  component: SectionProgress,
  args: {
    label: "Parts of this page",
    parts,
    children: parts.map((part) => (
      <Section key={part.id} title={part.title} headingId={part.id}>
        <Text as="p">Each part is a section headed by the question it answers; the line above links to each.</Text>
      </Section>
    )),
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("navigation", { name: "Parts of this page" })).toBeVisible();
    await expect(canvas.getAllByRole("link")).toHaveLength(3);
  },
} satisfies Meta<typeof SectionProgress>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The first part is being read; the line follows the reader as the page scrolls. */
export const Default: Story = {};

/** Long titles wrap under their numbers instead of scrolling the page. */
export const LongTitles: Story = {
  args: {
    parts: parts.map((part) => ({ ...part, title: `${part.title} A title long enough to wrap at narrow widths` })),
  },
};
