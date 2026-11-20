// SPDX-License-Identifier: AGPL-3.0-only

// What the finance screens share, /finance and /about/costs of the finance feature and the administrators'
// /admin/finance of the staff's (ADR-0067 §4, §5, §10, §17, §20; FR-902, FR-917): the reader's window over the
// periods (its length, how a payment longer than a month counts, and how far back it is), the choices and the way to
// earlier and later periods, the table of the totals (costs, income and the balance, each a figure a currency on its
// own line, never converted, an estimate marked on its currency's figure, the rules that cut the periods as its note),
// a period's costs by provider, and the members of each plan. Amounts and periods are written here with `Intl` in the
// reader's locale; a calendar date is no moment, so it is read where it is written, in UTC.

import {
  costBases,
  costGranularities,
  digitsOf,
  financePeriodAfter,
  type CostBasis,
  type CostBreakdownLine,
  type CostGranularity,
  type CostPeriodTotal,
  type CostTotalsRules,
  type FinancePeriod,
  type FinanceQuery,
  type IncomePeriodTotal,
  type IncomePlanCount,
  type IncomeTotalsRules,
  type Money,
} from "@my/app-data";
import {
  Box,
  dateFormat,
  FeedNotice,
  FinanceRow,
  FinanceTable,
  GroupHeading,
  Pager,
  SegmentChoice,
  Stack,
  TextButton,
  useLocale,
  useTimeZone,
  wallTime,
  type FinanceCell,
  type FinanceColumn,
  type FinanceFigure,
} from "@my/design-system";
import { useEffect, useState, type ReactNode } from "react";
import { stringsIn } from "./locale";

export type T = ReturnType<typeof stringsIn>["t"];

/** The strings of the finance screens, in the reader's locale, and the locale. */
export function useFinanceStrings(): { t: T; locale: string } {
  const { locale } = useLocale();
  return { t: stringsIn(locale).t, locale };
}

/** How many periods the window shows at once, by their length. */
const financeWindow: Readonly<Record<CostGranularity, number>> = { week: 8, month: 6, year: 5 };

const two = (n: number) => String(n).padStart(2, "0");

/** Today in `timeZone`, as a date of the calendar: what cuts "this month" (ADR-0067 §5). */
export function todayIn(timeZone: string | undefined, now: Date = new Date()): string {
  const wall = wallTime(now, timeZone);
  return `${String(wall.year).padStart(4, "0")}-${two(wall.month)}-${two(wall.day)}`;
}

/** How often the screen looks whether the date changed in the reader's zone. */
const dayCheck = 60_000;

/** Today in the reader's time zone, which follows the zone and turns at the date's change while the screen is open. */
function useToday(): string {
  const timeZone = useTimeZone();
  const [today, setToday] = useState(() => ({ timeZone, date: todayIn(timeZone) }));
  useEffect(() => {
    const look = () => {
      const date = todayIn(timeZone);
      setToday((was) => (was.timeZone === timeZone && was.date === date ? was : { timeZone, date }));
    };
    look();
    const timer = setInterval(look, dayCheck);
    return () => {
      clearInterval(timer);
    };
  }, [timeZone]);
  return today.date;
}

/** The reader's window over the periods, and what changes it. */
export type FinanceView = {
  readonly granularity: CostGranularity;
  readonly basis: CostBasis;
  /** How many windows back from the one that ends with today's period. */
  readonly offset: number;
  /** The periods of the window, the latest first. */
  readonly periods: readonly FinancePeriod[];
  /** The label of the period that holds today. */
  readonly current: string;
  /** What the totals are asked for. */
  readonly query: FinanceQuery;
  /** How many times the reader changed the window, which the live region says (0 before the first). */
  readonly changes: number;
  readonly choose: (granularity: CostGranularity) => void;
  readonly count: (basis: CostBasis) => void;
  readonly earlier: () => void;
  readonly later: () => void;
};

/** The reader's window over the periods: by month, spread over their cycle, ending with today's, at first. */
export function useFinanceView(): FinanceView {
  const today = useToday();
  const [granularity, setGranularity] = useState<CostGranularity>("month");
  const [basis, setBasis] = useState<CostBasis>("amortized");
  const [offset, setOffset] = useState(0);
  const [changes, setChanges] = useState(0);
  const size = financeWindow[granularity];
  const periods: FinancePeriod[] = [];
  for (let i = 0; i < size; i += 1) {
    const period = financePeriodAfter(granularity, today, -(offset * size + i));
    if (period !== undefined) periods.push(period);
  }
  const latest = periods[0];
  const oldest = periods.at(-1);
  const changed = () => {
    setChanges((n) => n + 1);
  };
  return {
    granularity,
    basis,
    offset,
    periods,
    current: financePeriodAfter(granularity, today, 0)?.label ?? "",
    query: {
      granularity,
      from: oldest?.startsOn ?? today,
      to: offset === 0 || latest === undefined ? today : latest.endsOn,
      basis,
    },
    changes,
    choose: (g) => {
      setGranularity(g);
      setOffset(0);
      changed();
    },
    count: (b) => {
      setBasis(b);
      changed();
    },
    earlier: () => {
      setOffset((n) => n + 1);
      changed();
    },
    later: () => {
      setOffset((n) => Math.max(0, n - 1));
      changed();
    },
  };
}

// ---- Words ------------------------------------------------------------------------------------------------------------

/** A date of the calendar as an instant at its UTC midnight, to format in UTC. */
const instantOf = (date: string) => new Date(`${date}T00:00:00Z`);

/** `money` as `format` writes it, with the typeset minus sign (U+2212) where Intl writes a hyphen-minus (en, ja). */
function typeset(format: Intl.NumberFormat, money: Money): string {
  return format
    .formatToParts(money.amount as `${number}`)
    .map((part) => (part.type === "minusSign" ? "\u2212" : part.value))
    .join("");
}

/** An amount in its currency, as the reader's language writes it. */
export function moneyText(locale: string, money: Money): string {
  // The digits the server keeps for the currency (ISO 4217), which Intl's own (CLDR) do not always match.
  const digits = digitsOf(money.currency);
  const format = new Intl.NumberFormat(locale, {
    style: "currency",
    currency: money.currency,
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
  return typeset(format, money);
}

/** The period of a period of the totals as the API gives it. */
export const periodOfAnswer = (p: { label: string; starts_on: string; ends_on: string }): FinancePeriod => ({
  label: p.label,
  startsOn: p.starts_on,
  endsOn: p.ends_on,
});

/** The name of `period` in the reader's language: "November 2026", "2026", "Week 47, 2026". */
export function periodName(t: T, locale: string, granularity: CostGranularity, period: FinancePeriod): string {
  if (granularity === "week") {
    const [year = "", week = ""] = period.label.split("-W");
    return t("finance.week_label", { week: Number(week), year: Number(year) });
  }
  const options: Intl.DateTimeFormatOptions =
    granularity === "month" ? { year: "numeric", month: "long" } : { year: "numeric" };
  return dateFormat(locale, "UTC", options).format(instantOf(period.startsOn));
}

/** The line under a period's name: a week's dates, and "So far" for the period of today. */
function periodCaption(
  t: T,
  locale: string,
  granularity: CostGranularity,
  period: FinancePeriod,
  current: boolean,
): string | undefined {
  if (granularity !== "week") return current ? t("finance.so_far") : undefined;
  const dates = dateFormat(locale, "UTC", { month: "short", day: "numeric" }).formatRange(
    instantOf(period.startsOn),
    instantOf(period.endsOn),
  );
  return current ? t("finance.week_dates_so_far", { dates }) : dates;
}

/**
 * Whether two spellings name one provider, as the server groups them (ADR-0067 §20): in NFKC and lower case, so that a
 * spelling an operator typed (`aws`) that wins over the import's (`AWS`) still names the imported provider.
 */
const grouped = (provider: string) => provider.normalize("NFKC").toLowerCase();

/** Whether `provider` is one of the imported providers of `sources`. */
const importedBy = (sources: CostTotalsRules["sources"] | undefined, provider: string) =>
  (sources ?? []).some((source) => grouped(source.provider) === grouped(provider));

/** Whether an amount of a currency of a period comes from its estimate. */
type Estimated = (currency: string) => boolean;

const notEstimated: Estimated = () => false;

/**
 * Which currencies of `period` come from its estimate: where the period is one, those of the lines of the imported
 * providers (`sources`, whose open billing month the estimate is), or every currency where no breakdown says which.
 */
function estimatedIn(period: CostPeriodTotal | undefined, sources: CostTotalsRules["sources"] | undefined): Estimated {
  if (period?.estimated !== true) return notEstimated;
  const lines = period.breakdown.filter((line) => importedBy(sources, line.provider));
  if (lines.length === 0) return () => true;
  const currencies = new Set(lines.map((line) => line.amount.currency));
  return (currency) => currencies.has(currency);
}

/** An amount with its currency's code (JPY 1,970), where amounts of several currencies stand one under another. */
export function moneyCoded(locale: string, money: Money): string {
  const digits = digitsOf(money.currency);
  const format = new Intl.NumberFormat(locale, {
    style: "currency",
    currency: money.currency,
    currencyDisplay: "code",
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
  return typeset(format, money);
}

/**
 * The figures of a cell: an amount a currency, each marked where it comes from an estimate, or None; each named by its
 * currency's code where `coded`.
 */
function figuresOf(
  t: T,
  locale: string,
  amounts: readonly Money[] | undefined,
  estimated: Estimated = notEstimated,
  coded = false,
): FinanceFigure[] {
  const shown = coded ? moneyCoded : moneyText;
  return amounts === undefined || amounts.length === 0
    ? [t("finance.none")]
    : amounts.map((m) => ({ shown: shown(locale, m), estimated: estimated(m.currency) }));
}

/**
 * A period's income with a zero in each currency it has costs in and no income, sorted by currency, so that a balance
 * below zero never stands beside an empty income.
 */
export function incomeBeside(net: readonly Money[], costs: readonly Money[]): Money[] {
  const zeros = costs
    .filter((c) => !net.some((m) => m.currency === c.currency))
    .map((c) => ({ amount: "0", currency: c.currency }));
  return [...net, ...zeros].toSorted((a, b) => a.currency.localeCompare(b.currency));
}

// ---- The choices and the pages ----------------------------------------------------------------------------------------

/**
 * The choice of the period's length, and of how a payment longer than a month counts, each named over it; the second
 * says what its option means, the one place that says it.
 */
export function FinanceChoices({ view, about = true }: { view: FinanceView; about?: boolean }) {
  const { t } = useFinanceStrings();
  return (
    <Stack gap="screen-row-gap">
      <SegmentChoice
        label={t("finance.period")}
        shown
        options={costGranularities.map((value) => ({ value, label: t(`finance.period.${value}`) }))}
        value={view.granularity}
        onChange={view.choose}
      />
      <SegmentChoice
        label={t("finance.basis")}
        shown
        fit
        options={costBases.map((value) => ({ value, label: t(`finance.basis.${value}`) }))}
        value={view.basis}
        onChange={view.count}
        about={
          about
            ? t(view.basis === "amortized" ? "finance.basis.amortized_about" : "finance.basis.cash_about")
            : undefined
        }
      />
    </Stack>
  );
}

/**
 * The way to the periods before those shown (Earlier, as the pager's Next) and back to the later ones up to today's
 * (Later, disabled once the window ends with today's period); after them, which periods are shown, written in the
 * reader's locale, the pager's live region saying it again whenever the reader changed the window.
 */
export function FinancePages({ view }: { view: FinanceView }) {
  const { t, locale } = useFinanceStrings();
  const latest = view.periods[0];
  const oldest = view.periods.at(-1);
  return (
    <Pager
      periods={view.granularity}
      previous={view.offset > 0 ? { onPress: view.later } : undefined}
      next={{ onPress: view.earlier }}
      position={
        latest !== undefined &&
        oldest !== undefined && (
          // A new node for each change, so that the live region says it again when only how payments count changed.
          <span key={view.changes}>
            {t("finance.showing", {
              from: periodName(t, locale, view.granularity, oldest),
              to: periodName(t, locale, view.granularity, latest),
            })}
          </span>
        )
      }
    />
  );
}

// ---- Reads ------------------------------------------------------------------------------------------------------------

/** What the screen read of one side. */
export type SideRead = { readonly status: "pending" | "error" | "success"; readonly refetch: () => void };

/** That the totals load, or could not be read, with Try again; nothing once every side shown is read. */
export function ReadNotice({ reads }: { reads: readonly SideRead[] }): ReactNode {
  const { t } = useFinanceStrings();
  if (reads.some((r) => r.status === "error"))
    return (
      <FeedNotice
        alert
        start
        action={
          <TextButton
            onPress={() => {
              for (const r of reads) if (r.status === "error") r.refetch();
            }}
          >
            {t("finance.retry")}
          </TextButton>
        }
      >
        {t("finance.failed")}
      </FeedNotice>
    );
  if (reads.some((r) => r.status === "pending")) return <FeedNotice start>{t("finance.loading")}</FeedNotice>;
  return null;
}

// ---- The totals -------------------------------------------------------------------------------------------------------

/** The size of an amount above zero, for a bar alone, and none for one at or below zero: no figure is written from it. */
export const barOf = (money: Money): number | undefined => {
  const n = Number(money.amount);
  return n > 0 ? n : undefined;
};

/** The largest amount above zero of `amounts`, for the bars; 0 for none. */
const largestOf = (amounts: readonly Money[]) => amounts.reduce((most, m) => Math.max(most, barOf(m) ?? 0), 0);

/**
 * The periods of the window that the server answered for, the latest first: a period after the last it has (today's
 * where the server's date is still the day before the reader's) is left out, never shown as having nothing.
 */
export function answeredPeriods(
  view: FinanceView,
  ...answers: (readonly { label: string }[] | undefined)[]
): FinancePeriod[] {
  const labels = answers.flatMap((periods) => (periods ?? []).map((p) => p.label));
  if (labels.length === 0) return [...view.periods];
  const last = labels.reduce((a, b) => (b > a ? b : a));
  return view.periods.filter((p) => p.label <= last);
}

/**
 * The totals of the window, the latest period first: its costs, its income and its balance, as far as each is given
 * (undefined: not shown), a figure a currency on its own line, never converted, an estimate marked on its currency's
 * figure; the rules that cut its periods as its note. A bar draws a figure's share of the largest where costs and
 * income stand side by side in one currency alone, and none for an amount at or below zero.
 */
export function TotalsTable({
  view,
  costs,
  income,
  balance,
  sources,
  incomeRules,
  rules = true,
  coded = false,
}: {
  view: FinanceView;
  costs: readonly CostPeriodTotal[] | undefined;
  income: readonly IncomePeriodTotal[] | undefined;
  balance: boolean;
  /** The imported providers' billing zones, where the costs' breakdown is shown. */
  sources: CostTotalsRules["sources"] | undefined;
  /** The income's rules, where the income is shown. */
  incomeRules: IncomeTotalsRules | undefined;
  /** The rules that cut the periods as the table's note; the administrators' screen, which knows them, leaves them out. */
  rules?: boolean;
  /** Each figure named by its currency's code, where the table holds several currencies a column. */
  coded?: boolean;
}) {
  const { t, locale } = useFinanceStrings();
  const columns: FinanceColumn[] = [
    ...(costs === undefined ? [] : [{ label: t("finance.column.costs"), figures: true }]),
    ...(income === undefined ? [] : [{ label: t("finance.column.income"), figures: true }]),
    ...(balance ? [{ label: t("finance.column.balance"), figures: true }] : []),
  ];
  const compared = [...(costs ?? []).flatMap((p) => p.totals), ...(income ?? []).flatMap((p) => p.net)];
  const currencies = new Set(compared.map((m) => m.currency));
  const largest = largestOf(compared);
  const bars = costs !== undefined && income !== undefined && currencies.size === 1 && largest > 0;
  const share = (amounts: readonly Money[]) => {
    const [only] = amounts;
    const bar = only === undefined ? undefined : barOf(only);
    return bars && amounts.length === 1 && bar !== undefined ? { share: bar / largest } : {};
  };
  const rows = answeredPeriods(view, costs, income);
  const estimated = (costs ?? []).some((p) => p.estimated && rows.some((r) => r.label === p.label));
  return (
    <FinanceTable
      label={t("finance.totals", { granularity: view.granularity })}
      head={t(`finance.period.${view.granularity}`)}
      columns={columns}
      estimated={estimated ? t("finance.estimate") : undefined}
      about={rules ? rulesOf(t, sources, incomeRules) : undefined}
    >
      {rows.map((period) => {
        const cost = costs?.find((p) => p.label === period.label);
        const got = income?.find((p) => p.label === period.label);
        const marked = estimatedIn(cost, sources);
        const net = incomeBeside(got?.net ?? [], cost?.totals ?? []);
        const cells: FinanceCell[] = [
          ...(costs === undefined
            ? []
            : [{ figures: figuresOf(t, locale, cost?.totals, marked, coded), ...share(cost?.totals ?? []) }]),
          ...(income === undefined ? [] : [{ figures: figuresOf(t, locale, net, notEstimated, coded), ...share(net) }]),
          ...(balance ? [{ figures: figuresOf(t, locale, got?.balance, marked, coded) }] : []),
        ];
        return (
          <FinanceRow
            key={period.label}
            head={periodName(t, locale, view.granularity, period)}
            caption={periodCaption(t, locale, view.granularity, period, period.label === view.current)}
            cells={cells}
          />
        );
      })}
    </FinanceTable>
  );
}

/**
 * The rules that cut the periods, short, as the totals' note (FR-902): calendar dates with no time zone, the week from
 * Monday, each currency apart; each imported provider's billing months; and where income is shown, each payment
 * provider's zone, what the income is, and that nobody's payments show. How a payment longer than a month counts is
 * said by its choice, and what an estimate is by the table's legend and the meter.
 */
function rulesOf(t: T, sources: CostTotalsRules["sources"] | undefined, income: IncomeTotalsRules | undefined): string {
  return [
    t("finance.rules.dates"),
    t("finance.rules.currencies"),
    ...(sources ?? []).map((s) => t("finance.rules.billing", { provider: s.provider, zone: s.time_zone })),
    ...(income === undefined
      ? []
      : [
          ...income.providers.map((p) => t("finance.rules.income_zone", { provider: p.provider, zone: p.time_zone })),
          t("finance.rules.net"),
          t("finance.rules.payers"),
        ]),
  ].join(" ");
}

/** A line under a table. */
function NoteLine({ children }: { children: string }) {
  return (
    <Box as="p" text="caption" tone="muted" breaks="phrase">
      {children}
    </Box>
  );
}

// ---- A period's costs -------------------------------------------------------------------------------------------------

/** A heading over what it heads, close under it. */
export function Titled({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Stack gap="entry-line">
      <GroupHeading>{title}</GroupHeading>
      {children}
    </Stack>
  );
}

/**
 * The costs of `period`, the newest the server answered, by provider, category and currency, a provider named once
 * over its categories, and their total, a figure a currency; a line of an imported provider, and its currency in the
 * total, are an estimate where the period is. Nothing for a period without costs.
 */
export function CostBreakdown({
  view,
  period,
  sources,
}: {
  view: FinanceView;
  period: CostPeriodTotal | undefined;
  /** The imported providers, whose lines are the period's estimate. */
  sources: CostTotalsRules["sources"];
}): ReactNode {
  const { t, locale } = useFinanceStrings();
  if (period === undefined || period.breakdown.length === 0) return null;
  const title = t("finance.breakdown", { period: periodName(t, locale, view.granularity, periodOfAnswer(period)) });
  const marked = estimatedIn(period, sources);
  const imported = (line: CostBreakdownLine) => period.estimated && importedBy(sources, line.provider);
  return (
    <Titled title={title}>
      <FinanceTable
        label={title}
        head={t("finance.column.provider")}
        columns={[{ label: t("finance.column.category") }, { label: t("finance.column.amount"), figures: true }]}
        estimated={period.estimated ? t("finance.estimate") : undefined}
        total={
          <FinanceRow
            head={t("finance.total")}
            cells={["", { figures: figuresOf(t, locale, period.totals, marked) }]}
          />
        }
      >
        {period.breakdown.map((line, i) => (
          <FinanceRow
            key={`${line.provider}\u0000${line.category}\u0000${line.amount.currency}`}
            head={line.provider}
            continued={period.breakdown[i - 1]?.provider === line.provider}
            cells={[
              t("finance.category", { category: line.category }),
              { figures: [moneyText(locale, line.amount)], estimated: imported(line) },
            ]}
          />
        ))}
      </FinanceTable>
    </Titled>
  );
}

/** Whether `provider` is an imported provider's, for a screen's item lines. */
export const importedProvider = importedBy;

/**
 * How many members each supporter plan has now, a row a plan with its headcount, and whether the income holds tips;
 * one line where no plan has members; nothing of what each paid.
 */
export function PlanMembers({
  plans,
  tips,
}: {
  plans: readonly IncomePlanCount[] | undefined;
  tips: boolean;
}): ReactNode {
  const { t } = useFinanceStrings();
  if (plans === undefined) return null;
  return (
    <Titled title={t("finance.plans")}>
      {plans.length === 0 ? (
        <NoteLine>{t("finance.plans_none")}</NoteLine>
      ) : (
        <FinanceTable
          label={t("finance.plans")}
          head={t("finance.column.plan")}
          columns={[{ label: t("finance.column.members"), figures: true }]}
        >
          {plans.map((plan) => (
            <FinanceRow
              key={plan.plan_id}
              head={plan.name}
              cells={[{ figures: [t("finance.members", { count: plan.members })] }]}
            />
          ))}
        </FinanceTable>
      )}
      {tips && <NoteLine>{t("finance.tips")}</NoteLine>}
    </Titled>
  );
}
