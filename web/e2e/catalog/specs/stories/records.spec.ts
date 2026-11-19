// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the lists of records of the staff's screens (web/design-system/src/records.stories.tsx, #3258) each
// show the state they are named by; a table of records reaches a screen reader as a table in both its layouts, wide,
// a record a row under the columns' names, and narrow, each record stacked, a fact's name shown before it read from
// its column's header alone, and the column it is sorted by told with its direction; and a record open in the app's
// frame has its list beside it from `wide`, the record marked, and stands alone under it. What each part says of its
// roles and names is its component tests' (records.dom.test.tsx); the stories' accessibility checks are the check of
// every story.

import type { Page } from "@playwright/test";
import { readCellByCell } from "../tables";
import { expect, from, minimum, pc, test } from "../test";

const records = (name: string) => `design-system-records--${name}`;

/** The table named `name` of a story. */
const table = (page: Page, name: string) => page.getByRole("table", { name, exact: true });

const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    records("reports-screen"),
    async (page) => {
      await expect(table(page, "Reports").getByRole("row")).toHaveCount(6);
      await expect(page.getByRole("status").filter({ hasText: "selected" })).toHaveText("1 selected");
      await expect(page.getByRole("checkbox", { name: "Select all", exact: true })).toBeChecked({
        indeterminate: true,
      });
      await expect(page.getByRole("columnheader", { name: "Received", exact: true })).toHaveAttribute(
        "aria-sort",
        "descending",
      );
      await expect(page.getByRole("button", { name: "Resolve…", exact: true })).toBeEnabled();
      await expect(page.getByRole("navigation", { name: "Pages", exact: true }).getByRole("button")).toHaveText([
        "Previous",
        "Next",
      ]);
    },
  ],
  [
    records("filters-applied"),
    async (page) => {
      const filters = page.getByRole("search", { name: "Filter reports", exact: true });
      await expect(filters.getByRole("searchbox", { name: "Search reports", exact: true })).toHaveValue("deals");
      await expect(filters.getByRole("button", { name: "State: Open", exact: true })).toBeVisible();
      await expect(filters.getByRole("button", { name: "Category: Spam", exact: true })).toBeVisible();
      await expect(filters.getByRole("status")).toHaveText("1 report");
      // Clear filters empties the search and every facet: the whole queue again.
      await filters.getByRole("button", { name: "Clear filters", exact: true }).click();
      await expect(filters.getByRole("searchbox")).toHaveValue("");
      await expect(filters.getByRole("button", { name: "State", exact: true })).toBeVisible();
      await expect(table(page, "Reports").getByRole("row")).toHaveCount(6);
    },
  ],
  [
    records("filters-at-rest"),
    async (page) => {
      await expect(page.getByRole("button", { name: "Clear filters", exact: true })).toHaveCount(0);
      // A facet's menu holds its values, the one of no filter checked; choosing one applies it.
      await page.getByRole("button", { name: "State", exact: true }).click();
      await expect(page.getByRole("menuitemradio", { name: "Any state", exact: true })).toBeChecked();
      await page.getByRole("menuitemradio", { name: "Open", exact: true }).click();
      await expect(page.getByRole("button", { name: "State: Open", exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: "Clear filters", exact: true })).toBeVisible();
    },
  ],
  [
    records("selecting"),
    async (page) => {
      await expect(page.getByRole("status").filter({ hasText: "selected" })).toHaveText([
        "4 selected",
        "None selected",
      ]);
      await expect(page.getByRole("button", { name: "Resolve…", exact: true })).toHaveCount(2);
      await expect(page.getByRole("button", { name: "Resolve…", exact: true, disabled: true })).toHaveCount(1);
    },
  ],
  [
    records("accounts"),
    async (page) => {
      await expect(page.getByRole("columnheader", { name: "Joined", exact: true })).toHaveAttribute(
        "aria-sort",
        "descending",
      );
      await expect(page.getByRole("columnheader", { name: "Posts", exact: true })).not.toHaveAttribute("aria-sort");
      await expect(table(page, "Accounts").getByRole("link")).toHaveCount(4);
    },
  ],
  [
    records("domain-actions"),
    async (page) => {
      await expect(table(page, "Domain actions").getByRole("row")).toHaveCount(5);
      for (const [state, count] of [
        ["Active", 2],
        ["Lifted", 1],
        ["Expired", 1],
      ] as const)
        await expect(table(page, "Domain actions").getByRole("cell", { name: state, exact: true })).toHaveCount(count);
    },
  ],
  [
    records("jobs"),
    async (page) => {
      await page.getByRole("button", { name: "Show more", exact: true }).click();
      await expect(table(page, "Jobs")).toHaveAttribute("aria-busy", "true");
    },
  ],
  [
    records("pages"),
    async (page) => {
      const pages = page.getByRole("navigation", { name: "Pages", exact: true });
      await expect(pages).toHaveCount(4);
      // Next on every page but the last, Previous on every page but the first: never a button that does nothing.
      await expect(pages.getByRole("link", { name: "Next", exact: true })).toHaveCount(2);
      await expect(pages.getByRole("link", { name: "Previous", exact: true })).toHaveCount(2);
    },
  ],
  [
    records("empty"),
    async (page) => {
      await expect(page.getByRole("table")).toHaveCount(0);
      await expect(
        page.getByText("No open reports. Reports from people and other servers show up here.", { exact: true }),
      ).toBeVisible();
      await expect(page.getByRole("button", { name: "Clear filters", exact: true })).toBeVisible();
    },
  ],
  [
    records("states"),
    async (page) => {
      for (const state of [
        "Open",
        "Assigned",
        "Resolved",
        "Active",
        "Expired",
        "Lifted",
        "Pending",
        "Failed",
        "Disabled",
      ])
        await expect(page.getByText(state, { exact: true })).toBeVisible();
    },
  ],
];

/** The accounts of the story `accounts` as they are read cell by cell, in every layout. */
const accounts = [
  ["@yuki Yuki Tanaka", "Admin", "4,812", "Mar 2, 2024", "Active"],
  ["@sora そら", "Moderator", "1,207", "Jun 18, 2024", "Active"],
  [
    "@kenji Asked to join: “I run the city’s cycling club and want a place for our rides.”",
    "",
    "0",
    "Today",
    "Awaiting approval",
  ],
  ["@oldtimer Suspended for spam", "", "36", "Jan 9, 2025", "Suspended"],
] as const;

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of the lists of records shows its state, and a table of records is read cell by cell at every width", async ({
  page,
  open,
}) => {
  for (const [id, shows] of states)
    await test.step(id, async () => {
      await open(id, { width: pc });
      await shows(page);
    });

  const expected = accounts.flatMap(([account, ...cells]) =>
    ["Role", "Posts", "Joined", "State"].map((column, i) => `${account} | ${column} | ${cells[i] ?? ""}`),
  );
  for (const [layout, width] of [
    ["wide", pc],
    ["narrow", minimum],
  ] as const)
    await test.step(`the accounts, ${layout}`, async () => {
      await open(records("accounts"), { width });
      await expect.poll(() => readCellByCell(page, "Accounts")).toEqual(expected);
      await expect(page.getByRole("columnheader", { name: "Joined", exact: true })).toHaveAttribute(
        "aria-sort",
        "descending",
      );
    });

  await test.step("a record open: its list beside it from `wide`, the record marked; under it, the record alone", async () => {
    await open(records("report-open"), { width: from.wide });
    const beside = page.getByRole("complementary", { name: "Reports", exact: true });
    await expect(beside.getByRole("table", { name: "Reports", exact: true })).toBeVisible();
    await expect(beside.getByRole("button", { name: "@deals4u@promo.example", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await open(records("report-open"), { width: from.wide - 1 });
    await expect(page.getByRole("table", { name: "Reports", exact: true })).toBeHidden();
    await expect(page.getByRole("button", { name: "Reports", exact: true })).toBeVisible();
  });
});
