// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { instanceSettingsQueryOptions, ServerSettings } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The server's policies of one topic, a page of the server settings for manage_settings: who may sign up, the members'
 * invites and the registration of apps. The `_` keeps it out of the general page's layout.
 */
export const Route = createFileRoute("/admin/settings_/registration")({
  staticData: { title: "client:admin.settings.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_settings", () =>
      context.queryClient.query({ ...instanceSettingsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: RegistrationSettingsPage,
  errorComponent: RegistrationSettingsError,
});

function RegistrationSettingsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.settings.title")}</PageHeading>
      <ServerSettings page="registration" />
    </>
  );
}

function RegistrationSettingsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.settings.title")} />;
}
