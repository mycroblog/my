// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { DocumentSettings, instanceTextsQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The commerce disclosures of the supporter plans (ADR-0072 §13), a page of the terms and policies for manage_settings.
 * The `_` keeps it out of the first page's layout.
 */
export const Route = createFileRoute("/admin/documents_/commerce-disclosure")({
  staticData: { title: "client:admin.documents.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_settings", () =>
      context.queryClient.query({ ...instanceTextsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: CommerceDisclosurePage,
  errorComponent: CommerceDisclosureError,
});

function CommerceDisclosurePage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.documents.title")}</PageHeading>
      <DocumentSettings documentKey="commerce_disclosure" />
    </>
  );
}

function CommerceDisclosureError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.documents.title")} />;
}
