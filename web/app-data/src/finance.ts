// SPDX-License-Identifier: AGPL-3.0-only
// The finance of the instance (ADR-0067, ADR-0070 §5; FR-902, FR-903, FR-905, FR-916, FR-917): the totals of its costs
// and of its income by week, month or year, as the instance publishes them to everyone (`finance_visibility`,
// `income_visibility`: nothing where private, the totals, the breakdown by provider and category, the members of each
// plan, the balance only where both sides are public), and as a holder of `view_finance` reads them (always the
// breakdown and the balance, the cost items' names and notes and the members of each plan where the settings show
// them). The periods are calendar dates cut by no time zone; only today is the reader's, which the screen gives (it
// reads the clock in the reader's zone), and the API is asked for dates. A cost longer than a month counts by day over
// what it covers (`amortized`, the default) or on the day it was paid (`cash`). Amounts are the API's (money.ts),
// which the screens write with `Intl.NumberFormat`; the funding meter of the month is the instance's.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import type { AppData } from "./app-data";
import { instanceQuery, type FundingMeter, type Instance } from "./instance";
import { useAppData } from "./provider";

type Schemas = components["schemas"];

/** A calendar date, `YYYY-MM-DD`, without a time or a time zone. */
export type CalendarDate = Schemas["CalendarDate"];
/** How long a period of the totals is. */
export type CostGranularity = Schemas["CostGranularity"];
/** How the totals place a payment: spread over what it covers (`amortized`), or on its date (`cash`). */
export type CostBasis = Schemas["CostBasis"];
/** The category of a cost. */
export type CostCategory = Schemas["CostCategory"];
/** The public totals of the costs. */
export type CostTotals = Schemas["CostTotals"];
/** A period of the costs: its totals by currency, its breakdown, and whether an estimate meets it. */
export type CostPeriodTotal = Schemas["CostPeriodTotal"];
/** A line of the breakdown: a provider, a category, and an amount. */
export type CostBreakdownLine = Schemas["CostBreakdownLine"];
/** How the totals cut the periods: the week's start, and each provider's billing time zone (with the breakdown). */
export type CostTotalsRules = Schemas["CostTotalsRules"];
/** The totals of the costs as a holder of `view_finance` reads them, with what the settings show of the items. */
export type FinanceCostTotals = Schemas["FinanceCostTotals"];
/** A period of the costs for a holder of `view_finance`, with its item lines. */
export type FinanceCostPeriodTotal = Schemas["FinanceCostPeriodTotal"];
/** An item line: a provider, a category, an amount, and its name and note where the settings show them. */
export type FinanceCostItemLine = Schemas["FinanceCostItemLine"];
/** What the settings show a holder of `view_finance`: the items' names, their notes, the income by plan. */
export type FinanceViewerSettings = Schemas["FinanceViewerSettings"];
/** The totals of the income (net of the provider's fees, refunds in their period). */
export type IncomeTotals = Schemas["IncomeTotals"];
/** A period of the income: its net by currency, and its balance against the costs where it is shown. */
export type IncomePeriodTotal = Schemas["IncomePeriodTotal"];
/** The members of a plan now. */
export type IncomePlanCount = Schemas["IncomePlanCount"];
/** How the income totals cut the periods and spread a payment that covers more than `amortize_after_days`. */
export type IncomeTotalsRules = Schemas["IncomeTotalsRules"];
/** What the instance publishes of its costs: nothing, the totals, or the breakdown too. */
export type FinanceVisibility = Schemas["FinanceVisibility"];
/** What the instance publishes of its income: nothing, the totals, or the members of each plan too. */
export type IncomeVisibility = Schemas["IncomeVisibility"];

/** The lengths of a period, the shortest first. */
export const costGranularities = ["week", "month", "year"] as const satisfies readonly CostGranularity[];

/** The ways to place a payment, the default first. */
export const costBases = ["amortized", "cash"] as const satisfies readonly CostBasis[];

/** The categories of a cost, in the server's order. */
export const costCategories = [
  "compute",
  "database",
  "storage",
  "network",
  "domain",
  "email",
  "software",
  "other",
] as const satisfies readonly CostCategory[];

/** The most periods one request of the totals takes (ADR-0067 §9): 104 weeks, 60 months, 20 years. */
export const financePeriodLimits: Readonly<Record<CostGranularity, number>> = { week: 104, month: 60, year: 20 };

// ---- The periods ------------------------------------------------------------------------------------------------------

/** A date of the calendar as its year, month (1–12) and day. */
type Civil = { readonly y: number; readonly m: number; readonly d: number };

/** The days from 1970-01-01 to `c` (proleptic Gregorian, H. Hinnant's days_from_civil). */
function dayNumber({ y, m, d }: Civil): number {
  const year = m <= 2 ? y - 1 : y;
  const era = Math.floor(year / 400);
  const yoe = year - era * 400;
  const doy = Math.floor((153 * (m + (m > 2 ? -3 : 9)) + 2) / 5) + d - 1;
  const doe = yoe * 365 + Math.floor(yoe / 4) - Math.floor(yoe / 100) + doy;
  return era * 146_097 + doe - 719_468;
}

/** The date `n` days from 1970-01-01. */
function civilOf(n: number): Civil {
  const z = n + 719_468;
  const era = Math.floor(z / 146_097);
  const doe = z - era * 146_097;
  const yoe = Math.floor((doe - Math.floor(doe / 1460) + Math.floor(doe / 36_524) - Math.floor(doe / 146_096)) / 365);
  const doy = doe - (365 * yoe + Math.floor(yoe / 4) - Math.floor(yoe / 100));
  const mp = Math.floor((5 * doy + 2) / 153);
  const d = doy - Math.floor((153 * mp + 2) / 5) + 1;
  const m = mp < 10 ? mp + 3 : mp - 9;
  return { y: era * 400 + yoe + (m <= 2 ? 1 : 0), m, d };
}

const two = (n: number) => String(n).padStart(2, "0");
const written = ({ y, m, d }: Civil): CalendarDate => `${String(y).padStart(4, "0")}-${two(m)}-${two(d)}`;

/** The date `date` names; undefined for one that is not `YYYY-MM-DD` or does not exist (2026-02-30). */
function parsed(date: CalendarDate): Civil | undefined {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  if (match === null) return undefined;
  const c = { y: Number(match[1]), m: Number(match[2]), d: Number(match[3]) };
  const back = civilOf(dayNumber(c));
  return back.y === c.y && back.m === c.m && back.d === c.d ? c : undefined;
}

/** A period of the totals: its first and last dates, and its label as the API writes it. */
export type FinancePeriod = {
  readonly startsOn: CalendarDate;
  readonly endsOn: CalendarDate;
  /** `2026-W17` (the ISO 8601 week), `2026-04`, or `2026`. */
  readonly label: string;
};

/** The ISO 8601 week of the day `n`: its year and number, and the day of its Monday. */
function isoWeek(n: number): { year: number; week: number; monday: number } {
  // 1970-01-01 was a Thursday: (n + 3) mod 7 is 0 on a Monday.
  const monday = n - ((((n + 3) % 7) + 7) % 7);
  const year = civilOf(monday + 3).y;
  const firstMonday = (() => {
    const jan4 = dayNumber({ y: year, m: 1, d: 4 });
    return jan4 - ((((jan4 + 3) % 7) + 7) % 7);
  })();
  return { year, week: (monday - firstMonday) / 7 + 1, monday };
}

/** The period of `granularity` that holds `date`; undefined for a date that does not exist. */
export function financePeriodOf(granularity: CostGranularity, date: CalendarDate): FinancePeriod | undefined {
  const c = parsed(date);
  if (c === undefined) return undefined;
  switch (granularity) {
    case "week": {
      const { year, week, monday } = isoWeek(dayNumber(c));
      return {
        startsOn: written(civilOf(monday)),
        endsOn: written(civilOf(monday + 6)),
        label: `${String(year)}-W${two(week)}`,
      };
    }
    case "month": {
      const next = c.m === 12 ? { y: c.y + 1, m: 1, d: 1 } : { y: c.y, m: c.m + 1, d: 1 };
      return {
        startsOn: written({ ...c, d: 1 }),
        endsOn: written(civilOf(dayNumber(next) - 1)),
        label: `${String(c.y).padStart(4, "0")}-${two(c.m)}`,
      };
    }
    case "year":
      return {
        startsOn: written({ y: c.y, m: 1, d: 1 }),
        endsOn: written({ y: c.y, m: 12, d: 31 }),
        label: String(c.y).padStart(4, "0"),
      };
  }
}

/** The period of `granularity` `count` periods after the one that holds `date` (before it, for a negative `count`). */
export function financePeriodAfter(
  granularity: CostGranularity,
  date: CalendarDate,
  count: number,
): FinancePeriod | undefined {
  const c = parsed(date);
  if (c === undefined) return undefined;
  switch (granularity) {
    case "week":
      return financePeriodOf("week", written(civilOf(dayNumber(c) + 7 * count)));
    case "month": {
      const months = c.y * 12 + (c.m - 1) + count;
      return financePeriodOf(
        "month",
        written({ y: Math.floor(months / 12), m: (((months % 12) + 12) % 12) + 1, d: 1 }),
      );
    }
    case "year":
      return financePeriodOf("year", written({ y: c.y + count, m: 1, d: 1 }));
  }
}

/** What the totals are asked for: the periods from the one that holds `from` to the one that holds `to`. */
export type FinanceQuery = {
  readonly granularity: CostGranularity;
  readonly from: CalendarDate;
  readonly to: CalendarDate;
  /** `amortized` when not given, as the server's default. */
  readonly basis?: CostBasis | undefined;
};

/**
 * The query of the last `count` periods of `granularity` up to the one that holds `today` (the reader's, from the clock
 * in their time zone), as many as one request takes at most; undefined for a date that does not exist.
 */
export function financeRange(
  granularity: CostGranularity,
  today: CalendarDate,
  count: number,
  basis?: CostBasis,
): FinanceQuery | undefined {
  const periods = Math.min(Math.max(Math.trunc(count), 1), financePeriodLimits[granularity]);
  const first = financePeriodAfter(granularity, today, 1 - periods);
  if (first === undefined) return undefined;
  return { granularity, from: first.startsOn, to: today, ...(basis === undefined ? {} : { basis }) };
}

// ---- What the instance publishes --------------------------------------------------------------------------------------

/**
 * What the instance publishes of its finance (ADR-0067 §8, §17): of the costs, nothing (`null`), the totals, or the
 * breakdown too; of the income, nothing, the totals, or the members of each plan too; the balance only where both
 * are published; and whether there is anything to show (the about page's section and its page).
 */
export type FinancePublished = {
  readonly costs: Exclude<FinanceVisibility, "private"> | null;
  readonly income: Exclude<IncomeVisibility, "private"> | null;
  readonly balance: boolean;
  readonly any: boolean;
};

/** What `instance` publishes of its finance. */
export function financePublished(
  instance: Pick<Instance, "finance_visibility" | "income_visibility">,
): FinancePublished {
  const costs = instance.finance_visibility === "private" ? null : instance.finance_visibility;
  const income = instance.income_visibility === "private" ? null : instance.income_visibility;
  return { costs, income, balance: costs !== null && income !== null, any: costs !== null || income !== null };
}

/** What the instance publishes of its finance; undefined until the instance was read. */
export function useFinancePublished(): FinancePublished | undefined {
  const instance = useQuery(instanceQuery(useAppData())).data;
  return instance === undefined ? undefined : financePublished(instance);
}

/** The funding meter of this month as the instance shows it; undefined while it shows none or until it was read. */
export function useFundingMeter(): FundingMeter | undefined {
  return useQuery(instanceQuery(useAppData())).data?.funding_meter;
}

// ---- Refusals ---------------------------------------------------------------------------------------------------------

/**
 * Why the server refused a read of the totals: `private`, what the instance does not publish (a 404); `forbidden`, a
 * reader without `view_finance` (a 403 `missing-permission`); `invalid`, more periods than one request takes or `to`
 * before `from` (a 422 naming `to`).
 */
export type FinanceRefusal = "private" | "forbidden" | "invalid";

/** What the server's refusal of a read of the totals was; null for another failure (the network, a 5xx). */
export function financeRefusal(error: unknown): FinanceRefusal | null {
  if (!(error instanceof ProblemError)) return null;
  if (error.status === 404) return "private";
  if (error.status === 403) return "forbidden";
  return error.status === 422 ? "invalid" : null;
}

// ---- Queries ----------------------------------------------------------------------------------------------------------

const queryOf = ({ granularity, from, to, basis }: FinanceQuery) => ({
  granularity,
  from,
  to,
  ...(basis === undefined ? {} : { basis }),
});

/** The public totals of the costs (a 404 where the instance publishes none). */
export function publicCostTotalsQuery({ queries }: AppData, q: FinanceQuery) {
  return queries.queryOptions("get", "/instance/cost_totals", { params: { query: queryOf(q) } });
}

/** The public totals of the income (a 404 where the instance publishes none). */
export function publicIncomeTotalsQuery({ queries }: AppData, q: FinanceQuery) {
  return queries.queryOptions("get", "/instance/income_totals", { params: { query: queryOf(q) } });
}

/** The totals of the costs for a holder of `view_finance`. */
export function financeCostTotalsQuery({ queries }: AppData, q: FinanceQuery) {
  return queries.queryOptions("get", "/finance/cost_totals", { params: { query: queryOf(q) } });
}

/** The totals of the income for a holder of `view_finance`. */
export function financeIncomeTotalsQuery({ queries }: AppData, q: FinanceQuery) {
  return queries.queryOptions("get", "/finance/income_totals", { params: { query: queryOf(q) } });
}

/** What a screen reads of a set of totals. */
export type TotalsRead<T> = {
  /** The totals; undefined until the server answered, and while not enabled. */
  readonly totals: T | undefined;
  readonly status: "pending" | "error" | "success";
  /** The failure of the last read (a ProblemError or NetworkError of mykit), or null. */
  readonly error: Error | null;
  /** Reads them again (the reader's Try again). */
  readonly refetch: () => void;
};

function totalsRead<T>(
  query: { data: T | undefined; status: "pending" | "error" | "success"; error: unknown; refetch: () => unknown },
  enabled: boolean,
): TotalsRead<T> {
  const refetch = () => {
    void query.refetch();
  };
  if (!enabled) return { totals: undefined, status: "pending", error: null, refetch };
  return { totals: query.data, status: query.status, error: query.error as Error | null, refetch };
}

/** What a screen reads of a set of public totals: also whether the server says they are no longer published. */
export type PublicTotalsRead<T> = TotalsRead<T> & {
  /**
   * The server answered that the instance publishes them no more (a 404): nothing is given then, neither what an
   * earlier read kept nor a failure to try again, and the instance is read again, so that what it publishes follows.
   */
  readonly unpublished: boolean;
};

type QueryRead<T> = Parameters<typeof totalsRead<T>>[0];

/** What `query` gives of public totals, read while `enabled`; the instance read again once they are no more. */
function usePublished<T>(query: QueryRead<T>, enabled: boolean): PublicTotalsRead<T> {
  const data = useAppData();
  const unpublished = enabled && financeRefusal(query.error) === "private";
  const { queryClient } = data;
  const instanceKey = instanceQuery(data).queryKey;
  useEffect(() => {
    if (unpublished) void queryClient.invalidateQueries({ queryKey: instanceKey });
  }, [unpublished, queryClient, instanceKey]);
  return { ...totalsRead(query, enabled && !unpublished), unpublished };
}

/**
 * The public totals of the costs of `q`; nothing is read, and nothing given, while `enabled` is false (where the
 * instance publishes no costs, `financePublished`), nor once the server says it publishes them no more.
 */
export function usePublicCostTotals(q: FinanceQuery, enabled = true): PublicTotalsRead<CostTotals> {
  return usePublished(useQuery({ ...publicCostTotalsQuery(useAppData(), q), enabled }), enabled);
}

/**
 * The public totals of the income of `q`; nothing is read, and nothing given, while `enabled` is false, nor once the
 * server says it publishes them no more.
 */
export function usePublicIncomeTotals(q: FinanceQuery, enabled = true): PublicTotalsRead<IncomeTotals> {
  return usePublished(useQuery({ ...publicIncomeTotalsQuery(useAppData(), q), enabled }), enabled);
}

/**
 * The totals of the costs of `q` for a holder of `view_finance` (`useRights().finance`); nothing is read, and nothing
 * given, while `enabled` is false.
 */
export function useFinanceCostTotals(q: FinanceQuery, enabled = true): TotalsRead<FinanceCostTotals> {
  return totalsRead(useQuery({ ...financeCostTotalsQuery(useAppData(), q), enabled }), enabled);
}

/** The totals of the income of `q` for a holder of `view_finance`; nothing is read, and nothing given, while not enabled. */
export function useFinanceIncomeTotals(q: FinanceQuery, enabled = true): TotalsRead<IncomeTotals> {
  return totalsRead(useQuery({ ...financeIncomeTotalsQuery(useAppData(), q), enabled }), enabled);
}
