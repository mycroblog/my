// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Box } from "./Box";
import { Stack } from "./Stack";
import { Hint, Text } from "./Text";

const meta = {
  title: "Layout/Box",
  component: Box,
  args: {
    border: "lg",
    padding: "4",
    children: (
      <Text as="p" wrap="anywhere">
        A framed block whose content flows as text:
        https://a-very-long-subdomain.example.social/users/someone/statuses/112233445566778899
      </Text>
    ),
  },
} satisfies Meta<typeof Box>;

export default meta;
type Story = StoryObj<typeof meta>;

/** `border="lg" padding="4"`: a framed block. */
export const Framed: Story = {};

/** Rows of a divided list that are size containers padded in the block direction (`container paddingY="3"`). */
export const ListRows: Story = {
  render: () => (
    <Stack as="ul" aria-label="Reports" divided>
      {["Spam from a new account", "تقرير باللغة العربية"].map((report) => (
        <Box as="li" key={report} container paddingY="3">
          <Text dir="auto" weight="semibold" align="matchParent">
            {report}
          </Text>{" "}
          <Hint as="span">2 hours ago</Hint>
        </Box>
      ))}
    </Stack>
  ),
};

/** `width="sm"`: a narrow form or field at most `max-w-sm` wide. */
export const NarrowWidth: Story = { args: { border: "md", padding: "3", width: "sm" } };

/** `background="sunken"`: a well inside a raised surface, such as the quote of the post a form replies to. */
export const Sunken: Story = { args: { background: "sunken" } };
