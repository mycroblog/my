// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps, notFound } from "@tanstack/react-router";
import { z } from "zod";
import { SupporterPlanDetail, supporterPlansQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The `_` keeps the page out of the list's layout at `/admin/supporters/plans`. */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One supporter plan (ADR-0072 §3): its facts, changing its terms and retiring it, for manage_supporters. There is no
 * GET of one plan, so the page reads the list of every plan; an id that is not in it is a page that does not exist.
 */
export const Route = createFileRoute("/admin/supporters_/plans_/$id")({
  staticData: { title: "client:admin.supporters.plan.page_title" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: ({ context, params: { id } }) =>
    loadWithPermission(context, "manage_supporters", async () => {
      const plans = await context.queryClient.query({
        ...supporterPlansQueryOptions(context.api),
        staleTime: Infinity,
      });
      if (!plans.items.some((plan) => plan.id === id)) notFound({ throw: true });
    }),
  component: PlanPage,
  errorComponent: PlanError,
});

function PlanPage() {
  const t = useT();
  const { id } = Route.useParams();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.supporters.plan.page_title")}</PageHeading>
        <Link href="/admin/supporters/plans" variant="standalone" size="sm">
          {t("client:admin.supporters.plan.back")}
        </Link>
      </Stack>
      {/* Another plan is another page: nothing of the previous one carries over (web-client/README.md R10). */}
      <SupporterPlanDetail key={id} id={id} />
    </>
  );
}

function PlanError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_supporters" heading={t("client:admin.supporters.plan.page_title")} />
  );
}
