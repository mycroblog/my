// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { RegistrationList, registrationsQueryOptions } from "../features/moderation";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The registrations that await approval, and approving or rejecting them (FR-721), for manage_accounts. */
export const Route = createFileRoute("/moderation/registrations")({
  staticData: { title: "client:admin.registrations.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_accounts", () =>
      context.queryClient.infiniteQuery({ ...registrationsQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: RegistrationsPage,
  errorComponent: RegistrationsError,
});

function RegistrationsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.registrations.title")}</PageHeading>
      <RegistrationList />
    </>
  );
}

function RegistrationsError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_accounts" heading={t("client:admin.registrations.title")} />;
}
