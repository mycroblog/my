// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { TranslationSettings } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The translation of posts: the providers, what each may receive, the limits, and the month's use, for manage_settings. */
export const Route = createFileRoute("/admin/translation")({
  staticData: { title: "client:admin.translation.title" },
  beforeLoad: requireSignIn,
  component: TranslationPage,
  errorComponent: TranslationError,
});

function TranslationPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.translation.title")}</PageHeading>
      <TranslationSettings />
    </>
  );
}

function TranslationError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.translation.title")} />;
}
