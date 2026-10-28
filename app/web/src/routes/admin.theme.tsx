// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { ThemeSettings, themeQueryOptions } from "../features/admin";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { AdminError, loadWithPermission } from "./-admin/adminRoute";
import { PageHeading } from "./-layout/PageHeading";

/** The server's theme (ADR-0059): overrides of the design tokens and a custom CSS for every visitor, for manage_settings. */
export const Route = createFileRoute("/admin/theme")({
  staticData: { title: "client:admin.theme.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    loadWithPermission(context, "manage_settings", () =>
      context.queryClient.query({ ...themeQueryOptions(context.api), staleTime: Infinity }),
    ),
  component: ThemePage,
  errorComponent: ThemeError,
});

function ThemePage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:admin.theme.title")}</PageHeading>
      <ThemeSettings />
    </>
  );
}

function ThemeError(props: ErrorComponentProps) {
  const t = useT();
  return <AdminError {...props} permission="manage_settings" heading={t("client:admin.theme.title")} />;
}
