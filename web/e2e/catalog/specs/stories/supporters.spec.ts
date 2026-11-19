// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the parts of the supporters' and the finance screens (web/design-system/src/supporter.stories.tsx,
// finance.stories.tsx, #3244) each show the state they are named by; each segment of a choice is a finger's target on
// a phone; and the totals' table reaches a screen reader as a table in both its layouts, wide, a period a row under
// the columns' names, and narrow, each period on a line of its own over its figures and the name of the periods'
// column for assistive technology alone, every figure its bar draws read in its cell, and a figure in two currencies a
// line a currency at both widths. What each part says of its roles
// and names is its component tests' (supporter.dom.test.tsx, finance.dom.test.tsx); the stories' accessibility
// checks are the check of every story.

import type { Page } from "@playwright/test";
import { smallestSide, textLines } from "../boxes";
import { readCellByCell } from "../tables";
import { expect, minimum, pc, test } from "../test";

const supporters = (name: string) => `design-system-supporters--${name}`;
const finance = (name: string) => `design-system-finance--${name}`;

/** The plans of the stories, each an item of the list of plans named by its heading. */
const plansOf = (page: Page) =>
  page
    .getByRole("list", { name: "Plans", exact: true })
    .getByRole("listitem", { name: /^(Supporter|Patron|Benefactor|The supporters of a small server by .+)$/ });

/** The plan named `name`. */
const planNamed = (page: Page, name: string) => page.getByRole("listitem", { name, exact: true });

/** The choices named `name` of a story. */
const choices = (page: Page, name: string) => page.getByRole("radiogroup", { name, exact: true });

const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    supporters("supporter-screen"),
    async (page) => {
      await expect(page.getByText("Renews on Dec 1, 2026", { exact: true })).toBeVisible();
      await expect(plansOf(page)).toHaveCount(3);
      await expect(choices(page, "Billing period")).toBeVisible();
      await expect(choices(page, "Amount").getByRole("radio")).toHaveCount(4);
      await expect(page.getByRole("button", { name: "Tip ¥500", exact: true })).toBeVisible();
    },
  ],
  [
    supporters("plans-monthly"),
    async (page) => {
      await expect(plansOf(page)).toHaveCount(3);
      for (const name of ["Supporter", "Patron", "Benefactor"])
        await expect(planNamed(page, name)).not.toHaveAttribute("aria-current");
      await expect(page.getByRole("radio", { name: "Monthly", exact: true })).toBeChecked();
      await expect(page.getByRole("button", { name: "Choose", exact: true })).toHaveCount(3);
      await expect(planNamed(page, "Supporter")).toContainText("¥300 / month");
      // The period's choice changes every plan's price; a press anywhere on a segment lands on its radio.
      await page.getByRole("radio", { name: "Yearly Save 10%", exact: true }).click();
      await expect(planNamed(page, "Supporter")).toContainText("¥3,240 / year");
    },
  ],
  [
    supporters("plans-yearly-with-yours"),
    async (page) => {
      const yours = planNamed(page, "Patron");
      await expect(yours).toHaveAttribute("aria-current", "true");
      await expect(yours).toContainText("Your plan");
      await expect(yours.getByRole("button")).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Switch", exact: true })).toHaveCount(2);
      await expect(planNamed(page, "Benefactor")).toContainText("Everything in Patron, plus:");
    },
  ],
  [
    supporters("plan-alone"),
    async (page) => {
      await expect(plansOf(page)).toHaveCount(1);
      await expect(plansOf(page).getByRole("list")).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Choose", exact: true })).toBeVisible();
    },
  ],
  [
    supporters("subscription-states"),
    async (page) => {
      for (const sentence of [
        "Renews on Dec 1, 2026",
        "Ends on Dec 1, 2026",
        "Changes to Patron, monthly, on Dec 1, 2026",
        "Your payment failed: your perks end on Nov 26, 2026 unless it goes through",
      ])
        await expect(page.getByText(sentence, { exact: true })).toBeVisible();
      for (const action of ["Resume", "Keep my plan", "Update the card"])
        await expect(page.getByRole("button", { name: action, exact: true })).toBeVisible();
    },
  ],
  [
    supporters("tip-amount-choice"),
    async (page) => {
      await expect(choices(page, "Amount")).toHaveCount(4);
      await expect(choices(page, "Amount").getByRole("radio")).toHaveCount(13);
      await expect(page.getByRole("radio", { checked: true })).toHaveCount(3);
      const closed = page.getByRole("radiogroup", {
        name: "Amount",
        exact: true,
        description: "A tip is under way in another tab. Finish or cancel it there first.",
      });
      await expect(closed.getByRole("radio", { disabled: true })).toHaveCount(3);
      await expect(closed.getByRole("radio", { checked: true })).toHaveAccessibleName("¥1,000");
    },
  ],
  [
    supporters("period-choices"),
    async (page) => {
      await expect(choices(page, "Billing period")).toHaveCount(2);
      await expect(page.getByRole("radio", { name: "Yearly Save 10%", exact: true })).toHaveCount(2);
    },
  ],
  [
    supporters("amounts"),
    async (page) => {
      for (const amount of ["¥500", "¥1,000 / month", "US$48.60 / year", "¥10,800 / year"])
        await expect(page.getByText(amount, { exact: true })).toBeVisible();
    },
  ],
  [
    finance("finance-screen"),
    async (page) => {
      await expect(page.getByRole("table", { name: "Totals by month", exact: true })).toBeVisible();
      await expect(page.getByRole("table", { name: "Costs of November 2026 by provider", exact: true })).toBeVisible();
      await page.getByRole("radio", { name: "When paid", exact: true }).click();
      await expect(
        page.getByRole("radiogroup", {
          name: "Count costs",
          exact: true,
          description: /^Each cost counts on the day it is paid/,
        }),
      ).toBeVisible();
    },
  ],
  [
    finance("totals-by-week"),
    async (page) => {
      await expect(page.getByRole("table", { name: "Totals by week", exact: true }).getByRole("rowheader")).toHaveText([
        /^2026-W47\s*Nov 16 – 22 · So far$/,
        /^2026-W46\s*Nov 9 – 15$/,
        /^2026-W45\s*Nov 2 – 8$/,
      ]);
    },
  ],
  [
    finance("totals-in-currencies"),
    async (page) => {
      await expect
        .poll(() => readCellByCell(page, "Totals by year"))
        .toContain("2026 So far | Costs | ¥131,400 US$512.30 Estimate");
    },
  ],
  [
    finance("costs-only"),
    async (page) => {
      await expect(
        page.getByRole("table", { name: "Costs by month", exact: true }).getByRole("columnheader"),
      ).toHaveText(["Month", "Costs"]);
    },
  ],
  [
    finance("breakdown-by-provider"),
    async (page) => {
      const name = "Costs of November 2026 by provider";
      await expect(page.getByRole("table", { name, exact: true }).getByRole("row")).toHaveCount(7);
      // The total is the last row, its figures a currency each, in the amounts' column.
      await expect
        .poll(async () => (await readCellByCell(page, name)).slice(-2))
        .toEqual(["Total | Category | ", "Total | Amount | ¥12,300 US$45.67 Estimate"]);
      // A provider named once on the page is still the header of each of its rows, and an estimate is read on its figure.
      await expect
        .poll(async () => (await readCellByCell(page, name)).slice(2, 4))
        .toEqual(["AWS | Category | Storage", "AWS | Amount | US$7.47 Estimate"]);
    },
  ],
  [
    finance("choices"),
    async (page) => {
      await expect(page.getByRole("radio", { name: "When paid", exact: true })).toBeChecked();
      await expect(page.getByRole("radio", { name: "Week", exact: true })).toBeChecked();
    },
  ],
];

/** The totals of the story `totals-by-month` as they are read cell by cell, in every layout. */
const months = [
  ["November 2026 So far", "¥12,300 Estimate", "¥9,840", "−¥2,460 Estimate"],
  ["October 2026", "¥14,950", "¥18,400", "¥3,450"],
  ["September 2026", "¥14,820", "¥11,200", "−¥3,620"],
  ["August 2026", "¥13,100", "¥0", "−¥13,100"],
] as const;

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of the supporters' and the finance parts shows its state, and the totals are read cell by cell at every width", async ({
  page,
  open,
}) => {
  for (const [id, shows] of states)
    await test.step(id, async () => {
      await open(id, { width: pc });
      await shows(page);
    });

  const expected = months.flatMap(([month, ...cells]) =>
    ["Costs", "Income", "Balance"].map((column, i) => `${month} | ${column} | ${cells[i] ?? ""}`),
  );
  for (const [layout, width] of [
    ["wide", pc],
    ["narrow", minimum],
  ] as const)
    await test.step(`the totals by month, ${layout}`, async () => {
      await open(finance("totals-by-month"), { width });
      await expect.poll(() => readCellByCell(page, "Totals by month")).toEqual(expected);
    });

  for (const [layout, width] of [
    ["wide", pc],
    ["narrow", minimum],
  ] as const)
    await test.step(`a figure in two currencies, ${layout}: each currency on a line of its own`, async () => {
      await open(finance("totals-in-currencies"), { width });
      const cell = page
        .getByRole("table", { name: "Totals by year", exact: true })
        .getByRole("cell", { name: /^¥142,800\s*US\$1,204\.88$/ });
      await expect.poll(() => textLines(cell)).toEqual(["¥142,800", "US$1,204.88"]);
    });

  await test.step("on a phone, each segment of a choice is a 44 px target", async () => {
    await open(supporters("tip-amount-choice"), { width: minimum });
    await expect.poll(() => smallestSide(page.getByRole("radio"))).toEqual({ count: 13, below: [] });
  });
});
