// SPDX-License-Identifier: AGPL-3.0-only

// The income as a holder of `manage_finance` reads it, at /admin/finance/income (ADR-0067 §15, §16; FR-917, FR-919):
// per week, month or year, the gross of the payment provider's money movements, its fees, the net and the balance
// against the costs, a figure a currency, never converted; the latest period's net by where it came from (the
// subscriptions, the tips, the rest of the provider's account); and the money movements one by one, newest first, of
// one kind or of every kind, each with when, what, the amounts, and who paid it while the account exists. Only this
// screen shows who paid what.

import {
  incomeKinds,
  useAdminIncomeTotals,
  useIncomeMovements,
  type AdminIncomePeriodTotal,
  type IncomeKind,
  type IncomeMovement,
  type Money,
} from "@my/app-data";
import {
  Pager,
  FeedNotice,
  FilterBar,
  FilterFacet,
  FinanceRow,
  FinanceTable,
  ScreenStack,
  ScreenTitle,
  TextButton,
  type FilterValue,
  type FinanceFigure,
} from "@my/design-system";
import type { ReactNode } from "react";
import {
  answeredPeriods,
  FinanceChoices,
  FinancePages,
  moneyText,
  periodName,
  periodOfAnswer,
  ReadNotice,
  useFinanceView,
  type FinanceView,
} from "../../finance-totals";
import type { StaffSearch, StaffUnderProps } from "../entries";
import { filtered, useAddressPages, Section, useStaffStrings, type T } from "../parts";
import { FinanceBack, useDay } from "./parts";

/** The figures of a cell: an amount a currency, or None. */
const figuresOf = (t: T, locale: string, amounts: readonly Money[] | undefined): FinanceFigure[] =>
  amounts === undefined || amounts.length === 0 ? [t("finance.none")] : amounts.map((m) => moneyText(locale, m));

/** The totals of the window: a period a row, its gross, fees, net and balance. */
function IncomeTable({ view, periods }: { view: FinanceView; periods: readonly AdminIncomePeriodTotal[] }) {
  const { t, locale } = useStaffStrings();
  return (
    <FinanceTable
      label={t("staff_finance.income.totals")}
      head={t(`finance.period.${view.granularity}`)}
      columns={[
        { label: t("staff_finance.income.column.gross"), figures: true },
        { label: t("staff_finance.income.column.fee"), figures: true },
        { label: t("staff_finance.income.column.net"), figures: true },
        { label: t("staff_finance.income.column.balance"), figures: true },
      ]}
    >
      {answeredPeriods(view, periods).map((period) => {
        const got = periods.find((p) => p.label === period.label);
        return (
          <FinanceRow
            key={period.label}
            head={periodName(t, locale, view.granularity, period)}
            cells={[
              { figures: figuresOf(t, locale, got?.gross) },
              { figures: figuresOf(t, locale, got?.fee) },
              { figures: figuresOf(t, locale, got?.net) },
              { figures: figuresOf(t, locale, got?.balance) },
            ]}
          />
        );
      })}
    </FinanceTable>
  );
}

/** The net of a period by where it came from: the subscriptions, the tips, and the rest of the provider's account. */
function BySource({ view, period }: { view: FinanceView; period: AdminIncomePeriodTotal | undefined }): ReactNode {
  const { t, locale } = useStaffStrings();
  if (period === undefined || period.net_by_source.length === 0) return null;
  const title = t("staff_finance.income.by_source", {
    period: periodName(t, locale, view.granularity, periodOfAnswer(period)),
  });
  return (
    <Section title={title}>
      <FinanceTable
        label={title}
        head={t("staff_finance.income.column.source")}
        columns={[{ label: t("staff_finance.income.column.net"), figures: true }]}
      >
        {period.net_by_source.map((line) => (
          <FinanceRow
            key={line.source}
            head={t("staff_finance.income.source", { source: line.source })}
            cells={[{ figures: figuresOf(t, locale, line.net) }]}
          />
        ))}
      </FinanceTable>
    </Section>
  );
}

/** The kind of movements the address asks for; every kind where it names none. */
const kindOf = (search: StaffSearch): IncomeKind | undefined => incomeKinds.find((k) => k === search["kind"]);

/** Who paid a movement, as its row's head names them. */
const payerOf = (t: T, m: IncomeMovement): string => {
  if (m.username !== undefined) return `@${m.username}`;
  return t(
    m.subscription_id !== undefined || m.tip_id !== undefined
      ? "staff_finance.income.erased"
      : "staff_finance.income.none",
  );
};

/**
 * The money movements, newest first, of the kind the address names, a page at a time, the page kept in the address: a
 * movement a row, its amounts in columns.
 */
function Movements({ search, onSearch }: { search: StaffSearch; onSearch: (search: StaffSearch) => void }) {
  const pages = useAddressPages({ search, onSearch });
  const { t, locale } = useStaffStrings();
  const day = useDay();
  const kind = kindOf(search);
  const list = useIncomeMovements({ kind }, pages.cursor);
  const values: FilterValue[] = incomeKinds.map((k) => ({
    id: k,
    label: t("staff_finance.income.kind", { kind: k }),
    icon: "funding",
  }));
  let shown: ReactNode;
  if (list.status !== "success" && !list.stale)
    shown =
      list.status === "error" ? (
        <FeedNotice alert start action={<TextButton onPress={list.refetch}>{t("staff.retry")}</TextButton>}>
          {t("staff_finance.income.movements.failed")}
        </FeedNotice>
      ) : (
        <FeedNotice start>{t("staff_finance.income.movements.loading")}</FeedNotice>
      );
  else if (list.items.length === 0)
    shown = (
      <FeedNotice start>
        {t(kind === undefined ? "staff_finance.income.movements.empty" : "staff_finance.income.movements.none")}
      </FeedNotice>
    );
  else
    shown = (
      <FinanceTable
        label={t("staff_finance.income.movements")}
        head={t("staff_finance.income.column.on")}
        columns={[
          { label: t("staff_finance.income.column.payer") },
          { label: t("staff_finance.income.column.gross"), figures: true },
          { label: t("staff_finance.income.column.fee"), figures: true },
          { label: t("staff_finance.income.column.net"), figures: true },
        ]}
      >
        {list.items.map((m) => (
          <FinanceRow
            key={m.id}
            head={day(m.occurred_on)}
            caption={t("staff_finance.income.what", { kind: m.kind, source: m.source })}
            cells={[
              payerOf(t, m),
              { figures: [moneyText(locale, m.gross)] },
              { figures: [moneyText(locale, m.fee)] },
              { figures: [moneyText(locale, m.net)] },
            ]}
          />
        ))}
      </FinanceTable>
    );
  return (
    <Section title={t("staff_finance.income.movements")}>
      <FilterBar
        label={t("staff_finance.income.filters")}
        applied={kind !== undefined}
        onClear={() => {
          onSearch(filtered(search, { kind: undefined }));
        }}
      >
        <FilterFacet
          label={t("staff_finance.income.kind.label")}
          any={t("staff_finance.income.kind.any")}
          values={values}
          chosen={kind}
          onChoose={(chosen) => {
            onSearch(filtered(search, { kind: chosen }));
          }}
        />
      </FilterBar>
      {shown}
      <Pager {...pages.pager(list.next)} />
    </Section>
  );
}

/** The income: its totals per period, the latest period's net by source, and the money movements. */
export function IncomeAdminScreen({ search, onSearch, go }: StaffUnderProps): ReactNode {
  const { t } = useStaffStrings();
  const view = useFinanceView();
  const totals = useAdminIncomeTotals(view.query);
  const title = <ScreenTitle back={<FinanceBack to="root" go={go} />}>{t("staff_finance.income")}</ScreenTitle>;
  if (totals.forbidden)
    return (
      <ScreenStack>
        {title}
        <FeedNotice lead>{t("staff_finance.forbidden")}</FeedNotice>
      </ScreenStack>
    );
  const got = totals.value;
  return (
    <ScreenStack>
      {title}
      <FinanceChoices view={view} about={false} />
      {got === undefined ? <ReadNotice reads={[totals]} /> : <IncomeTable view={view} periods={got.periods} />}
      <FinancePages view={view} />
      <BySource view={view} period={got?.periods.at(-1)} />
      <Movements search={search} onSearch={onSearch} />
    </ScreenStack>
  );
}
