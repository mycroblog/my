// SPDX-License-Identifier: AGPL-3.0-only
// Section stories: a titled region on each surface, with a description, and busy while its content loads.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Button } from "./Button";
import { Section } from "./Section";
import { Stack } from "./Stack";
import { Text } from "./Text";

const meta = {
  title: "Layout/Section",
  component: Section,
  args: {
    title: "Password",
    children: (
      <>
        <Text as="p">Change the password you sign in with.</Text>
        <div>
          <Button size="sm">Change password</Button>
        </div>
      </>
    ),
  },
} satisfies Meta<typeof Section>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A region laid on the page: the `h2` names the `section`. */
export const Default: Story = {
  play: async ({ canvas }) => {
    const heading = canvas.getByRole("heading", { level: 2, name: "Password" });
    await expect(heading).toBeVisible();
    await expect(canvas.getByRole("region", { name: "Password" })).toContainElement(heading);
  },
};

/** The description sits under the title and describes the region. */
export const WithDescription: Story = {
  args: { description: "Use at least 12 characters." },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("region", { name: "Password" })).toHaveAccessibleDescription(
      "Use at least 12 characters.",
    );
  },
};

/** `surface="card"`: a region of a detail page or a pane on a raised card. */
export const Card: Story = { args: { surface: "card" } };

/** `surface="band"`: a notice at the top of a list, with a rule under it. */
export const Band: Story = { args: { surface: "band", title: "Announcements" } };

/** A chapter of an editorial page (the landing page): its number and serif title, the content at the measure of running text. */
export const Chapter: Story = { args: { surface: "chapter", number: 2, title: "What do we agree to?" } };

/** `surface="rows"`: full-width rows that bring their own padding; the heading takes the same padding. */
export const Rows: Story = {
  args: {
    surface: "rows",
    title: "Replies",
    children: (
      <Stack as="ul" divided>
        <Stack as="li" paddingX="4" paddingY="3">
          <Text>The first reply</Text>
        </Stack>
        <Stack as="li" paddingX="4" paddingY="3">
          <Text>The second reply</Text>
        </Stack>
      </Stack>
    ),
  },
};

/** `busy` marks the region while its content loads. */
export const Busy: Story = {
  args: { busy: true },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("region", { name: "Password" })).toHaveAttribute("aria-busy", "true");
  },
};
