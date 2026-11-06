// SPDX-License-Identifier: AGPL-3.0-only
// The overview of the costs screen (`/admin/finance`, ADR-0067 §7, ADR-0067 §16, manage_finance;
// web-client/screens/admin-finance.md): the totals of the costs and of the income by week, month or year under tabs,
// amortized (the default) or in cash, with the rules that cut their periods next to them, and the income's gross,
// fees, net, balance and the members of each plan under the same tabs. The granularity and the basis are in the URL
// (web-client/README.md S3); the other pages of the screen are links under the heading.
import { Card, Heading, Hint, Stack } from "@my/ui";
import { useId } from "react";
import { useT } from "../../../lib/i18n";
import {
  type CostBasis,
  type CostGranularity,
  chartsTotals,
  CostTotalsPanel,
  CostTotalsView,
  financeCurrency,
  FinanceOverview,
  IncomeTotalsView,
} from "../../costs";
import { Section } from "../../moderation";
import { useAdminIncomeTotals, useCostTotals } from "../hooks/useFinance";
import { FinanceScreen } from "./FinanceParts";

export type FinanceTotalsProps = {
  /** The granularity and the basis of the totals, from the URL. */
  granularity: CostGranularity;
  basis: CostBasis;
  /** Today's date on the viewer's device, which ends the shown periods (ADR-0067 §5). */
  today: string;
  /** Selects another granularity or basis: the route puts it into the URL. */
  onTotalsChange: (next: { granularity: CostGranularity; basis: CostBasis }) => void;
};

export function FinanceTotals({ granularity, basis, today, onTotalsChange }: FinanceTotalsProps) {
  const t = useT();
  return (
    <FinanceScreen page="overview">
      <Hint size="inherit">{t("client:admin.finance.description")}</Hint>
      <Section surface="card" title={t("client:admin.finance.totals.title")}>
        <Card>
          <CostTotalsPanel granularity={granularity} basis={basis} onTotalsChange={onTotalsChange}>
            <Totals granularity={granularity} basis={basis} today={today} />
          </CostTotalsPanel>
        </Card>
      </Section>
    </FinanceScreen>
  );
}

/**
 * The overview of the selected periods (FinanceOverview), their costs, then their income with the gross, the fees and
 * the balance under the same basis: the table of the income stays for its gross and fees.
 */
function Totals({ granularity, basis, today }: { granularity: CostGranularity; basis: CostBasis; today: string }) {
  const t = useT();
  const totals = useCostTotals(granularity, basis, today);
  const income = useAdminIncomeTotals(granularity, basis, today);
  const incomeId = useId();
  const charted = chartsTotals(financeCurrency(totals.periods, income.periods));
  return (
    <Stack gap="8">
      <FinanceOverview granularity={granularity} costs={totals.periods} income={income.periods} />
      <CostTotalsView totals={totals} breakdown table={!charted} />
      <Stack as="section" gap="3" aria-labelledby={incomeId}>
        <Heading level={3} id={incomeId}>
          {t("client:income.title")}
        </Heading>
        <IncomeTotalsView totals={income} detailed headingLevel={4} />
      </Stack>
    </Stack>
  );
}
