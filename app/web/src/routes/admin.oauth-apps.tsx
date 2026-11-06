// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { z } from "zod";
import { OAuthAppList, type OAuthClientFilter, oauthClientsQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** `registration`: keeps the apps registered by one route; omitted, or anything unknown, shows every app. */
const searchSchema = z.object({
  registration: z.enum(["first_party", "user", "metadata_document", "dynamic"]).optional().catch(undefined),
});

/**
 * The OAuth apps (ADR-0090 §1), the first page of the screen: every client with the route it registered by, and
 * disabling and enabling them, for manage_oauth_apps. The denied hosts of Client ID Metadata Documents are
 * /admin/oauth-apps/hosts.
 */
export const Route = createFileRoute("/admin/oauth-apps")({
  validateSearch: searchSchema,
  loaderDeps: ({ search }): { filter: OAuthClientFilter } => ({ filter: { registration: search.registration } }),
  staticData: { title: "client:admin.oauth_apps.title" },
  beforeLoad: requireSignIn,
  loader: ({ context, deps }) =>
    loadWithPermission(context, "manage_oauth_apps", () =>
      context.queryClient.infiniteQuery({ ...oauthClientsQueryOptions(context.api, deps.filter), staleTime: Infinity }),
    ),
  component: OAuthAppsPage,
  errorComponent: OAuthAppsError,
});

function OAuthAppsPage() {
  const t = useT();
  const { filter } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:admin.oauth_apps.title")}</PageHeading>
      <OAuthAppList
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
