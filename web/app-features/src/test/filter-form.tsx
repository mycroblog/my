// SPDX-License-Identifier: AGPL-3.0-only

// A keyword filter's editor for its tests (#3202), on either DOM simulation: the editor of a new filter or of one of
// the reader's, drawn on a fake server once its route loaded what it opens with, what it told its owner, and the parts
// a test finds on it.

import { instanceQuery, viewerQuery } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import { fireEvent, screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { onTestFinished } from "bun:test";
import { FilterEdit, loadFilterEdit, type FilterEditProps } from "../settings/filter-edit";
import { fakeData, renderFeature, type FakeData } from "./feature";
import { runtimeSignals } from "./profile-form";

/** The editor on its server, the reader's hands, the filter it edits, and what it told its owner, in order. */
export type DrawnFilter = FakeData & {
  user: UserEvent;
  /** The ID of the filter in the editor; none for a new one. */
  id: string | undefined;
  /** `back` for the way back taken, `unsaved:true|false` for each report of unsaved changes, `stay`, `leave`. */
  told: string[];
};

/**
 * Tells React, until the test ends, that the server's answers reach the screen when they come and not inside an act()
 * of the test: a test waits for what it asserts.
 */
export function answersOutsideAct(): void {
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
}

/**
 * Draws a filter's editor on a server of `options`, in the main landmark as the shell places it, once the shell read
 * the instance and the reader's account and the route loaded the editor: of the reader's filter at the index `filter`
 * (oldest first), of the filter of that ID, or of a new filter; `leaving` draws it as the app does while the reader is
 * asked whether to leave with unsaved changes.
 */
export async function filterForm(
  options: FakeOptions = {},
  {
    filter,
    locale = "en",
    leaving = false,
    on = fakeData(options),
  }: { filter?: number | string; locale?: string; leaving?: boolean; on?: FakeData } = {},
): Promise<DrawnFilter> {
  answersOutsideAct();
  runtimeSignals();
  const { data, fake } = on;
  const id = typeof filter === "number" ? fake.filterPath(filter).replace("/settings/filters/", "") : filter;
  await data.queryClient.query(instanceQuery(data));
  await data.queryClient.query(viewerQuery(data));
  // A filter the server does not have is the editor's to say: its route loads what there is.
  await loadFilterEdit(data, id).catch(() => undefined);
  const told: string[] = [];
  const asked: FilterEditProps["leaving"] = {
    onStay: () => told.push("stay"),
    onLeave: () => told.push("leave"),
  };
  const user = renderFeature(
    data,
    <main>
      <FilterEdit
        id={id}
        onBack={() => told.push("back")}
        onUnsaved={(unsaved) => told.push(`unsaved:${String(unsaved)}`)}
        leaving={leaving ? asked : undefined}
      />
    </main>,
    locale,
  );
  await within(screen.getByRole("main")).findAllByRole("heading");
  return { ...on, user, id, told };
}

/** The section of the editor titled `name`; the Words' heading says how many there are after its name. */
export const section = (name: string) =>
  within(screen.getByRole("region", { name: new RegExp(`^${name}(\\s*[\\d,]+)?$`) }));
export const nameField = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Name" });
export const actionChoice = () => screen.getByRole<HTMLSelectElement>("combobox", { name: "Posts that match" });
export const endsChoice = () => screen.getByRole<HTMLSelectElement>("combobox", { name: "Ends" });
/** The field of the date and time the filter ends at, there once Ends is at a date and time. */
export const endsAt = () => screen.queryByLabelText<HTMLInputElement>("Ends at");
/** The field of the word numbered `n` (from 1). */
export const wordField = (n: number) =>
  screen.getByRole<HTMLInputElement>("textbox", { name: `Word or phrase ${String(n)}` });
/** The words of the editor in order, each as what its field holds and whether it matches whole words only. */
export const words = () =>
  within(screen.getByRole("list", { name: "Words" }))
    .getAllByRole("listitem")
    .map((row) => [
      within(row).getByRole<HTMLInputElement>("textbox").value,
      within(row).getByRole<HTMLInputElement>("checkbox", { name: "Whole word" }).checked,
    ]);
export const addWord = () => screen.getByRole<HTMLButtonElement>("button", { name: "Add a word" });
/** The places a filter applies, in the editor's order. */
export const placeNames = ["Home and lists", "Notifications", "Public timelines", "Threads", "Profiles"] as const;
export const place = (name: (typeof placeNames)[number]) => screen.getByRole<HTMLInputElement>("checkbox", { name });
/** The button that submits the editor: Add for a new filter, Save for one of the reader's. */
export const submitButton = () => screen.getByRole<HTMLButtonElement>("button", { name: /^(Add|Save)$/ });
/** The checkbox Whole word of the word numbered `n` (from 1). */
export const wholeWord = (n: number) => {
  const row = within(screen.getByRole("list", { name: "Words" })).getAllByRole("listitem")[n - 1];
  if (row === undefined) throw new Error(`test: the editor has no word ${String(n)}`);
  return within(row).getByRole<HTMLInputElement>("checkbox", { name: "Whole word" });
};
/** The button that removes the word numbered `n` (from 1). */
export const removeWord = (n: number) =>
  screen.getByRole<HTMLButtonElement>("button", { name: `Remove word ${String(n)}` });
/** What the choice `select` offers, in its order. */
export const offered = (select: HTMLSelectElement) =>
  within(select)
    .getAllByRole("option")
    .map((option) => option.textContent);
/** What the choice `select` is at: the words of its option. */
export const chosen = (select: HTMLSelectElement) => select.selectedOptions[0]?.textContent;
/** Sets the date and time the filter ends at, as the browser's own field reports a choice: `YYYY-MM-DDTHH:mm`, or empty. */
export function setEndsAt(value: string): void {
  const field = endsAt();
  if (field === null) throw new Error("test: the editor shows no date and time");
  fireEvent.change(field, { target: { value } });
}
/** The fields of the editor that say their value is not valid, by their names, in the editor's order. */
export const invalid = () =>
  [
    ["Name", nameField()] as const,
    ["Posts that match", actionChoice()] as const,
    ["Ends", endsChoice()] as const,
    ["Ends at", endsAt()] as const,
    ...screen
      .queryAllByRole("textbox", { name: /^Word or phrase \d+$/ })
      .map((field, i) => [`Word or phrase ${String(i + 1)}`, field] as const),
    ...placeNames.map((name) => [name, place(name)] as const),
  ]
    .filter(([, field]) => field?.getAttribute("aria-invalid") === "true")
    .map(([name]) => name);
/** What the status regions of the page say; the submit bar keeps one from the start, empty while it has nothing to say. */
export const said = () =>
  screen
    .queryAllByRole("status")
    .map((status) => status.textContent)
    .filter((words) => words !== "");
/** The writes the editor sent since `from`, each as `METHOD /path`. */
export const writes = ({ requests }: FakeData, from = 0) => requests.slice(from).filter((r) => !r.startsWith("GET "));
/** Puts `text` in `field` in place of what it holds, as one paste: no key decides anything here. */
export async function rewrite(user: UserEvent, field: HTMLElement, text: string): Promise<void> {
  await user.clear(field);
  if (text === "") return;
  await user.click(field);
  await user.paste(text);
}
/** What the alerts of the page say. */
export const alerts = () => screen.queryAllByRole("alert").map((alert) => alert.textContent);
/** What describes `element`, part by part. */
export const describedBy = (element: HTMLElement) =>
  (element.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? "");
