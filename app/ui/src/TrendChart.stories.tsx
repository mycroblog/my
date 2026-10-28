// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn } from "storybook/test";
import { TrendChart, type TrendChartProps, type TrendChartView } from "./TrendChart";

const labels = {
  view: "Show as",
  chart: "Chart",
  table: "Table",
  period: "Day",
  missing: "No data",
  empty: "Nothing was recorded in this period.",
};

const days = ["Oct 22", "Oct 23", "Oct 24", "Oct 25", "Oct 26", "Oct 27", "Oct 28"].map((label) => ({
  id: label,
  label,
}));

const count = new Intl.NumberFormat("en");

/** A chart on a section's raised surface, keeping the view as a screen keeps it in the URL. */
function Chart({
  view: initial = "chart",
  onViewChange,
  ...props
}: Partial<TrendChartProps> & Pick<TrendChartProps, "series" | "periods">) {
  const [view, setView] = useState<TrendChartView>(initial);
  return (
    <div className="rounded-lg bg-surface-raised p-4">
      <TrendChart
        title="Posts per day"
        summary="Peaked at 120 on Oct 26; 84 on Oct 28, up 12% from the day before."
        formatValue={(value) => count.format(value)}
        labels={labels}
        {...props}
        view={view}
        onViewChange={(next) => {
          setView(next);
          onViewChange?.(next);
        }}
      />
    </div>
  );
}

const meta = {
  title: "Data/TrendChart",
  component: Chart,
  args: {
    periods: days,
    series: [
      { id: "local", name: "Local posts", values: [40, 62, 75, 120, 96, 75, 84] },
      { id: "remote", name: "Posts from other servers", values: [20, 35, null, 48, 52, 44, 61] },
    ],
    onViewChange: fn(),
  },
} satisfies Meta<typeof Chart>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Lines: one image named by the title and described by the summary, a legend in text, and the switch to the table. */
export const Lines: Story = {
  play: async ({ canvas }) => {
    const chart = canvas.getByRole("img", { name: "Posts per day" });
    await expect(chart).toHaveAccessibleDescription(
      "Peaked at 120 on Oct 26; 84 on Oct 28, up 12% from the day before.",
    );
    await expect(canvas.getByRole("radio", { name: "Chart" })).toBeChecked();
    // The legend names each series in text.
    await expect(canvas.getAllByRole("listitem").map((item) => item.textContent)).toEqual([
      "Local posts",
      "Posts from other servers",
    ]);
  },
};

/** Three series of bars over a day's hours: the axis labels a few hours only, the first and the last always. */
export const Bars: Story = {
  args: {
    kind: "bar",
    title: "Errors per hour",
    summary: "Most at 09:00 with 14; 3 in the last hour.",
    periods: Array.from({ length: 12 }, (_, hour) => {
      const label = `${String(hour * 2).padStart(2, "0")}:00`;
      return { id: label, label };
    }),
    series: [
      { id: "5xx", name: "Server errors", values: [2, 1, 0, 3, 9, 14, 7, 5, 4, 2, 6, 3] },
      { id: "timeout", name: "Timeouts", values: [1, 0, 0, 1, 4, 6, 2, null, 1, 0, 2, 1] },
      { id: "rejected", name: "Rejected deliveries", values: [0, 0, 1, 1, 2, 3, 3, 2, 1, 1, 1, 0] },
    ],
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("img", { name: "Errors per hour" })).toBeVisible();
    await expect(canvas.getByText("00:00")).toBeVisible();
    await expect(canvas.getByText("22:00")).toBeVisible();
  },
};

/** A balance below zero: the bars grow down from the zero line. */
export const Negative: Story = {
  args: {
    kind: "bar",
    title: "Balance per month",
    summary: "Lowest in August at −30; 45 in October.",
    formatValue: (value) => count.format(value),
    periods: ["Jul", "Aug", "Sep", "Oct"].map((label) => ({ id: label, label })),
    series: [{ id: "balance", name: "Balance", values: [12, -30, 8, 45] }],
  },
};

/** The switch shows the same values as a table, a row per period and a column per series; a missing value says so. */
export const Table: Story = {
  play: async ({ canvas, userEvent, args }) => {
    await userEvent.click(canvas.getByRole("radio", { name: "Table" }));
    await expect(args.onViewChange).toHaveBeenLastCalledWith("table");
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
    const table = canvas.getByRole("table", { name: "Posts per day" });
    await expect(table).toBeVisible();
    await expect(canvas.getByRole("columnheader", { name: "Posts from other servers" })).toBeVisible();
    await expect(canvas.getByRole("rowheader", { name: "Oct 24" })).toBeVisible();
    await expect(canvas.getByRole("cell", { name: "No data" })).toBeVisible();
  },
};

/** Forced colors draw every series in CanvasText with its dash and the newest values in Highlight; captured so. */
export const ForcedColors: Story = {
  tags: ["forced-colors"],
  args: {
    series: [
      { id: "local", name: "Local posts", values: [40, 62, 75, 120, 96, 75, 84] },
      { id: "remote", name: "Posts from other servers", values: [20, 35, null, 48, 52, 44, 61] },
      { id: "boosts", name: "Boosts", values: [10, 12, 30, 24, 18, 15, 22] },
    ],
  },
};

/** Bars in forced colors: the first series filled, the others outlined with their dash. */
export const ForcedColorsBars: Story = {
  tags: ["forced-colors"],
  args: {
    ...ForcedColors.args,
    kind: "bar",
  },
};

/** Without a period there is no chart and no switch, only the sentence that says so. */
export const Empty: Story = {
  args: { periods: [], series: [{ id: "local", name: "Local posts", values: [] }] },
  play: async ({ canvas }) => {
    await expect(canvas.getByText(labels.empty)).toBeVisible();
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
    await expect(canvas.queryByRole("radiogroup")).not.toBeInTheDocument();
  },
};

/** A single period: the line is a dot and the bar stands in the middle. */
export const OnePoint: Story = {
  args: {
    summary: "84 on Oct 28.",
    periods: [{ id: "Oct 28", label: "Oct 28" }],
    series: [{ id: "local", name: "Local posts", values: [84] }],
  },
  render: (args) => (
    <div className="flex flex-col gap-4">
      <Chart {...args} />
      <Chart {...args} kind="bar" title="Posts on the day" />
    </div>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getAllByRole("img")).toHaveLength(2);
  },
};
