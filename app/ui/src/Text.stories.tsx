// SPDX-License-Identifier: AGPL-3.0-only
// Text, Hint and InlineError stories: the roles, wrapping of long words and people's words in any direction, and an
// error that is announced only when it is the result of the user's action.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Stack } from "./Stack";
import { Hint, InlineError, Text } from "./Text";

const meta = {
  title: "Display/Text",
  component: Text,
  args: { children: "Body text in the size and color around it." },
} satisfies Meta<typeof Text>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

/** The sizes, tones and weights a run of text may take. */
export const Roles: Story = {
  render: () => (
    <Stack gap="2">
      <Text as="p" size="lg">
        Focused content
      </Text>
      <Text as="p">Body</Text>
      <Text as="p" weight="semibold">
        A name
      </Text>
      <Text as="p" weight="medium">
        A label of a group
      </Text>
      <Text as="p" size="xs">
        A label
      </Text>
      <Text as="p" tone="warning">
        A warning
      </Text>
      <Text as="p" tone="success">
        A success
      </Text>
      <Text as="p" numeric>
        1,234,567.89
      </Text>
      <Text as="code" mono selectAll dir="ltr">
        INVITE-7F3K-2Q9P
      </Text>
    </Stack>
  ),
};

/** Long words wrap anywhere; people's words keep their line breaks and line up with the page's direction (G9). */
export const LongWords: Story = {
  render: () => (
    <Stack gap="3">
      <Text as="p" wrap="anywhere">
        https://a-very-long-subdomain.example.social/users/someone_with_a_long_name/statuses/112233445566778899
      </Text>
      <Text block dir="auto" align="matchParent" whitespace="preWrap" wrap="anywhere">
        {"مرحبا بالجميع\nهذا سطر ثان"}
      </Text>
      <Text as="p" lineClamp={2} wrap="anywhere">
        A memo that goes on for a long while, much longer than two lines on a phone, so that the end of it is cut off
        with an ellipsis instead of pushing the rest of the row down the page.
      </Text>
    </Stack>
  ),
};

/** Secondary text (`text-sm text-fg-muted`), and the same tone at the size around it. */
export const Hints: Story = {
  render: () => (
    <Stack gap="2">
      <Hint>Members see this list. Only staff can change it.</Hint>
      <Hint size="inherit">Nothing was paid in this period.</Hint>
      <Hint as="span" dir="auto" whitespace="preLine" wrap="anywhere">
        {"Paid yearly\nRenewed on 1 April"}
      </Hint>
    </Stack>
  ),
};

/** An error that came with the page is read in place, not announced. */
export const InlineErrorStatic: Story = {
  render: () => <InlineError>Couldn&apos;t load the totals: the server took too long to answer.</InlineError>,
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
  },
};

/** The failure of the user's own action is announced (`role="alert"`), here at the size around it. */
export const InlineErrorAlert: Story = {
  render: () => (
    <InlineError alert size="inherit">
      Couldn&apos;t save the changes: a role with this key already exists.
    </InlineError>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("alert")).toHaveTextContent("Couldn't save the changes");
  },
};

/** The small upper-case label above a heading. */
export const Eyebrow: Story = {
  render: () => (
    <Text as="p" eyebrow>
      Welcome to my
    </Text>
  ),
};
