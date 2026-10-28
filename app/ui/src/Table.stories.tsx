// SPDX-License-Identifier: AGPL-3.0-only
// Table stories: totals named by a hidden caption, a compact table named by a visible heading, and long values that
// wrap instead of scrolling the page. The plays check the names of the table, its columns and its rows.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Cluster } from "./Cluster";
import { Heading } from "./Heading";
import { Stack } from "./Stack";
import { Table, TableBody, TableCell, TableColumnHeader, TableHead, TableRow, TableRowHeader } from "./Table";
import { Tag } from "./Tag";

const periods = [
  { name: "October 2026", total: "$12.40 · €3.10", estimated: true },
  {
    name: "September 2026",
    total: "$1,234.56 · €78.90 · ¥12,000 · £4.20 · CHF 9.99 · A$15.00 · C$7.25",
    estimated: false,
  },
  { name: "August 2026", total: "$0.00", estimated: false },
];

const meta = {
  title: "Display/Table",
  component: Table,
  args: {
    caption: "Totals by month, as paid",
    children: (
      <>
        <TableHead>
          <TableColumnHeader>Period</TableColumnHeader>
          <TableColumnHeader>Total</TableColumnHeader>
        </TableHead>
        <TableBody>
          {periods.map((period) => (
            <TableRow key={period.name}>
              <TableRowHeader>{period.name}</TableRowHeader>
              <TableCell numeric wrap="anywhere">
                <Cluster as="span" align="center" gap="2">
                  {period.total}
                  {period.estimated && <Tag>Estimated</Tag>}
                </Cluster>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </>
    ),
  },
} satisfies Meta<typeof Table>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Totals at body size with a rule under every row, named by a visually hidden caption. */
export const Regular: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("table", { name: "Totals by month, as paid" })).toBeVisible();
    await expect(canvas.getByRole("columnheader", { name: "Total" })).toBeVisible();
    await expect(canvas.getByRole("rowheader", { name: "October 2026" })).toBeVisible();
  },
};

/** Many short values at `text-sm` in fixed columns, named by the heading above it; values in any script wrap anywhere. */
export const Compact: Story = {
  args: { caption: undefined, density: "compact", "aria-labelledby": "table-skipped" },
  render: (args) => (
    <Stack gap="2">
      <Heading level={3} id="table-skipped">
        Skipped lines
      </Heading>
      <Table {...args}>
        <TableHead>
          <TableColumnHeader width="narrow">Line</TableColumnHeader>
          <TableColumnHeader>Value</TableColumnHeader>
          <TableColumnHeader width="wide">Reason</TableColumnHeader>
        </TableHead>
        <TableBody>
          {[
            [3, "spam.example", "Already blocked"],
            [
              12,
              "a-very-long-subdomain-of-a-server-that-has-no-spaces-at-all.example.social",
              "Not a domain or an address",
            ],
            [27, "مثال.example", "Already blocked"],
          ].map(([line, value, reason]) => (
            <TableRow key={String(line)}>
              <TableCell numeric>{`Line ${String(line)}`}</TableCell>
              <TableCell dir="auto" wrap="anywhere" align="matchParent">
                {value}
              </TableCell>
              <TableCell>{reason}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Stack>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("table", { name: "Skipped lines" })).toBeVisible();
    await expect(canvas.getAllByRole("row")).toHaveLength(4);
  },
};
