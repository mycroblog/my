// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { RuleSettings, rulesQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The server's rules (FR-705): adding, editing, reordering and retiring them, for manage_rules. */
export const Route = createFileRoute("/admin/rules")({
  staticData: { title: "client:admin.rules.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_rules", () =>
      context.queryClient.query({ ...rulesQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: RulesPage,
  errorComponent: RulesError,
});

function RulesPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.rules.title")}</PageHeading>
      <RuleSettings />
    </>
  );
}

function RulesError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_rules" heading={t("client:admin.rules.title")} />;
}
