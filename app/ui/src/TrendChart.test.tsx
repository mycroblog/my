// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { I18nProvider } from "react-aria-components";
import { describe, expect, it, vi } from "vitest";
import { TrendChart, type TrendChartProps, type TrendChartView } from "./TrendChart";

const labels = {
  view: "Show as",
  chart: "Chart",
  table: "Table",
  period: "Day",
  missing: "No data",
  empty: "Nothing was recorded.",
};

const base = {
  title: "Posts per day",
  summary: "Peaked at 9 on Tue.",
  periods: [
    { id: "mon", label: "Mon" },
    { id: "tue", label: "Tue" },
    { id: "wed", label: "Wed" },
  ],
  series: [
    { id: "local", name: "Local", values: [3, 9, 4] },
    { id: "remote", name: "Remote", values: [1, null, 2] },
  ],
  formatValue: (value: number) => `${String(value)} posts`,
  labels,
} satisfies Partial<TrendChartProps>;

/** A chart whose view is the screen's state, as the screen keeps it in the URL. */
function Stateful(props: Partial<TrendChartProps> & { onView?: (view: TrendChartView) => void }) {
  const [view, setView] = useState<TrendChartView>("chart");
  return (
    <TrendChart
      {...base}
      {...props}
      view={view}
      onViewChange={(next) => {
        setView(next);
        props.onView?.(next);
      }}
    />
  );
}

describe("TrendChart", () => {
  it("is one image named by its title and described by its summary", () => {
    render(<Stateful />);
    const chart = screen.getByRole("img", { name: "Posts per day" });
    expect(chart).toHaveAccessibleDescription("Peaked at 9 on Tue.");
    // The value axis starts at zero and steps by a nice amount, formatted by the caller.
    expect(within(chart).getByText("0 posts")).toBeInTheDocument();
    expect(within(chart).getByText("10 posts")).toBeInTheDocument();
    expect(within(chart).getByText("Wed")).toBeInTheDocument();
  });

  it("switches to a table of the same values and back", async () => {
    const user = userEvent.setup();
    const onView = vi.fn();
    render(<Stateful onView={onView} />);
    await user.click(screen.getByRole("radio", { name: "Table" }));
    expect(onView).toHaveBeenLastCalledWith("table");
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    const table = screen.getByRole("table", { name: "Posts per day" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((header) => header.textContent),
    ).toEqual(["Day", "Local", "Remote"]);
    // The header row, then a row per period: Tuesday is the third.
    const tuesday = within(table).getAllByRole("row")[2];
    if (tuesday === undefined) throw new Error("no row of Tuesday");
    expect(within(tuesday).getByRole("rowheader")).toHaveTextContent("Tue");
    expect(
      within(tuesday)
        .getAllByRole("cell")
        .map((cell) => cell.textContent),
    ).toEqual(["9 posts", "No data"]);
    await user.click(screen.getByRole("radio", { name: "Chart" }));
    expect(onView).toHaveBeenLastCalledWith("chart");
    expect(screen.getByRole("img", { name: "Posts per day" })).toBeInTheDocument();
  });

  it("says that there is no data instead of drawing an empty chart", () => {
    render(<Stateful periods={[]} />);
    expect(screen.getByText("Nothing was recorded.")).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
  });

  it("puts the newest period on the left in right-to-left text", () => {
    const xs = (locale: string) => {
      const { container, unmount } = render(
        <I18nProvider locale={locale}>
          <Stateful kind="bar" series={[{ id: "local", name: "Local", values: [3, 9, 4] }]} />
        </I18nProvider>,
      );
      // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- the bars are drawn and have no role.
      const result = [...container.querySelectorAll("rect")].map((bar) => Number(bar.getAttribute("x")));
      unmount();
      return result;
    };
    const ltr = xs("en-US");
    const rtl = xs("ar-EG");
    expect(ltr).toHaveLength(3);
    // Oldest to newest: rising x from the left in English, falling in Arabic.
    expect(ltr[0]).toBeLessThan(ltr[2] ?? 0);
    expect(rtl[0]).toBeGreaterThan(rtl[2] ?? 0);
  });

  it("draws at most three series", () => {
    render(<Stateful series={["a", "b", "c", "d"].map((id) => ({ id, name: `Series ${id}`, values: [1, 2, 3] }))} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.queryByText("Series d")).not.toBeInTheDocument();
  });
});
