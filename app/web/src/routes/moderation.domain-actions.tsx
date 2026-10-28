// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import {
  type DomainActionFilter,
  domainActionFilters,
  DomainActions,
  domainActionsQueryOptions,
} from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `filter`: the tab of the list; omitted, or anything unknown, shows the actions in effect. `domain`: the one
 * domain whose actions to list and to fill in the form (the link from a remote server's page); omitted, or
 * anything that is not a domain's length, lists every domain (web-client/README.md S3).
 */
const searchSchema = z.object({
  filter: z.enum(domainActionFilters).exclude(["active"]).optional().catch(undefined),
  domain: z.string().trim().min(1).max(253).optional().catch(undefined),
});

/** The address of the list under `filter`, of every domain. */
function listHref(filter: DomainActionFilter): string {
  return filter === "active" ? "/moderation/domain-actions" : `/moderation/domain-actions?filter=${filter}`;
}

/** The moderation actions on remote domains, and taking and lifting them (FR-702), for manage_domains. */
export const Route = createFileRoute("/moderation/domain-actions")({
  validateSearch: searchSchema,
  loaderDeps: ({ search: { filter, domain } }): { filter: DomainActionFilter; domain: string | undefined } => ({
    filter: filter ?? "active",
    domain,
  }),
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_domains", () =>
      context.queryClient.infiniteQuery({ ...domainActionsQueryOptions(context.api, deps), staleTime: Infinity }),
    ),
  component: DomainActionsPage,
  errorComponent: DomainActionsError,
});

function DomainActionsPage() {
  const t = useT();
  const { filter, domain } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  const changeFilter = (next: DomainActionFilter) => {
    void navigate({ search: (previous) => ({ ...previous, filter: next === "active" ? undefined : next }) });
  };
  return (
    <>
      <PageHeading>{t("client:admin.domain_actions.title")}</PageHeading>
      <DomainActions filter={filter} domain={domain} onFilterChange={changeFilter} everyDomainHref={listHref(filter)} />
    </>
  );
}

function DomainActionsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_domains" heading={t("client:admin.domain_actions.title")} />;
}
