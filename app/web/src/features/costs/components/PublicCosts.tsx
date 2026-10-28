// SPDX-License-Identifier: AGPL-3.0-only
import { EmptyState, Hint, Link, RegionError, Spinner, Stack } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { instanceQueryOptions } from "../../about";
import { Section } from "../../moderation";
import { usePublicIncome } from "../hooks/useIncome";
import { usePublicCosts } from "../hooks/usePublicCosts";
import { CostTotalsPanel, type CostTotalsPanelProps, CostTotalsView } from "./CostTotals";
import { chartsTotals, financeCurrency, FinanceOverview } from "./FinanceOverview";
import { IncomeState } from "./IncomeTotals";

export type PublicCostsProps = Omit<CostTotalsPanelProps, "children"> & {
  /** Today's date on the viewer's device, which ends the shown periods (ADR-0067 §5). */
  today: string;
};

/**
 * The body of the public costs page (`/about/costs`, FR-903, FR-917, ADR-0067 §8, ADR-0073 §7), under its heading:
 * what the instance publishes of its running costs and of its income, by week, month or year, amortized or in cash.
 * The costs come with the breakdown of the current period by provider and category when the instance publishes it;
 * the income with the members of each plan when the instance publishes them, and with the balance only while the
 * costs are public too. The names and memos of the items, the individual payments and the subscribers are never
 * published. When the instance publishes neither, the page says so.
 */
export function PublicCosts({ today, ...panel }: PublicCostsProps) {
  const t = useT();
  const api = useApi();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const breakdown = instance?.finance_visibility === "breakdown";
  const costs = usePublicCosts(panel.granularity, panel.basis, today, breakdown);
  const income = usePublicIncome(panel.granularity, panel.basis, today);
  // The overview waits for every part the instance publishes, so that its numbers do not change under the reader.
  const costPeriods = costs.status === "ready" ? costs.totals.periods : undefined;
  const incomePeriods = income.status === "ready" ? income.totals.periods : undefined;
  const ready =
    (costs.status === "ready" || costs.status === "hidden") &&
    (income.status === "ready" || income.status === "hidden");
  const charted = ready && chartsTotals(financeCurrency(costPeriods, incomePeriods));
  if (costs.status === "hidden" && income.status === "hidden") {
    return (
      <EmptyState
        headingLevel={2}
        heading={t("client:costs.public.hidden")}
        action={
          <Link href="/about" variant="standalone">
            {t("client:costs.public.about")}
          </Link>
        }
      />
    );
  }
  return (
    <Stack gap="6">
      <p>{t("client:costs.public.description")}</p>
      <CostTotalsPanel {...panel}>
        <Stack gap="8">
          {ready && <FinanceOverview granularity={panel.granularity} costs={costPeriods} income={incomePeriods} />}
          {costs.status !== "hidden" && (
            <Section surface="card" title={t("client:costs.public.costs")}>
              {instance?.finance_visibility === "totals" && (
                <Hint size="inherit">{t("client:costs.public.totals_only")}</Hint>
              )}
              {costs.status === "loading" && (
                <div aria-busy="true">
                  <Spinner label={t("client:costs.public.loading")} />
                </div>
              )}
              {costs.status === "error" && (
                <RegionError
                  heading={t("client:costs.public.failed")}
                  reason={costs.reason}
                  retryLabel={t("client:costs.public.retry")}
                  onRetry={costs.retry}
                  isRetrying={costs.isRetrying}
                />
              )}
              {costs.status === "ready" && (
                <CostTotalsView totals={costs.totals} breakdown={costs.breakdown} table={!charted} />
              )}
            </Section>
          )}
          {income.status !== "hidden" && (
            <Section surface="card" title={t("client:income.title")}>
              {/* The balance would tell the costs that the instance keeps to itself (ADR-0073 §7). */}
              {costs.status === "hidden" && <Hint size="inherit">{t("client:income.public.no_balance")}</Hint>}
              <IncomeState income={income} table={!charted} />
            </Section>
          )}
        </Stack>
      </CostTotalsPanel>
    </Stack>
  );
}
