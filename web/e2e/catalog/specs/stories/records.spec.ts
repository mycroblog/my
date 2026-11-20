// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the lists of records of the staff's screens (web/design-system/src/records.stories.tsx, #3258) each
// show the state they are named by; a table of records reaches a screen reader as a table in both its layouts, wide,
// a record a row under the columns' names, and narrow, each record stacked, a fact's name shown before it read from
// its column's header alone, a row's checkbox a cell of its own, and the column it is sorted by told with its
// direction, which a press on a column's name moves or turns, on a phone the one sort control's menu; a press anywhere
// in a row opens its record; and a record open in the app's frame has its list beside it from `wide`, the record
// marked, and stands alone under it. What each part says of its roles and names is its component tests'
// (records.dom.test.tsx); the stories' accessibility checks are the check of every story.

import type { Locator, Page } from "@playwright/test";
import { boxOf } from "../boxes";
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
      await expect(filters.getByRole("searchbox", { name: "Search reports", exact: true })).toHaveValue("kuro");
      await expect(filters.getByRole("button", { name: "State: Assigned", exact: true })).toBeVisible();
      await expect(filters.getByRole("button", { name: "Category: Harassment", exact: true })).toBeVisible();
      const mine = filters.getByRole("button", { name: "Assigned to me", exact: true });
      await expect(mine).toHaveAttribute("aria-pressed", "true");
      await expect(filters.getByRole("status")).toHaveText("1 report");
      // Clear filters empties the search and every facet: the whole queue again.
      await filters.getByRole("button", { name: "Clear filters", exact: true }).click();
      await expect(filters.getByRole("searchbox")).toHaveValue("");
      await expect(filters.getByRole("button", { name: "State", exact: true })).toBeVisible();
      await expect(mine).toHaveAttribute("aria-pressed", "false");
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
      await expect(table(page, "Jobs")).not.toHaveAttribute("aria-busy", "true");
      await page.getByRole("button", { name: "Show more", exact: true }).click();
      await expect(table(page, "Jobs")).toHaveAttribute("aria-busy", "true");
      // While the next jobs come, Show more asks for them no more.
      await expect(page.getByRole("button", { name: "Show more", exact: true })).toBeDisabled();
    },
  ],
  [
    // The pager under them (pager.stories.tsx): the first page, a middle one, the last, the next on its way, the next
    // failed, and a middle page whose position is not said.
    "design-system-pager--pages",
    async (page) => {
      const pages = page.getByRole("navigation", { name: "Pages", exact: true });
      await expect(pages).toHaveCount(6);
      // Next leads on from every page but the last, Previous from every page but the first, where each stays,
      // disabled, so that a press that reached the last page leaves the focus on its button.
      await expect(pages.getByRole("link", { name: "Next", exact: true })).toHaveCount(2);
      await expect(pages.getByRole("link", { name: "Previous", exact: true })).toHaveCount(2);
      await expect(pages.getByRole("button", { name: "Previous", exact: true, disabled: true })).toHaveCount(1);
      // The last page's Next, and the one on its way, which takes no second press.
      await expect(pages.getByRole("button", { name: "Next", exact: true, disabled: true })).toHaveCount(2);
      // The page that failed is asked for again by its own button, under the sentence that says so.
      await expect(pages.getByRole("button", { name: "Try again", exact: true })).toHaveCount(1);
      await expect(page.getByRole("alert")).toHaveText("The next page could not be loaded.");
      await expect(pages.getByRole("status")).toHaveText(["Page 1", "Page 3", "Page 7", "Page 3", "Page 3", ""]);
    },
  ],
  [
    records("empty"),
    async (page) => {
      await expect(page.getByRole("table")).toHaveCount(0);
      await expect(page.getByText("No open reports.", { exact: true })).toBeVisible();
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

/** The reports of the story `reports-screen`, latest first, as they are read cell by cell, in every layout. */
const queue = [
  ["@deals4u@promo.example Reported by @mika and 11 others · Report 1042", "Spam", "12", "3m ago", "Open"],
  ["@kuro@social.example Reported by @hana · Report 1041", "Harassment", "2", "41m ago", "Assigned to you"],
  ["@newsbot@feeds.example Forwarded by feeds.example · Report 1040", "Other", "1", "2h ago", "Open"],
  ["@ren Reported by @sora · Report 1038", "Rule 3: Mark sensitive media", "3", "5h ago", "Assigned"],
  ["@aoi@pixel.example Reported by @yuki · Report 1035", "Illegal content", "1", "1d ago", "Resolved"],
] as const;

/** Each column's name and whether it says it is sorted by it, and in which direction. */
const sortedBy = (page: Page) =>
  Promise.all(
    ["Select all", "Account", "Category", "Reports", "Received", "State"].map(
      async (name) =>
        `${name} ${await table(page, "Reports")
          .getByRole("columnheader", { name, exact: true })
          .evaluate((header) => header.ariaSort ?? "unsorted")}`,
    ),
  );

const layouts = [
  ["wide", pc],
  ["narrow", minimum],
] as const;

// A table whose rows are picked is read by row and column as well: each row's checkbox a cell of its own column,
// under Select all, and an account's handle as one word, as its button is named.
test("a table of records whose rows are picked is read cell by cell at every width", async ({ page, open }) => {
  const columns = ["Select all", "Category", "Reports", "Received", "State"];
  const expected = queue.flatMap(([report, ...cells]) => {
    const account = report.slice(0, report.indexOf(" "));
    return columns.map(
      (column, i) => `${report} | ${column} | ${i === 0 ? `Select ${account}` : (cells[i - 1] ?? "")}`,
    );
  });
  for (const [layout, width] of layouts)
    await test.step(layout, async () => {
      await open(records("reports-screen"), { width });
      await expect.poll(() => readCellByCell(page, "Reports")).toEqual(expected);
    });
});

/** On a phone, the one sort control's menu: choose `name` in it, a column or a direction. */
async function sortOnPhone(page: Page, control: string, name: string) {
  await page.getByRole("button", { name: control, exact: true }).click();
  await page
    .getByRole("menu", { name: "Sort by", exact: true })
    .getByRole("menuitemradio", { name, exact: true })
    .click();
}

// The sort, which a press on a column's name moves or turns (on a phone, the one sort control's menu), is said on
// the column at every width.
test("a table of records says what it is sorted by as it is sorted, at every width", async ({ page, open }) => {
  const sorts = {
    wide: {
      byReports: () => page.getByRole("button", { name: "Reports", exact: true }).click(),
      turn: () => page.getByRole("button", { name: "Reports", exact: true }).click(),
    },
    narrow: {
      byReports: () => sortOnPhone(page, "Received Descending", "Reports"),
      turn: () => sortOnPhone(page, "Reports Descending", "Ascending"),
    },
  };
  for (const [layout, width] of layouts)
    await test.step(layout, async () => {
      await open(records("reports-screen"), { width });
      await expect
        .poll(() => sortedBy(page))
        .toEqual([
          "Select all unsorted",
          "Account unsorted",
          "Category unsorted",
          "Reports unsorted",
          "Received descending",
          "State unsorted",
        ]);

      await sorts[layout].byReports();
      await expect.poll(() => sortedBy(page)).toContain("Reports descending");
      await expect.poll(() => sortedBy(page)).toContain("Received unsorted");
      await sorts[layout].turn();
      await expect.poll(() => sortedBy(page)).toContain("Reports ascending");
      // The rows follow: the fewest reports first, two of one report in the order they were in.
      await expect(table(page, "Reports").getByRole("rowheader")).toHaveText([
        /^@newsbot@feeds\.example/,
        /^@aoi@pixel\.example/,
        /^@kuro@social\.example/,
        /^@ren/,
        /^@deals4u@promo\.example/,
      ]);
    });
});

// What a pointer meets of a row is the record: a press on any of its cells opens it, at every width, while its
// checkbox picks it and opens nothing.
test("a press anywhere in a record's row opens it, and its checkbox only picks it, at every width", async ({
  page,
  open,
}) => {
  const kuro = () => table(page, "Reports").getByRole("button", { name: "@kuro@social.example", exact: true });
  for (const [layout, width] of layouts)
    await test.step(layout, async () => {
      await open(records("reports-screen"), { width });
      // The record's cover lies over its row, the checkbox's mark over its box: presses where they are drawn, as a
      // pointer's.
      const press = async (part: Locator) => {
        const box = await boxOf(part);
        await page.mouse.click((box.left + box.right) / 2, (box.top + box.bottom) / 2);
      };
      const pick = table(page, "Reports").getByRole("checkbox", { name: "Select @kuro@social.example", exact: true });
      await press(pick);
      await expect(pick).toBeChecked();
      await expect(kuro()).not.toHaveAttribute("aria-current");
      await press(table(page, "Reports").getByRole("cell", { name: "Harassment", exact: true }));
      await expect(kuro()).toHaveAttribute("aria-current", "true");
    });
});

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

  await test.step("a press anywhere on a record opens it, a fact as much as its head", async () => {
    await open(records("reports-screen"), { width: pc });
    const reports = table(page, "Reports");
    const fact = await reports.getByRole("cell", { name: "Harassment", exact: true }).boundingBox();
    if (fact === null) throw new Error("no cell Harassment");
    // The record's cover takes the press, as it would a finger's; Playwright's click would refuse the covered cell.
    await page.mouse.click(fact.x + fact.width / 2, fact.y + fact.height / 2);
    await expect(reports.getByRole("button", { name: "@kuro@social.example", exact: true })).toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  await test.step("a record open: its list beside it from `wide`, the record marked; under it, the record alone", async () => {
    await open(records("report-open"), { width: from.wide });
    const beside = page.getByRole("complementary", { name: "Reports", exact: true });
    await expect(beside.getByRole("table", { name: "Reports", exact: true })).toBeVisible();
    await expect(beside.getByRole("button", { name: "@deals4u@promo.example", exact: true })).toHaveAttribute(
      "aria-current",
      "true",
    );
    await open(records("report-open"), { width: from.wide - 1 });
    await expect(page.getByRole("table", { name: "Reports", exact: true })).toBeHidden();
    await expect(page.getByRole("button", { name: "Reports", exact: true })).toBeVisible();
  });
});
