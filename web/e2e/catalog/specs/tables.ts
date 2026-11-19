// SPDX-License-Identifier: AGPL-3.0-only

// How a story's table reaches a screen reader: read off the tree Chromium gives assistive technology (the DevTools
// protocol's Accessibility domain), which no DOM simulation has, cell by cell, each cell with its row's header and its
// column's.

import type { Page } from "@playwright/test";

/**
 * The table named `name` as Chromium gives it to assistive technology, read cell by cell: for each row after the
 * first, each of its cells but its header as `<the row's header> | <the column's header> | <the cell>`, a column's
 * header counted by the cell's place in its row, the row's header wherever it stands (after a row's checkbox, say);
 * the first row is the columns' headers, and a row that does not line up with it, or has not one header, or a table
 * that is not there, is told as such. A name is the one Chromium computes, its spaces folded.
 */
export async function readCellByCell(page: Page, name: string): Promise<string[]> {
  const cdp = await page.context().newCDPSession(page);
  try {
    await cdp.send("Accessibility.enable");
    const { nodes } = await cdp.send("Accessibility.getFullAXTree");
    type Node = (typeof nodes)[number];
    const byId = new Map(nodes.map((node) => [node.nodeId, node]));
    const roleOf = (node: Node) => String(node.role?.value ?? "");
    const nameOf = (node: Node) =>
      String(node.name?.value ?? "")
        .replace(/\s+/g, " ")
        .trim();
    /** The nodes under `node` of the roles `roles`, not looking inside one of them, in the tree's order. */
    const under = (node: Node, roles: readonly string[]): Node[] =>
      (node.childIds ?? []).flatMap((id) => {
        const child = byId.get(id);
        if (child === undefined) return [];
        if (!child.ignored && roles.includes(roleOf(child))) return [child];
        return under(child, roles);
      });
    const table = nodes.find((node) => !node.ignored && roleOf(node) === "table" && nameOf(node) === name);
    if (table === undefined) return [`no table named ${name}`];
    const [head, ...rows] = under(table, ["row"]);
    if (head === undefined) return ["no row"];
    const columns = under(head, ["columnheader", "rowheader", "cell"]);
    if (columns.some((column) => roleOf(column) !== "columnheader"))
      return [`a first row of ${columns.map(roleOf).join(", ")}`];
    return rows.flatMap((row) => {
      const cells = under(row, ["columnheader", "rowheader", "cell"]);
      const headers = cells.filter((cell) => roleOf(cell) === "rowheader");
      const [header] = headers;
      if (header === undefined || headers.length !== 1 || cells.length !== columns.length)
        return [`a row of ${cells.length === 0 ? "nothing" : cells.map(roleOf).join(", ")}`];
      return cells.flatMap((cell, i) => {
        if (cell === header) return [];
        const column = columns[i];
        return [
          `${nameOf(header)} | ${column ? nameOf(column) : ""} | ${roleOf(cell) === "cell" ? nameOf(cell) : roleOf(cell)}`,
        ];
      });
    });
  } finally {
    await cdp.detach();
  }
}
