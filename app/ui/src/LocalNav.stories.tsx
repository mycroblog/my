// SPDX-License-Identifier: AGPL-3.0-only
// The pages of one screen under its heading: links, the current one marked by a bar under it. At 320 px the links
// wrap onto a second line rather than scroll.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocalNav } from "./LocalNav";
import { PageHeading } from "./PageHeading";
import { Stack } from "./Stack";

const meta = {
  title: "Navigation/LocalNav",
  component: LocalNav,
  args: {
    label: "Finance pages",
    items: [
      { href: "#overview", label: "Overview" },
      { href: "#income", label: "Income" },
      { href: "#costs", label: "Costs", current: true },
      { href: "#imports", label: "Bill imports" },
    ],
  },
  render: (args) => (
    <Stack gap="4">
      <PageHeading>Finance</PageHeading>
      <LocalNav {...args} />
    </Stack>
  ),
} satisfies Meta<typeof LocalNav>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The screen's pages as links in a named `nav`; the current one is `aria-current="page"`. */
export const Default: Story = {
  play: async ({ canvas }) => {
    const nav = canvas.getByRole("navigation", { name: "Finance pages" });
    await expect(within(nav).getAllByRole("link")).toHaveLength(4);
    await expect(within(nav).getByRole("link", { name: "Costs" })).toHaveAttribute("aria-current", "page");
  },
};

/** Long page names (a translation) wrap onto more lines instead of scrolling the page. */
export const LongLabels: Story = {
  args: {
    items: [
      { href: "#overview", label: "Übersicht der Einnahmen und Ausgaben" },
      { href: "#income", label: "Einnahmen", current: true },
      { href: "#costs", label: "Kosten der Infrastruktur" },
      { href: "#imports", label: "Importierte Rechnungen der Anbieter" },
    ],
  },
};
