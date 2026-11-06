// SPDX-License-Identifier: AGPL-3.0-only
import { BackLinkHeading } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { CostItemDetail, costItemQueryOptions } from "../features/admin";
import { deviceToday } from "../features/costs";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One recorded cost (ADR-0067), for manage_finance: its facts and prices, changing its label, its price and its end,
 * and deleting it. An unknown or deleted item is the route's error (404). The `_`s keep the page out of the layouts
 * of the overview and of the list.
 */
export const Route = createFileRoute("/admin/finance_/costs_/$id")({
  staticData: { title: "client:admin.finance.item.title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  loaderDeps: () => ({ today: deviceToday() }),
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_finance", () =>
      context.queryClient.query({ ...costItemQueryOptions(context.api, id), staleTime: Infinity }),
    ),
  component: CostItemPage,
  errorComponent: CostItemError,
});

function CostItemHeading() {
  const t = useT();
  return (
    <BackLinkHeading backHref="/admin/finance/costs" backLabel={t("client:admin.finance.item.back")}>
      <PageHeading>{t("client:admin.finance.item.title")}</PageHeading>
    </BackLinkHeading>
  );
}

function CostItemPage() {
  const { id } = Route.useParams();
  const { today } = Route.useLoaderDeps();
  return (
    <>
      <CostItemHeading />
      {/* Another item is another page: nothing typed for the previous one carries over (web-client/README.md R10). */}
      <CostItemDetail key={id} id={id} today={today} />
    </>
  );
}

function CostItemError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_finance" heading={t("client:admin.finance.item.title")} />;
}
