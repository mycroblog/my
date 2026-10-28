// SPDX-License-Identifier: AGPL-3.0-only
// Grid stories: columns that appear from a small or a medium container width.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Grid } from "./Grid";
import { Stack } from "./Stack";
import { Text } from "./Text";

const items = [
  "Read the timeline",
  "Write posts",
  "Manage reports",
  "Manage the custom emoji of the instance",
  "Invite people",
];

const meta = {
  title: "Layout/Grid",
  component: Grid,
  args: {
    as: "ul",
    gap: "6",
    rowGap: "1",
    children: items.map((item) => (
      <li key={item}>
        <Text wrap="anywhere">{item}</Text>
      </li>
    )),
  },
  decorators: [
    (Story) => (
      <Stack container>
        <Story />
      </Stack>
    ),
  ],
} satisfies Meta<typeof Grid>;

export default meta;
type Story = StoryObj<typeof meta>;

/** One column, then two from @sm: `gap="6" rowGap="1"`. */
export const TwoColumns: Story = {};

/** Three columns from @md. */
export const ThreeColumnsAtMedium: Story = { args: { columns: 3, at: "md", gap: "2", rowGap: "2" } };
