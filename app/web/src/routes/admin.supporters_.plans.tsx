// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { SupporterPlans, supporterPlanStates, supporterPlansQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The plans shown: those in a state (`?state=`); omitted, or anything unknown, shows every one (web-client/README.md S3). */
const searchSchema = z.object({ state: z.enum(supporterPlanStates).optional().catch(undefined) });

/** The supporter plans (ADR-0072), a page of the supporters screen: the list, which leads to each plan's page, for manage_supporters. */
export const Route = createFileRoute("/admin/supporters_/plans")({
  staticData: { title: "client:admin.supporters.plans.page_title" },
  validateSearch: searchSchema,
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_supporters", () =>
      context.queryClient.query({ ...supporterPlansQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: PlansPage,
  errorComponent: PlansError,
});

function PlansPage() {
  const t = useT();
  const { state } = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.supporters.plans.page_title")}</PageHeading>
      <SupporterPlans
        state={state}
        onStateChange={(next) => {
          void navigate({ search: next === undefined ? {} : { state: next } });
        }}
      />
    </>
  );
}

function PlansError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError {...props} permission="manage_supporters" heading={t("client:admin.supporters.plans.page_title")} />
  );
}
