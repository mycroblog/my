// SPDX-License-Identifier: AGPL-3.0-only

// The finance table (#3244), as assistive technology gets it: a table named by its label, whose first row names the
// columns and whose every other row is headed by its period or its provider, its cells in the columns' order, a figure
// a currency and the word of an estimate read in its cell, the bars hidden; the total last. How it stands at a width
// is its stories' (Finance), and how Chromium reads it cell by cell is the catalog's (web/e2e/catalog).

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import { FinanceRow, FinanceTable } from "./finance";

const flow = [
  { label: "Costs", figures: true },
  { label: "Income", figures: true },
  { label: "Balance", figures: true },
] as const;

test("the totals are a table: the columns' names first, then a row a period headed by it", () => {
  render(
    <FinanceTable label="Totals by month" head="Month" columns={flow} estimated="Estimate">
      <FinanceRow
        head="November 2026"
        caption="So far"
        cells={[
          { figures: ["¥12,300", "US$45.67"], estimated: true, share: 0.8 },
          { figures: ["¥9,840"], share: 0.6 },
          { figures: ["−¥2,460"] },
        ]}
      />
      <FinanceRow
        head="October 2026"
        cells={[{ figures: ["¥14,950"] }, { figures: ["¥18,400"] }, { figures: ["¥3,450"] }]}
      />
    </FinanceTable>,
  );

  const table = screen.getByRole("table", { name: "Totals by month" });
  const [head, ...rows] = within(table).getAllByRole("row");
  expect(
    within(head as HTMLElement)
      .getAllByRole("columnheader")
      .map((c) => c.textContent),
  ).toEqual(["Month", "Costs", "Income", "Balance"]);
  expect(rows.map((row) => within(row).getByRole("rowheader").textContent)).toEqual([
    "November 2026So far",
    "October 2026",
  ]);
  expect(
    within(rows[0] as HTMLElement)
      .getAllByRole("cell")
      .map((c) => c.textContent),
  ).toEqual(["¥12,300 EstimateUS$45.67 Estimate", "¥9,840", "−¥2,460"]);
  // The bars draw what the figures say, and assistive technology hears the figures alone.
  for (const bar of table.querySelectorAll('[aria-hidden="true"]')) expect(bar.textContent).toBe("");
});

test("a breakdown's total is its last row, its cells in the columns' order", () => {
  render(
    <FinanceTable
      label="Costs by provider"
      head="Provider"
      columns={[{ label: "Category" }, { label: "Amount", figures: true }]}
      estimated="Estimate"
      total={<FinanceRow head="Total" cells={["", { figures: ["¥10,650"] }]} />}
    >
      <FinanceRow head="Sakura" cells={["Servers", { figures: ["¥10,650"] }]} />
    </FinanceTable>,
  );

  const rows = within(screen.getByRole("table", { name: "Costs by provider" })).getAllByRole("row");
  expect(rows.map((row) => row.textContent)).toEqual([
    "ProviderCategoryAmount",
    "SakuraServers¥10,650",
    "Total¥10,650",
  ]);
});

test("a provider's next category keeps its provider as its row's header, and an estimate is read on its figure alone", () => {
  render(
    <FinanceTable
      label="Costs by provider"
      head="Provider"
      columns={[{ label: "Category" }, { label: "Amount", figures: true }]}
      estimated="Estimate"
      about="Calendar months, by date."
    >
      <FinanceRow head="AWS" cells={["Servers", { figures: ["US$38.20"] }]} />
      <FinanceRow
        head="AWS"
        continued
        cells={["Storage", { figures: ["¥100", { shown: "US$7.47", estimated: true }] }]}
      />
    </FinanceTable>,
  );

  const table = screen.getByRole("table", { name: "Costs by provider", description: "Calendar months, by date." });
  const rows = within(table).getAllByRole("row");
  expect(rows.map((row) => within(row).queryByRole("rowheader")?.textContent ?? null)).toEqual([null, "AWS", "AWS"]);
  expect(
    within(rows[2] as HTMLElement)
      .getAllByRole("cell")
      .map((c) => c.textContent),
  ).toEqual(["Storage", "¥100US$7.47 Estimate"]);
});

test("a cell of figures in two currencies draws no bar, whatever its share", () => {
  render(
    <FinanceTable label="Totals" head="Month" columns={[{ label: "Costs", figures: true }]}>
      <FinanceRow head="November 2026" cells={[{ figures: ["¥12,300", "US$45.67"], share: 0.5 }]} />
      <FinanceRow head="October 2026" cells={[{ figures: ["¥14,950"], share: 1 }]} />
    </FinanceTable>,
  );

  const rows = within(screen.getByRole("table", { name: "Totals" })).getAllByRole("row");
  expect(rows.map((row) => row.querySelectorAll('span[aria-hidden="true"]').length)).toEqual([0, 0, 1]);
});
