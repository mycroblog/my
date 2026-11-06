// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import {
  instanceSettingsQueryOptions,
  instanceTextsQueryOptions,
  ownInstanceQueryOptions,
  ServerSettings,
} from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The general page of the server settings, for manage_settings: its name, which people see in place of its domain, its
 * descriptions, how to reach its operators and its thumbnail. The other pages of the screen are its child routes.
 */
export const Route = createFileRoute("/admin/settings")({
  staticData: { title: "client:admin.settings.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_settings", () =>
      Promise.all([
        context.queryClient.query({ ...instanceTextsQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.query({ ...ownInstanceQueryOptions(context.api), staleTime: Infinity }),
        context.queryClient.query({ ...instanceSettingsQueryOptions(context.api), staleTime: Infinity }),
      ]),
    ),
  component: ServerSettingsPage,
  errorComponent: ServerSettingsError,
});

function ServerSettingsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.settings.title")}</PageHeading>
      <ServerSettings page="general" />
    </>
  );
}

function ServerSettingsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.settings.title")} />;
}
