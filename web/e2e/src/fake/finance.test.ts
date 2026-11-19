// SPDX-License-Identifier: AGPL-3.0-only

// The fake's finance (finance.ts), as the screens of the finance rely on them: nothing where the instance keeps it
// private, the totals or the breakdown where it publishes them, the income beside them and the balance only where both
// are public, the two ways of counting a cost longer than a month, the estimate of the open month, and what a holder of
// `view_finance` reads. Unit tests, without a DOM.

import { describe, expect, test } from "bun:test";
import type { components } from "@mycroblog/mykit";
import { createFakeApi, fakeOrigin, type FakeApi, type FakeFinanceOptions, type FakeOptions } from "./index";

type Schemas = components["schemas"];
type Answered<T> = { status: number; body: T };

async function ask<T>(fake: FakeApi, path: string): Promise<Answered<T>> {
  const response = await fake.serve(new Request(`${fakeOrigin}/api/v1${path}`));
  return { status: response.status, body: (await response.json()) as T };
}

const problemOf = (a: Answered<unknown>) => [a.status, (a.body as Schemas["Problem"]).type];
const today = () => new Date().toISOString().slice(0, 10);
const fakeWith = (finance: FakeFinanceOptions, more: FakeOptions = {}) => createFakeApi({ ...more, finance });
const costs = (fake: FakeApi, query: string) => ask<Schemas["CostTotals"]>(fake, `/instance/cost_totals?${query}`);
const income = (fake: FakeApi, query: string) => ask<Schemas["IncomeTotals"]>(fake, `/instance/income_totals?${query}`);

describe("the public finance", () => {
  test("private by default: the instance says so and the totals are a 404", async () => {
    const fake = createFakeApi();
    const instance = (await ask<Schemas["Instance"]>(fake, "/instance")).body;
    expect([instance.finance_visibility, instance.income_visibility]).toEqual(["private", "private"]);
    expect((await costs(fake, `granularity=month&from=2026-01-01&to=${today()}`)).status).toBe(404);
    expect((await income(fake, `granularity=month&from=2026-01-01&to=${today()}`)).status).toBe(404);
  });

  test("the totals alone leave out the breakdown and the providers' rules; the breakdown has both", async () => {
    const query = "granularity=month&from=2026-03-01&to=2026-04-30";
    const totals = (await costs(fakeWith({ costs: "totals" }), query)).body;
    expect(totals.periods.map((p) => [p.label, p.totals.map((t) => t.currency), p.breakdown])).toEqual([
      ["2026-03", ["JPY", "USD"], []],
      ["2026-04", ["JPY", "USD"], []],
    ]);
    expect(totals.rules.sources).toEqual([]);
    const breakdown = (await costs(fakeWith({ costs: "breakdown" }), query)).body;
    expect(breakdown.periods[0]?.breakdown.map((l) => l.provider)).toContain("AWS");
    expect(breakdown.rules.sources.map((s) => [s.provider, s.time_zone])).toEqual([["AWS", "UTC"]]);
  });

  test("a yearly cost spreads by day when amortized, and falls on its date in cash", async () => {
    const fake = fakeWith({ costs: "breakdown" });
    const domainIn = async (basis: string) =>
      (await costs(fake, `granularity=month&from=2026-04-01&to=2026-05-31&basis=${basis}`)).body.periods.map(
        (p) => p.breakdown.find((l) => l.provider === "Gandi")?.amount.amount,
      );
    // ¥1,650 a year from each 04-10 over 365 days (ADR-0067 §4): April has the last 9 days of 2025's (1650 − 1609)
    // and the first 21 of 2026's (94), May 31 more of 2026's (235 − 94).
    const [amortized, cash] = [await domainIn("amortized"), await domainIn("cash")];
    expect(amortized).toEqual(["135", "141"]);
    expect(cash).toEqual(["1650", undefined]);
  });

  test("weeks are ISO weeks from Monday, and the periods stop at today's", async () => {
    const weeks = (await costs(fakeWith({ costs: "totals" }), "granularity=week&from=2026-01-01&to=2026-01-12")).body;
    expect(weeks.periods.map((p) => [p.label, p.starts_on, p.ends_on])).toEqual([
      ["2026-W01", "2025-12-29", "2026-01-04"],
      ["2026-W02", "2026-01-05", "2026-01-11"],
      ["2026-W03", "2026-01-12", "2026-01-18"],
    ]);
    const ahead = (await costs(fakeWith({ costs: "totals" }), "granularity=year&from=2025-01-01&to=2099-12-31")).body;
    expect(ahead.periods.at(-1)?.label).toBe(today().slice(0, 4));
    const tooMany = await costs(fakeWith({ costs: "totals" }), "granularity=month&from=2020-01-01&to=2026-01-31");
    expect([tooMany.status, (tooMany.body as unknown as Schemas["Problem"]).field]).toEqual([422, "to"]);
  });

  test("the open month is an estimate where the options say so", async () => {
    const month = `granularity=month&from=${today()}&to=${today()}`;
    expect((await costs(fakeWith({ costs: "totals", estimated: true }), month)).body.periods[0]?.estimated).toBe(true);
    expect((await costs(fakeWith({ costs: "totals" }), month)).body.periods[0]?.estimated).toBe(false);
  });

  test("the income: its net, the members of each plan where shown, the balance only where the costs are public", async () => {
    const query = "granularity=month&from=2026-03-01&to=2026-03-31";
    // In cash, March has its monthly payments alone; amortized, a part of the yearly ones of the June before too.
    const alone = (await income(fakeWith({ income: "totals" }), `${query}&basis=cash`)).body;
    expect([alone.periods[0]?.net, alone.periods[0]?.balance, alone.plans]).toEqual([
      [{ amount: "4520", currency: "JPY" }],
      undefined,
      undefined,
    ]);
    const amortized = (await income(fakeWith({ income: "totals" }), query)).body;
    expect(Number(amortized.periods[0]?.net[0]?.amount)).toBeGreaterThan(4520);
    const both = (await income(fakeWith({ income: "plans", costs: "totals" }, { supporters: {} }), query)).body;
    expect(both.periods[0]?.balance?.map((m) => m.currency)).toEqual(["JPY", "USD"]);
    expect(both.plans?.map((p) => [p.name, p.members])).toEqual([
      ["Supporter", 2],
      ["Patron", 2],
    ]);
  });

  test("the funding meter of the month, as the options show it", async () => {
    const meter = async (options: FakeFinanceOptions) =>
      (await ask<Schemas["Instance"]>(fakeWith(options), "/instance")).body.funding_meter;
    expect([(await meter({}))?.visibility, (await meter({}))?.income]).toEqual(["percent", undefined]);
    const both = await meter({ costs: "totals", income: "totals", meter: "amounts" });
    expect(both?.target).toEqual({ amount: "20000", currency: "JPY" });
    const none = await meter({ meter: "none" });
    expect(none).toBeUndefined();
  });

  test("the meter shows only as the visibility of both sides allows (FR-905)", async () => {
    const meter = async (options: FakeFinanceOptions) =>
      (await ask<Schemas["Instance"]>(fakeWith(options), "/instance")).body.funding_meter;
    // The share alone only while both sides are private; the amounts only while both are public; none for one side.
    const shown = [
      await meter({ costs: "totals", meter: "percent" }),
      await meter({ meter: "amounts" }),
      await meter({ income: "totals", meter: "amounts" }),
    ];
    expect(shown).toEqual([undefined, undefined, undefined]);
  });

  test("the month's costs are a target only in one currency, and their estimate is the meter's only then", async () => {
    const meter = async (options: FakeFinanceOptions) =>
      (await ask<Schemas["Instance"]>(fakeWith(options), "/instance")).body.funding_meter;
    const both = { costs: "totals", income: "totals", meter: "amounts", estimated: true } as const;
    // In yen and dollars the costs make no target; an amount the operators set has no estimate in it.
    const mixed = await meter({ ...both, meterTarget: "costs" });
    const set = await meter(both);
    expect([mixed, set?.estimated]).toEqual([undefined, false]);
    const dollars = await meter({ ...both, meterTarget: "costs", currencies: ["USD"] });
    expect([dollars?.target?.currency, dollars?.target_is_costs, dollars?.estimated]).toEqual(["USD", true, true]);
  });

  test("the income counts no movement after today", async () => {
    const fake = fakeWith({ viewer: {} });
    const year = new Date().getUTCFullYear() + 1;
    const ahead = await ask<Schemas["IncomeTotals"]>(
      fake,
      `/finance/income_totals?granularity=year&from=${String(year)}-01-01&to=${String(year)}-12-31&basis=cash`,
    );
    expect(ahead.body.periods.map((p) => p.net)).toEqual([[]]);
  });
});

describe("the finance for a holder of view_finance", () => {
  const query = "granularity=month&from=2026-03-01&to=2026-03-31";
  const finance = (fake: FakeApi, kind: "cost" | "income") =>
    ask<Schemas["FinanceCostTotals"] & Schemas["IncomeTotals"]>(fake, `/finance/${kind}_totals?${query}`);

  test("refused without the permission", async () => {
    expect(problemOf(await finance(createFakeApi(), "cost"))).toEqual([403, "/api/v1/problems/missing-permission"]);
  });

  test("the breakdown always; the items' names and notes as the settings show them", async () => {
    const none = (await finance(fakeWith({ viewer: {} }), "cost")).body;
    expect([none.periods[0]?.breakdown.length !== 0, none.periods[0]?.items, none.viewer.item_names]).toEqual([
      true,
      [],
      false,
    ]);
    const memos = (await finance(fakeWith({ viewer: { itemMemos: true } }), "cost")).body.periods[0]?.items ?? [];
    expect(memos.some((i) => i.memo !== undefined) && memos.every((i) => i.name === undefined)).toBe(true);
    const names = (await finance(fakeWith({ viewer: { itemNames: true } }), "cost")).body.periods[0]?.items ?? [];
    expect(names.map((i) => i.name)).toContain("VPS");
  });

  test("the income with its balance whatever is published; the plans where the settings show them", async () => {
    const totals = (await finance(fakeWith({ viewer: {} }, { supporters: {} }), "income")).body;
    expect([totals.periods[0]?.balance !== undefined, totals.plans]).toEqual([true, undefined]);
    const plans = (await finance(fakeWith({ viewer: { income: "plans" } }, { supporters: {} }), "income")).body;
    expect(plans.plans?.length).toBe(2);
    // The admin holds view_finance and manage_finance: every item line, named, and the plans.
    const admin = fakeWith({}, { viewer: "hana", supporters: {} });
    expect((await finance(admin, "income")).body.plans?.length).toBe(2);
    expect((await finance(admin, "cost")).body.periods[0]?.items.every((i) => i.name !== undefined)).toBe(true);
  });
});
