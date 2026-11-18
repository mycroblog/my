// SPDX-License-Identifier: AGPL-3.0-only

// The table of channels (#3201, ADR-0110 §5), as assistive technology gets it: a table named by its section, whose
// first row names the columns and whose every other row is named by its kind; a cell a checkbox named by its row and
// its column, which a press and the Space key change; the first row's cell mixed while some of its column are on; a
// cell that cannot be changed disabled and described by its row's reason; a channel a kind does not have an empty cell
// in its column's place; and what a cell says of its save in a status region of its row, there before it speaks, behind
// the name of its channel. How the table stands at a width is a layout test on its stories (Channel table).

import { expect, test } from "bun:test";
import { SaveState } from "@my/design-system";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { ChannelCell, ChannelGap, ChannelRow, ChannelTable, type ChannelColumn } from "./channels";

const columns: readonly ChannelColumn[] = [
  { id: "app", label: "In the app" },
  { id: "push", label: "Push" },
  { id: "email", label: "Email" },
];

/** Draws a table of `columns` named From other people around `rows`, after taking the one before it away. */
function table(rows: ReactNode) {
  cleanup();
  render(
    <ChannelTable label="From other people" kind="Notification" columns={columns}>
      {rows}
    </ChannelTable>,
  );
  return { user: userEvent.setup(), table: within(screen.getByRole("table", { name: "From other people" })) };
}

/** A cell of `column` that is `on`, tells `changed` of a press, and says nothing of a save. */
const cell = (column: string, on: boolean, changed: (on: boolean) => void = () => undefined, more = {}) => (
  <ChannelCell key={column} column={column} isSelected={on} onChange={changed} save={null} {...more} />
);

/** The checkbox of the row and the column named. */
const box = (row: string, column: string) =>
  screen.getByRole<HTMLInputElement>("checkbox", { name: new RegExp(`^${row}\\s*${column}$`) });

/** What a checkbox is at, as it is announced: on, off, or mixed. */
const stateOf = (input: HTMLInputElement) =>
  input.indeterminate || input.getAttribute("aria-checked") === "mixed" ? "mixed" : input.checked ? "on" : "off";

/** What describes `control`, part by part. */
const describedBy = (control: HTMLElement) =>
  (control.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? "");

/** The row whose header begins with `name`. */
function rowOf(name: string): HTMLElement {
  const row = screen.getAllByRole("row").find((one) =>
    within(one)
      .queryAllByRole("rowheader")
      .some((header) => header.textContent.startsWith(name)),
  );
  if (row === undefined) throw new Error(`test: no row is named ${name}`);
  return row;
}

/** What the status regions of the row named `name` say; nothing while they have nothing to say. */
const saidIn = (name: string) =>
  within(rowOf(name))
    .queryAllByRole("status")
    .map((status) => status.textContent)
    .filter((words) => words !== "");

test("is a table named by its section, whose first row names what a row is and each channel, and whose rows are named by their kinds", () => {
  const { table: drawn } = table(
    <>
      <ChannelRow all label="All">
        {columns.map(({ id }) => cell(id, true))}
      </ChannelRow>
      <ChannelRow label="Mentions">{columns.map(({ id }) => cell(id, false))}</ChannelRow>
      <ChannelRow label="New followers">{columns.map(({ id }) => cell(id, false))}</ChannelRow>
    </>,
  );

  expect(drawn.getAllByRole("columnheader").map((header) => header.textContent)).toEqual([
    "Notification",
    "In the app",
    "Push",
    "Email",
  ]);
  const [head, ...rows] = drawn.getAllByRole("row");
  if (head === undefined) throw new Error("test: the table has no row");
  // The columns' names stand in a row of their own, which holds no checkbox.
  expect(within(head).getAllByRole("columnheader")).toHaveLength(4);
  expect(within(head).queryAllByRole("checkbox")).toHaveLength(0);
  expect(rows.map((row) => within(row).getByRole("rowheader").textContent.trim())).toEqual([
    "All",
    "Mentions",
    "New followers",
  ]);
  // A row holds a cell under each channel, in the columns' order.
  for (const row of rows) expect(within(row).getAllByRole("cell")).toHaveLength(columns.length);
});

test("a cell is a checkbox named by its row and its column, on or off as it is given", () => {
  table(
    <ChannelRow label="Mentions">
      {cell("app", true)}
      {cell("push", false)}
      {cell("email", true)}
    </ChannelRow>,
  );

  expect(screen.getAllByRole("checkbox")).toHaveLength(3);
  expect(stateOf(box("Mentions", "In the app"))).toBe("on");
  expect(stateOf(box("Mentions", "Push"))).toBe("off");
  expect(stateOf(box("Mentions", "Email"))).toBe("on");
  // A cell that can be changed needs no reason.
  expect(describedBy(box("Mentions", "Push"))).toEqual([]);
});

test("a press, and the Space key, tell the owner what the cell would become, once, and the cell stays as it is given", async () => {
  const changes: [string, boolean][] = [];
  const { user } = table(
    <ChannelRow label="Mentions">
      {cell("app", true, (on) => changes.push(["app", on]))}
      {cell("push", false, (on) => changes.push(["push", on]))}
      {cell("email", false, (on) => changes.push(["email", on]))}
    </ChannelRow>,
  );

  await user.tab();
  expect(document.activeElement === box("Mentions", "In the app")).toBe(true);
  await user.keyboard(" ");
  await user.click(box("Mentions", "Push"));

  expect(changes).toEqual([
    ["app", false],
    ["push", true],
  ]);
  // The owner keeps the cells: nothing changes until it gives another value.
  expect(stateOf(box("Mentions", "In the app"))).toBe("on");
  expect(stateOf(box("Mentions", "Push"))).toBe("off");
});

test("the first row's cell is mixed while some of its column are on, and a press of it asks for the whole column on", async () => {
  const changes: boolean[] = [];
  const { user } = table(
    <>
      <ChannelRow all label="All">
        {cell("app", true)}
        {cell("push", false, (on) => changes.push(on), { isMixed: true })}
        {cell("email", false)}
      </ChannelRow>
      <ChannelRow label="Mentions">
        {cell("app", true)}
        {cell("push", true)}
        {cell("email", false)}
      </ChannelRow>
    </>,
  );

  expect(stateOf(box("All", "In the app"))).toBe("on");
  expect(stateOf(box("All", "Push"))).toBe("mixed");
  expect(stateOf(box("All", "Email"))).toBe("off");

  await user.click(box("All", "Push"));

  expect(changes).toEqual([true]);
});

test("a cell that cannot be changed is disabled, described by its row's reason, and a press of it tells the owner nothing", async () => {
  const changes: boolean[] = [];
  const fixed = "Always arrives in the app and by email.";
  const off = "“In the app” is off, so it does not arrive any other way.";
  const { user } = table(
    <>
      <ChannelRow label="Closure of this server" reason={fixed}>
        {cell("app", true, (on) => changes.push(on), { isDisabled: true })}
        {cell("push", false, (on) => changes.push(on))}
        {cell("email", true, (on) => changes.push(on), { isDisabled: true })}
      </ChannelRow>
      <ChannelRow label="Favourites" reason={off}>
        {cell("app", false)}
        {cell("push", false, (on) => changes.push(on), { isDisabled: true })}
        {cell("email", false, (on) => changes.push(on), { isDisabled: true })}
      </ChannelRow>
    </>,
  );

  // A fixed cell stands on, an unavailable one off: both say why.
  expect(box("Closure of this server", "In the app").disabled).toBe(true);
  expect(stateOf(box("Closure of this server", "In the app"))).toBe("on");
  expect(describedBy(box("Closure of this server", "In the app"))).toEqual([fixed]);
  expect(describedBy(box("Closure of this server", "Email"))).toEqual([fixed]);
  expect(box("Favourites", "Push").disabled).toBe(true);
  expect(stateOf(box("Favourites", "Push"))).toBe("off");
  expect(describedBy(box("Favourites", "Push"))).toEqual([off]);
  expect(describedBy(box("Favourites", "Email"))).toEqual([off]);
  // The reason is read with the row's name, and is the row's alone.
  expect(within(rowOf("Closure of this server")).getByRole("rowheader").textContent).toMatch(
    new RegExp(`^Closure of this server\\s*${fixed.replaceAll(".", "\\.")}$`),
  );
  // The cells of the row that can be changed stay enabled.
  expect(box("Closure of this server", "Push").disabled).toBe(false);
  expect(box("Favourites", "In the app").disabled).toBe(false);

  await user.click(box("Closure of this server", "In the app"));
  await user.click(box("Favourites", "Email"));

  expect(changes).toEqual([]);
});

test("the Tab key passes a disabled cell", async () => {
  const { user } = table(
    <ChannelRow label="Closure of this server" reason="Always arrives in the app.">
      {cell("app", true, () => undefined, { isDisabled: true })}
      {cell("push", false)}
      {cell("email", false)}
    </ChannelRow>,
  );

  await user.tab();
  expect(document.activeElement === box("Closure of this server", "Push")).toBe(true);
  await user.tab();
  expect(document.activeElement === box("Closure of this server", "Email")).toBe(true);
});

test("a channel a kind does not have is an empty cell in its column's place, with no checkbox", () => {
  table(
    <ChannelRow label="New posts">
      {cell("app", true)}
      {cell("push", false)}
      <ChannelGap />
    </ChannelRow>,
  );

  const cells = within(rowOf("New posts")).getAllByRole("cell");
  expect(cells.map((one) => within(one).queryAllByRole("checkbox").length)).toEqual([1, 1, 0]);
  expect(cells[2]?.textContent).toBe("");
  expect(screen.queryAllByRole("checkbox", { name: /Email/ })).toHaveLength(0);
});

test("what a cell says of its save is in a status region of its row, there before it speaks, and Try again is the row's", async () => {
  let retried = 0;
  const rows = (save: ReactNode) => (
    <>
      <ChannelRow label="Mentions">
        {cell("app", true)}
        <ChannelCell column="push" isSelected={false} onChange={() => undefined} save={save} />
        {cell("email", false)}
      </ChannelRow>
      <ChannelRow label="New followers">{columns.map(({ id }) => cell(id, false))}</ChannelRow>
    </>
  );

  table(rows(null));
  // A live region exists before it speaks: each row keeps its own, empty.
  expect(within(rowOf("Mentions")).queryAllByRole("status")).not.toHaveLength(0);
  expect(saidIn("Mentions")).toEqual([]);

  // A row has several cells and one line: the state names the channel it is about.
  table(rows(<SaveState state="saving" />));
  expect(saidIn("Mentions").join()).toMatch(/^Push\s*Saving…$/);
  // The save is that row's: the other says nothing.
  expect(saidIn("New followers")).toEqual([]);

  table(rows(<SaveState state="saved" />));
  expect(saidIn("Mentions").join()).toMatch(/^Push\s*Saved$/);

  const { user } = table(rows(<SaveState state="failed" onRetry={() => (retried += 1)} />));
  expect(saidIn("Mentions").join()).toMatch(/^Push\s*This could not be saved\.\s*Try again$/);
  expect(within(rowOf("New followers")).queryAllByRole("button")).toHaveLength(0);

  await user.click(within(rowOf("Mentions")).getByRole("button", { name: "Try again" }));

  expect(retried).toBe(1);
});

test("a save state shows in the row beside its reason, which still describes the row's disabled cells", () => {
  const off = "“In the app” is off, so it does not arrive any other way.";
  table(
    <ChannelRow label="Favourites" reason={off}>
      <ChannelCell column="app" isSelected={false} onChange={() => undefined} save={<SaveState state="saving" />} />
      {cell("push", false, () => undefined, { isDisabled: true })}
      {cell("email", false, () => undefined, { isDisabled: true })}
    </ChannelRow>,
  );

  expect(saidIn("Favourites").join()).toMatch(/^In the app\s*Saving…$/);
  expect(describedBy(box("Favourites", "Push"))).toEqual([off]);
});
