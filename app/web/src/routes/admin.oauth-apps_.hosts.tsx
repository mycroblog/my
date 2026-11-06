// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { hostDenialsQueryOptions, OAuthHostDenials } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The denied hosts of Client ID Metadata Documents (ADR-0090 §1), a page of the OAuth apps screen: denying a host and
 * lifting a denial, for manage_oauth_apps. The `_` keeps the page out of the list's layout at `/admin/oauth-apps`.
 */
export const Route = createFileRoute("/admin/oauth-apps_/hosts")({
  staticData: { title: "client:admin.oauth_apps.hosts.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_oauth_apps", () =>
      context.queryClient.infiniteQuery({ ...hostDenialsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: OAuthHostsPage,
  errorComponent: OAuthHostsError,
});

function OAuthHostsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.oauth_apps.title")}</PageHeading>
      <OAuthHostDenials />
    </>
  );
}

function OAuthHostsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_oauth_apps" heading={t("client:admin.oauth_apps.title")} />;
}
