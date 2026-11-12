// SPDX-License-Identifier: AGPL-3.0-only
// Heading stories: the roles by level, a look apart from the level, long names and codes, and the heading that focus
// moves to from script.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Button } from "./Button";
import { Heading } from "./Heading";
import { Stack } from "./Stack";

const meta = {
  title: "Display/Heading",
  component: Heading,
  args: { level: 2, children: "Replies" },
} satisfies Meta<typeof Heading>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Level 2 is a section title (`text-lg font-semibold`). */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { level: 2, name: "Replies" })).toBeVisible();
  },
};

/** The look follows the level by default: page, section, then subsection. */
export const Levels: Story = {
  render: () => (
    <Stack gap="2">
      <Heading level={1}>Administration</Heading>
      <Heading level={2}>Roles</Heading>
      <Heading level={3}>Moderator</Heading>
      <Heading level={4}>Rights</Heading>
    </Stack>
  ),
};

/** `size` sets the look apart from the level: an `h2` at body size, an `h2` as a small group label. */
export const SizeApartFromLevel: Story = {
  render: () => (
    <Stack gap="2">
      <Heading level={2} size="subsection">
        @alice
      </Heading>
      <Heading level={2} size="label" tone="fg">
        Notifications
      </Heading>
    </Stack>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getAllByRole("heading", { level: 2 })).toHaveLength(2);
  },
};

/** Names, domains and codes wrap anywhere; a code is monospaced and reads left to right in every locale. */
export const LongWords: Story = {
  render: () => (
    <Stack gap="2">
      <Heading level={3} wrap="anywhere">
        a-very-long-subdomain-of-a-federated-server-that-has-no-spaces.example.social
      </Heading>
      <Heading level={3} mono wrap="anywhere" dir="ltr">
        fetch_remote_account_with_a_very_long_job_type_name
      </Heading>
      <Heading level={3} dir="auto">
        خادم المثال
      </Heading>
    </Stack>
  ),
};

/** A heading that focus moves to after an action removed the focused control: focusable from script, not by Tab. */
export const Focusable: Story = {
  render: () => (
    <Stack gap="2" align="start">
      <Heading level={3} id="heading-focus-target" focusable wrap="anywhere">
        example.com
      </Heading>
      <Button onPress={() => document.getElementById("heading-focus-target")?.focus()}>Lift the suspension</Button>
    </Stack>
  ),
  play: async ({ canvas, userEvent }) => {
    const heading = canvas.getByRole("heading", { level: 3, name: "example.com" });
    await expect(heading).toHaveAttribute("tabindex", "-1");
    const button = canvas.getByRole("button", { name: "Lift the suspension" });
    await userEvent.click(button);
    await expect(heading).toHaveFocus();
    // Back to the resting state.
    await userEvent.unhover(button);
  },
};
