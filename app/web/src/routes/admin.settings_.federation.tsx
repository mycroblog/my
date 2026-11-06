// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { instanceSettingsQueryOptions, ServerSettings } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The server's policies of one topic, a page of the server settings for manage_settings: the federation, the list of
 * the domain actions and the decision hooks. The `_` keeps it out of the general page's layout.
 */
export const Route = createFileRoute("/admin/settings_/federation")({
  staticData: { title: "client:admin.settings.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_settings", () =>
      context.queryClient.query({ ...instanceSettingsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: FederationSettingsPage,
  errorComponent: FederationSettingsError,
});

function FederationSettingsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.settings.title")}</PageHeading>
      <ServerSettings page="federation" />
    </>
  );
}

function FederationSettingsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.settings.title")} />;
}
