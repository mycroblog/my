// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  type HookAppealFilter,
  hookAppealFilters,
  hookAppealsQueryOptions,
  type HookDecisionFilter,
  hookDecisionFilters,
  hookDecisionsQueryOptions,
  HookDecisions,
  hookSummaryQueryOptions,
} from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `hook`: the hook the list of decisions keeps; omitted, or anything unknown, shows every one. `appeals`: `all` shows
 * every appeal of a refused post; omitted, or anything unknown, those that await a moderator (web-client/README.md S3).
 */
const searchSchema = z.object({
  hook: z.enum(hookDecisionFilters).exclude(["all"]).optional().catch(undefined),
  appeals: z.enum(hookAppealFilters).exclude(["pending"]).optional().catch(undefined),
});

/**
 * What the external reviews said, their counts and latency, cancelling a decision, and the appeals of refused posts
 * (ADR-0147 §9), for manage_reports.
 */
export const Route = createFileRoute("/moderation/hook-decisions")({
  validateSearch: searchSchema,
  loaderDeps: ({ search: { hook, appeals } }): { filter: HookDecisionFilter; appealFilter: HookAppealFilter } => ({
    filter: hook ?? "all",
    appealFilter: appeals ?? "pending",
  }),
  staticData: { title: "client:admin.hook_decisions.title" },
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_reports", () =>
      Promise.all([
        context.queryClient.infiniteQuery({
          ...hookDecisionsQueryOptions(context.api, deps.filter),
          staleTime: Infinity,
        }),
        context.queryClient.query({ ...hookSummaryQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.infiniteQuery({
          ...hookAppealsQueryOptions(context.api, deps.appealFilter),
          staleTime: Infinity,
        }),
      ]),
    ),
  component: HookDecisionsPage,
  errorComponent: HookDecisionsError,
});

function HookDecisionsPage() {
  const t = useT();
  const { filter, appealFilter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  const changeFilter = (next: HookDecisionFilter) => {
    void navigate({ search: (previous) => ({ ...previous, hook: next === "all" ? undefined : next }) });
  };
  const changeAppealFilter = (next: HookAppealFilter) => {
    void navigate({ search: (previous) => ({ ...previous, appeals: next === "pending" ? undefined : next }) });
  };
  return (
    <>
      <PageHeading>{t("client:admin.hook_decisions.title")}</PageHeading>
      <HookDecisions
        filter={filter}
        onFilterChange={changeFilter}
        appealFilter={appealFilter}
        onAppealFilterChange={changeAppealFilter}
      />
    </>
  );
}

function HookDecisionsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_reports" heading={t("client:admin.hook_decisions.title")} />;
}
