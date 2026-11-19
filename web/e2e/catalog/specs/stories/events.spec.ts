// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the parts of an event (web/design-system/src/event.stories.tsx, #3236) each show the state they are
// named by, and the date poll's table reaches a screen reader as a table in both its layouts: wide, a row a date
// under a row of the columns' names; and stacked under 32rem, each date on its lines and the columns' row for
// assistive technology alone. Both are read off the tree Chromium gives assistive technology (the DevTools protocol's
// Accessibility domain), which no DOM simulation has. What each part says of its roles and names is its component
// tests' (web/design-system/src/event.dom.test.tsx); the stories' accessibility checks are the check of every story.

import type { Locator, Page } from "@playwright/test";
import { shownTexts } from "../boxes";
import { expect, minimum, pc, test } from "../test";

/** The story of the parts of an event named `name`. */
const story = (name: string) => `design-system-events--${name}`;

/** The date poll of a story. */
const pollOf = (page: Page) => page.getByRole("table", { name: "Date poll", exact: true });

/** The row of the date poll that names its columns. */
const columnsRow = (page: Page) =>
  pollOf(page)
    .getByRole("row")
    .filter({ has: page.getByRole("columnheader", { name: "Date", exact: true }) });

/** The groups of the reader's answer named `name` (an answer alone; a date's answer in the poll). */
const answers = (page: Page, name: string | RegExp) =>
  page.getByRole("radiogroup", { name, exact: typeof name === "string" });

/**
 * The table named `name` as Chromium gives it to assistive technology, read cell by cell: for each row after the
 * first, each of its cells as `<the row's header> | <the column's header> | <the cell>`, a column's header counted by
 * the cell's place in its row; the first row is the columns' headers, and a row that does not line up with it, or a
 * table that is not there, is told as such. A name is the one Chromium computes, its spaces folded.
 */
async function readCellByCell(page: Page, name: string): Promise<string[]> {
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
      const [date, ...cells] = under(row, ["columnheader", "rowheader", "cell"]);
      if (date === undefined || roleOf(date) !== "rowheader" || cells.length + 1 !== columns.length)
        return [`a row of ${[date, ...cells].map((cell) => (cell ? roleOf(cell) : "nothing")).join(", ")}`];
      return cells.map(
        (cell, i) =>
          `${nameOf(date)} | ${columns[i + 1] ? nameOf(columns[i + 1] as Node) : ""} | ${
            roleOf(cell) === "cell" ? nameOf(cell) : roleOf(cell)
          }`,
      );
    });
  } finally {
    await cdp.detach();
  }
}

/** Says that `part` holds `count` radios, and that none of them can be chosen. */
const everyRadioDisabled = async (part: Locator | Page, count: number) => {
  await expect(part.getByRole("radio")).toHaveCount(count);
  await expect(part.getByRole("radio", { disabled: false })).toHaveCount(0);
};

const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    "event-page",
    async (page) => {
      await expect(
        page.getByRole("heading", { name: "Book club: The Left Hand of Darkness", exact: true }),
      ).toBeVisible();
      await expect(page.getByRole("list", { name: "Where", exact: true })).toBeVisible();
      await expect(answers(page, "Your answer")).toBeVisible();
      await expect(pollOf(page)).toBeVisible();
    },
  ],
  [
    "event-lines",
    async (page) => {
      const lines = page.getByRole("list", { name: "Upcoming events", exact: true }).getByRole("link");
      await expect(lines).toHaveCount(6);
      await expect(lines.filter({ hasText: /Cancelled/ })).toHaveCount(1);
      await expect(lines.filter({ hasText: /Ended/ })).toHaveCount(1);
      await expect(lines.filter({ hasText: /Date to be decided/ })).toHaveCount(1);
    },
  ],
  [
    "in-the-column-beside",
    async (page) => {
      await expect(page.getByRole("heading", { name: /^Upcoming events$/ })).toBeVisible();
      await expect(page.getByRole("link", { name: "Release party for v0.2", exact: true })).toBeVisible();
    },
  ],
  [
    "times",
    async (page) => {
      await expect(page.getByText("Sat 2:00 PM (Tokyo)", { exact: true })).toHaveCount(2);
      await expect(page.getByText("Mon, Nov 23, 7:00 – 8:30 PM", { exact: true })).toBeVisible();
    },
  ],
  [
    "answer-unanswered",
    async (page) => {
      await expect(answers(page, "Your answer").getByRole("radio")).toHaveCount(3);
      await expect(page.getByRole("radio", { checked: true })).toHaveCount(0);
      // The choice carries no explanatory words: its answers alone.
      await expect(answers(page, "Your answer")).toHaveText(/^Going\s*Maybe\s*Not going\s*$/);
      // Each radio covers its whole answer, so a press on the control itself chooses it.
      const maybe = page.getByRole("radio", { name: "Maybe", exact: true });
      await maybe.click();
      await expect(maybe).toBeChecked();
    },
  ],
  [
    "answer-saving",
    async (page) => {
      await expect(answers(page, "Your answer")).toHaveCount(4);
      const said = page.getByRole("status");
      await expect(said.filter({ hasText: /^Saving…$/ })).toHaveCount(1);
      await expect(said.filter({ hasText: /^Saved$/ })).toHaveCount(1);
      await expect(said.filter({ hasText: /^This could not be saved\.\s*Try again$/ })).toHaveCount(1);
      await expect(page.getByRole("button", { name: "Try again", exact: true })).toBeVisible();
    },
  ],
  [
    "answer-at-capacity",
    async (page) => {
      // The short reason of a disabled answer stays, and describes the choice.
      await expect(
        page.getByRole("radiogroup", {
          name: "Your answer",
          exact: true,
          description: "The event is full: no one more can go.",
        }),
      ).toBeVisible();
      await expect(page.getByText("The event is full: no one more can go.", { exact: true })).toBeVisible();
      await expect(page.getByRole("radio", { name: "Going", exact: true })).toBeDisabled();
      await expect(page.getByRole("radio", { name: "Maybe", exact: true })).toBeEnabled();
      await expect(page.getByRole("radio", { name: "Not going", exact: true })).toBeEnabled();
    },
  ],
  [
    "answer-disabled",
    async (page) => {
      await expect(answers(page, "Your answer")).toHaveCount(2);
      await everyRadioDisabled(page, 6);
      await expect(page.getByRole("radio", { checked: true })).toHaveCount(1);
      await expect(page.getByText("You can answer once a date is fixed.", { exact: true })).toBeVisible();
      await expect(
        page.getByText("The event has started: answers can no longer change.", { exact: true }),
      ).toBeVisible();
    },
  ],
  [
    "date-poll",
    async (page) => {
      await expect(pollOf(page).getByRole("rowheader")).toHaveCount(4);
      await expect(pollOf(page).getByRole("columnheader", { name: "Actions", exact: true })).toHaveCount(0);
      await expect(pollOf(page).getByRole("radio", { checked: true })).toHaveCount(3);
    },
  ],
  [
    "date-poll-for-organizer",
    async (page) => {
      await expect(pollOf(page).getByRole("columnheader", { name: "Actions", exact: true })).toHaveCount(1);
      await expect(
        pollOf(page).getByRole("button", { name: /^Options for (Sat|Sun), Dec \d+(, \d:00 PM)?$/ }),
      ).toHaveCount(4);
      await expect(page.getByRole("status").filter({ hasText: /^Saved$/ })).toHaveCount(1);
    },
  ],
  [
    "date-poll-confirmed",
    async (page) => {
      await expect(
        pollOf(page)
          .getByRole("rowheader")
          .filter({ hasText: /Confirmed$/ }),
      ).toHaveCount(1);
      await expect(pollOf(page).getByRole("columnheader", { name: "Actions", exact: true })).toHaveCount(0);
      await everyRadioDisabled(pollOf(page), 12);
      await expect(pollOf(page).getByRole("radio", { checked: true })).toHaveCount(3);
    },
  ],
  [
    "date-actions-open",
    async (page) => {
      await expect(page.getByRole("menuitem", { name: "Confirm this date…", exact: true })).toBeVisible();
      await expect(page.getByRole("menuitem", { name: "Withdraw this date…", exact: true })).toBeVisible();
    },
  ],
  [
    "places-before-the-start",
    async (page) => {
      await expect(page.getByText("Opens at the start", { exact: true })).toBeVisible();
      await expect(
        page.getByRole("link", { name: "Open the map (opens maps.example.com in a new tab)", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("link", { name: "The stream (opens video.example.org in a new tab)", exact: true }),
      ).toBeVisible();
    },
  ],
  [
    "places-room-open-or-not",
    async (page) => {
      await expect(page.getByRole("link", { name: "Join the room", exact: true })).toBeVisible();
      await expect(
        page.getByText("Could not open: the organizer was hosting another room", { exact: true }),
      ).toBeVisible();
    },
  ],
  [
    "places-hidden",
    async (page) => {
      await expect(page.getByRole("list", { name: "Where", exact: true })).toHaveCount(2);
      // A place the reader may not see shows its kind alone, with no words of why.
      const items = page.getByRole("listitem");
      await expect(items.filter({ hasText: /^In person$/ })).toHaveCount(1);
      await expect(items.filter({ hasText: /^Online$/ })).toHaveCount(1);
      await expect(page.getByRole("link")).toHaveCount(0);
    },
  ],
];

/** The date poll of the story `date-poll` as it is read cell by cell, in every layout. */
const first = "Sat, Dec 5, 1:00 PM Sat 6:00 PM (Tokyo)";
const dates = [first, "Sun, Dec 6, 1:00 PM", "Sat, Dec 12, 1:00 PM", "Sat, Dec 12, 6:00 PM"];
const counts = [
  ["5", "2", "1"],
  ["3", "4", "1"],
  ["1", "0", "6"],
  ["0", "0", "0"],
];

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of an event's parts shows its state, and the date poll is read cell by cell at every width", async ({
  page,
  open,
}) => {
  for (const [name, shows] of states)
    await test.step(name, async () => {
      await open(story(name), { width: pc });
      await shows(page);
    });

  const expected = dates.flatMap((date, row) => [
    ...["Can go", "Maybe", "Cannot"].map((column, i) => `${date} | ${column} | ${counts[row]?.[i] ?? ""}`),
    // The answer's cell is read as the radio group it holds, which its date and its column name.
    `${date} | Your answer | ${date} Your answer`,
  ]);
  for (const [layout, width, columnsShown] of [
    ["wide", pc, true],
    ["stacked", minimum, false],
  ] as const)
    await test.step(`the date poll, ${layout}`, async () => {
      await open(story("date-poll"), { width });
      await expect(columnsRow(page)).toHaveCount(1);
      await expect
        .poll(async () => (await shownTexts(columnsRow(page))).length > 0, { message: "the columns' row is shown" })
        .toBe(columnsShown);
      await expect.poll(() => readCellByCell(page, "Date poll")).toEqual(expected);
      for (const date of dates) await expect(answers(page, yoursOn(date))).toHaveCount(1);
      // Each radio covers its whole answer, so a press on the control itself answers, in either layout.
      const cannot = answers(page, yoursOn("Sat, Dec 12, 6:00 PM")).getByRole("radio", { name: "Cannot", exact: true });
      await cannot.click();
      await expect(cannot).toBeChecked();
    });

  await test.step("on a phone, each answer of the poll and each date's actions are a 44 px target", async () => {
    await open(story("date-poll-for-organizer"), { width: minimum });
    await expect.poll(() => smallestSide(pollOf(page).getByRole("radio"))).toEqual({ count: 12, below: [] });
    await expect
      .poll(() => smallestSide(pollOf(page).getByRole("button", { name: /^Options for .+$/ })))
      .toEqual({ count: 4, below: [] });
  });
});

/** The group of the reader's answer on the date whose header reads `date`, then the column's name. */
const yoursOn = (date: string) => new RegExp(`^${date.replace(/[()]/g, "\\$&").replace(/ /g, "\\s*")}\\s*Your answer$`);

/**
 * How many elements `targets` finds, and the sides of those smaller than the decided target (44 CSS px in both
 * directions; the decider's decision on #3236), as `<width>×<height>`.
 */
const smallestSide = (targets: Locator) =>
  targets.evaluateAll((elements) => {
    const below = elements
      .map((element) => element.getBoundingClientRect())
      .filter(({ width, height }) => width < 44 || height < 44)
      .map(({ width, height }) => `${Math.round(width)}×${Math.round(height)}`);
    return { count: elements.length, below };
  });
