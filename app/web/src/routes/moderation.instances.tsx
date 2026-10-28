// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { type InstanceFilter, instanceFilters, InstanceList, instancesQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `state`: the state filter of the list; omitted, or anything unknown, shows every server (web-client/README.md S3). */
const searchSchema = z.object({
  state: z.enum(instanceFilters).exclude(["all"]).optional().catch(undefined),
});

/** The health of the remote servers that deliveries failed to reach (federation.md §7.3), for manage_domains. */
export const Route = createFileRoute("/moderation/instances")({
  staticData: { title: "client:admin.instances.title" },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { state } }): { filter: InstanceFilter } => ({ filter: state ?? "all" }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps: { filter } }) =>
    loadWithPermission(context, "manage_domains", () =>
      context.queryClient.infiniteQuery({ ...instancesQueryOptions(context.api, filter), staleTime: Infinity }),
    ),
  component: InstancesPage,
  errorComponent: InstancesError,
});

function InstancesPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  const changeFilter = (next: InstanceFilter) => {
    void navigate({ search: next === "all" ? {} : { state: next } });
  };
  return (
    <>
      <PageHeading>{t("client:admin.instances.title")}</PageHeading>
      <InstanceList filter={filter} onFilterChange={changeFilter} />
    </>
  );
}

function InstancesError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.instances.title")} />;
}
