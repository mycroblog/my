// SPDX-License-Identifier: AGPL-3.0-only
import { Link, Stack } from "@my/ui";
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { NewSupporterPlan } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** Creating a supporter plan (ADR-0072 §3), for manage_supporters; once it is created, the plan's page opens. */
export const Route = createFileRoute("/admin/supporters_/plans_/new")({
  staticData: { title: "client:admin.supporters.new.title" },
  beforeLoad: requireSignIn,
  component: NewPlanPage,
  errorComponent: NewPlanError,
});

function NewPlanPage() {
  const t = useT();
  return (
    <>
      <Stack align="start" gap="1">
        <PageHeading>{t("client:admin.supporters.new.title")}</PageHeading>
        <Link href="/admin/supporters/plans" variant="standalone" size="sm">
          {t("client:admin.supporters.plan.back")}
        </Link>
      </Stack>
      <NewSupporterPlan />
    </>
  );
}

function NewPlanError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_supporters" heading={t("client:admin.supporters.new.title")} />;
}
