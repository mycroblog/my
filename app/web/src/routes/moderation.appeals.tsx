// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { type AppealFilter, AppealQueue, appealsQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `state`: the tab; omitted, or anything unknown, shows the appeals awaiting a decision (web-client/README.md S3). */
const searchSchema = z.object({
  state: z.enum(["decided"]).optional().catch(undefined),
});

/** The appeals of moderation actions, awaiting a decision or decided, newest first (FR-707), for manage_accounts. */
export const Route = createFileRoute("/moderation/appeals")({
  staticData: { title: "client:admin.appeals.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { state } }): { filter: AppealFilter } => ({ filter: state ?? "pending" }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { filter } }) =>
    loadWithPermission(context, "manage_accounts", () =>
      context.queryClient.infiniteQuery({ ...appealsQueryOptions(context.api, filter), staleTime: Infinity }),
    ),
  component: AppealsPage,
  errorComponent: AppealsError,
});

function AppealsPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.appeals.title")}</PageHeading>
      <AppealQueue
        filter={filter}
        onFilterChange={(next) => {
          void navigate({ search: { state: next === "pending" ? undefined : next } });
        }}
      />
    </>
  );
}

function AppealsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.appeals.title")} />;
}
