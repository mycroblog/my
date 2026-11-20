// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurements of the operations' story (web/app/src/operations.stories.tsx, #2952, ADR-0167 §1) as a browser
// draws them and a screen reader reads them: each metric's table gives each figure with its route and its samples and
// its column, and every figure ends where its column's head ends, at a computer's width and at the narrowest, and in a
// language written right to left at its left. What the screen shows and does on the fake is the app's route tests'
// (web/app/src/operations.dom.test.tsx).

import type { Locator } from "@playwright/test";
import { readCellByCell } from "../tables";
import { expect, minimum, pc, test } from "../test";

const story = "screens-operations--ux-measurements";

/** The tables of the measurements, one a metric, in the screen's order. */
const tables = [
  "Time for a screen to appear (LCP)",
  "Time to answer a press (INP)",
  "Long frames",
  "Time to navigate",
  "Time to answer an action",
  "Call round-trip time",
  "Call packet loss",
];

/**
 * For each of `tables`, the columns of figures whose texts do not all end where the column's head ends, on the side a
 * line ends (`right` left to right, `left` right to left), each as `<head>: <figure> ends at <x>`, and how many columns
 * of figures it has; the rows' heads are left out.
 */
const unaligned = (tables: Locator, side: "left" | "right") =>
  tables.evaluateAll((elements, edge) => {
    const textEdge = (cell: Element) => {
      const range = document.createRange();
      range.selectNodeContents(cell);
      const rects = [...range.getClientRects()].filter((rect) => rect.width > 0);
      return Math.round(
        edge === "right" ? Math.max(...rects.map((r) => r.right)) : Math.min(...rects.map((r) => r.left)),
      );
    };
    return elements.map((element) => {
      const rows = [...element.querySelectorAll("[role=row]")].map((row) => [
        ...row.querySelectorAll("[role=columnheader],[role=cell]"),
      ]);
      const [head = [], ...body] = rows;
      const columns = head.slice(1);
      const off = columns.flatMap((column, i) => {
        const at = textEdge(column);
        return body.flatMap((cells) => {
          const cell = cells[i];
          if (cell === undefined) return [`${column.textContent}: a row without it`];
          const its = textEdge(cell);
          return Math.abs(its - at) <= 1 ? [] : [`${column.textContent}: ${cell.textContent} ends at ${String(its)}`];
        });
      });
      return { columns: columns.length, off };
    });
  }, side);

test("a measurement's table reads each figure with its route, its samples and its column", async ({ page, open }) => {
  await open(story, { width: pc });
  await expect(
    page.getByRole("main").getByRole("table", { name: "Time for a screen to appear (LCP)", exact: true }),
  ).toBeVisible();

  // A route breaks after each of its slashes where it wraps, where Chromium's name puts a space: read without it (a
  // count of samples follows the route's last slash after a space of its own).
  const read = async (name: string) =>
    (await readCellByCell(page, name)).map((cell) => cell.replace(/\/ (?=\D)/g, "/"));
  await expect
    .poll(() => read("Time for a screen to appear (LCP)"))
    .toEqual([
      "/_app/ 689 samples | Up to 2.5 sec | 98%",
      "/_app/ 689 samples | Over 4 sec | 1%",
      "/_app/ 689 samples | 75th percentile | 1 sec",
      "/_app/statuses/$id 303 samples | Up to 2.5 sec | 95%",
      "/_app/statuses/$id 303 samples | Over 4 sec | 2%",
      "/_app/statuses/$id 303 samples | 75th percentile | 1.5 sec",
      "/_app/notifications 87 samples | Up to 2.5 sec | 99%",
      "/_app/notifications 87 samples | Over 4 sec | 0%",
      "/_app/notifications 87 samples | 75th percentile | 1 sec",
    ]);
  await expect.poll(() => read("Call packet loss")).toEqual(["p2p 28 samples | 75th percentile | 1%"]);
});

test("every figure of a measurement ends where its column's head ends", async ({ page, open }) => {
  for (const [width, locale, side] of [
    [pc, "en", "right"],
    [minimum, "en", "right"],
    [pc, "ar-XB", "left"],
  ] as const)
    await test.step(`${String(width)} px, ${locale}`, async () => {
      await open(story, { width, locale });
      const all = page.getByRole("main").getByRole("table");
      await expect(all).toHaveCount(tables.length);
      // LCP and INP have their two shares and the percentile; every other metric its percentile alone.
      await expect
        .poll(() => unaligned(all, side))
        .toEqual([
          { columns: 3, off: [] },
          { columns: 3, off: [] },
          ...tables.slice(2).map(() => ({ columns: 1, off: [] })),
        ]);
    });
});
