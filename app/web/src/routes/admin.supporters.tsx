// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  type SupporterSubscriptionFilter,
  SupporterSettings,
  supporterPlansQueryOptions,
  supporterProgramQueryOptions,
  supporterSubscriptionStates,
  supporterSubscriptionsQueryOptions,
} from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The subscriptions shown: of a plan (`?plan=`) and in a state (`?state=`); omitted, or anything unknown, shows every one (web-client/README.md S3). */
const searchSchema = z.object({
  plan: z.string().optional().catch(undefined),
  state: z.enum(supporterSubscriptionStates).optional().catch(undefined),
});

/** The supporter program (ADR-0072): enabling it, its plans and their subscriptions, for manage_supporters. */
export const Route = createFileRoute("/admin/supporters")({
  staticData: { title: "client:admin.supporters.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): SupporterSubscriptionFilter => ({ planId: search.plan, state: search.state }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_supporters", () =>
      Promise.all([
        context.queryClient.query({ ...supporterProgramQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.query({ ...supporterPlansQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.infiniteQuery({
          ...supporterSubscriptionsQueryOptions(context.api, deps),
          staleTime: Infinity,
        }),
      ]),
    ),
  component: SupportersPage,
  errorComponent: SupportersError,
});

function SupportersPage() {
  const t = useT();
  const filter = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.supporters.title")}</PageHeading>
      <SupporterSettings
        filter={filter}
        onFilterChange={(next) => {
          void navigate({
            search: {
              ...(next.planId === undefined ? {} : { plan: next.planId }),
              ...(next.state === undefined ? {} : { state: next.state }),
            },
          });
        }}
      />
    </>
  );
}

function SupportersError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_supporters" heading={t("client:admin.supporters.title")} />;
}
