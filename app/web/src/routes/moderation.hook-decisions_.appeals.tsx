// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  type HookAppealFilter,
  hookAppealFilters,
  HookAppeals,
  hookAppealsQueryOptions,
  HookReviewPages,
} from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `state`: `all` shows every appeal of a refused post; omitted, or anything unknown, those that await a moderator
 * (web-client/README.md S3).
 */
const searchSchema = z.object({
  state: z.enum(hookAppealFilters).exclude(["pending"]).optional().catch(undefined),
});

/**
 * The appeals page of the automated reviews screen: the authors' appeals of the posts that the review of local posts
 * refused, newest first (ADR-0147 §9), for manage_reports. The `_` keeps the page out of the layout of
 * `/moderation/hook-decisions`, the screen's first page.
 */
export const Route = createFileRoute("/moderation/hook-decisions_/appeals")({
  validateSearch: searchSchema,
  loaderDeps: ({ search: { state } }): { filter: HookAppealFilter } => ({ filter: state ?? "pending" }),
  staticData: { title: "client:admin.hook_appeals.title" },
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_reports", () =>
      context.queryClient.infiniteQuery({
        ...hookAppealsQueryOptions(context.api, deps.filter),
        staleTime: Infinity,
      }),
    ),
  component: HookAppealsPage,
  errorComponent: HookAppealsError,
});

function HookAppealsPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.hook_decisions.title")}</PageHeading>
      <HookReviewPages current="appeals" />
      <HookAppeals
        filter={filter}
        onFilterChange={(next) => {
          void navigate({ search: { state: next === "pending" ? undefined : next } });
        }}
      />
    </>
  );
}

function HookAppealsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_reports" heading={t("client:admin.hook_decisions.title")} />;
}
