// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  type SupporterSubscriptionFilter,
  SupporterSubscriptions,
  supporterPlansQueryOptions,
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

/**
 * The subscriptions to the supporter plans (ADR-0072), a page of the supporters screen: the list narrowed to a plan and
 * a state, which leads to each subscription's page, for manage_supporters.
 */
export const Route = createFileRoute("/admin/supporters_/subscriptions")({
  staticData: { title: "client:admin.supporters.subscriptions.page_title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search }): SupporterSubscriptionFilter => ({ planId: search.plan, state: search.state }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_supporters", () =>
      Promise.all([
        context.queryClient.query({ ...supporterPlansQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.infiniteQuery({
          ...supporterSubscriptionsQueryOptions(context.api, deps),
          staleTime: Infinity,
        }),
      ]),
    ),
  component: SubscriptionsPage,
  errorComponent: SubscriptionsError,
});

function SubscriptionsPage() {
  const t = useT();
  const filter = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.supporters.subscriptions.page_title")}</PageHeading>
      <SupporterSubscriptions
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

function SubscriptionsError(props: ErrorComponentProps) {
  const t = useT();
  return (
    <AdminError
      {...props}
      permission="manage_supporters"
      heading={t("client:admin.supporters.subscriptions.page_title")}
    />
  );
}
