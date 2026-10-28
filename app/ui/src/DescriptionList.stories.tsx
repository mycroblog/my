// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DescriptionItem, DescriptionList } from "./DescriptionList";

const meta = {
  title: "Display/DescriptionList",
  component: DescriptionList,
  args: {
    columns: 3,
    children: (
      <>
        <DescriptionItem term="State">Unavailable</DescriptionItem>
        <DescriptionItem term="Since">17 Oct 2026, 09:30</DescriptionItem>
        <DescriptionItem term="Days with failures">7 days</DescriptionItem>
        <DescriptionItem term="Last failure">16 Oct 2026</DescriptionItem>
        <DescriptionItem term="Reason">Deliveries failed on 7 days</DescriptionItem>
        <DescriptionItem term="Deliveries last resumed">None</DescriptionItem>
      </>
    ),
  },
} satisfies Meta<typeof DescriptionList>;

export default meta;
type Story = StoryObj<typeof meta>;

/** One column in a narrow container, two from @sm, three from @2xl. */
export const ThreeColumns: Story = {};

export const FourColumns: Story = { args: { columns: 4 } };

/** Long values from people and servers wrap anywhere instead of scrolling the page. */
export const LongValues: Story = {
  args: {
    columns: 1,
    children: (
      <>
        <DescriptionItem term="Idempotency key">
          <code dir="ltr">
            deliver:https://very-long-subdomain.example.social/users/someone/statuses/112233445566778899
          </code>
        </DescriptionItem>
        <DescriptionItem term="Last error">
          <span dir="auto">Post &quot;https://example.com/inbox&quot;: dial tcp: lookup example.com: no such host</span>
        </DescriptionItem>
      </>
    ),
  },
};
