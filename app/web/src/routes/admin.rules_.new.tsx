// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewRule } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** Adding a rule after the others (FR-705), for manage_rules. The `_` keeps the page out of the list's layout. */
export const Route = createFileRoute("/admin/rules_/new")({
  staticData: { title: "client:admin.rules.new.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => loadWithPermission(context, "manage_rules", () => Promise.resolve()),
  component: NewRulePage,
  errorComponent: NewRuleError,
});

function NewRuleHeading() {
  const t = useT();
  return (
    <Stack align="start" gap="1">
      <PageHeading>{t("client:admin.rules.new.title")}</PageHeading>
      <Link href="/admin/rules" variant="standalone" size="sm">
        {t("client:admin.rule.back")}
      </Link>
    </Stack>
  );
}

function NewRulePage() {
  return (
    <>
      <NewRuleHeading />
      <NewRule />
    </>
  );
}

function NewRuleError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_rules" heading={t("client:admin.rules.new.title")} />;
}
