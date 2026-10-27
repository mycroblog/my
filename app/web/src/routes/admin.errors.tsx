// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { type ErrorFilter, ErrorGroups, errorGroupsQueryOptions, errorSourceFilters, errorStateFilters } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `state` and `source`: the filters of the list; omitted, or anything unknown, shows the open groups of both sources
 * (web-client/README.md S3).
 */
const searchSchema = z.object({
  state: z.enum(errorStateFilters).exclude(["open"]).optional().catch(undefined),
  source: z.enum(errorSourceFilters).exclude(["all"]).optional().catch(undefined),
});

/** The groups of the errors that the server and the web client report (ADR-0103 §8), for manage_errors. */
export const Route = createFileRoute("/admin/errors")({
  staticData: { title: "client:admin.errors.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { state, source } }): { filter: ErrorFilter } => ({ filter: { state: state ?? "open", source: source ?? "all" } }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { filter } }) =>
    loadWithPermission(context, "manage_errors", () => context.queryClient.infiniteQuery({ ...errorGroupsQueryOptions(context.api, filter), staleTime: Infinity })),
  component: ErrorsPage,
  errorComponent: ErrorsError,
});

function ErrorsPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.errors.title")}</PageHeading>
      <ErrorGroups
        filter={filter}
        onFilterChange={(next) => {
          void navigate({ search: { state: next.state === "open" ? undefined : next.state, source: next.source === "all" ? undefined : next.source } });
        }}
      />
    </>
  );
}

function ErrorsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_errors" heading={t("client:admin.errors.title")} />;
}
