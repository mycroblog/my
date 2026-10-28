// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { DocumentSettings, instanceTextsQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The terms of service and the privacy policy that the server publishes (#489), for manage_settings. */
export const Route = createFileRoute("/admin/documents")({
  staticData: { title: "client:admin.documents.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_settings", () =>
      context.queryClient.query({ ...instanceTextsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: DocumentsPage,
  errorComponent: DocumentsError,
});

function DocumentsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.documents.title")}</PageHeading>
      <DocumentSettings />
    </>
  );
}

function DocumentsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.documents.title")} />;
}
