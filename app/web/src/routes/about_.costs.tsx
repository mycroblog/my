// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { instanceQueryOptions } from "../features/about";
import {
  type CostBasis,
  type CostGranularity,
  deviceToday,
  PublicCosts,
  publicCostTotalsQueryOptions,
  publicIncomeTotalsQueryOptions,
} from "../features/costs";
import { useT } from "../lib/i18n";
import { useInstanceName } from "../lib/instanceName";
import { InfoLayout } from "./-layout/InfoLayout";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/**
 * `granularity` and `basis` of the totals; omitted, or anything unknown, shows the months amortized (ADR-0067 §4,
 * web-client/README.md S3).
 */
const searchSchema = z.object({
  granularity: z.enum(["week", "year"]).optional().catch(undefined),
  basis: z.enum(["cash"]).optional().catch(undefined),
});

/**
 * The instance's running costs and income as its settings finance_visibility and income_visibility publish them
 * (FR-903, FR-917, ADR-0067 §8, ADR-0073 §7), for everyone, signed in or not. The `_` keeps the page out of the layout of `/about`. The totals end with today's period on the
 * viewer's device.
 */
export const Route = createFileRoute("/about_/costs")({
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { granularity: CostGranularity; basis: CostBasis; today: string } => ({
    granularity: search.granularity ?? "month",
    basis: search.basis ?? "amortized",
    today: deviceToday(),
  }),
  loader: async ({ context: { queryClient, api }, deps: { granularity, basis, today } }) => {
    // No failure fails the page: the shell reads the instance too, and the totals show their own failure, or the
    // setting that publishes nothing (404), in the page. Cached data is shown at once (web-client/README.md S7).
    await Promise.all([
      queryClient.query({ ...instanceQueryOptions(api), staleTime: Infinity }).catch(() => undefined),
      queryClient
        .query({ ...publicCostTotalsQueryOptions(api, granularity, basis, today), staleTime: Infinity })
        .catch(() => undefined),
      queryClient
        .query({ ...publicIncomeTotalsQueryOptions(api, granularity, basis, today), staleTime: Infinity })
        .catch(() => undefined),
    ]);
  },
  component: CostsPage,
  errorComponent: CostsError,
});

function CostsPage() {
  const { granularity, basis, today } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <InfoLayout heading={<CostsHeading />}>
      <PublicCosts
        granularity={granularity}
        basis={basis}
        today={today}
        onTotalsChange={(next) => {
          void navigate({
            search: {
              ...(next.granularity === "month" ? {} : { granularity: next.granularity }),
              ...(next.basis === "amortized" ? {} : { basis: next.basis }),
            },
          });
        }}
      />
    </InfoLayout>
  );
}

/** The page's heading, which names the instance. */
function CostsHeading() {
  const t = useT();
  const instance = useInstanceName();
  return <PageHeading>{t("client:costs.public.title", { instance })}</PageHeading>;
}

function CostsError(props: ErrorComponentProps) {
  const t = useT();
  const instance = useInstanceName();
  return <RouteError {...props} heading={t("client:costs.public.title", { instance })} />;
}
