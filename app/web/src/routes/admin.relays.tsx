// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { RelaySettings, relaysQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The federation relays (ADR-0061): registering and joining them, leaving, joining again and deleting them, for manage_settings. */
export const Route = createFileRoute("/admin/relays")({
  staticData: { title: "client:admin.relays.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_settings", () =>
      context.queryClient.query({ ...relaysQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: RelaysPage,
  errorComponent: RelaysError,
});

function RelaysPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.relays.title")}</PageHeading>
      <RelaySettings />
    </>
  );
}

function RelaysError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.relays.title")} />;
}
