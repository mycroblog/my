// SPDX-License-Identifier: AGPL-3.0-only
// The totals of the instance's income as the administrators' costs screen, the public costs page and the finances
// page show them (ADR-0073 §5–§7, FR-916, FR-917): the table of the periods, newest first, with the net income of
// each currency, the gross and the fees on the costs screen only, and the balance against the costs where the answer
// carries it; the members of each supporter plan now where they are shown; and the rules that date and place the
// payments. Nothing here names a payment or a subscriber. Plan names are the operators' words, shown with dir="auto".
import {
  Cluster,
  Heading,
  Hint,
  RegionError,
  Spinner,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableColumnHeader,
  TableHead,
  TableRow,
  TableRowHeader,
  Text,
} from "@my/ui";
import { useId } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { CostBasis, CostGranularity, Money } from "../api/costs";
import type { IncomePeriodTotal, IncomePlanCount, IncomeTotals } from "../api/income";
import { useFormatTotals, usePeriodName } from "../hooks/useCostFormat";

const granularityLabels: Readonly<Record<CostGranularity, ClientMessageKey>> = {
  week: "client:costs.granularity.week",
  month: "client:costs.granularity.month",
  year: "client:costs.granularity.year",
};

const basisLabels: Readonly<Record<CostBasis, ClientMessageKey>> = {
  amortized: "client:costs.basis.amortized",
  cash: "client:costs.basis.cash",
};

/** A period of the income totals, with the gross and the fees on the administrators' screen. */
type ShownIncomePeriod = IncomePeriodTotal & { gross?: readonly Money[]; fee?: readonly Money[] };

/** The income totals of any of the screens: the administrators' periods also have the gross and the fees. */
type ShownIncomeTotals = Omit<IncomeTotals, "periods"> & { periods: readonly ShownIncomePeriod[] };

export type IncomeTotalsViewProps = {
  totals: ShownIncomeTotals;
  /** Shows the gross and the provider's fees of each period: the costs screen only (ADR-0073 §6). */
  detailed?: boolean;
  /** The level of the headings of the members of each plan and of the rules: 3 below an h2, 4 below an h3. */
  headingLevel?: 3 | 4;
  /** Shows the table of the periods: false where the overview's chart shows them (FinanceOverview). */
  table?: boolean;
};

/** Formats a balance of each currency with its sign, never converted: "+¥1,200, −$3.40". */
function useFormatBalance(): (balance: readonly Money[]) => string {
  const intl = useIntl();
  const t = useT();
  return (balance) => {
    if (balance.length === 0) return t("client:costs.totals.none");
    const amounts = balance.map((money) =>
      intl.formatNumber(Number(money.amount), {
        style: "currency",
        currency: money.currency,
        signDisplay: "exceptZero",
      }),
    );
    return intl.formatList(amounts, { type: "unit", style: "short" });
  };
}

/** The income totals of the shown periods, newest first, the members of each plan, and the rules that place them. */
export function IncomeTotalsView({ totals, detailed = false, headingLevel = 3, table = true }: IncomeTotalsViewProps) {
  const t = useT();
  const periodName = usePeriodName();
  const formatTotals = useFormatTotals();
  const formatBalance = useFormatBalance();
  const { granularity, basis } = totals;
  const newestFirst = totals.periods.toReversed();
  // The answer carries the balance only where it is shown: on the public page, while the costs are public too.
  const balance = totals.periods.some((period) => period.balance !== undefined);
  return (
    <Stack gap="4">
      {table && (
        <Table
          caption={t("client:income.totals.caption", {
            granularity: t(granularityLabels[granularity]),
            basis: t(basisLabels[basis]),
          })}
        >
          <TableHead>
            <TableColumnHeader>{t("client:costs.totals.period")}</TableColumnHeader>
            {detailed && (
              <>
                <TableColumnHeader>{t("client:income.totals.gross")}</TableColumnHeader>
                <TableColumnHeader>{t("client:income.totals.fee")}</TableColumnHeader>
              </>
            )}
            <TableColumnHeader>{t("client:income.totals.net")}</TableColumnHeader>
            {balance && <TableColumnHeader>{t("client:income.totals.balance")}</TableColumnHeader>}
          </TableHead>
          <TableBody>
            {newestFirst.map((period) => (
              <TableRow key={period.label}>
                <TableRowHeader>{periodName(granularity, period)}</TableRowHeader>
                {detailed && (
                  <>
                    <TableCell numeric wrap="anywhere">
                      {formatTotals(period.gross ?? [])}
                    </TableCell>
                    <TableCell numeric wrap="anywhere">
                      {formatTotals(period.fee ?? [])}
                    </TableCell>
                  </>
                )}
                <TableCell numeric wrap="anywhere">
                  {formatTotals(period.net)}
                </TableCell>
                {balance && (
                  <TableCell numeric wrap="anywhere">
                    {formatBalance(period.balance ?? [])}
                  </TableCell>
                )}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      {totals.plans !== undefined && <PlanMembers plans={totals.plans} headingLevel={headingLevel} />}
      <IncomeRules totals={totals} balance={balance} headingLevel={headingLevel} />
    </Stack>
  );
}

/** The members of each supporter plan now (ADR-0073 §7): never per period, never the plans' amounts. */
function PlanMembers({ plans, headingLevel }: { plans: readonly IncomePlanCount[]; headingLevel: 3 | 4 }) {
  const t = useT();
  const headingId = useId();
  return (
    <Stack as="section" gap="2" aria-labelledby={headingId}>
      <Heading level={headingLevel} id={headingId}>
        {t("client:income.plans.title")}
      </Heading>
      <Hint size="inherit">{t("client:income.plans.description")}</Hint>
      {plans.length === 0 ? (
        <Hint size="inherit">{t("client:income.plans.empty")}</Hint>
      ) : (
        <Stack as="ul" gap="1">
          {plans.map((plan) => (
            <Cluster as="li" gap="2" rowGap="0" key={plan.plan_id}>
              {/* The administrator's words, in a language the app does not know (web-client/README.md Y10). */}
              <Text wrap="anywhere" dir="auto" lang="">
                {plan.name}
              </Text>
              <Text numeric>{t("client:income.plans.members", { count: plan.members })}</Text>
            </Cluster>
          ))}
        </Stack>
      )}
    </Stack>
  );
}

/** How the income totals date and place the payments (ADR-0073 §3, §5), and what the balance is. */
function IncomeRules({
  totals,
  balance,
  headingLevel,
}: {
  totals: ShownIncomeTotals;
  balance: boolean;
  headingLevel: 3 | 4;
}) {
  const t = useT();
  const intl = useIntl();
  const headingId = useId();
  return (
    <Stack as="section" gap="2" tone="muted" aria-labelledby={headingId}>
      <Heading level={headingLevel} id={headingId} tone="fg">
        {t("client:income.rules.title")}
      </Heading>
      <Stack as="ul" gap="1" marker="disc">
        <li>{t("client:income.rules.net")}</li>
        {totals.rules.providers.map((provider) => (
          <li key={provider.provider}>
            {t("client:income.rules.provider", { provider: provider.provider, timeZone: provider.time_zone })}
          </li>
        ))}
        <li>
          {totals.basis === "amortized"
            ? t("client:income.rules.amortized", { days: intl.formatNumber(totals.rules.amortize_after_days) })
            : t("client:income.rules.cash")}
        </li>
        {balance && <li>{t("client:income.rules.balance")}</li>}
      </Stack>
    </Stack>
  );
}

/** The income of a page as its hook gives it, once the instance is known to show it. */
type ShownIncome =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; totals: ShownIncomeTotals };

/**
 * The income totals of the public costs page and the finances page while they load, when they fail, and once read;
 * `table` as IncomeTotalsView's.
 */
export function IncomeState({ income, table = true }: { income: ShownIncome; table?: boolean }) {
  const t = useT();
  if (income.status === "loading") {
    return (
      <div aria-busy="true">
        <Spinner label={t("client:income.loading")} />
      </div>
    );
  }
  if (income.status === "error") {
    return (
      <RegionError
        heading={t("client:income.failed")}
        reason={income.reason}
        retryLabel={t("client:income.retry")}
        onRetry={income.retry}
        isRetrying={income.isRetrying}
      />
    );
  }
  return <IncomeTotalsView totals={income.totals} table={table} />;
}
