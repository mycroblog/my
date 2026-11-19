// SPDX-License-Identifier: AGPL-3.0-only

// The server's costs and income as it publishes them to everyone, at /about/costs (ADR-0067 §8, §17, §22; FR-902,
// FR-903, FR-905, FR-917), signed in or not: this month's funding meter where the server shows it; the totals of the
// costs and of the income per week, month or year, beside each other, and the balance where both are published, a
// figure a currency, never converted; the rules that cut the periods; the latest period's costs by provider where the
// breakdown is published; the members of each plan where they are. Never who paid what; and where a side is not
// published, the page says so and reads nothing of it. A side the server stops publishing while the page is open (its
// totals answered 404) is not published from then on: nothing of what was read stays, and the instance is read again.

import {
  useFinancePublished,
  useFundingMeter,
  usePublicCostTotals,
  usePublicIncomeTotals,
  type FinancePublished,
} from "@my/app-data";
import { FeedNotice, ScreenStack, Stack } from "@my/design-system";
import type { ReactNode } from "react";
import { FundingMeterView } from "./meter";
import {
  CostBreakdown,
  FinanceChoices,
  FinancePages,
  PlanMembers,
  ReadNotice,
  Titled,
  TotalsTable,
  useFinanceStrings,
  useFinanceView,
} from "./parts";

/** The page of the server's published costs and income. */
export function PublicFinanceScreen(): ReactNode {
  const { t } = useFinanceStrings();
  const published = useFinancePublished();
  const meter = useFundingMeter();
  if (published === undefined) return <FeedNotice start>{t("finance.loading")}</FeedNotice>;
  return (
    <ScreenStack>
      {meter !== undefined && (
        <Titled title={t("finance.meter")}>
          <FundingMeterView meter={meter} detail />
        </Titled>
      )}
      {published.any ? (
        <PublishedTotals published={published} meter={meter !== undefined} />
      ) : (
        <Unpublished meter={meter !== undefined} />
      )}
    </ScreenStack>
  );
}

/** That nothing is published: the totals, under a meter, or the costs and income. */
function Unpublished({ meter }: { meter: boolean }) {
  const { t } = useFinanceStrings();
  return <FeedNotice lead>{t(meter ? "finance.unpublished_totals" : "finance.unpublished")}</FeedNotice>;
}

/** The totals of what the server publishes, in the reader's window. */
function PublishedTotals({ published, meter }: { published: FinancePublished; meter: boolean }) {
  const { t } = useFinanceStrings();
  const view = useFinanceView();
  const costs = usePublicCostTotals(view.query, published.costs !== null);
  const income = usePublicIncomeTotals(view.query, published.income !== null);
  // A side is shown while the instance says it is published and the server has not said since that it is not.
  const costsShown = published.costs !== null && !costs.unpublished;
  const incomeShown = published.income !== null && !income.unpublished;
  if (!costsShown && !incomeShown) return <Unpublished meter={meter} />;
  const reads = [...(costsShown ? [costs] : []), ...(incomeShown ? [income] : [])];
  // What holds its totals stays read: a read again that failed takes nothing away.
  const read = reads.every((r) => r.totals !== undefined);
  // The provider's billing zones, only while the breakdown is published: the totals alone name no provider.
  const sources = published.costs === "breakdown" ? costs.totals?.rules.sources : undefined;
  return (
    <Stack gap="entry-room">
      <FinanceChoices view={view} />
      <Stack gap="entry-line">
        {read ? (
          <TotalsTable
            view={view}
            costs={costsShown ? costs.totals?.periods : undefined}
            income={incomeShown ? income.totals?.periods : undefined}
            balance={costsShown && incomeShown && published.balance}
            sources={sources}
            incomeRules={incomeShown ? income.totals?.rules : undefined}
          />
        ) : (
          <ReadNotice reads={reads} />
        )}
        {!costsShown && <FeedNotice start>{t("finance.unpublished_costs")}</FeedNotice>}
        {!incomeShown && <FeedNotice start>{t("finance.unpublished_income")}</FeedNotice>}
      </Stack>
      <FinancePages view={view} />
      {read && costsShown && published.costs === "breakdown" && (
        <CostBreakdown view={view} period={costs.totals?.periods.at(-1)} sources={sources ?? []} />
      )}
      {read && incomeShown && published.income === "plans" && (
        <PlanMembers plans={income.totals?.plans} tips={income.totals?.tips === true} />
      )}
    </Stack>
  );
}
