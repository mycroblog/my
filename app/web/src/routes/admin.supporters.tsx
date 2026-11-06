// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { SupporterProgram, supporterProgramQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The supporter program (ADR-0072), the first page of the supporters screen: enabling it and who may read the
 * supporter list in one form, and the state of the payment provider, for manage_supporters. Its plans and their
 * subscriptions are the screen's other pages (LocalNav).
 */
export const Route = createFileRoute("/admin/supporters")({
  staticData: { title: "client:admin.supporters.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_supporters", () =>
      context.queryClient.query({ ...supporterProgramQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: SupportersPage,
  errorComponent: SupportersError,
});

function SupportersPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.supporters.title")}</PageHeading>
      <SupporterProgram />
    </>
  );
}

function SupportersError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_supporters" heading={t("client:admin.supporters.title")} />;
}
