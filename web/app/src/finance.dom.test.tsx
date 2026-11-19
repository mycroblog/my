// SPDX-License-Identifier: AGPL-3.0-only

// The finance screens (ADR-0067 §4, §5, §8, §17; ADR-0070 §5; FR-902, FR-916), on the fake API: what a story cannot
// reach, as it follows a press. The period's length changes the totals' rows and is said; the way a payment counts
// asks the server again and says what it means; Earlier and Later page through the periods by whole windows, the focus
// kept; a figure a currency, a zero of income beside a currency's costs, a balance a currency, an estimate on the
// imported provider's currency, a bar only for a figure in one currency; a server that publishes nothing is asked
// nothing, and a side it stops publishing goes; the billing rules only with the breakdown; today is the reader's date
// and moves on with it; the rail's funding meter leads to the public page; and the finance page tells a reader who
// may not read it so, and shows a holder the items as the settings allow. What each state looks like is the stories'
// (finance.stories.tsx).

import { describe, expect, onTestFinished, setSystemTime, test } from "bun:test";
import { financePeriodAfter } from "@my/app-data";
import { fakeOrigin, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { heldRepeatsOf, main, renderApp, words } from "./test/app";
import { fakeApp } from "./test/server";

const published: FakeOptions["finance"] = { costs: "breakdown", income: "totals", meter: "none" };

/** The table of the totals by `granularity`. */
const totals = (granularity: "week" | "month" | "year") =>
  main().findByRole("table", { name: `Totals by ${granularity}` });

/** The heads of the rows of `table`, the columns' row left out. */
const heads = (table: HTMLElement) =>
  within(table)
    .getAllByRole("rowheader")
    .map((h) => words(h).trim());

/** Waits until one of the page's live regions says `sentence`. */
const says = (sentence: RegExp) =>
  waitFor(() => {
    const said = screen.queryAllByRole("status").map((s) => words(s));
    if (!said.some((text) => sentence.test(text))) throw new Error(`test: the page says ${JSON.stringify(said)}`);
  });

/** The month that holds the day `n` months before today, as the table heads it. */
const monthBack = (n: number) => {
  const today = new Date();
  const first = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth() - n, 1));
  return new Intl.DateTimeFormat("en", { timeZone: "UTC", year: "numeric", month: "long" }).format(first);
};

test("the public page shows the last six months, the latest first, and a week's choice shows eight weeks and says so", async () => {
  const app = await renderApp("/about/costs", { finance: published });
  const byMonth = await totals("month");
  expect(heads(byMonth)).toHaveLength(6);
  expect(heads(byMonth)[0]).toStartWith(monthBack(0));
  await app.user.click(main().getByRole("radio", { name: "Week" }));
  const byWeek = await totals("week");
  expect(heads(byWeek)).toHaveLength(8);
  expect(heads(byWeek)[0]).toMatch(/^Week \d+, \d{4}/);
  await says(/^Showing Week \d+, \d{4} to Week \d+, \d{4}\.$/);
});

test("In the period paid asks for the totals by the day each payment was made, and says what it means", async () => {
  const app = await renderApp("/about/costs", { finance: published });
  await totals("month");
  const choice = main().getByRole("radiogroup", { name: "How payments count" });
  await app.user.click(within(choice).getByRole("radio", { name: "In the period paid" }));
  expect(within(choice).getByRole<HTMLInputElement>("radio", { name: "In the period paid" }).checked).toBe(true);
  const about = document.getElementById(choice.getAttribute("aria-describedby") ?? "");
  expect(about === null ? "" : words(about)).toStartWith("Each payment counts on the day it was paid");
  // The table shows the server's cash totals: their currencies in the latest month's costs cell.
  const today = new Date().toISOString().slice(0, 10);
  const from = `${today.slice(0, 8)}01`;
  const answer = await app.fake.serve(
    new Request(`${fakeOrigin}/api/v1/instance/cost_totals?granularity=month&from=${from}&to=${today}&basis=cash`),
  );
  const body = (await answer.json()) as { periods: { totals: { amount: string; currency: string }[] }[] };
  const shown = (body.periods[0]?.totals ?? []).map((m) =>
    new Intl.NumberFormat("en", { style: "currency", currency: m.currency }).format(m.amount as `${number}`),
  );
  await waitFor(async () => {
    const row = within(await totals("month")).getAllByRole("row")[1];
    for (const figure of shown) expect(words(row as HTMLElement)).toContain(figure);
  });
});

test("Earlier months shows the six before, Later months comes back, and the focus stays on the way", async () => {
  const app = await renderApp("/about/costs", { finance: published });
  await totals("month");
  await app.user.click(main().getByRole("button", { name: "Earlier months" }));
  await waitFor(async () => {
    expect(heads(await totals("month"))[0]).toStartWith(monthBack(6));
  });
  await says(new RegExp(`^Showing ${monthBack(11)} to ${monthBack(6)}\\.$`));
  const later = main().getByRole("button", { name: "Later months" });
  await app.user.click(later);
  await waitFor(async () => {
    expect(heads(await totals("month"))[0]).toStartWith(monthBack(0));
  });
  // Later is gone with the latest periods shown: the focus is on Earlier.
  expect(main().queryByRole("button", { name: "Later months" })).toBeNull();
  await waitFor(() => {
    expect(app.focused()).toBe(main().getByRole("button", { name: "Earlier months" }));
  });
});

test("a server that publishes nothing says so, and is asked for no totals", async () => {
  await renderApp("/about/costs", { finance: { meter: "none" } });
  expect(await main().findByText("This server does not publish its costs or income.")).toBeDefined();
  expect(main().queryByRole("table")).toBeNull();
  expect(main().queryByRole("radiogroup")).toBeNull();
});

test("costs the server stops publishing while the page is open go, with no Try again, and the instance is read again", async () => {
  const on = fakeApp({ finance: published });
  const app = await renderApp("/about/costs", { on });
  await totals("month");
  // The server now keeps its costs private: their totals answer 404, and the instance says so once read again.
  on.fake.script("GET /instance/cost_totals", { fail: 404 });
  const instance = on.fake.script("GET /instance", {
    change: (body) => ({ ...body, finance_visibility: "private" as const }),
  });
  await app.user.click(main().getByRole("button", { name: "Earlier months" }));
  expect(await main().findByText("This server does not publish its costs.")).toBeDefined();
  await instance.arrived();
  expect(main().queryByRole("button", { name: "Try again" })).toBeNull();
  const byMonth = await totals("month");
  expect(
    within(byMonth)
      .getAllByRole("columnheader")
      .map((h) => words(h)),
  ).toEqual(["Month", "Income"]);
  expect(main().queryByRole("table", { name: / by provider$/ })).toBeNull();
});

test("a visitor reads the published totals", async () => {
  await renderApp("/about/costs", { finance: published, visitor: true });
  expect(heads(await totals("month"))).toHaveLength(6);
  expect(await main().findByRole("table", { name: new RegExp(`^${monthBack(0)} by provider$`) })).toBeDefined();
});

test("the funding meter in the column beside the timeline leads to the public page", async () => {
  await renderApp("/", { finance: { ...published, meter: "amounts" } });
  const around = await screen.findByRole("complementary");
  const heading = await within(around).findByRole("link", { name: "Funding" });
  expect(heading.getAttribute("href")).toBe("/about/costs");
});

test("the finance page tells a reader who may not read it so", async () => {
  await renderApp("/finance", { finance: { meter: "none" } });
  expect(
    await main().findByText(
      "Only people allowed to read this server's finances can see this page. To read them, ask the server's administrators.",
    ),
  ).toBeDefined();
  expect(main().queryByRole("table")).toBeNull();
});

test("a holder of view_finance reads the items' names and notes only where the settings show them", async () => {
  await renderApp("/finance", { finance: { viewer: {}, meter: "none" } });
  expect(heads(await totals("month"))).toHaveLength(6);
  expect(main().queryByRole("table", { name: / by item$/ })).toBeNull();
});

test("a holder of view_finance with names and notes shown reads the month's items", async () => {
  await renderApp("/finance", { finance: { viewer: { itemNames: true, itemMemos: true }, meter: "none" } });
  const items = await main().findByRole("table", { name: `${monthBack(0)} by item` });
  expect(heads(items).some((h) => h.startsWith("VPS") && h.endsWith("2 vCPU, 4 GB, Tokyo zone"))).toBe(true);
});

/** The row of `table` headed by `head` (a period's name, a provider), its caption ("So far") allowed after it. */
function rowOf(table: HTMLElement, head: string): HTMLElement {
  const row = within(table)
    .getAllByRole("row")
    .find((r) => {
      const header = within(r).queryAllByRole("rowheader")[0];
      // The caption follows the name with no space between ("November 2026So far").
      return header !== undefined && words(header).startsWith(head) && !/^\d/.test(words(header).slice(head.length));
    });
  if (row === undefined) throw new Error(`test: no row of ${table.getAttribute("aria-label") ?? ""} is headed ${head}`);
  return row;
}

/** The cells of `row` after its header, each as its words. */
const cells = (row: HTMLElement) =>
  within(row)
    .getAllByRole("cell")
    .map((c) => words(c));

/** An amount as the screen writes it in en. */
const written = (m: { amount: string; currency: string }) =>
  new Intl.NumberFormat("en", { style: "currency", currency: m.currency }).format(m.amount as `${number}`);

/** The lengths of the bars drawn in `cell`: what is hidden from assistive technology and holds a fill of a length. */
const barsIn = (cell: HTMLElement) =>
  [...cell.querySelectorAll('[aria-hidden="true"]')].flatMap((hidden) => {
    const fill = hidden.firstElementChild;
    return fill instanceof HTMLElement && fill.style.inlineSize !== "" ? [fill.style.inlineSize] : [];
  });

/** A period `n` windows of `granularity` before the one that holds today (UTC), as the table heads it. */
function headBack(granularity: "week" | "year", n: number): string {
  const today = new Date().toISOString().slice(0, 10);
  const label = financePeriodAfter(granularity, today, -n)?.label ?? "";
  if (granularity === "year") return label;
  const [year = "", week = ""] = label.split("-W");
  return `Week ${String(Number(week))}, ${year}`;
}

test.each([
  ["Week", "week", 8, "Earlier weeks"],
  ["Year", "year", 5, "Earlier years"],
] as const)(
  "a choice of %s shows its window of periods, and Earlier steps back by a whole window",
  async (choice, granularity, count, earlier) => {
    const app = await renderApp("/about/costs", { finance: published });
    await totals("month");
    const period = main().getByRole("radiogroup", { name: "Period" });
    await app.user.click(within(period).getByRole("radio", { name: choice }));
    expect(heads(await totals(granularity))).toHaveLength(count);
    expect(heads(await totals(granularity))[0]).toStartWith(headBack(granularity, 0));
    await app.user.click(main().getByRole("button", { name: earlier }));
    await waitFor(async () => {
      expect(heads(await totals(granularity))[0]).toStartWith(headBack(granularity, count));
    });
    expect(heads(await totals(granularity))).toHaveLength(count);
    expect(heads(await totals(granularity)).at(-1)).toStartWith(headBack(granularity, 2 * count - 1));
  },
);

test("each currency is a figure of its own, and the income has a zero in a currency it has costs in and no income", async () => {
  const app = await renderApp("/about/costs", { finance: published });
  const table = await totals("month");
  const today = new Date().toISOString().slice(0, 10);
  const answer = await app.fake.serve(
    new Request(
      `${fakeOrigin}/api/v1/instance/income_totals?granularity=month&from=${today.slice(0, 8)}01&to=${today}&basis=amortized`,
    ),
  );
  type Money = { amount: string; currency: string };
  const body = (await answer.json()) as { periods: { net: Money[]; balance?: Money[] }[] };
  const latest = body.periods.at(-1);
  // The fake's income is in yen alone, its costs in yen and dollars.
  expect(latest?.net.map((m) => m.currency)).toEqual(["JPY"]);
  const [, income, balance] = cells(rowOf(table, monthBack(0)));
  expect(income).toBe([...(latest?.net ?? []), { amount: "0", currency: "USD" }].map(written).join(""));
  // The balance of each currency, as the server answers it: never one sum of yen and dollars.
  expect(balance).toBe((latest?.balance ?? []).map(written).join(""));
  expect(latest?.balance?.map((m) => m.currency)).toEqual(["JPY", "USD"]);
});

test("an estimate is marked on the figures in its imported provider's currency, however the breakdown spells the provider", async () => {
  const on = fakeApp({ finance: { ...published, estimated: true } });
  // The breakdown spells the imported provider in full width: the server groups it with AWS (NFKC, lower case).
  on.fake.script("GET /instance/cost_totals", {
    change: (body) => ({
      ...body,
      periods: body.periods.map((p) => ({
        ...p,
        breakdown: p.breakdown.map((line) => (line.provider === "AWS" ? { ...line, provider: "ＡＷＳ" } : line)),
      })),
    }),
  });
  await renderApp("/about/costs", { on });
  const table = await totals("month");
  const [costs, , balance] = cells(rowOf(table, monthBack(0)));
  expect(costs).toMatch(/^¥[\d,]+\$[\d,.]+ Estimate$/);
  expect(balance).toMatch(/^-?¥[\d,]+-?\$[\d,.]+ Estimate$/);
  // The month before is closed.
  expect(cells(rowOf(table, monthBack(1))).join(" ")).not.toContain("Estimate");
  const breakdown = await main().findByRole("table", { name: `${monthBack(0)} by provider` });
  expect(cells(rowOf(breakdown, "ＡＷＳ")).at(-1)).toMatch(/^\$[\d,.]+ Estimate$/);
  // A line in the same currency that is not imported is no estimate.
  expect(cells(rowOf(breakdown, "Cloudflare")).at(-1)).toMatch(/^\$[\d,.]+$/);
});

test.each([
  ["in yen alone, each figure above zero draws a bar", ["JPY"], 1],
  ["in yen and dollars, a cell of the two draws none", ["JPY", "USD"], 0],
] as const)("the totals' costs and income %s", async (_, currencies, bars) => {
  await renderApp("/about/costs", { finance: { ...published, costs: "totals", currencies } });
  const row = rowOf(await totals("month"), monthBack(1));
  const [costs, income] = within(row).getAllByRole("cell");
  expect([costs, income].map((c) => (c === undefined ? "none" : (words(c).match(/[¥$]/g) ?? []).join("")))).toEqual(
    currencies.length === 1 ? ["¥", "¥"] : ["¥$", "¥$"],
  );
  expect([costs, income].map((c) => (c === undefined ? -1 : barsIn(c).length))).toEqual([bars, bars]);
});

test("the income the server stops publishing while the page is open goes, with no Try again, and the instance is read again", async () => {
  const on = fakeApp({ finance: published });
  const app = await renderApp("/about/costs", { on });
  await totals("month");
  on.fake.script("GET /instance/income_totals", { fail: 404 });
  const instance = on.fake.script("GET /instance", {
    change: (body) => ({ ...body, income_visibility: "private" as const }),
  });
  await app.user.click(main().getByRole("button", { name: "Earlier months" }));
  expect(await main().findByText("This server does not publish its income.")).toBeDefined();
  await instance.arrived();
  expect(main().queryByRole("button", { name: "Try again" })).toBeNull();
  await waitFor(async () => {
    expect(
      within(await totals("month"))
        .getAllByRole("columnheader")
        .map((h) => words(h)),
    ).toEqual(["Month", "Costs"]);
  });
  // The costs stay, with their breakdown.
  expect(await main().findByRole("table", { name: / by provider$/ })).toBeDefined();
});

test.each([
  ["totals", false],
  ["breakdown", true],
] as const)("with the costs' %s published, the providers' billing rules are said: %p", async (costs, said) => {
  await renderApp("/about/costs", { finance: { costs, meter: "none" } });
  await totals("month");
  const page = words(screen.getByRole("main"));
  expect(page).toContain("Periods are calendar dates with no time zone, and a week starts on Monday.");
  expect(page.includes("AWS bills by calendar month in UTC")).toBe(said);
});

describe("today", () => {
  test("is the reader's date in their time zone, and moves on when the date changes while the page is open", async () => {
    // In UTC it is December already; in Los Angeles still the last evening of November.
    setSystemTime(new Date("2026-12-01T03:00:00Z"));
    onTestFinished(() => {
      setSystemTime();
    });
    // The page looks at the date once a minute: the test takes the minute's look.
    const minute = heldRepeatsOf(60_000);
    await renderApp("/about/costs", {
      finance: published,
      preferences: { time_zone: "America/Los_Angeles", time_zone_follows_browser: false },
    });
    await waitFor(async () => {
      expect(heads(await totals("month"))[0]).toStartWith("November 2026");
    });
    expect(heads(await totals("month"))).toHaveLength(6);
    // It turns midnight in Los Angeles: the page's next look at the date shows December.
    setSystemTime(new Date("2026-12-01T09:00:00Z"));
    expect(minute.tick()).toBeGreaterThan(0);
    await waitFor(async () => {
      expect(heads(await totals("month"))[0]).toStartWith("December 2026");
    });
  });
});
