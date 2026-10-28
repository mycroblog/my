// SPDX-License-Identifier: AGPL-3.0-only
import { Hint, RegionError, Spinner, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { Section, MissingPermission } from "../../moderation";
import type { FinanceViewerSettings } from "../api/costs";
import { useFinanceAccess, useFinanceCosts } from "../hooks/useFinanceCosts";
import { useFinanceIncome } from "../hooks/useIncome";
import { CostTotalsPanel, type CostTotalsPanelProps, CostTotalsView } from "./CostTotals";
import { IncomeState } from "./IncomeTotals";

/** What the instance shows of the items to the holders of view_finance, for the sentence that says it. */
function shownOfItems(viewer: FinanceViewerSettings): "both" | "names" | "memos" | "none" {
  if (viewer.item_names && viewer.item_memos) return "both";
  if (viewer.item_names) return "names";
  if (viewer.item_memos) return "memos";
  return "none";
}

export type FinanceCostsProps = Omit<CostTotalsPanelProps, "children"> & {
  /** Today's date on the viewer's device, which ends the shown periods (ADR-0067 §5). */
  today: string;
};

/**
 * The body of the finances page (`/finance`, ADR-0070 §5, ADR-0073 §6), under its heading, for the holders of
 * view_finance: the totals of the instance's costs and of its income by week, month or year, amortized or in cash.
 * The costs always come with the breakdown of the current period by provider and category, and the lines of its items
 * while the instance shows their names or memos (a holder of manage_finance reads them all); the income always with
 * the balance, and with the members of each plan when the instance shows them. It says what the instance does not
 * show. Nobody's payments are shown, and recording the costs is the costs screen's (`/admin/finance`). A viewer
 * without the permission sees why.
 */
export function FinanceCosts({ today, ...panel }: FinanceCostsProps) {
  const t = useT();
  const access = useFinanceAccess();
  const costs = useFinanceCosts(panel.granularity, panel.basis, today);
  const income = useFinanceIncome(panel.granularity, panel.basis, today);
  if (!access.allowed) return <MissingPermission permission="view_finance" />;
  return (
    <Stack gap="6">
      <p>{t("client:finance.description")}</p>
      {costs.status === "ready" && !access.manage && (
        <Hint size="inherit">{t("client:finance.items_shown", { shown: shownOfItems(costs.totals.viewer) })}</Hint>
      )}
      <CostTotalsPanel {...panel}>
        <Stack gap="8">
          <Section surface="card" title={t("client:finance.costs.title")}>
            {costs.status === "loading" && (
              <div aria-busy="true">
                <Spinner label={t("client:finance.loading")} />
              </div>
            )}
            {costs.status === "error" && (
              <RegionError
                heading={t("client:finance.failed")}
                reason={costs.reason}
                retryLabel={t("client:finance.retry")}
                onRetry={costs.retry}
                isRetrying={costs.isRetrying}
              />
            )}
            {costs.status === "ready" && <CostTotalsView totals={costs.totals} breakdown items={costs.items} />}
          </Section>
          <Section surface="card" title={t("client:income.title")}>
            <IncomeState income={income} />
          </Section>
        </Stack>
      </CostTotalsPanel>
    </Stack>
  );
}
