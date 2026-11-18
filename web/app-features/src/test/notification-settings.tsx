// SPDX-License-Identifier: AGPL-3.0-only

// Notifications, the entry of the settings (#3201), for its tests: the entry drawn alone on a fake server, in a browser
// with the parts it turns on and off (browser.ts) and on a device that keeps nothing from the test before, and how a
// test finds what the entry shows: a table's cells by their row and column, a section by its heading, and what a row
// says of a save.

import { onTestFinished } from "bun:test";
import { instanceQuery, setNotificationSound, setNotificationSoundKinds } from "@my/app-data";
import type { FakeNotificationRow, FakeOptions } from "@my/e2e/fake";
import { cleanup, screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { SettingsEntryScreen } from "../settings/screen";
import { pushBrowser, quietDevice, type PushBrowser, type SeenByBrowser } from "./browser";
import { fakeData, renderFeature, type FakeData } from "./feature";

/** A kind of notification, as the device keeps the kinds that sound. */
type Kind = Parameters<typeof setNotificationSoundKinds>[0][number];

/**
 * The kinds of notification the rows of `rows` hold, sorted: a row's `types`, or the one kind that bears its group's
 * name, as the fake answers a row the seed gives no types.
 */
export const kindsOf = (rows: readonly FakeNotificationRow[]): Kind[] =>
  rows.flatMap((row) => row.types ?? [row.group as Kind]).sort();

/** Where the entry is drawn, besides its server: the browser, and what the device keeps of the sound. */
export type Device = {
  /** The browser's notifications, push and audio; one that asks and subscribes by default. */
  browser?: PushBrowser;
  /** The notification sound is on on this device, for these kinds; off by default, as on a new device. */
  sounding?: readonly Kind[];
  /** The language the entry reads in; `en` by default. */
  locale?: string;
};

/** The entry drawn: its server, the reader's hands, and what the browser saw. */
export type Drawn = FakeData & { user: UserEvent; seen: SeenByBrowser };

/**
 * The entry on a server of `options` that a test may script before the entry reads it (`script`), drawn in the main
 * landmark as the shell places it, once the shell read the instance. The server's answers reach the screen when they
 * come, not inside an act() of the test, so React is told so until the test ends, and a test waits for what it asserts.
 */
export async function drawnSettings(
  options: FakeOptions = {},
  { browser, sounding, locale }: Device = {},
  script: (on: FakeData) => void = () => undefined,
): Promise<Drawn> {
  quietDevice();
  const seen = pushBrowser(browser);
  if (sounding !== undefined) {
    setNotificationSound(true);
    setNotificationSoundKinds(sounding, true);
  }
  const on = fakeData(options);
  script(on);
  const user = await drawnOn(on, locale);
  return { ...on, user, seen };
}

/** Draws the entry on `on`, a server a test made, after taking away what was drawn before; the device stays as it is. */
export async function drawnOn(on: FakeData, locale?: string): Promise<UserEvent> {
  cleanup();
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  await on.data.queryClient.query(instanceQuery(on.data));
  return renderFeature(
    on.data,
    <main>
      <SettingsEntryScreen entry="notifications" onBack={() => undefined} languages={["en", "ja"]} />
    </main>,
    locale,
  );
}

/** The entry of `drawnSettings`, with its tables drawn: the rows and the policy are read. */
export async function readSettings(options: FakeOptions = {}, device: Device = {}): Promise<Drawn> {
  const drawn = await drawnSettings(options, device);
  await screen.findAllByRole("table");
  return drawn;
}

/** The section of the screen titled `name`. */
export const section = (name: string) => within(screen.getByRole("region", { name }));

/** The titles of the screen's sections, in their order. */
export const sections = () =>
  screen.getAllByRole("region").map((part) => within(part).getAllByRole("heading")[0]?.textContent);

/** The switch named `name`. */
export const toggle = (name: string) => screen.getByRole<HTMLInputElement>("switch", { name });

const escaped = (words: string) => words.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** The cells of the table named `table` that stand in the row named `row` under the column named `column`. */
const boxes = (table: string, row: string, column: string) =>
  within(screen.getByRole("table", { name: table })).queryAllByRole<HTMLInputElement>("checkbox", {
    name: new RegExp(`^${escaped(row)}\\s*${escaped(column)}$`),
  });

/** The cell of the table named `table` in the row named `row` under the column named `column`: a checkbox. */
export function cell(table: string, row: string, column: string): HTMLInputElement {
  const [found, ...others] = boxes(table, row, column);
  if (found === undefined || others.length > 0)
    throw new Error(`test: ${String(others.length + (found === undefined ? 0 : 1))} cells are named ${row} ${column}`);
  return found;
}

/** Whether the row named `row` of the table named `table` has a cell under `column`. */
export const hasCell = (table: string, row: string, column: string) => boxes(table, row, column).length > 0;

/** What a cell is at, as it is announced: on, off, or mixed, and whether the reader can change it. */
export const stateOf = (input: HTMLInputElement) =>
  `${input.indeterminate || input.getAttribute("aria-checked") === "mixed" ? "mixed" : input.checked ? "on" : "off"}${input.disabled ? ", disabled" : ""}`;

/** The names of the columns of the table named `table`, after the one that names the rows. */
export const columnsOf = (table: string) =>
  within(screen.getByRole("table", { name: table }))
    .getAllByRole("columnheader")
    .map((header) => header.textContent)
    .slice(1);

/** The row of the table named `table` whose header begins with `name`. */
export function rowOf(table: string, name: string): HTMLElement {
  const row = within(screen.getByRole("table", { name: table }))
    .getAllByRole("row")
    .find((one) =>
      within(one)
        .queryAllByRole("rowheader")
        .some((header) => header.textContent.startsWith(name)),
    );
  if (row === undefined) throw new Error(`test: no row of ${table} is named ${name}`);
  return row;
}

/**
 * What the rows of the table named `table` are named, in their order, the first row (All) left out: the words each
 * row gives its cells' names, before the column's. A row's header also holds what describes its cells and what it says
 * of a save.
 */
export const rowNames = (table: string) =>
  within(screen.getByRole("table", { name: table }))
    .getAllByRole("row")
    .slice(2)
    .map((row) => {
      const [first] = (within(row).getAllByRole("checkbox")[0]?.getAttribute("aria-labelledby") ?? "").split(" ");
      return document.getElementById(first ?? "")?.textContent ?? "";
    });

/**
 * What the status regions among `regions` say: each row that saves at once keeps one from the start, empty while it
 * has nothing to say, so a state is found by its words.
 */
export const said = (regions: readonly HTMLElement[]) =>
  regions.map((status) => status.textContent).filter((words) => words !== "");

/** What the row named `row` of the table named `table` says of a save. */
export const saidIn = (table: string, row: string) => said(within(rowOf(table, row)).queryAllByRole("status")).join();

/** What describes `control`, part by part. */
export const describedBy = (control: HTMLElement) =>
  (control.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? "");

/** The names of the tables: a member's, and the one a moderator has besides. */
export const people = "From other people";
export const moderators = "For moderators";
export const server = "From this server";
