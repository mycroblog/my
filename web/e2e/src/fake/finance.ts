// SPDX-License-Identifier: AGPL-3.0-only

// The fake's finance (ADR-0067, ADR-0070 §5): the totals of the instance's costs and income by week, month or year, as
// it publishes them (`finance_visibility`, `income_visibility`) and as a holder of `view_finance` reads them, computed
// as the server computes them from the fake's own cost items and income: a period is a range of calendar dates with no
// time zone (ISO weeks from Monday), a cost paid less often than monthly spreads by day over what it covers under
// `amortized` with the cumulative rounding, an imported usage charge spreads over its billing month under both bases,
// and the current month's charge is an estimate where the options say so. Amounts are never converted.

import { day } from "./common";
import type { Answer, Handlers, Refusal, Schema } from "./spec";

type Money = Schema<"Money">;
type Granularity = Schema<"CostGranularity">;
type Basis = Schema<"CostBasis">;
type Category = Schema<"CostCategory">;

/** What a fake server publishes of its finance, and what a holder of `view_finance` reads. */
export type FakeFinanceOptions = {
  /** What the instance publishes of its costs: nothing by default (`private`), the totals, or the breakdown too. */
  costs?: Schema<"FinanceVisibility">;
  /** What it publishes of its income: nothing by default, the totals, or the members of each plan too. */
  income?: Schema<"IncomeVisibility">;
  /** The usage charge imported for this month is not closed by its provider: the month's costs are an estimate. */
  estimated?: boolean;
  /**
   * The reader holds `view_finance` (the admin holds it whatever this says), and what the settings show them: the
   * cost items' names, their notes, the income by plan (`plans`) or its totals alone (by default).
   */
  viewer?: { itemNames?: boolean; itemMemos?: boolean; income?: Schema<"FinanceViewerIncome"> };
  /**
   * The funding meter of the month as the settings ask for it: the share alone (`percent`, by default), with the income
   * and the target (`amounts`), or none (`none`). As the server does (ADR-0067 §22), the share alone shows only while
   * the costs and the income are both private, the amounts only while both are public, and nothing while one side is.
   */
  meter?: "percent" | "amounts" | "none";
  /**
   * The month's target: an amount the operators set (¥20,000 by default), or the month's costs (`costs`), which make a
   * target only while they are in one currency.
   */
  meterTarget?: Money | "costs";
  /** The currencies the costs are paid in, both of JPY and USD by default: one alone leaves out the other's costs. */
  currencies?: readonly string[];
};

/** What the finance handlers take from the fake. */
export type FinanceContext = {
  readonly options: FakeFinanceOptions | undefined;
  /** When the seed ran, in ms since the epoch: its UTC date is today. */
  readonly started: number;
  /** The reader's permissions. */
  readonly permissions: () => readonly Schema<"Permission">[];
  /** The members of each plan now, and whether the server takes tips. */
  readonly planCounts: () => Schema<"IncomePlanCount">[];
  readonly tips: boolean;
  readonly problem: (status: number, title: string, more?: Partial<Schema<"Problem">>) => Refusal;
  readonly notFound: () => Refusal;
};

// ---- Dates as day numbers (days from 1970-01-01, UTC) -----------------------------------------------------------------

const dayOf = (date: string) => Math.round(Date.parse(`${date}T00:00:00Z`) / day);
const dateOf = (n: number) => new Date(n * day).toISOString().slice(0, 10);
const ymd = (n: number) => {
  const [y = 0, m = 1, d = 1] = dateOf(n).split("-").map(Number);
  return { y, m, d };
};
const civil = (y: number, m: number, d: number) => Math.round(Date.UTC(y, m - 1, d) / day);
const lastDay = (y: number, m: number) => civil(y, m + 1, 1) - civil(y, m, 1);
const validDate = (date: string | null): date is string =>
  date !== null && /^\d{4}-\d{2}-\d{2}$/.test(date) && dateOf(dayOf(date)) === date;

/** The period of `g` that holds the day `n`: its first and last days and its label. */
function periodOf(g: Granularity, n: number): { start: number; end: number; label: string } {
  const { y, m } = ymd(n);
  if (g === "year") return { start: civil(y, 1, 1), end: civil(y, 12, 31), label: String(y) };
  if (g === "month")
    return { start: civil(y, m, 1), end: civil(y, m + 1, 1) - 1, label: `${String(y)}-${String(m).padStart(2, "0")}` };
  const monday = n - ((((n + 3) % 7) + 7) % 7);
  const isoYear = ymd(monday + 3).y;
  const jan4 = civil(isoYear, 1, 4);
  const week = (monday - (jan4 - ((((jan4 + 3) % 7) + 7) % 7))) / 7 + 1;
  return { start: monday, end: monday + 6, label: `${String(isoYear)}-W${String(week).padStart(2, "0")}` };
}

// ---- The costs -------------------------------------------------------------------------------------------------------

/** The digits of a currency's minor unit: the fake's two. */
const digits: Readonly<Record<string, number>> = { JPY: 0, USD: 2 };
const money = (minor: number, currency: string): Money => {
  const scale = digits[currency] ?? 2;
  const sign = minor < 0 ? "-" : "";
  const abs = Math.abs(minor);
  return {
    amount:
      scale === 0
        ? `${sign}${String(abs)}`
        : `${sign}${String(Math.floor(abs / 10 ** scale))}.${String(abs % 10 ** scale).padStart(scale, "0")}`,
    currency,
  };
};

/** A cost item: how often it is paid (every N months or years), from when, how much. */
type Item = {
  name: string;
  provider: string;
  category: Category;
  memo?: string;
  every: { unit: "months" | "years"; n: number };
  startsOn: string;
  minor: number;
  currency: string;
};

const items: readonly Item[] = [
  {
    name: "VPS",
    provider: "Sakura Cloud",
    category: "compute",
    memo: "2 vCPU, 4 GB, Tokyo zone",
    every: { unit: "months", n: 1 },
    startsOn: "2025-01-15",
    minor: 3300,
    currency: "JPY",
  },
  {
    name: "Media bucket",
    provider: "Cloudflare",
    category: "storage",
    memo: "R2 bucket media-prod",
    every: { unit: "months", n: 1 },
    startsOn: "2025-03-01",
    minor: 500,
    currency: "USD",
  },
  {
    name: "Mail delivery",
    provider: "Postmark",
    category: "email",
    every: { unit: "months", n: 1 },
    startsOn: "2025-02-01",
    minor: 1500,
    currency: "USD",
  },
  {
    name: "Backups",
    provider: "Backblaze",
    category: "storage",
    memo: "Paid each quarter",
    every: { unit: "months", n: 3 },
    startsOn: "2025-01-05",
    minor: 900,
    currency: "JPY",
  },
  {
    name: "Domain demo.test",
    provider: "Gandi",
    category: "domain",
    memo: "Renews each April",
    every: { unit: "years", n: 1 },
    startsOn: "2025-04-10",
    minor: 1650,
    currency: "JPY",
  },
];

/** The day of the k-th payment of `item`, counted from its start (a day past the month's end is its last day). */
function paymentDay(item: Item, k: number): number {
  const { y, m, d } = ymd(dayOf(item.startsOn));
  const months = (item.every.unit === "years" ? 12 : 1) * item.every.n * k;
  const year = y + Math.floor((m - 1 + months) / 12);
  const month = ((m - 1 + months) % 12) + 1;
  return civil(year, month, Math.min(d, lastDay(year, month)));
}

/** Whether `item` spreads over what it covers under `amortized`: paid less often than monthly. */
const longer = (item: Item) => item.every.unit === "years" || item.every.n >= 2;

/** The part of `minor`, spread by day over `[from, from + days)`, that falls in `[start, end]`, cumulatively rounded. */
function spread(minor: number, from: number, days: number, start: number, end: number): number {
  const a = Math.min(Math.max(start - from, 0), days);
  const b = Math.min(Math.max(end - from + 1, 0), days);
  return Math.floor((minor * b) / days) - Math.floor((minor * a) / days);
}

/** What `item` costs in `[start, end]` under `basis`. */
function itemIn(item: Item, basis: Basis, start: number, end: number): number | undefined {
  let total = 0;
  let met = false;
  for (let k = 0; ; k += 1) {
    const paid = paymentDay(item, k);
    if (paid > end) break;
    const next = paymentDay(item, k + 1);
    if (basis === "amortized" && longer(item)) {
      if (next - 1 < start) continue;
      total += spread(item.minor, paid, next - paid, start, end);
      met = true;
    } else if (paid >= start) {
      total += item.minor;
      met = true;
    }
  }
  return met ? total : undefined;
}

/** The imported usage charge of AWS for the month that starts on the day `month`, in cents. */
const awsMonth = (month: number) => 1240 + (ymd(month).m % 4) * 135;
const awsFrom = dayOf("2025-06-01");

/** What the imported charges cost in `[start, end]`, and whether the open month's estimate meets it. */
function importedIn(start: number, end: number, today: number, estimated: boolean) {
  let total = 0;
  let met = false;
  let estimate = false;
  for (let month = periodOf("month", Math.max(start, awsFrom)).start; month <= end;) {
    const last = periodOf("month", month).end;
    if (last >= start && month >= awsFrom) {
      total += spread(awsMonth(month), month, last - month + 1, start, end);
      met = true;
      if (estimated && today >= month && today <= last) estimate = true;
    }
    month = last + 1;
  }
  return met ? { total, estimate } : undefined;
}

type Line = { provider: string; category: Category; minor: number; currency: string; name: string; memo?: string };

/** The cost lines of `[start, end]`, each item's and the imported charge's. */
function linesIn(
  basis: Basis,
  start: number,
  end: number,
  today: number,
  estimated: boolean,
  currencies: readonly string[],
) {
  const lines: Line[] = items.flatMap((item) => {
    if (!currencies.includes(item.currency)) return [];
    const minor = itemIn(item, basis, start, end);
    return minor === undefined
      ? []
      : [
          {
            provider: item.provider,
            category: item.category,
            minor,
            currency: item.currency,
            name: item.name,
            ...(item.memo === undefined ? {} : { memo: item.memo }),
          },
        ];
  });
  const imported = currencies.includes("USD") ? importedIn(start, end, today, estimated) : undefined;
  if (imported !== undefined)
    lines.push({ provider: "AWS", category: "compute", minor: imported.total, currency: "USD", name: "Amazon EC2" });
  return { lines, estimate: imported?.estimate === true };
}

/** The sum of `lines` by currency, sorted by currency code. */
const sumByCurrency = (lines: readonly { minor: number; currency: string }[]) =>
  [...new Set(lines.map((l) => l.currency))].toSorted().map((currency) =>
    money(
      lines.filter((l) => l.currency === currency).reduce((s, l) => s + l.minor, 0),
      currency,
    ),
  );

/** The breakdown of `lines` by provider, category and currency. */
function breakdownOf(lines: readonly Line[]): Schema<"CostBreakdownLine">[] {
  const keys = [...new Set(lines.map((l) => `${l.provider}\u0000${l.category}\u0000${l.currency}`))].toSorted();
  return keys.map((key) => {
    const [provider = "", category = "other", currency = ""] = key.split("\u0000");
    const minor = lines
      .filter((l) => l.provider === provider && l.category === category && l.currency === currency)
      .reduce((s, l) => s + l.minor, 0);
    return { provider, category: category as Category, amount: money(minor, currency) };
  });
}

// ---- The income ------------------------------------------------------------------------------------------------------

/** A movement of the income, net of the provider's fee: its day, and the days it covers when longer than a month. */
type Movement = { on: number; minor: number; covers?: number };

/** The income's movements up to `until`: the monthly subscriptions, the yearly ones, the tips, and a refund. */
function movements(until: number, tips: boolean): Movement[] {
  const out: Movement[] = [];
  for (let month = dayOf("2025-02-01"); month <= until; month = periodOf("month", month).end + 1) {
    const { y, m } = ymd(month);
    out.push({ on: civil(y, m, 3), minor: 4520 });
    if (tips && month >= dayOf("2026-01-01")) out.push({ on: civil(y, m, 18), minor: 950 });
    if (m === 6) out.push({ on: civil(y, m, 20), minor: 14_350, covers: 365 });
  }
  out.push({ on: dayOf("2026-08-12"), minor: -500 });
  return out.filter((mv) => mv.on <= until);
}

/** The payments that cover more than this many days spread by day under `amortized`. */
const amortizeAfterDays = 31;

function incomeIn(moves: readonly Movement[], basis: Basis, start: number, end: number): number | undefined {
  let total = 0;
  let met = false;
  for (const mv of moves) {
    if (basis === "amortized" && mv.covers !== undefined && mv.covers > amortizeAfterDays) {
      if (mv.on > end || mv.on + mv.covers - 1 < start) continue;
      total += spread(mv.minor, mv.on, mv.covers, start, end);
      met = true;
    } else if (mv.on >= start && mv.on <= end) {
      total += mv.minor;
      met = true;
    }
  }
  return met ? total : undefined;
}

// ---- The handlers -----------------------------------------------------------------------------------------------------

type TotalsQuery = { get: (name: "granularity" | "from" | "to" | "basis") => string | null };

/** The handlers of the finance, and the instance's fields of it. */
export function financeHandlers(c: FinanceContext): {
  handlers: Handlers;
  instance: Pick<Schema<"Instance">, "finance_visibility" | "income_visibility">;
  meter: (month: string) => Schema<"FundingMeter"> | undefined;
} {
  const o: FakeFinanceOptions = c.options ?? {};
  const costsShown = o.costs ?? "private";
  const incomeShown = o.income ?? "private";
  const estimated = o.estimated === true;
  const currencies = o.currencies ?? ["JPY", "USD"];
  const today = Math.floor(c.started / day);
  const invalid = (field: string) =>
    c.problem(422, "This could not be done.", { detail: "This value is not valid. Check what you entered.", field });

  /** The periods a query asks for, or the 422 of what it names wrongly; public ones stop at today's. */
  function periods(query: TotalsQuery, upToToday: boolean) {
    const asked = query.get("granularity");
    if (asked !== "week" && asked !== "month" && asked !== "year") return invalid("granularity");
    const granularity: Granularity = asked;
    const named = query.get("basis") ?? "amortized";
    if (named !== "amortized" && named !== "cash") return invalid("basis");
    const basis: Basis = named;
    const from = query.get("from");
    const to = query.get("to");
    if (!validDate(from)) return invalid("from");
    if (!validDate(to)) return invalid("to");
    const first = periodOf(granularity, dayOf(from));
    const last = periodOf(granularity, upToToday ? Math.min(dayOf(to), today) : dayOf(to));
    if (dayOf(to) < dayOf(from)) return invalid("to");
    const out: { start: number; end: number; label: string }[] = [];
    if (!upToToday || dayOf(from) <= today)
      for (let p = first; p.start <= last.start; p = periodOf(granularity, p.end + 1)) out.push(p);
    const limit = { week: 104, month: 60, year: 20 }[granularity];
    if (out.length > limit) return invalid("to");
    return { granularity, basis, periods: out };
  }

  const sources: Schema<"CostSourceRule">[] = [{ source: "aws", provider: "AWS", time_zone: "UTC" }];
  const incomeRules: Schema<"IncomeTotalsRules"> = {
    week_start: "monday",
    providers: [{ provider: "Stripe", time_zone: "UTC" }],
    amortize_after_days: amortizeAfterDays,
  };
  const may = (...names: Schema<"Permission">[]) => names.some((p) => c.permissions().includes(p));
  const missingPermission = () =>
    c.problem(403, "You do not have permission", { type: "/api/v1/problems/missing-permission" });

  /** The cost periods of `q`, with their lines. */
  const costPeriods = (q: Exclude<ReturnType<typeof periods>, Refusal>) =>
    q.periods.map((p) => {
      const { lines, estimate } = linesIn(q.basis, p.start, p.end, today, estimated, currencies);
      return { p, lines, estimate };
    });

  /** The income periods of `q`, with the balance against the costs where `balance`. */
  function incomeOf(q: Exclude<ReturnType<typeof periods>, Refusal>, balance: boolean): Schema<"IncomePeriodTotal">[] {
    // No movement after today: a period ahead of it has none yet.
    const moves = movements(Math.min(q.periods.at(-1)?.end ?? today, today), c.tips);
    return q.periods.map((p) => {
      const minor = incomeIn(moves, q.basis, p.start, p.end);
      const net = minor === undefined ? [] : [money(minor, "JPY")];
      if (!balance) return { label: p.label, starts_on: dateOf(p.start), ends_on: dateOf(p.end), net };
      const { lines } = linesIn(q.basis, p.start, p.end, today, estimated, currencies);
      const sides = [
        ...(minor === undefined ? [] : [{ minor, currency: "JPY" }]),
        ...lines.map((l) => ({ minor: -l.minor, currency: l.currency })),
      ];
      return {
        label: p.label,
        starts_on: dateOf(p.start),
        ends_on: dateOf(p.end),
        net,
        balance: sumByCurrency(sides),
      };
    });
  }

  const handlers: Handlers = {
    "GET /instance/cost_totals": ({ query }): Answer<"GET /instance/cost_totals"> => {
      if (costsShown === "private") return c.notFound();
      const q = periods(query, true);
      if ("problem" in q) return q;
      const breakdown = costsShown === "breakdown";
      return {
        status: 200,
        body: {
          granularity: q.granularity,
          basis: q.basis,
          periods: costPeriods(q).map(({ p, lines, estimate }) => ({
            label: p.label,
            starts_on: dateOf(p.start),
            ends_on: dateOf(p.end),
            totals: sumByCurrency(lines),
            breakdown: breakdown ? breakdownOf(lines) : [],
            estimated: estimate,
          })),
          rules: { week_start: "monday", sources: breakdown ? sources : [] },
        },
      };
    },

    "GET /instance/income_totals": ({ query }): Answer<"GET /instance/income_totals"> => {
      if (incomeShown === "private") return c.notFound();
      const q = periods(query, true);
      if ("problem" in q) return q;
      return {
        status: 200,
        body: {
          granularity: q.granularity,
          basis: q.basis,
          periods: incomeOf(q, costsShown !== "private"),
          ...(incomeShown === "plans" ? { plans: c.planCounts(), tips: c.tips } : {}),
          rules: incomeRules,
        },
      };
    },

    "GET /finance/cost_totals": ({ query }): Answer<"GET /finance/cost_totals"> => {
      if (!may("view_finance", "manage_finance")) return missingPermission();
      const q = periods(query, false);
      if ("problem" in q) return q;
      const manager = may("manage_finance");
      const names = manager || o.viewer?.itemNames === true;
      const memos = manager || o.viewer?.itemMemos === true;
      return {
        status: 200,
        body: {
          granularity: q.granularity,
          basis: q.basis,
          periods: costPeriods(q).map(({ p, lines, estimate }) => ({
            label: p.label,
            starts_on: dateOf(p.start),
            ends_on: dateOf(p.end),
            totals: sumByCurrency(lines),
            breakdown: breakdownOf(lines),
            estimated: estimate,
            items:
              names || memos
                ? lines.map((l) => ({
                    provider: l.provider,
                    category: l.category,
                    amount: money(l.minor, l.currency),
                    ...(names ? { name: l.name } : {}),
                    ...(memos && l.memo !== undefined ? { memo: l.memo } : {}),
                  }))
                : [],
          })),
          rules: { week_start: "monday", sources },
          viewer: {
            item_names: o.viewer?.itemNames === true,
            item_memos: o.viewer?.itemMemos === true,
            income: o.viewer?.income ?? "totals",
          },
        },
      };
    },

    "GET /finance/income_totals": ({ query }): Answer<"GET /finance/income_totals"> => {
      if (!may("view_finance", "manage_finance")) return missingPermission();
      const q = periods(query, false);
      if ("problem" in q) return q;
      const plans = may("manage_finance") || o.viewer?.income === "plans" || incomeShown === "plans";
      return {
        status: 200,
        body: {
          granularity: q.granularity,
          basis: q.basis,
          periods: incomeOf(q, true),
          ...(plans ? { plans: c.planCounts(), tips: c.tips } : {}),
          rules: incomeRules,
        },
      };
    },
  };

  /**
   * The funding meter of the month `month` (`2026-11`), as ADR-0067 §22 shows it: this month's net income up to today
   * over the target, the operators' amount or the month's costs while they are in one currency; an estimate only where
   * the target is those costs and an imported charge among them is one; and only as the visibility of both sides allows.
   */
  const meter = (month: string): Schema<"FundingMeter"> | undefined => {
    const asked = o.meter ?? "percent";
    const bothPrivate = costsShown === "private" && incomeShown === "private";
    const bothPublic = costsShown !== "private" && incomeShown !== "private";
    if (asked === "none" || (asked === "percent" && !bothPrivate) || (asked === "amounts" && !bothPublic))
      return undefined;
    const start = dayOf(`${month}-01`);
    const end = periodOf("month", start).end;
    const spentThisMonth = linesIn("amortized", start, end, today, estimated, currencies);
    const setTarget = o.meterTarget ?? { amount: "20000", currency: "JPY" };
    let target: { minor: number; currency: string };
    if (setTarget === "costs") {
      const spent = sumByCurrency(spentThisMonth.lines);
      const [only] = spent;
      if (spent.length !== 1 || only === undefined) return undefined;
      target = { minor: Number(only.amount.replace(".", "")), currency: only.currency };
      if (target.minor <= 0) return undefined;
    } else {
      const scale = digits[setTarget.currency] ?? 2;
      target = { minor: Math.round(Number(setTarget.amount) * 10 ** scale), currency: setTarget.currency };
    }
    // The income in the target's currency alone, the fake's being in yen; none after today.
    const income = target.currency === "JPY" ? (incomeIn(movements(today, c.tips), "amortized", start, end) ?? 0) : 0;
    const percent = income <= 0 ? 0 : Math.floor((income * 100) / target.minor);
    return {
      visibility: asked,
      label: month,
      starts_on: dateOf(start),
      ends_on: dateOf(end),
      percent,
      covered: income >= target.minor,
      estimated: setTarget === "costs" && spentThisMonth.estimate,
      rail: true,
      ...(asked === "amounts"
        ? {
            income: money(income, target.currency),
            target: money(target.minor, target.currency),
            target_is_costs: setTarget === "costs",
          }
        : {}),
    };
  };

  return {
    handlers,
    instance: { finance_visibility: costsShown, income_visibility: incomeShown },
    meter,
  };
}
