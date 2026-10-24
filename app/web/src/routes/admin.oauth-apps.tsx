// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { hostDenialsQueryOptions, OAuthAppSettings, type OAuthAppTab, type OAuthClientFilter, oauthClientsQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * `tab`: the denied hosts of Client ID Metadata Documents; omitted, or anything unknown, shows the apps.
 * `registration`: keeps the apps registered by one route; omitted, or anything unknown, shows every app.
 */
const searchSchema = z.object({
  tab: z.literal("hosts").optional().catch(undefined),
  registration: z.enum(["first_party", "user", "metadata_document", "dynamic"]).optional().catch(undefined),
});

/**
 * The OAuth apps (ADR-0090 §1): every client with the route it registered by, disabling and enabling them, and
 * denying the hosts of Client ID Metadata Documents, for manage_oauth_apps.
 */
export const Route = createFileRoute("/admin/oauth-apps")({
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { tab: OAuthAppTab; filter: OAuthClientFilter } => ({
    tab: search.tab ?? "apps",
    filter: { registration: search.registration },
  }),
  staticData: { title: "client:admin.oauth_apps.title" },
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_oauth_apps", () =>
      deps.tab === "hosts"
        ? context.queryClient.infiniteQuery({ ...hostDenialsQueryOptions(context.api), staleTime: Infinity })
        : context.queryClient.infiniteQuery({ ...oauthClientsQueryOptions(context.api, deps.filter), staleTime: Infinity }),
    ),
  component: OAuthAppsPage,
  errorComponent: OAuthAppsError,
});

function OAuthAppsPage() {
  const t = useT();
  const { tab, filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.oauth_apps.title")}</PageHeading>
      <OAuthAppSettings
        tab={tab}
        onTabChange={(next) => {
          void navigate({ search: next === "hosts" ? { tab: "hosts" } : { registration: filter.registration } });
        }}
        filter={filter}
        onFilterChange={(next) => {
          void navigate({ search: { registration: next.registration } });
        }}
      />
    </>
  );
}

function OAuthAppsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_oauth_apps" heading={t("client:admin.oauth_apps.title")} />;
}
