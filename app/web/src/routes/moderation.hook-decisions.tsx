// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
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

/** `hook`: the hook the list keeps; omitted, or anything unknown, shows every one (web-client/README.md S3). */
const searchSchema = z.object({
  hook: z.enum(hookDecisionFilters).exclude(["all"]).optional().catch(undefined),
});

/** What the external reviews said, their counts and latency, and cancelling a decision (ADR-0147 §9), for manage_reports. */
export const Route = createFileRoute("/moderation/hook-decisions")({
  validateSearch: searchSchema,
  loaderDeps: ({ search: { hook } }): { filter: HookDecisionFilter } => ({ filter: hook ?? "all" }),
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
      ]),
    ),
  component: HookDecisionsPage,
  errorComponent: HookDecisionsError,
});

function HookDecisionsPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  const changeFilter = (next: HookDecisionFilter) => {
    void navigate({ search: (previous) => ({ ...previous, hook: next === "all" ? undefined : next }) });
  };
  return (
    <>
      <PageHeading>{t("client:admin.hook_decisions.title")}</PageHeading>
      <HookDecisions filter={filter} onFilterChange={changeFilter} />
    </>
  );
}

function HookDecisionsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_reports" heading={t("client:admin.hook_decisions.title")} />;
}
