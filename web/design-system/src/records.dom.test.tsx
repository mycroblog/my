// SPDX-License-Identifier: AGPL-3.0-only

// The lists of records of the staff's screens (#3258), as assistive technology and the keyboard get them: a table
// named by its label, its first row the columns' names, each record a row headed by its head, its cells in the
// columns' order, a fact's name shown on a phone hidden from what is read; the column it is sorted by and its
// direction (aria-sort), and a press on a column's name that sorts by it or turns it; select all over the rows'
// checkboxes, how many are picked, said, and the actions waiting while none is; the open record marked; the filters
// as a search landmark, a facet saying its value, Clear filters only while one is applied; the pages as a navigation
// of the pages there are. How each stands at a width is its stories' (Records), and how Chromium reads the table cell
// by cell is the catalog's (web/e2e/catalog).

import { expect, mock, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import {
  FilterBar,
  FilterFacet,
  FilterToggle,
  RecordPages,
  RecordTable,
  StateMark,
  type RecordColumn,
  type RecordRow,
  type RecordSort,
} from "./records";

type Key = "category" | "reports" | "state";

const columns: readonly RecordColumn<Key>[] = [
  { key: "category", label: "Category" },
  { key: "reports", label: "Reports", kind: "figures", sorts: "descending" },
  { key: "state", label: "State", kind: "fit", beside: true },
];

const rows: readonly RecordRow<Key>[] = [
  {
    id: "1042",
    head: "@deals4u@promo.example",
    caption: "Reported by @mika",
    cells: { category: "Spam", reports: "12", state: <StateMark state="open">Open</StateMark> },
  },
  {
    id: "1041",
    head: "@kuro@social.example",
    cells: { category: "Harassment", reports: "2", state: <StateMark state="assigned">Assigned</StateMark> },
  },
  { id: "1040", head: "@newsbot@feeds.example", cells: { reports: "1", state: "" } },
];

test("the records are a table: the columns' names first, then a row a record headed by it, its cells by column", () => {
  render(<RecordTable label="Reports" head={{ label: "Account" }} columns={columns} rows={rows} />);

  const table = screen.getByRole("table", { name: "Reports" });
  const [head, ...records] = within(table).getAllByRole("row");
  expect(
    within(head as HTMLElement)
      .getAllByRole("columnheader")
      .map((c) => c.textContent),
  ).toEqual(["Account", "Category", "Reports", "State"]);
  expect(records.map((row) => within(row).getByRole("rowheader").textContent)).toEqual([
    "@deals4u@promo.exampleReported by @mika",
    "@kuro@social.example",
    "@newsbot@feeds.example",
  ]);
  // A fact's name, shown before it on a phone, is no part of the cell's name: the column's header says it.
  expect(
    within(records[0] as HTMLElement)
      .getAllByRole("cell")
      .map((c) => c.textContent.replace(/^(Category|Reports)/, "")),
  ).toEqual(["Spam", "12", "Open"]);
  for (const name of within(table)
    .getAllByText("Category")
    .filter((n) => n.closest('[role="cell"]')))
    expect(name.closest('[aria-hidden="true"]')).not.toBeNull();
  // A record that has none of a fact keeps its cell, empty, so that the columns stay in line.
  expect(within(records[2] as HTMLElement).getAllByRole("cell")).toHaveLength(3);
});

test("the column it is sorted by says so and in which direction; a press sorts by a column or turns its direction", async () => {
  const user = userEvent.setup();
  const onSort = mock<(sort: RecordSort<Key>) => void>(() => undefined);
  function Sorted() {
    const [sort, setSort] = useState<RecordSort<Key>>({ by: "head", direction: "ascending" });
    return (
      <RecordTable
        label="Reports"
        head={{ label: "Account", sorts: "ascending" }}
        columns={columns}
        rows={rows}
        sort={sort}
        onSort={(next) => {
          onSort(next);
          setSort(next);
        }}
      />
    );
  }
  render(<Sorted />);

  const header = (name: string) => screen.getByRole("columnheader", { name: new RegExp(`^${name}`) });
  expect(header("Account").getAttribute("aria-sort")).toBe("ascending");
  expect(header("Reports").hasAttribute("aria-sort")).toBe(false);
  // A column that does not sort has no button.
  expect(within(header("Category")).queryByRole("button")).toBeNull();

  await user.click(screen.getByRole("button", { name: "Reports" }));
  expect(onSort).toHaveBeenLastCalledWith({ by: "reports", direction: "descending" });
  expect(header("Reports").getAttribute("aria-sort")).toBe("descending");
  expect(header("Account").hasAttribute("aria-sort")).toBe(false);

  await user.click(screen.getByRole("button", { name: "Reports" }));
  expect(onSort).toHaveBeenLastCalledWith({ by: "reports", direction: "ascending" });
});

test("the phone's one sort control says the column and the direction, and its menu sorts by a column or turns the direction", async () => {
  const user = userEvent.setup();
  const sorts: RecordSort<Key>[] = [];
  function Sorted() {
    const [sort, setSort] = useState<RecordSort<Key>>({ by: "reports", direction: "descending" });
    return (
      <RecordTable
        label="Reports"
        head={{ label: "Account", sorts: "ascending" }}
        columns={columns}
        rows={rows}
        sort={sort}
        onSort={(next) => {
          sorts.push(next);
          setSort(next);
        }}
      />
    );
  }
  render(<Sorted />);
  const table = screen.getByRole("table", { name: "Reports" });
  /** The strip's sort control, outside the table: the wide table's own are its columns' names. */
  const control = (name: string) => {
    const found = screen.getByRole("button", { name });
    expect(table.contains(found)).toBe(false);
    return found;
  };
  const choose = async (name: string) => {
    const menu = screen.getByRole("menu", { name: "Sort by" });
    await user.click(within(menu).getByRole("menuitemradio", { name }));
  };
  const chosen = () =>
    within(screen.getByRole("menu", { name: "Sort by" }))
      .getAllByRole("menuitemradio")
      .filter((item) => item.getAttribute("aria-checked") === "true")
      .map((item) => item.textContent);

  await user.click(control("Reports Descending"));
  // The columns that sort, and the directions, each the one in force checked.
  expect(
    within(screen.getByRole("menu", { name: "Sort by" }))
      .getAllByRole("menuitemradio")
      .map((item) => item.textContent),
  ).toEqual(["Account", "Reports", "Ascending", "Descending"]);
  expect(chosen()).toEqual(["Reports", "Descending"]);
  await choose("Ascending");
  expect(sorts.at(-1)).toEqual({ by: "reports", direction: "ascending" });
  expect(
    within(table)
      .getByRole("columnheader", { name: /^Reports/ })
      .getAttribute("aria-sort"),
  ).toBe("ascending");

  await user.click(control("Reports Ascending"));
  await choose("Account");
  expect(sorts.at(-1)?.by).toBe("head");
  expect(
    within(table)
      .getByRole("columnheader", { name: /^Account/ })
      .getAttribute("aria-sort"),
  ).not.toBeNull();
  expect(
    within(table)
      .getByRole("columnheader", { name: /^Reports/ })
      .hasAttribute("aria-sort"),
  ).toBe(false);
});

test("rows are picked by their checkboxes or all at once, how many is said, and the actions wait while none is", async () => {
  const user = userEvent.setup();
  const resolve = mock(() => undefined);
  function Picking() {
    const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
    return (
      <RecordTable
        label="Reports"
        head={{ label: "Account" }}
        columns={columns}
        rows={rows}
        selection={{
          selected,
          onChange: setSelected,
          actions: [{ id: "resolve", label: "Resolve…", onPress: resolve }],
        }}
      />
    );
  }
  render(<Picking />);

  // The wide table's select all is in its header; the phone's, in the strip over it, says the same.
  const table = screen.getByRole("table", { name: "Reports" });
  const all = within(table).getByRole("checkbox", { name: "Select all" });
  const phone = screen.getAllByRole("checkbox", { name: "Select all" }).filter((box) => !table.contains(box));
  expect(phone).toHaveLength(1);
  const action = screen.getByRole("button", { name: "Resolve…" });
  expect(screen.getByRole("status").textContent).toBe("None selected");
  expect(action.hasAttribute("disabled")).toBe(true);

  await user.click(screen.getByRole("checkbox", { name: "Select @kuro@social.example" }));
  expect(screen.getByRole("status").textContent).toBe("1 selected");
  expect((all as HTMLInputElement).indeterminate).toBe(true);
  expect((phone[0] as HTMLInputElement).indeterminate).toBe(true);
  expect(action.hasAttribute("disabled")).toBe(false);
  await user.click(action);
  expect(resolve).toHaveBeenCalledTimes(1);

  // From the keyboard: select all picks every row, and again none.
  all.focus();
  await user.keyboard(" ");
  expect(screen.getByRole("status").textContent).toBe("3 selected");
  expect((all as HTMLInputElement).checked).toBe(true);
  await user.keyboard(" ");
  expect(screen.getByRole("status").textContent).toBe("None selected");
});

test("what is picked is of the rows shown: a record no longer shown drops out with the next change", async () => {
  const user = userEvent.setup();
  const told: string[][] = [];
  render(
    <RecordTable
      label="Reports"
      head={{ label: "Account" }}
      columns={columns}
      rows={rows}
      selection={{
        selected: new Set(["1042", "990"]),
        onChange: (next) => told.push([...next].sort()),
        actions: [{ id: "resolve", label: "Resolve…", onPress: () => undefined }],
      }}
    />,
  );
  await user.click(screen.getByRole("checkbox", { name: "Select @kuro@social.example" }));
  expect(told).toEqual([["1041", "1042"]]);
});

/** A record's head by its name, whose chances to break (before each @ and .) a DOM simulation may read as spaces. */
const headNamed = (head: string) => screen.getByRole("button", { name: (name) => name.replace(/\s/g, "") === head });

test("a record opens by a press on its head, and the open one is marked", async () => {
  const user = userEvent.setup();
  const onOpen = mock<(id: string) => void>(() => undefined);
  render(
    <RecordTable
      label="Reports"
      head={{ label: "Account" }}
      columns={columns}
      rows={rows}
      current="1041"
      onOpen={onOpen}
    />,
  );

  await user.click(headNamed("@deals4u@promo.example"));
  expect(onOpen).toHaveBeenCalledWith("1042");
  // A record open beside its list is the current one of the list, not a page.
  expect(headNamed("@kuro@social.example").getAttribute("aria-current")).toBe("true");
  expect(headNamed("@deals4u@promo.example").hasAttribute("aria-current")).toBe(false);
});

test("a table with no row gives its place to what says why", () => {
  render(
    <RecordTable
      label="Reports"
      head={{ label: "Account" }}
      columns={columns}
      rows={[]}
      empty={<p>No open reports.</p>}
    />,
  );
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.getByText("No open reports.")).toBeTruthy();
});

test("the filters are a search landmark: a facet says the value applied, and Clear filters stands only while one is", async () => {
  const user = userEvent.setup();
  const onClear = mock(() => undefined);
  const bar = (applied: boolean) => (
    <FilterBar
      label="Filter reports"
      search={{ label: "Search reports", value: "", onChange: () => undefined, clearLabel: "Clear search" }}
      applied={applied}
      onClear={onClear}
      summary="5 reports"
    >
      <FilterFacet
        label="State"
        any="Any state"
        values={[{ id: "open", label: "Open", icon: "stateOpen" }]}
        chosen={applied ? "open" : undefined}
        onChoose={() => undefined}
      />
      <FilterToggle label="Assigned to me" on={applied} onChange={() => undefined} />
    </FilterBar>
  );
  const { rerender } = render(bar(false));

  const search = screen.getByRole("search", { name: "Filter reports" });
  expect(within(search).getByRole("searchbox", { name: "Search reports" })).toBeTruthy();
  expect(within(search).getByRole("button", { name: "State" })).toBeTruthy();
  expect(within(search).getByRole("button", { name: "Assigned to me" }).getAttribute("aria-pressed")).toBe("false");
  expect(within(search).queryByRole("button", { name: "Clear filters" })).toBeNull();
  expect(within(search).getByRole("status").textContent).toBe("5 reports");

  rerender(bar(true));
  expect(within(search).getByRole("button", { name: "State: Open" })).toBeTruthy();
  expect(within(search).getByRole("button", { name: "Assigned to me" }).getAttribute("aria-pressed")).toBe("true");
  await user.click(within(search).getByRole("button", { name: "Clear filters" }));
  expect(onClear).toHaveBeenCalledTimes(1);
  // Clear filters goes away with what it cleared: the focus moves to the search, never to the page's start.
  expect(document.activeElement === within(search).getByRole("searchbox", { name: "Search reports" })).toBe(true);
});

test("the pages are a navigation that keeps Previous and Next in place, disabled where there is no page", () => {
  const { rerender } = render(<RecordPages next={{ href: "/moderation/reports?after=1035" }} />);
  const pages = screen.getByRole("navigation", { name: "Pages" });
  expect(
    within(pages)
      .getAllByRole("link")
      .map((l) => l.textContent),
  ).toEqual(["Next"]);
  // The first page keeps its Previous where it stands, disabled, so that the focus never falls out of the pages.
  expect(within(pages).getByRole<HTMLButtonElement>("button", { name: "Previous" }).disabled).toBe(true);

  rerender(<RecordPages previous={{ onPress: () => undefined }} next={{ onPress: () => undefined }} />);
  expect(
    within(screen.getByRole("navigation", { name: "Pages" }))
      .getAllByRole("button")
      .map((b) => b.textContent),
  ).toEqual(["Previous", "Next"]);

  rerender(<RecordPages />);
  expect(screen.queryByRole("navigation")).toBeNull();
});
