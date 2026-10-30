// SPDX-License-Identifier: AGPL-3.0-only
// Shared by the lists whose actions take a row off them (the settings lists, the admin dead jobs), move a row to
// another place (the rules) or take the pressed button off a row that stays (lifting a moderation action).
import { type RefObject, useEffect, useRef } from "react";

/**
 * Keeps focus when a row of a list goes away with the button that had focus: once the list has rendered
 * without the row, focus moves to the first button of the row that took its place, else of the row before
 * it, else to `fallback` when given (the field that adds to the list), else to the page's heading (an empty list has
 * no rows left).
 *
 * Returns `removed(id)`, to call when the row of `id` is taken off the list. Rows carry `data-row-id`.
 */
export function useFocusAfterRemoval(
  list: RefObject<HTMLElement | null>,
  ids: readonly string[],
  fallback?: () => HTMLElement | null | undefined,
): (id: string) => void {
  /** The row being removed and where it was. */
  const pendingRef = useRef<{ id: string; index: number } | null>(null);

  // Runs after every render: focus is moved in the DOM, outside React, once the row is gone.
  useEffect(() => {
    const removal = pendingRef.current;
    if (removal === null || ids.includes(removal.id)) return;
    pendingRef.current = null;
    const rows = list.current?.querySelectorAll<HTMLElement>("[data-row-id]") ?? [];
    const row = rows[Math.min(removal.index, rows.length - 1)];
    const target =
      row?.querySelector<HTMLElement>("button") ?? fallback?.() ?? document.querySelector<HTMLElement>("main h1");
    target?.focus();
  });

  return (id) => {
    // The place among the rows shown: a paged table shows a slice of `ids`, so the index in `ids` is not the row's place on the page.
    const shown = Array.from(list.current?.querySelectorAll<HTMLElement>("[data-row-id]") ?? []);
    const shownIndex = shown.findIndex((row) => row.dataset["rowId"] === id);
    pendingRef.current = { id, index: shownIndex >= 0 ? shownIndex : ids.indexOf(id) };
  };
}

/**
 * Keeps focus on a row that moved to another place in the list (reordering the rules): after the next render,
 * focus moves to the row's heading (`h3`), so that it is where the row is now. Returns `moved(id)`, to call once
 * the list has the new order.
 */
export function useFocusHeadingAfterMove(list: RefObject<HTMLElement | null>): (id: string) => void {
  const pendingRef = useRef<string | null>(null);
  // Runs after every render: focus is moved in the DOM, outside React, once the list has rendered the new order.
  useEffect(() => {
    const id = pendingRef.current;
    if (id === null) return;
    pendingRef.current = null;
    list.current?.querySelector<HTMLElement>(`[data-row-id="${id}"] h3`)?.focus();
  });
  return (id) => {
    pendingRef.current = id;
  };
}

/**
 * Keeps focus when a lifted action stays in the list without its lift button: once the row has rendered
 * without the button, focus moves to the row's heading (`h3`), or to its row header (`th`, with `tabIndex={-1}`) in a table;
 * `heading` selects another heading for rows that have one (the `h2` of the list of a page). Returns `lifted(id)`, to
 * call when the action is lifted.
 */
export function useFocusHeadingAfterLift(
  list: RefObject<HTMLElement | null>,
  heading = "h3, th",
): (id: string) => void {
  const pendingRef = useRef<string | null>(null);
  // Runs after every render: focus is moved in the DOM, outside React, once the button is gone.
  useEffect(() => {
    const id = pendingRef.current;
    if (id === null) return;
    const row = list.current?.querySelector<HTMLElement>(`[data-row-id="${id}"]`);
    if (row?.querySelector("button") != null) return;
    pendingRef.current = null;
    row?.querySelector<HTMLElement>(heading)?.focus();
  });
  return (id) => {
    pendingRef.current = id;
  };
}
