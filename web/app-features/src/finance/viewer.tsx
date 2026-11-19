// SPDX-License-Identifier: AGPL-3.0-only

// The finance of the server as a holder of `view_finance` reads it, at /finance (ADR-0070 §1, §5; ADR-0067 §16;
// FR-916), a place of the menu for them: the totals of the costs and of the income per week, month or year and the
// balance, a figure a currency, never converted; the rules that cut the periods; the latest period's costs by
// provider; its cost items, with their names and their notes only where the server's settings show them; and the
// members of each plan only where they are set to show. Never who paid what. A reader the server does not let read it
// is told so, and nothing else.

import {
  financeRefusal,
  useFinanceCostTotals,
  useFinanceIncomeTotals,
  type CostTotalsRules,
  type FinanceCostItemLine,
  type FinanceCostPeriodTotal,
} from "@my/app-data";
import { FeedNotice, FinanceRow, FinanceTable, ScreenStack, type FinanceColumn } from "@my/design-system";
import type { ReactNode } from "react";
import {
  CostBreakdown,
  FinanceChoices,
  FinancePages,
  moneyText,
  periodName,
  periodOfAnswer,
  importedProvider,
  PlanMembers,
  ReadNotice,
  Titled,
  TotalsTable,
  useFinanceStrings,
  useFinanceView,
  type FinanceView,
} from "./parts";

/** The finance page of a holder of `view_finance`. */
export function FinanceScreen(): ReactNode {
  const { t } = useFinanceStrings();
  const view = useFinanceView();
  const costs = useFinanceCostTotals(view.query);
  const income = useFinanceIncomeTotals(view.query);
  if (financeRefusal(costs.error) === "forbidden" || financeRefusal(income.error) === "forbidden")
    return <FeedNotice lead>{t("finance.forbidden")}</FeedNotice>;
  // What holds its totals stays read: a read again that failed takes nothing away.
  const spent = costs.totals;
  const got = income.totals;
  const pages = <FinancePages view={view} />;
  if (spent === undefined || got === undefined)
    return (
      <ScreenStack>
        <FinanceChoices view={view} />
        <ReadNotice reads={[costs, income]} />
        {pages}
      </ScreenStack>
    );
  // The newest period the server answered: the reader's today may be a day ahead of the server's.
  const latest = spent.periods.at(-1);
  return (
    <ScreenStack>
      <FinanceChoices view={view} />
      <TotalsTable
        view={view}
        costs={spent.periods}
        income={got.periods}
        balance
        sources={spent.rules.sources}
        incomeRules={got.rules}
      />
      {pages}
      <CostBreakdown view={view} period={latest} sources={spent.rules.sources} />
      <CostItems view={view} period={latest} sources={spent.rules.sources} />
      <PlanMembers plans={got.plans} tips={got.tips === true} />
    </ScreenStack>
  );
}

/**
 * The cost items of `period`, the newest the server answered, where the server shows their names or their notes: an item a
 * row, headed by its name (else its provider), its note under it, its amount in its currency. Nothing where it shows
 * neither, or for a period without items.
 */
function CostItems({
  view,
  period,
  sources,
}: {
  view: FinanceView;
  period: FinanceCostPeriodTotal | undefined;
  /** The imported providers, whose lines are the period's estimate. */
  sources: CostTotalsRules["sources"];
}): ReactNode {
  const { t, locale } = useFinanceStrings();
  if (period === undefined || period.items.length === 0) return null;
  const named = period.items.some((item) => item.name !== undefined);
  const title = t("finance.items", { period: periodName(t, locale, view.granularity, periodOfAnswer(period)) });
  const columns: FinanceColumn[] = [
    ...(named ? [{ label: t("finance.column.provider") }] : []),
    { label: t("finance.column.category") },
    { label: t("finance.column.amount"), figures: true },
  ];
  const cellsOf = (item: FinanceCostItemLine) => [
    ...(named ? [item.provider] : []),
    t("finance.category", { category: item.category }),
    {
      figures: [moneyText(locale, item.amount)],
      estimated: period.estimated && importedProvider(sources, item.provider),
    },
  ];
  // The server's order, which may repeat a line (one provider, category and amount, without names): the n-th of its kind.
  const seen = new Map<string, number>();
  const keys = period.items.map((item) => {
    const kind = [item.provider, item.category, item.name ?? "", item.amount.currency, item.amount.amount].join(
      "\u0000",
    );
    const n = (seen.get(kind) ?? 0) + 1;
    seen.set(kind, n);
    return `${kind}\u0000${String(n)}`;
  });
  return (
    <Titled title={title}>
      <FinanceTable
        label={title}
        head={t(named ? "finance.column.item" : "finance.column.provider")}
        columns={columns}
        estimated={period.estimated ? t("finance.estimate") : undefined}
      >
        {period.items.map((item, i) => (
          <FinanceRow key={keys[i]} head={item.name ?? item.provider} caption={item.memo} cells={cellsOf(item)} />
        ))}
      </FinanceTable>
    </Titled>
  );
}
