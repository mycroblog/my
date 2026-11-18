// SPDX-License-Identifier: AGPL-3.0-only

// What the route tests of the entry Account and data read of its screens (settings-account-data.dom.test.tsx,
// settings-account-leave.dom.test.tsx): what the fake server holds, the sections by their titles, the rows of their
// lists, what the statuses say, what describes a control, and the dialogs over the screen.

import { fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, words } from "./app";

/** What the fake server holds at `path` of the client API, as it answers a read of it. */
export async function heldBy<T>(fake: FakeApi, path: string): Promise<T> {
  const answer = await fake.serve(new Request(`${fakeOrigin}/api/v1${path}`));
  return (await answer.json()) as T;
}

/** The title of a section or a list as a reader hears it: the words of what names it. */
export const titleOf = (named: HTMLElement): string =>
  (named.getAttribute("aria-labelledby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id))
    .map((title) => (title === null ? "" : words(title)))
    .join(" ");

/** The titles of the screen's sections, in their order. */
export const sections = (): string[] => main().queryAllByRole("region").map(titleOf);

/** The section titled `name`. */
export function section(name: string): HTMLElement {
  const found = main()
    .queryAllByRole("region")
    .filter((region) => titleOf(region) === name);
  const [one] = found;
  if (one === undefined || found.length > 1)
    throw new Error(`test: ${String(found.length)} sections are titled "${name}": ${JSON.stringify(sections())}`);
  return one;
}

/** Waits until the screen has a section titled `name`, and returns it. */
export const sectionRead = (name: string): Promise<HTMLElement> => waitFor(() => section(name));

/** What the rows of the lists named `name` in `part` say, each as one line. */
export const rowsOf = (part: HTMLElement, name: string): string[] =>
  within(part)
    .getAllByRole("list", { name })
    .flatMap((list) => within(list).getAllByRole("listitem"))
    .map(words);

/** What the status regions inside `part` say, the ones with nothing to say left out. */
export const said = (part: HTMLElement = screen.getByRole("main")): string[] =>
  within(part)
    .queryAllByRole("status")
    .map(words)
    .filter((line) => line !== "");

/** Waits until a status inside the part `part` finds says `says`. */
export const saying = (part: () => HTMLElement, says: string): Promise<void> =>
  waitFor(() => {
    if (!said(part()).includes(says))
      throw new Error(`test: no status says "${says}": ${JSON.stringify(said(part()))}`);
  });

/** The names of the buttons inside `part`, in their order: a glyph's label, else its words. */
export const buttons = (part: HTMLElement): string[] =>
  within(part)
    .queryAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? words(button));

/** The words of what describes `control`, part by part. */
export const describedBy = (control: Element): string[] =>
  (control.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => {
      const part = document.getElementById(id);
      return part === null ? "" : words(part);
    });

/** Waits until what `element` finds has the focus. */
export const focusOn = (element: () => Element): Promise<void> =>
  waitFor(() => {
    if (document.activeElement !== element())
      throw new Error(`test: the focus is on ${words(document.activeElement ?? document.body).slice(0, 80)}`);
  });

/** The confirmations open over the screen. */
export const confirmations = (): HTMLElement[] => screen.queryAllByRole("alertdialog");

/** Waits until no dialog of either kind is open. */
export const closed = (): Promise<void> =>
  waitFor(() => {
    if (screen.queryAllByRole("dialog").length + confirmations().length > 0)
      throw new Error("test: a dialog is still open");
  });

/** A medium date as the screens write one in English, each space as one. */
export const dayOf = (iso: string): string =>
  new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(new Date(iso)).replace(/\s+/g, " ");

/** Waits one turn of the event loop, for a request that would come late. */
export const aTurn = (): Promise<unknown> => new Promise((resolve) => setTimeout(resolve, 0));
