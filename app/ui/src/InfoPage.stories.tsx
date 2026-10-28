// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Heading } from "./Heading";
import { InfoPage } from "./InfoPage";
import { PageHeading } from "./PageHeading";
import { Stack } from "./Stack";
import { Text } from "./Text";

const sections = ["Description", "Rules", "Terms and policies", "Running costs", "Contact"];

const meta = {
  title: "Layout/InfoPage",
  component: InfoPage,
  parameters: { layout: "fullscreen" },
  args: {
    contentsLabel: "On this page",
    heading: <PageHeading spark="✳">About my.example</PageHeading>,
    children: sections.map((title) => (
      <Stack as="section" key={title} aria-labelledby={`info-${title}`} gap="3">
        <Heading level={2} id={`info-${title}`}>
          {title}
        </Heading>
        <Text as="p">
          A reading column of about 680px. Long texts wrap and never need horizontal scroll, even at 320px.
        </Text>
        <Text as="p">The table of contents follows the headings of the column and marks the section being read.</Text>
      </Stack>
    )),
  },
} satisfies Meta<typeof InfoPage>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The contents fold above the column. */
export const Default: Story = {};

/** `placement="beside"`: where the margin is free, the contents stand sticky beside the column. */
export const Beside: Story = {
  args: { placement: "beside" },
  parameters: { viewport: { defaultViewport: "desktop" } },
};
