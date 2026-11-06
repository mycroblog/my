// SPDX-License-Identifier: AGPL-3.0-only
// ListTable stories: the list of an admin screen as a table in a wide container and as cards in a narrow one (320 px),
// from the same column definitions. The plays check that each form names its rows and keeps every value's header.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Button } from "./Button";
import { Card } from "./Card";
import { Link } from "./Link";
import { ListTable, type ListTableColumns } from "./ListTable";
import { Text } from "./Text";

type Block = { id: string; domain: string; state: "active" | "lifted"; issued: string };

const blocks: Block[] = [
  { id: "1", domain: "spam.example", state: "active", issued: "Oct 1, 2026, 9:30 AM" },
  {
    id: "2",
    domain: "a-very-long-subdomain-of-a-server-that-has-no-spaces-at-all.example.social",
    state: "active",
    issued: "Sep 28, 2026, 11:02 PM",
  },
  { id: "3", domain: "مثال.example", state: "lifted", issued: "Aug 3, 2026, 4:15 PM" },
];

const columns: ListTableColumns<Block> = [
  {
    id: "domain",
    header: "Domain",
    wrap: "anywhere",
    cell: (block) => (
      <Link href={`#${block.id}`} dir="auto">
        {block.domain}
      </Link>
    ),
  },
  {
    id: "state",
    header: "State",
    cell: (block) => (
      <Text {...(block.state === "active" ? { weight: "semibold" as const } : {})}>
        {block.state === "active" ? "In effect" : "Lifted"}
      </Text>
    ),
  },
  { id: "issued", header: "Issued", cell: (block) => block.issued },
  {
    id: "actions",
    kind: "actions",
    header: "Actions",
    cell: (block) =>
      block.state === "active" && (
        <Button size="sm" aria-label={`Lift the block of ${block.domain}`}>
          Lift
        </Button>
      ),
  },
];

const meta = {
  title: "Display/ListTable",
  component: ListTable<Block>,
  args: { items: blocks, columns, getId: (block) => block.id, caption: "Domain blocks in effect", headingLevel: 2 },
  render: (args) => (
    <Card padding="0">
      <ListTable {...args} />
    </Card>
  ),
} satisfies Meta<typeof ListTable<Block>>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A wide container: a table, the domain as each row's header and the actions' header read but not shown. */
export const Wide: Story = {
  play: async ({ canvas }) => {
    const table = canvas.getByRole("table", { name: "Domain blocks in effect" });
    await expect(within(table).getAllByRole("columnheader")).toHaveLength(4);
    await expect(within(table).getByRole("rowheader", { name: "spam.example" })).toBeVisible();
    await expect(within(table).getAllByRole("button", { name: /^Lift the block/ })).toHaveLength(2);
  },
};

/** A container of 320 px: one card per row, the domain as its heading and every other value under its column header. */
export const Narrow: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxInlineSize: "320px" }}>
        <Story />
      </div>
    ),
  ],
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("table")).toBeNull();
    const list = canvas.getByRole("list", { name: "Domain blocks in effect" });
    const cards = within(list).getAllByRole("listitem");
    await expect(cards).toHaveLength(3);
    for (const card of cards) {
      await expect(within(card).getByRole("heading", { level: 2 })).toBeVisible();
      await expect(
        within(card)
          .getAllByRole("term")
          .map((term) => term.textContent),
      ).toEqual(["State", "Issued"]);
    }
    await expect(within(list).getAllByRole("button", { name: /^Lift the block/ })).toHaveLength(2);
  },
};
